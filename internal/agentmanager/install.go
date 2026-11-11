package agentmanager

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	agentfiles "rye/agent-manager"
	"rye/internal/compute"
	"rye/internal/vmconfig"
	"rye/internal/vmexec"
)

// packagePath holds the package in the VM while the script unpacks it.
const packagePath = "/tmp/agent-manager.tgz"

// script runs as the user that owns agent-manager. It unpacks the package beside the installed agent-manager and runs
// its install.sh. install.sh installs the dependencies there and only then replaces the installed agent-manager.
const script = `set -eu
rm -rf ~/.local/share/agent-manager.new
mkdir -p ~/.local/share/agent-manager.new
tar -xz -C ~/.local/share/agent-manager.new -f ` + packagePath + `
rm -f ` + packagePath + `
sh ~/.local/share/agent-manager.new/install.sh`

// managerWait bounds the wait for the user's systemd manager after linger turns on.
var managerWait = 30 * time.Second

// Install installs Node.js and agent-manager in vm for user, and writes the install's output to out.
func Install(ctx context.Context, r vmexec.Runner, vm, user string, out io.Writer) error {
	v := vmexec.New(r, vm)
	uid, gid, err := v.IDs(ctx, user)
	if err != nil {
		return err
	}
	if _, err := v.Root(ctx, "sh", "-c", vmconfig.NodeSetup); err != nil {
		return err
	}
	// systemctl --user needs the user's systemd manager. A command from the guest agent has no login session to
	// start one, and linger starts it and keeps it running.
	if _, err := v.Root(ctx, "loginctl", "enable-linger", user); err != nil {
		return err
	}
	manager := "user@" + strconv.Itoa(uid) + ".service"
	for deadline := time.Now().Add(managerWait); ; {
		active, err := v.OK(ctx, "systemctl", "is-active", "--quiet", manager)
		if err != nil {
			return err
		}
		if active {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s in %s did not start within %s", manager, vm, managerWait)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	pkg, err := tarball()
	if err != nil {
		return err
	}
	file := compute.File{Path: packagePath, Body: bytes.NewReader(pkg), Size: int64(len(pkg)), Mode: 0o600, UID: uid, GID: gid}
	if err := v.Push(ctx, file); err != nil {
		return err
	}
	home := "/home/" + user
	stdout, err := v.Exec(ctx, compute.Command{Args: []string{"sh", "-c", script}, UID: uid, GID: gid, Dir: home,
		Env: map[string]string{
			"HOME":            home,
			"USER":            user,
			"PATH":            "/usr/local/bin:/usr/bin:/bin",
			"XDG_RUNTIME_DIR": "/run/user/" + strconv.Itoa(uid),
		}})
	_, _ = io.WriteString(out, stdout)
	if err != nil {
		return fmt.Errorf("installing agent-manager in %s: %w", vm, err)
	}
	_, err = v.Root(ctx, "sh", "-c", vmconfig.ClaudeLink(home))
	return err
}

// tarball returns agent-manager's files as a gzipped tar.
func tarball() ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if err := tw.AddFS(agentfiles.FS); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
