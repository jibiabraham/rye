package platform

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"rye/internal/agentmanager"
	"rye/internal/compute/incus"
	"rye/internal/hostkeys"
	"rye/internal/vmconfig"
	"rye/internal/vmexec"
)

// Paths on the admin VM.
const (
	adminACL       = "rye-admin"
	adminKeyName   = "rye-admin"
	adminSSHDPath  = "/etc/ssh/sshd_config.d/10-rye.conf"
	adminUnitPath  = "/etc/systemd/system/ryed-local.service"
	adminSudoers   = "/etc/sudoers.d/90-rye-" + vmconfig.AdminUser
	adminUserHome  = "/home/" + vmconfig.AdminUser
	adminSSHConfig = adminUserHome + "/.ssh/config"
)

// AdminUp creates, upgrades or starts the admin VM from the build in dist. The VM runs agent-manager and ryed
// -local, and reaches the service VM and the guests over SSH as the host does. It writes each step to out.
func AdminUp(ctx context.Context, client *incus.Admin, dist string, set Settings, out io.Writer) error {
	step := func(name string) { _, _ = fmt.Fprintln(out, "==> "+name) }
	step("checks")
	current, err := currentDeploy(ctx, client)
	if err != nil {
		return err
	}
	if current.ServiceIP == "" {
		return fmt.Errorf("no service VM %s; run make up", ServiceVM)
	}
	addr, err := Choose(Settings{}, current)
	if err != nil {
		return err
	}
	key, err := ownerKey(set)
	if err != nil {
		return err
	}
	step("firewall " + adminACL + ", SSH from this host only")
	if err := client.PutACL(ctx, adminACL, adminFirewall(addr)); err != nil {
		return err
	}
	step("VM " + AdminVM + " at " + addr.Admin.String())
	err = ensureVM(ctx, client, AdminVM, map[string]string{"limits.cpu": "2", "limits.memory": "4GiB"},
		map[string]map[string]string{
			"root": {"type": "disk", "path": "/", "pool": current.Pool, "size": "16GiB"},
			"eth0": {
				"type": "nic", "network": Bridge, "ipv4.address": addr.Admin.String(), "security.acls": adminACL,
				"security.mac_filtering": "true", "security.ipv4_filtering": "true", "security.ipv6_filtering": "true",
				"ipv6.address": "none",
			},
		})
	if err != nil {
		return err
	}
	vm := vmexec.New(client.VM(), AdminVM)
	if err := setupAdmin(ctx, vm, key); err != nil {
		return err
	}
	step("ryed and the GUI")
	if err := vm.Copy(ctx, "/usr/local/bin/ryed", filepath.Join(dist, "ryed"), 0o755, 0, 0); err != nil {
		return err
	}
	for _, args := range [][]string{{"systemctl", "daemon-reload"}, {"systemctl", "enable", "ryed-local"},
		{"systemctl", "restart", "ryed-local"}} {
		if _, err := vm.Root(ctx, args...); err != nil {
			return err
		}
	}
	step("agent-manager")
	if err := agentmanager.Install(ctx, client.VM(), AdminVM, vmconfig.AdminUser, out); err != nil {
		return err
	}
	step("SSH to the platform")
	if err := adminSSH(ctx, client, vm, addr); err != nil {
		return err
	}
	step("host key in ~/" + KnownHosts)
	if err := trustHostKey(ctx, vm, filepath.Join(set.Home, KnownHosts), addr.Admin.String()); err != nil {
		return err
	}
	step("ready. Next:")
	_, err = fmt.Fprint(out, `    ssh -t rye-admin claude                    log in to Claude once with /login, then /exit
    ssh -N -L 8081:127.0.0.1:8080 rye-admin    then open http://localhost:8081
The VM powers off after 30 minutes without agent-manager activity. make admin starts it again.
`)
	return err
}

// setupAdmin gives the admin VM's user sudo and ownerKey, and configures sshd, automatic upgrades and ryed -local's
// unit.
func setupAdmin(ctx context.Context, v vmexec.VM, ownerKey string) error {
	err := installPackages(ctx, v, map[string]string{"openssh-server": "/usr/sbin/sshd", "git": "/usr/bin/git",
		"curl": "/usr/bin/curl", "unattended-upgrades": "/usr/bin/unattended-upgrade"})
	if err != nil {
		return err
	}
	user := vmconfig.AdminUser
	uid, gid, err := ensureUser(ctx, v, user, "/bin/bash")
	if err != nil {
		return err
	}
	if err := v.Write(ctx, adminSudoers, user+" ALL=(ALL) NOPASSWD:ALL\n", 0o440, 0, 0); err != nil {
		return err
	}
	if err := authorize(ctx, v, user, uid, gid, ownerKey); err != nil {
		return err
	}
	if err := configureSSHD(ctx, v, adminSSHDPath, vmconfig.SSHD(user)); err != nil {
		return err
	}
	if err := v.Write(ctx, vmconfig.AutoUpgradesPath, vmconfig.AutoUpgrades, 0o644, 0, 0); err != nil {
		return err
	}
	return v.Write(ctx, adminUnitPath, ryedLocalUnit, 0o644, 0, 0)
}

// adminSSH lets the admin VM's own key log in to the service VM and, through the key endpoint, to guests. The
// admin VM gets the service VM's host key from Incus and rye's SSH aliases. Guests' host keys are taken on first
// use, as ryed does, because an agent's Bash call can't answer a prompt. Each run rewrites the admin VM's
// known_hosts.d/rye with the service VM's key alone.
func adminSSH(ctx context.Context, client *incus.Admin, v vmexec.VM, addr Addresses) error {
	user := vmconfig.AdminUser
	pub, err := ownKey(ctx, v, user, adminKeyName)
	if err != nil {
		return err
	}
	service := vmexec.New(client.VM(), ServiceVM)
	ryeUID, ryeGID, err := service.IDs(ctx, "rye")
	if err != nil {
		return err
	}
	if err := authorize(ctx, service, "rye", ryeUID, ryeGID, pub); err != nil {
		return err
	}
	key, err := hostKey(ctx, service)
	if err != nil {
		return err
	}
	uid, gid, err := v.IDs(ctx, user)
	if err != nil {
		return err
	}
	dir := adminUserHome + "/" + filepath.Dir(KnownHosts)
	if _, err := v.Root(ctx, "install", "-d", "-o", user, "-g", user, "-m", "700", dir); err != nil {
		return err
	}
	line := addr.Service.String() + " " + key + "\n"
	if err := v.Write(ctx, adminUserHome+"/"+KnownHosts, line, 0o600, uid, gid); err != nil {
		return err
	}
	config := "Host *.rye\n  StrictHostKeyChecking accept-new\n\n" + vmconfig.SSHConfig(addr.Service)
	return v.Write(ctx, adminSSHConfig, config, 0o600, uid, gid)
}

// AdminDown removes the admin VM, its key from the service VM, its firewall and its entry in rye's known_hosts
// file in home.
func AdminDown(ctx context.Context, client *incus.Admin, home string, out io.Writer) error {
	step := func(name string) { _, _ = fmt.Fprintln(out, "==> "+name) }
	vm, err := client.Instance(ctx, AdminVM)
	if err != nil {
		return err
	}
	if vm != nil {
		step("VM " + AdminVM)
		if err := client.DeleteInstance(ctx, AdminVM); err != nil {
			return err
		}
		if address := vm.Devices["eth0"]["ipv4.address"]; address != "" {
			if err := hostkeys.Remove(filepath.Join(home, KnownHosts), address); err != nil {
				return err
			}
		}
	}
	service, err := client.Instance(ctx, ServiceVM)
	if err != nil {
		return err
	}
	switch {
	case service != nil && service.Status == "Running":
		step("key " + adminKeyName + " from the service VM")
		if err := unauthorize(ctx, vmexec.New(client.VM(), ServiceVM), "rye", adminKeyName); err != nil {
			return err
		}
	case service != nil:
		step("service VM not running; the " + adminKeyName + " key stays in its authorized_keys until a later " +
			"make admin-remove")
	}
	step("firewall " + adminACL)
	if err := client.DeleteACL(ctx, adminACL); err != nil {
		return err
	}
	step("done")
	return nil
}

// unauthorize removes the keys with comment from user's authorized_keys.
func unauthorize(ctx context.Context, v vmexec.VM, user, comment string) error {
	current, err := authorizedKeys(ctx, v, user)
	if err != nil || current == "" {
		return err
	}
	uid, gid, err := v.IDs(ctx, user)
	if err != nil {
		return err
	}
	return v.Write(ctx, authorizedKeysPath(user), withoutComment(current, comment), 0o600, uid, gid)
}

// withoutComment returns authorized_keys content without the keys whose comment is comment.
func withoutComment(current, comment string) string {
	var kept []string
	for _, line := range strings.Split(strings.TrimRight(current, "\n"), "\n") {
		if !strings.HasSuffix(line, " "+comment) {
			kept = append(kept, line)
		}
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, "\n") + "\n"
}
