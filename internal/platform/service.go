package platform

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"rye/internal/compute/incus"
	"rye/internal/vmconfig"
	"rye/internal/vmexec"
)

// Paths on the service VM.
const (
	ryedUnitPath = "/etc/systemd/system/ryed.service"
	sshdDropIn   = "/etc/ssh/sshd_config.d/10-rye.conf"
)

// serviceSSHD lets the rye user log in with a key and run only rye, with forwarding only to the GUI port and to
// guests' sshd.
var serviceSSHD = vmconfig.SSHD("rye",
	"Match User rye",
	"  ForceCommand /usr/local/bin/rye",
	"  AllowTcpForwarding local",
	"  # The GUI tunnel and ProxyJump to guests' sshd.",
	"  PermitOpen 127.0.0.1:8080 *:22",
	"  AllowStreamLocalForwarding no",
	"  AllowAgentForwarding no",
	"  X11Forwarding no",
	"  PermitTTY no",
)

// autoReboot restarts the service VM at 04:00 when an upgrade needs it, such as a new kernel.
const autoReboot = `Unattended-Upgrade::Automatic-Reboot "true";
Unattended-Upgrade::Automatic-Reboot-Time "04:00";
`

// setupService configures the service VM's rye user and the keys that log in as it, sshd, ryed's client certificate
// for Incus, automatic upgrades and Tailscale. ownerKey may log in as rye.
func setupService(ctx context.Context, v vmexec.VM, ownerKey string) error {
	err := installPackages(ctx, v, map[string]string{"openssh-server": "/usr/sbin/sshd",
		"unattended-upgrades": "/usr/bin/unattended-upgrade"})
	if err != nil {
		return err
	}
	uid, gid, err := ensureUser(ctx, v, "rye", "/bin/sh")
	if err != nil {
		return err
	}
	if _, err := v.Root(ctx, "mkdir", "-p", "/etc/rye"); err != nil {
		return err
	}
	if _, err := v.Root(ctx, "install", "-d", "-o", "rye", "-g", "rye", "-m", "700", vmconfig.StateDir); err != nil {
		return err
	}
	// Guests accept the service's own key through the key endpoint, which serves rye's authorized_keys.
	own, err := ownKey(ctx, v, "rye", "rye-service")
	if err != nil {
		return err
	}
	if err := authorize(ctx, v, "rye", uid, gid, ownerKey, own); err != nil {
		return err
	}
	if err := configureSSHD(ctx, v, sshdDropIn, serviceSSHD); err != nil {
		return err
	}
	hasKey, err := v.OK(ctx, "test", "-s", vmconfig.ClientKeyPath)
	if err != nil {
		return err
	}
	if !hasKey {
		certPEM, keyPEM, err := clientCertificate(time.Now())
		if err != nil {
			return err
		}
		if err := v.Write(ctx, vmconfig.ClientKeyPath, string(keyPEM), 0o640, 0, gid); err != nil {
			return err
		}
		if err := v.Write(ctx, vmconfig.ClientCertPath, string(certPEM), 0o644, 0, 0); err != nil {
			return err
		}
	}
	if err := v.Write(ctx, vmconfig.AutoUpgradesPath, vmconfig.AutoUpgrades+autoReboot, 0o644, 0, 0); err != nil {
		return err
	}
	return installTailscale(ctx, v)
}

// installPackages installs, with apt, each package whose file is missing. packages maps a package to a file it holds.
func installPackages(ctx context.Context, v vmexec.VM, packages map[string]string) error {
	var missing []string
	for pkg, file := range packages {
		installed, err := v.OK(ctx, "test", "-e", file)
		if err != nil {
			return err
		}
		if !installed {
			missing = append(missing, pkg)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	slices.Sort(missing)
	if _, err := v.Root(ctx, "apt-get", "update", "-qq"); err != nil {
		return err
	}
	_, err := v.Root(ctx, append([]string{"apt-get", "install", "-y", "-qq"}, missing...)...)
	return err
}

// ensureUser creates user with a home directory and an empty ~/.ssh when it's missing, and returns its IDs.
func ensureUser(ctx context.Context, v vmexec.VM, user, shell string) (uid, gid int, err error) {
	exists, err := v.OK(ctx, "id", user)
	if err != nil {
		return 0, 0, err
	}
	if !exists {
		if _, err := v.Root(ctx, "useradd", "-m", "-s", shell, user); err != nil {
			return 0, 0, err
		}
	}
	if _, err := v.Root(ctx, "install", "-d", "-o", user, "-g", user, "-m", "700", "/home/"+user+"/.ssh"); err != nil {
		return 0, 0, err
	}
	return v.IDs(ctx, user)
}

// ownKey returns user's public key, ~/.ssh/id_ed25519.pub, which ssh-keygen makes with comment when it's missing.
func ownKey(ctx context.Context, v vmexec.VM, user, comment string) (string, error) {
	path := "/home/" + user + "/.ssh/id_ed25519"
	exists, err := v.OK(ctx, "test", "-s", path)
	if err != nil {
		return "", err
	}
	if !exists {
		if _, err := v.Root(ctx, "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", comment, "-f", path); err != nil {
			return "", err
		}
		if _, err := v.Root(ctx, "chown", user+":"+user, path, path+".pub"); err != nil {
			return "", err
		}
	}
	key, err := v.Read(ctx, path+".pub")
	return strings.TrimSpace(key), err
}

// authorize adds each key to user's authorized_keys once, and keeps the keys already there.
func authorize(ctx context.Context, v vmexec.VM, user string, uid, gid int, keys ...string) error {
	current, err := authorizedKeys(ctx, v, user)
	if err != nil {
		return err
	}
	return v.Write(ctx, authorizedKeysPath(user), withKeys(current, keys...), 0o600, uid, gid)
}

func authorizedKeysPath(user string) string { return "/home/" + user + "/.ssh/authorized_keys" }

// authorizedKeys returns user's authorized_keys, which is empty when the file is missing.
func authorizedKeys(ctx context.Context, v vmexec.VM, user string) (string, error) {
	exists, err := v.OK(ctx, "test", "-e", authorizedKeysPath(user))
	if err != nil || !exists {
		return "", err
	}
	return v.Read(ctx, authorizedKeysPath(user))
}

// configureSSHD writes an sshd drop-in at path and restarts sshd.
func configureSSHD(ctx context.Context, v vmexec.VM, path, config string) error {
	if err := v.Write(ctx, path, config, 0o644, 0, 0); err != nil {
		return err
	}
	_, err := v.Root(ctx, "systemctl", "restart", "ssh")
	return err
}

// withKeys returns authorized_keys content with each key added once.
func withKeys(current string, keys ...string) string {
	var lines []string
	if current != "" {
		lines = strings.Split(strings.TrimRight(current, "\n"), "\n")
	}
	for _, key := range keys {
		if !slices.Contains(lines, key) {
			lines = append(lines, key)
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// clientCertificate makes ryed's key and its self-signed client certificate for the Incus API, valid for ten years
// from now.
func clientCertificate(now time.Time) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, nil, err
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "rye"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), nil
}

// installTailscale installs Tailscale from its apt repository and turns on its own updates. make tailscale logs it
// in and serves the GUI on the tailnet.
func installTailscale(ctx context.Context, v vmexec.VM) error {
	installed, err := v.OK(ctx, "test", "-x", "/usr/bin/tailscale")
	if err != nil {
		return err
	}
	if !installed {
		release, err := v.Root(ctx, "sh", "-c", ". /etc/os-release && echo $VERSION_CODENAME")
		if err != nil {
			return err
		}
		base := "https://pkgs.tailscale.com/stable/ubuntu/" + strings.TrimSpace(release)
		for _, args := range [][]string{
			{"curl", "-fsSL", base + ".noarmor.gpg", "-o", "/usr/share/keyrings/tailscale-archive-keyring.gpg"},
			{"curl", "-fsSL", base + ".tailscale-keyring.list", "-o", "/etc/apt/sources.list.d/tailscale.list"},
			{"apt-get", "update", "-qq"},
			{"apt-get", "install", "-y", "-qq", "tailscale"},
		} {
			if _, err := v.Root(ctx, args...); err != nil {
				return err
			}
		}
	}
	if _, err := v.Root(ctx, "systemctl", "enable", "--now", "tailscaled"); err != nil {
		return err
	}
	_, err = v.Root(ctx, "tailscale", "set", "--auto-update")
	return err
}

// installBuild puts a build on the service VM and restarts ryed. ryed reaches Incus at bridgeIP and trusts only
// serverCert.
func installBuild(ctx context.Context, v vmexec.VM, dist, serverCert, bridgeIP, pool string) error {
	for _, name := range []string{"rye", "ryed"} {
		if err := v.Copy(ctx, "/usr/local/bin/"+name, filepath.Join(dist, name), 0o755, 0, 0); err != nil {
			return err
		}
	}
	config, err := json.MarshalIndent(incus.Config{URL: "https://" + bridgeIP + ":8443", Project: Project, Pool: pool,
		Bridge: Bridge}, "", "  ")
	if err != nil {
		return err
	}
	for path, data := range map[string]string{
		ryedUnitPath:             ryedUnit,
		vmconfig.ServerCertPath:  serverCert,
		vmconfig.IncusConfigPath: string(config) + "\n",
	} {
		if err := v.Write(ctx, path, data, 0o644, 0, 0); err != nil {
			return err
		}
	}
	// ryed serves the docs archive on docs.int.rye and reads it on the first request after the restart below.
	if err := v.Copy(ctx, vmconfig.StateDir+"/"+sourceFile, filepath.Join(dist, sourceFile), 0o644, 0, 0); err != nil {
		return err
	}
	for _, args := range [][]string{{"systemctl", "daemon-reload"}, {"systemctl", "enable", "ryed"}, {"systemctl", "restart", "ryed"}} {
		if _, err := v.Root(ctx, args...); err != nil {
			return err
		}
	}
	return nil
}
