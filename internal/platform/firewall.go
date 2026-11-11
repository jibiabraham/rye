package platform

import "rye/internal/compute/incus"

// guestFirewall rejects guests' traffic to private ranges and the host, and lets in SSH from the service VM.
func guestFirewall(a Addresses) incus.ACL {
	return incus.ACL{
		Description: "Guest egress",
		Egress:      guestEgress(a),
		Ingress: []incus.Rule{
			{Action: "allow", Source: a.Service.String() + "/32", Protocol: "tcp", DestinationPort: "22", State: "enabled"},
		},
	}
}

// guestEgress rejects traffic to private ranges and the host, and allows the rest. Incus applies a reject before an
// allow, whatever their order.
func guestEgress(a Addresses) []incus.Rule {
	return []incus.Rule{
		{Action: "reject", Destination: "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,169.254.0.0/16,100.64.0.0/10," +
			"127.0.0.0/8," + a.Bridge.String() + "/32", State: "enabled"},
		{Action: "allow", Destination: "0.0.0.0/0", State: "enabled"},
	}
}

// serviceFirewall lets in SSH from the host and the admin VM, and the guest endpoint on port 80 from the bridge.
// Guests can't hold the admin address, because DHCP leaves it out and guest NICs filter IPv4.
func serviceFirewall(a Addresses) incus.ACL {
	return incus.ACL{
		Description: "Service VM",
		Egress:      []incus.Rule{{Action: "allow", State: "enabled"}},
		Ingress: []incus.Rule{
			{Action: "allow", Source: a.Bridge.String() + "/32", Protocol: "tcp", DestinationPort: "22", State: "enabled"},
			{Action: "allow", Source: a.Admin.String() + "/32", Protocol: "tcp", DestinationPort: "22", State: "enabled"},
			{Action: "allow", Source: a.Subnet().String(), Protocol: "tcp", DestinationPort: "80", State: "enabled"},
		},
	}
}

// guestNIC is the NIC in project rye's default profile, which every guest gets.
func guestNIC() map[string]map[string]string {
	return map[string]map[string]string{"eth0": {
		"type": "nic", "name": "eth0", "network": Bridge,
		"security.mac_filtering": "true", "security.ipv4_filtering": "true", "security.ipv6_filtering": "true",
		"security.port_isolation": "true", "security.acls": guestACL, "ipv6.address": "none",
	}}
}

// adminFirewall lets in SSH from the host only, and limits egress as a guest's is.
func adminFirewall(a Addresses) incus.ACL {
	return incus.ACL{
		Description: "Admin VM",
		Egress:      guestEgress(a),
		Ingress: []incus.Rule{
			{Action: "allow", Source: a.Bridge.String() + "/32", Protocol: "tcp", DestinationPort: "22", State: "enabled"},
		},
	}
}
