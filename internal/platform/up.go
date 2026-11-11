package platform

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"rye/internal/compute/incus"
	"rye/internal/vmconfig"
	"rye/internal/vmexec"
)

// Settings are ryectl up's choices from its environment. Empty values take their defaults.
type Settings struct {
	// Pool is the storage pool for the service VM and the guests. Empty keeps the current deploy's, else "default".
	Pool string
	// BridgeIP and ServiceIP place the bridge and the service VM.
	BridgeIP, ServiceIP string
	// Key is the public key file that may log in to the service VM. Empty means ~/.ssh/id_ed25519.pub.
	Key string
	// Home is the home directory of the user running ryectl, for the SSH key and known_hosts.
	Home string
}

// Up creates or upgrades the service VM, and the bridge, firewalls and project around it, from the build in dist. It
// writes each step to out.
func Up(ctx context.Context, client *incus.Admin, dist string, set Settings, out io.Writer) error {
	step := func(name string) { _, _ = fmt.Fprintln(out, "==> "+name) }
	version, err := os.ReadFile(filepath.Clean(filepath.Join(dist, versionFile)))
	if err != nil {
		return fmt.Errorf("no build in %s; run make build: %w", dist, err)
	}
	step("checks, for build " + strings.TrimSpace(string(version)))
	addr, pool, err := checks(ctx, client, set)
	if err != nil {
		return err
	}
	key, err := ownerKey(set)
	if err != nil {
		return err
	}
	step("Incus API on " + addr.Bridge.String() + ":8443")
	if err := network(ctx, client, addr); err != nil {
		return err
	}
	step("firewalls")
	if err := firewalls(ctx, client, addr); err != nil {
		return err
	}
	step("project " + Project)
	if err := project(ctx, client); err != nil {
		return err
	}
	step("service VM " + ServiceVM)
	err = ensureVM(ctx, client, ServiceVM, map[string]string{"limits.cpu": "2", "limits.memory": "2GiB"},
		map[string]map[string]string{
			"root": {"type": "disk", "path": "/", "pool": pool, "size": "10GiB"},
			"eth0": {"type": "nic", "network": Bridge, "ipv4.address": addr.Service.String(), "security.acls": serviceACL},
		})
	if err != nil {
		return err
	}
	vm := vmexec.New(client.VM(), ServiceVM)
	if err := setupService(ctx, vm, key); err != nil {
		return err
	}
	step("restricted certificate")
	cert, err := vm.Read(ctx, vmconfig.ClientCertPath)
	if err != nil {
		return err
	}
	if err := client.TrustRestricted(ctx, certName, []byte(cert), []string{Project}); err != nil {
		return err
	}
	step("rye, ryed, the GUI and the docs")
	server, err := client.Server(ctx)
	if err != nil {
		return err
	}
	if err := installBuild(ctx, vm, dist, server.Environment.Certificate, addr.Bridge.String(), pool); err != nil {
		return err
	}
	step("host key in ~/" + KnownHosts)
	if err := trustHostKey(ctx, vm, filepath.Join(set.Home, KnownHosts), addr.Service.String()); err != nil {
		return err
	}
	step("ready. Next:")
	_, err = fmt.Fprint(out, `    dist/ryectl ssh-config               once, for the SSH aliases
    ssh rye new <vm>
    ssh rye agent-manager install <vm>   to install agent-manager in a VM
    ssh rye agent-manager install --all  after an upgrade, in every VM with agent-manager
`)
	return err
}

// checks returns the addresses and the pool to use, after checking the pool, the bridge's current address and the
// host's routes.
func checks(ctx context.Context, client *incus.Admin, set Settings) (Addresses, string, error) {
	current, err := currentDeploy(ctx, client)
	if err != nil {
		return Addresses{}, "", err
	}
	pool := cmp.Or(set.Pool, current.Pool, "default")
	found, err := client.PoolExists(ctx, pool)
	if err != nil {
		return Addresses{}, "", err
	}
	if !found {
		return Addresses{}, "", fmt.Errorf("no storage pool %s; set POOL=", pool)
	}
	// An unreadable route table is treated as empty.
	table, _ := os.ReadFile("/proc/net/route")
	addr, err := checkAddresses(set, current, string(table))
	if err != nil {
		return addr, "", err
	}
	return addr, pool, nil
}

// checkAddresses returns the addresses to use. It refuses to move the current deploy's bridge, and refuses a subnet
// that routeTable, in /proc/net/route's format, routes through an interface other than the bridge.
func checkAddresses(set, current Settings, routeTable string) (Addresses, error) {
	addr, err := Choose(set, current)
	if err != nil {
		return addr, err
	}
	// Changing the bridge's address would leave every guest's address and the firewalls behind.
	if current.BridgeIP != "" && current.BridgeIP != addr.Bridge.String() {
		return addr, fmt.Errorf("%s is on %s; moving it to %s deletes every VM; see "+
			"docs/how-to/deploy-options.md, %q", Bridge, current.BridgeIP, addr.Bridge, "Move a deploy to another subnet")
	}
	if through := RoutedElsewhere(routeTable, addr.Subnet()); through != "" {
		return addr, fmt.Errorf("this host already routes %s through %s; set BRIDGE_IP and SERVICE_IP to a free /24, "+
			"e.g. BRIDGE_IP=198.19.0.1 SERVICE_IP=198.19.0.10", addr.Subnet(), through)
	}
	return addr, nil
}

// currentDeploy returns the bridge and service addresses and the pool of an earlier deploy. A rerun keeps them.
func currentDeploy(ctx context.Context, client *incus.Admin) (Settings, error) {
	var current Settings
	network, err := client.Network(ctx, Bridge)
	if err != nil {
		return current, err
	}
	current.BridgeIP, _, _ = strings.Cut(network["ipv4.address"], "/")
	vm, err := client.Instance(ctx, ServiceVM)
	if err != nil {
		return current, err
	}
	if vm != nil {
		current.ServiceIP = vm.Devices["eth0"]["ipv4.address"]
		current.Pool = vm.Devices["root"]["pool"]
	}
	return current, nil
}

// ownerKey returns the public key that may log in to the service VM, Key or else ~/.ssh/id_ed25519.pub.
func ownerKey(set Settings) (string, error) {
	path := set.Key
	if path == "" {
		path = filepath.Join(set.Home, ".ssh", "id_ed25519.pub")
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("no SSH key at %s; create one with ssh-keygen -t ed25519, or set KEY=", path)
	}
	if err != nil {
		return "", fmt.Errorf("cannot read SSH key %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// network creates the bridge, points int.rye at the service VM, and has Incus listen on the bridge for ryed.
func network(ctx context.Context, client *incus.Admin, addr Addresses) error {
	err := client.EnsureNetwork(ctx, Bridge,
		map[string]string{"ipv4.address": addr.Bridge.String() + "/24", "ipv4.nat": "true", "ipv6.address": "none"},
		map[string]string{"dns.domain": "rye", "raw.dnsmasq": "address=/int.rye/" + addr.Service.String(),
			"ipv4.dhcp.ranges": addr.DHCPRanges()})
	if err != nil {
		return err
	}
	server, err := client.Server(ctx)
	if err != nil {
		return err
	}
	listen := addr.Bridge.String() + ":8443"
	switch current := server.Config["core.https_address"]; current {
	case "":
		return client.SetServerConfig(ctx, "core.https_address", listen)
	case listen, "[::]:8443", ":8443", "0.0.0.0:8443":
		return nil
	default:
		return fmt.Errorf("core.https_address is %s; rye needs %s", current, listen)
	}
}

func firewalls(ctx context.Context, client *incus.Admin, addr Addresses) error {
	if err := client.PutACL(ctx, guestACL, guestFirewall(addr)); err != nil {
		return err
	}
	return client.PutACL(ctx, serviceACL, serviceFirewall(addr))
}

// project creates project rye, limited to managed NICs on the bridge, and gives its default profile the guest NIC.
func project(ctx context.Context, client *incus.Admin) error {
	err := client.EnsureProject(ctx, Project, map[string]string{"restricted": "true", "restricted.devices.nic": "managed",
		"restricted.networks.access": Bridge})
	if err != nil {
		return err
	}
	return client.PutProfile(ctx, Project, "default", "Guest VM", guestNIC())
}

// ensureVM creates the VM name in the default project when it's missing, starts it, and waits for its first boot.
// An existing VM keeps its config and devices.
func ensureVM(ctx context.Context, client *incus.Admin, name string, config map[string]string,
	devices map[string]map[string]string) error {
	found, err := client.Instance(ctx, name)
	if err != nil {
		return err
	}
	if found == nil {
		if err := client.CreateInstance(ctx, name, config, devices); err != nil {
			return err
		}
	}
	if found == nil || found.Status != "Running" {
		if err := client.Start(ctx, name); err != nil {
			return err
		}
	}
	return client.VM().WaitForCloudInit(ctx, name, io.Discard)
}

// SSHConfig returns the SSH client blocks for the current deploy, which rye ssh-config also prints.
func SSHConfig(ctx context.Context, client *incus.Admin) (string, error) {
	current, err := currentDeploy(ctx, client)
	if err != nil {
		return "", err
	}
	if current.ServiceIP == "" {
		return "", fmt.Errorf("no service VM %s; run make up", ServiceVM)
	}
	addr, err := Choose(Settings{}, current)
	if err != nil {
		return "", err
	}
	return vmconfig.SSHConfig(addr.Service), nil
}
