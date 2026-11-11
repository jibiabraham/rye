package vmconfig

// The service VM's files. rye and ryed read and write them, and ryectl up puts the build's files there.
const (
	// StateDir holds what rye and ryed keep across upgrades.
	StateDir = "/var/lib/rye"
	// IntegrationsFile, in StateDir, holds the integrations and their attachments.
	IntegrationsFile = "integrations.json"
	// KnownHostsFile, in StateDir, holds the guests' host keys, which ryed takes on first use.
	KnownHostsFile = "known_hosts"
	// IntegrationsPath and KnownHostsPath are those files' paths.
	IntegrationsPath = StateDir + "/" + IntegrationsFile
	KnownHostsPath   = StateDir + "/" + KnownHostsFile
	// SourcePath is the git archive of the build, which ryed serves as the docs.
	SourcePath = StateDir + "/source.tar"

	// IncusConfigPath holds ryed's Incus address and project.
	IncusConfigPath = "/etc/rye/config.json"
	// ServerCertPath is the Incus server's certificate, which ryed trusts.
	ServerCertPath = "/etc/rye/server.crt"
	// ClientCertPath and ClientKeyPath are ryed's restricted Incus client certificate.
	ClientCertPath = "/etc/rye/client.crt"
	ClientKeyPath  = "/etc/rye/client.key"

	// RyeKeysPath is the rye user's authorized_keys, which ryed also serves to guests on keys.int.rye.
	RyeKeysPath = "/home/rye/.ssh/authorized_keys"

	// AgentManagerPort is the port agent-manager listens on, on each VM's loopback interface.
	AgentManagerPort = "7420"
)
