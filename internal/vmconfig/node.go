package vmconfig

import "fmt"

// NodeMajor is the Node.js release line that agent-manager runs on in VMs. It names an LTS line.
const NodeMajor = 26

// NodeSetup is a root shell script that adds NodeSource's apt repository for NodeMajor, lets unattended-upgrades
// update it, and installs Node.js when the VM has another release line or none.
var NodeSetup = fmt.Sprintf(`set -eu
export DEBIAN_FRONTEND=noninteractive
install -d -m 755 /etc/apt/keyrings
curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key -o /etc/apt/keyrings/nodesource.asc
printf 'Types: deb\nURIs: https://deb.nodesource.com/node_%[1]d.x\nSuites: nodistro\nComponents: main\nSigned-By: /etc/apt/keyrings/nodesource.asc\n' >/etc/apt/sources.list.d/nodesource.sources
printf 'Unattended-Upgrade::Origins-Pattern {\n  "site=deb.nodesource.com";\n};\n' >/etc/apt/apt.conf.d/52unattended-nodesource
case "$(node --version 2>/dev/null)" in
v%[1]d.*) ;;
*) apt-get update -qq && apt-get install -y -qq nodejs ;;
esac
`, NodeMajor)

// ClaudeLink returns the command that links the claude CLI from agent-manager's install in home into /usr/local/bin,
// for every shell in the VM.
func ClaudeLink(home string) string {
	return "ln -sfn " + home + "/.local/share/agent-manager/claude /usr/local/bin/claude"
}
