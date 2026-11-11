// Package platform brings the bridge, the firewalls, project rye and the service VM on an Incus host up to a build.
// It runs on the Incus host through the admin socket.
package platform

// Names that the platform gives what it creates on the host.
const (
	Bridge     = "ryebr0"
	Project    = "rye"
	ServiceVM  = "rye-service"
	AdminVM    = "rye-admin"
	guestACL   = "rye-guest"
	serviceACL = "rye-service"
	// certName is the service VM's restricted certificate in the host's trust store.
	certName = "rye"
)

// KnownHosts is the known_hosts file, under the home directory, that holds rye's host keys. rye ssh-config points
// rye's SSH aliases at it.
const KnownHosts = ".ssh/known_hosts.d/rye"

// Files in a build directory, which scripts/build.sh writes.
const (
	sourceFile  = "source.tar"
	versionFile = "VERSION"
)
