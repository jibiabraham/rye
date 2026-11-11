package vmconfig

import (
	"net/netip"
	"strings"
	"testing"
)

func TestSSHConfig(t *testing.T) {
	config := SSHConfig(netip.MustParseAddr("198.19.0.10"))
	if want := "Host rye-admin\n  HostName 198.19.0.11\n  User admin\n"; !strings.Contains(config, want) {
		t.Errorf("SSHConfig(198.19.0.10) = %q, want it to contain %q", config, want)
	}
}
