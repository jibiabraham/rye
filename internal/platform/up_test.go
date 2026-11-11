package platform

import (
	"context"
	"slices"
	"strings"
	"testing"

	"rye/internal/compute/incus"
)

func TestCheckAddresses(t *testing.T) {
	const header = "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\n"
	// 198.18.0.0 is 000012C6 in /proc/net/route's little-endian hex, and 00FFFFFF is a /24 mask.
	tests := []struct {
		name             string
		set, current     Settings
		table            string
		bridge, service  string
		wantErrSubstring string
	}{
		{name: "defaults", table: header, bridge: "198.18.0.1", service: "198.18.0.10"},
		{
			name:    "current deploy kept",
			current: Settings{BridgeIP: "198.19.0.1", ServiceIP: "198.19.0.20"},
			table:   header,
			bridge:  "198.19.0.1", service: "198.19.0.20",
		},
		{
			name:             "moved bridge",
			set:              Settings{BridgeIP: "198.19.0.1", ServiceIP: "198.19.0.10"},
			current:          Settings{BridgeIP: "198.18.0.1", ServiceIP: "198.18.0.10"},
			table:            header,
			wantErrSubstring: Bridge,
		},
		{
			name:             "routed subnet",
			table:            header + "eth0\t000012C6\t00000000\t0001\t0\t0\t0\t00FFFFFF\n",
			wantErrSubstring: "eth0",
		},
		{
			name:   "bridge route only",
			table:  header + "ryebr0\t000012C6\t00000000\t0001\t0\t0\t0\t00FFFFFF\n",
			bridge: "198.18.0.1", service: "198.18.0.10",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := checkAddresses(tt.set, tt.current, tt.table)
			if tt.wantErrSubstring != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSubstring) {
					t.Errorf("checkAddresses() error = %v, want one naming %s", err, tt.wantErrSubstring)
				}
				return
			}
			if err != nil {
				t.Fatalf("checkAddresses() = %v, want nil", err)
			}
			if a.Bridge.String() != tt.bridge || a.Service.String() != tt.service {
				t.Errorf("checkAddresses() = bridge %s, service %s, want bridge %s, service %s",
					a.Bridge, a.Service, tt.bridge, tt.service)
			}
		})
	}
}

func TestCurrentDeploy(t *testing.T) {
	service := &incus.Instance{Status: "Running", Devices: map[string]map[string]string{
		"root": {"type": "disk", "path": "/", "pool": "fast"},
		"eth0": {"type": "nic", "network": Bridge, "ipv4.address": "198.19.0.10"},
	}}
	bridge := map[string]map[string]string{Bridge: {"ipv4.address": "198.19.0.1/24"}}
	tests := []struct {
		name      string
		networks  map[string]map[string]string
		instances map[string]*incus.Instance
		want      Settings
	}{
		{
			name:      "deployed",
			networks:  bridge,
			instances: map[string]*incus.Instance{ServiceVM: service},
			want:      Settings{BridgeIP: "198.19.0.1", ServiceIP: "198.19.0.10", Pool: "fast"},
		},
		{name: "no service VM", networks: bridge, want: Settings{BridgeIP: "198.19.0.1"}},
		{name: "nothing", want: Settings{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newFakeIncus(t, &fakeIncus{networks: tt.networks, instances: tt.instances})
			got, err := currentDeploy(context.Background(), client)
			if err != nil {
				t.Fatalf("currentDeploy() = %v, want nil", err)
			}
			if got != tt.want {
				t.Errorf("currentDeploy() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestEnsureVM(t *testing.T) {
	const name = "rye-test"
	tests := []struct {
		name     string
		existing *incus.Instance
		want     []string
	}{
		{name: "missing", want: []string{"create " + name, "start " + name}},
		{name: "stopped", existing: &incus.Instance{Status: "Stopped"}, want: []string{"start " + name}},
		{name: "running", existing: &incus.Instance{Status: "Running"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeIncus{instances: map[string]*incus.Instance{}}
			if tt.existing != nil {
				f.instances[name] = tt.existing
			}
			client := newFakeIncus(t, f)
			if err := ensureVM(context.Background(), client, name, map[string]string{"limits.cpu": "1"}, nil); err != nil {
				t.Fatalf("ensureVM() = %v, want nil", err)
			}
			if got := f.recorded(); !slices.Equal(got, tt.want) {
				t.Errorf("calls = %q, want %q", got, tt.want)
			}
		})
	}
}
