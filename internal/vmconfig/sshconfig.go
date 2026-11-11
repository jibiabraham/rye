package vmconfig

import (
	"fmt"
	"net/netip"
)

// AdminOctet is the last octet of the admin VM's address, in the service VM's /24.
const AdminOctet = 11

// GuestUser is the login user in guest VMs, who owns agent-manager there.
const GuestUser = "dev"

// AdminUser is the login user on the admin VM, who owns agent-manager there.
const AdminUser = "admin"

// AdminAddress returns the admin VM's address in service's /24.
func AdminAddress(service netip.Addr) netip.Addr {
	octets := service.As4()
	octets[3] = AdminOctet
	return netip.AddrFrom4(octets)
}

// SSHConfig returns the SSH client blocks for a deploy whose service VM is at service. rye names the service VM for
// commands, rye-gui forwards the GUI, rye-admin logs in to the admin VM, and <vm>.rye logs in to a VM through
// the service VM. The last block keeps their host keys in their own file, unhashed. ryectl up writes the service VM's
// key there, and ryectl up admin the admin VM's.
func SSHConfig(service netip.Addr) string {
	return fmt.Sprintf(`Host rye
  HostName %[1]s
  User rye
  RequestTTY no

Host rye-gui
  HostName %[1]s
  User rye
  LocalForward 8080 127.0.0.1:8080
  SessionType none

Host rye-admin
  HostName %[2]s
  User %[3]s

Host *.rye
  User %[4]s
  ProxyJump rye

Host rye rye-gui rye-admin *.rye
  UserKnownHostsFile ~/.ssh/known_hosts.d/rye
  HashKnownHosts no
`, service, AdminAddress(service), AdminUser, GuestUser)
}
