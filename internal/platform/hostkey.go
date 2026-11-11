package platform

import (
	"context"
	"fmt"
	"strings"

	"rye/internal/hostkeys"
	"rye/internal/vmexec"
)

// trustHostKey writes a VM's ed25519 host key, read through Incus, as the entry for address in the known_hosts
// file at path. The first ssh to the VM checks it.
func trustHostKey(ctx context.Context, v vmexec.VM, path, address string) error {
	key, err := hostKey(ctx, v)
	if err != nil {
		return err
	}
	return hostkeys.Set(path, address, key)
}

// hostKey returns a VM's ed25519 host key, read through Incus, as its type and key.
func hostKey(ctx context.Context, v vmexec.VM) (string, error) {
	pub, err := v.Read(ctx, "/etc/ssh/ssh_host_ed25519_key.pub")
	if err != nil {
		return "", err
	}
	fields := strings.Fields(pub)
	if len(fields) < 2 {
		return "", fmt.Errorf("%s has no ed25519 host key", v.Name)
	}
	return fields[0] + " " + fields[1], nil
}
