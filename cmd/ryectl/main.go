// Command ryectl brings rye's part of this Incus host up to the build that holds it, which scripts/build.sh writes.
// It runs on the Incus host, as a member of incus-admin.
//
//	ryectl up             creates or upgrades the service VM from the build in ryectl's directory
//	ryectl down           removes the service VM and its certificate, and keeps the VMs
//	ryectl up admin       creates, upgrades or starts the admin VM from the build
//	ryectl down admin     removes the admin VM, its key and its firewall
//	ryectl ssh-config     writes the SSH aliases for this deploy to ~/.ssh/config.d/rye
//
// up reads POOL, BRIDGE_IP, SERVICE_IP and KEY from the environment, and up admin reads KEY.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"rye/internal/compute/incus"
	"rye/internal/platform"
)

const usage = `usage:
  ryectl up             create or upgrade the service VM from this build
  ryectl down           remove the service VM and its certificate; VMs stay
  ryectl up admin       create, upgrade or start the admin VM from this build
  ryectl down admin     remove the admin VM, its key and its firewall
  ryectl ssh-config     write the SSH aliases for this deploy to ~/.ssh/config.d/rye
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:])
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ryectl:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	command := strings.Join(args, " ")
	switch command {
	case "up", "down", "up admin", "down admin", "ssh-config":
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", command)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	client, err := incus.ConnectAdmin(ctx, incus.SocketPath)
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	dist := filepath.Dir(self)
	set := platform.Settings{Pool: os.Getenv("POOL"), BridgeIP: os.Getenv("BRIDGE_IP"), ServiceIP: os.Getenv("SERVICE_IP"),
		Key: os.Getenv("KEY"), Home: home}
	switch command {
	case "up":
		return platform.Up(ctx, client, dist, set, os.Stdout)
	case "down":
		return platform.Down(ctx, client, home, os.Stdout)
	case "up admin":
		return platform.AdminUp(ctx, client, dist, set, os.Stdout)
	case "down admin":
		return platform.AdminDown(ctx, client, home, os.Stdout)
	default:
		config, err := platform.SSHConfig(ctx, client)
		if err != nil {
			return err
		}
		included, err := platform.WriteSSHConfig(home, config)
		if err != nil {
			return err
		}
		fmt.Println("Wrote ~/.ssh/config.d/rye")
		if !included {
			fmt.Println("Add this line at the top of ~/.ssh/config, before any Host block: " + platform.SSHInclude)
		}
		return nil
	}
}
