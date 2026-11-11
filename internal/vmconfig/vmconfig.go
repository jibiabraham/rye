// Package vmconfig holds the configuration that the VMs share and that ryectl, rye and ryed agree on. That is the
// service VM's file layout, the sshd drop-in, the automatic-upgrade settings, Node.js's setup and the SSH client config.
package vmconfig

import (
	"strings"
)

// AutoUpgradesPath is where apt reads AutoUpgrades.
const AutoUpgradesPath = "/etc/apt/apt.conf.d/20auto-upgrades"

// AutoUpgrades turns on the daily package list update and unattended security upgrades.
const AutoUpgrades = `APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
`

// SSHD is an sshd_config drop-in that allows key logins for user only. Each extra line follows the shared ones.
func SSHD(user string, extra ...string) string {
	lines := append([]string{
		"PasswordAuthentication no",
		"KbdInteractiveAuthentication no",
		"PermitRootLogin no",
		"AllowUsers " + user,
	}, extra...)
	return strings.Join(lines, "\n") + "\n"
}
