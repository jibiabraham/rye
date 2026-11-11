package platform

import "testing"

func TestChoose(t *testing.T) {
	tests := []struct {
		name                   string
		set, current           Settings
		bridge, service, admin string
		wantErr                bool
	}{
		{
			name:   "defaults",
			bridge: "198.18.0.1", service: "198.18.0.10", admin: "198.18.0.11",
		},
		{
			name:    "current deploy",
			current: Settings{BridgeIP: "198.19.0.1", ServiceIP: "198.19.0.10"},
			bridge:  "198.19.0.1", service: "198.19.0.10", admin: "198.19.0.11",
		},
		{
			name:    "setting wins",
			set:     Settings{BridgeIP: "198.20.0.1", ServiceIP: "198.20.0.9"},
			current: Settings{BridgeIP: "198.19.0.1"},
			bridge:  "198.20.0.1", service: "198.20.0.9", admin: "198.20.0.11",
		},
		{name: "service outside subnet", set: Settings{ServiceIP: "198.19.0.10"}, wantErr: true},
		{name: "service on admin", set: Settings{ServiceIP: "198.18.0.11"}, wantErr: true},
		{name: "service on bridge", set: Settings{ServiceIP: "198.18.0.1"}, wantErr: true},
		{name: "unparsable bridge", set: Settings{BridgeIP: "nope"}, wantErr: true},
		{name: "ipv6 service", set: Settings{ServiceIP: "::1"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := Choose(tt.set, tt.current)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Choose(%+v, %+v) = %+v, want an error", tt.set, tt.current, a)
				}
				return
			}
			if err != nil {
				t.Fatalf("Choose(%+v, %+v) = %v, want nil", tt.set, tt.current, err)
			}
			if a.Bridge.String() != tt.bridge || a.Service.String() != tt.service || a.Admin.String() != tt.admin {
				t.Errorf("Choose(%+v, %+v) = %+v, want bridge %s, service %s, admin %s",
					tt.set, tt.current, a, tt.bridge, tt.service, tt.admin)
			}
		})
	}
}

func TestDHCPRanges(t *testing.T) {
	tests := []struct {
		name    string
		current Settings
		want    string
	}{
		{name: "default bridge", want: "198.18.0.2-198.18.0.10,198.18.0.12-198.18.0.254"},
		{
			name:    "moved bridge",
			current: Settings{BridgeIP: "198.19.0.1", ServiceIP: "198.19.0.10"},
			want:    "198.19.0.2-198.19.0.10,198.19.0.12-198.19.0.254",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := Choose(Settings{}, tt.current)
			if err != nil {
				t.Fatal(err)
			}
			if got := a.DHCPRanges(); got != tt.want {
				t.Errorf("DHCPRanges() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRoutedElsewhere(t *testing.T) {
	a, err := Choose(Settings{}, Settings{})
	if err != nil {
		t.Fatal(err)
	}
	// 198.18.0.0 is 000012C6 in /proc/net/route's little-endian hex, and 00FFFFFF is a /24 mask.
	table := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\n" +
		"eth0\t00000000\t01480A0A\t0003\t0\t0\t0\t00000000\n" +
		"ryebr0\t000012C6\t00000000\t0001\t0\t0\t0\t00FFFFFF\n"
	tests := []struct {
		name  string
		table string
		want  string
	}{
		{name: "bridge route only", table: table, want: ""},
		{name: "second route", table: table + "eth1\t000012C6\t00000000\t0001\t0\t0\t0\t00FFFFFF\n", want: "eth1"},
		{name: "wider route", table: table + "eth1\t000012C6\t00000000\t0001\t0\t0\t0\t0000FFFF\n", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RoutedElsewhere(tt.table, a.Subnet()); got != tt.want {
				t.Errorf("RoutedElsewhere(%q, %v) = %q, want %q", tt.table, a.Subnet(), got, tt.want)
			}
		})
	}
}
