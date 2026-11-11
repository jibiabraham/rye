package platform

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"rye/internal/compute"
	"rye/internal/compute/incus"
)

func TestAdminDown(t *testing.T) {
	const (
		keys     = "/home/rye/.ssh/authorized_keys"
		hosts    = "198.18.0.10 ssh-ed25519 SERVICE\n198.18.0.11 ssh-ed25519 ADMIN\n"
		ownerKey = "ssh-ed25519 AAAA owner"
		bothKeys = ownerKey + "\nssh-ed25519 BBBB " + adminKeyName + "\n"
	)
	adminVM := func() *incus.Instance {
		return &incus.Instance{Status: "Running", Devices: map[string]map[string]string{
			"eth0": {"type": "nic", "ipv4.address": "198.18.0.11"},
		}}
	}
	replies := map[string]compute.Result{"id -u rye": {Stdout: "1001\n"}, "id -g rye": {Stdout: "1001\n"}}
	tests := []struct {
		name      string
		instances map[string]*incus.Instance
		files     map[string]string
		wantVMs   []string
		wantKeys  string
		wantHosts string
		wantOut   string
	}{
		{
			name:      "running service VM",
			instances: map[string]*incus.Instance{AdminVM: adminVM(), ServiceVM: {Status: "Running"}},
			files:     map[string]string{keys: bothKeys},
			wantVMs:   []string{ServiceVM},
			wantKeys:  ownerKey + "\n",
			wantHosts: "198.18.0.10 ssh-ed25519 SERVICE\n",
		},
		{
			name:      "stopped service VM",
			instances: map[string]*incus.Instance{AdminVM: adminVM(), ServiceVM: {Status: "Stopped"}},
			files:     map[string]string{keys: bothKeys},
			wantVMs:   []string{ServiceVM},
			wantKeys:  bothKeys,
			wantHosts: "198.18.0.10 ssh-ed25519 SERVICE\n",
			wantOut:   "key stays",
		},
		{
			name:      "nothing exists",
			wantHosts: hosts,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			known := filepath.Join(home, KnownHosts)
			if err := os.MkdirAll(filepath.Dir(known), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(known, []byte(hosts), 0o600); err != nil {
				t.Fatal(err)
			}
			f := &fakeIncus{instances: tt.instances, files: tt.files, replies: replies}
			client := newFakeIncus(t, f)
			var out bytes.Buffer
			if err := AdminDown(context.Background(), client, home, &out); err != nil {
				t.Fatalf("AdminDown() = %v, want nil", err)
			}
			var vms []string
			for name := range f.instances {
				vms = append(vms, name)
			}
			slices.Sort(vms)
			if !slices.Equal(vms, tt.wantVMs) {
				t.Errorf("VMs afterwards = %q, want %q", vms, tt.wantVMs)
			}
			if got := f.files[keys]; got != tt.wantKeys {
				t.Errorf("%s = %q, want %q", keys, got, tt.wantKeys)
			}
			if calls := f.recorded(); !slices.Contains(calls, "delete acl "+adminACL) {
				t.Errorf("calls = %q, want them to include %q", calls, "delete acl "+adminACL)
			}
			got, err := os.ReadFile(filepath.Clean(known))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.wantHosts {
				t.Errorf("%s = %q, want %q", KnownHosts, got, tt.wantHosts)
			}
			if !strings.Contains(out.String(), tt.wantOut) {
				t.Errorf("output = %q, want it to contain %q", out.String(), tt.wantOut)
			}
		})
	}
}
