package platform

import _ "embed" // the ryed units

// ryedUnit is ryed's systemd unit on the service VM.
//
//go:embed ryed.service
var ryedUnit string

// ryedLocalUnit is the admin VM's unit for ryed -local, which serves the GUI for that VM's agent-manager.
//
//go:embed ryed-local.service
var ryedLocalUnit string
