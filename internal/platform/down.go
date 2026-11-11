package platform

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"rye/internal/compute/incus"
	"rye/internal/hostkeys"
)

// Down removes the service VM, its certificate and its entry in rye's known_hosts file in home. Guests and their data stay, and so
// do the bridge, the firewalls and the project, which the next Up reuses.
func Down(ctx context.Context, client *incus.Admin, home string, out io.Writer) error {
	step := func(name string) { _, _ = fmt.Fprintln(out, "==> "+name) }
	current, err := currentDeploy(ctx, client)
	if err != nil {
		return err
	}
	step("service VM " + ServiceVM)
	if err := client.DeleteInstance(ctx, ServiceVM); err != nil {
		return err
	}
	step("certificate " + certName)
	if err := client.Distrust(ctx, certName); err != nil {
		return err
	}
	if current.ServiceIP != "" {
		if err := hostkeys.Remove(filepath.Join(home, KnownHosts), current.ServiceIP); err != nil {
			return err
		}
	}
	step("done")
	return nil
}
