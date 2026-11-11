package platform

import (
	"bufio"
	"cmp"
	"encoding/binary"
	"fmt"
	"math/bits"
	"net/netip"
	"strconv"
	"strings"

	"rye/internal/vmconfig"
)

// Defaults in the RFC 2544 benchmarking range, outside the private ranges that guests are blocked from. A
// host that already uses this /24, such as a rye VM, sets BRIDGE_IP and SERVICE_IP to another one.
const (
	defaultBridgeIP  = "198.18.0.1"
	defaultServiceIP = "198.18.0.10"
)

// Addresses are the bridge's, the service VM's and the admin VM's, all in one /24. Guest DHCP leaves the admin
// address out, and guest NICs filter IPv4 to the DHCP ranges.
type Addresses struct {
	Bridge, Service, Admin netip.Addr
}

// Choose picks the bridge and service addresses from their settings, else from the current deploy, else the
// defaults. An empty string is unset. The admin VM is always at .11 of the bridge's /24.
func Choose(set, current Settings) (Addresses, error) {
	var a Addresses
	var err error
	if a.Bridge, err = pick("BRIDGE_IP", set.BridgeIP, current.BridgeIP, defaultBridgeIP); err != nil {
		return a, err
	}
	if a.Service, err = pick("SERVICE_IP", set.ServiceIP, current.ServiceIP, defaultServiceIP); err != nil {
		return a, err
	}
	a.Admin = vmconfig.AdminAddress(a.Bridge)
	switch {
	case !a.Subnet().Contains(a.Service):
		return a, fmt.Errorf("SERVICE_IP %s is outside the /24 of BRIDGE_IP %s", a.Service, a.Bridge)
	case a.Service == a.Bridge || a.Service == a.Admin:
		return a, fmt.Errorf("SERVICE_IP %s is the bridge's or the admin VM's address", a.Service)
	}
	return a, nil
}

// pick parses the first value that is set.
func pick(name string, values ...string) (netip.Addr, error) {
	v := cmp.Or(values...)
	if v == "" {
		return netip.Addr{}, fmt.Errorf("no %s", name)
	}
	addr, err := netip.ParseAddr(v)
	if err != nil || !addr.Is4() {
		return netip.Addr{}, fmt.Errorf("%s %q isn't an IPv4 address", name, v)
	}
	return addr, nil
}

// Subnet is the bridge's /24.
func (a Addresses) Subnet() netip.Prefix { return netip.PrefixFrom(a.Bridge, 24).Masked() }

// At returns the address in the bridge's /24 whose last octet is n.
func (a Addresses) At(n byte) string {
	octets := a.Bridge.As4()
	octets[3] = n
	return netip.AddrFrom4(octets).String()
}

// DHCPRanges leave the admin address out of guest DHCP.
func (a Addresses) DHCPRanges() string {
	c := a.Admin.As4()[3]
	return a.At(2) + "-" + a.At(c-1) + "," + a.At(c+1) + "-" + a.At(254)
}

// RoutedElsewhere returns the interface of a route to subnet that doesn't go through the bridge, from the text of
// /proc/net/route. A second route to the bridge's /24 takes over the host's own uplink and cuts the host off.
func RoutedElsewhere(table string, subnet netip.Prefix) string {
	scanner := bufio.NewScanner(strings.NewReader(table))
	scanner.Scan() // The header.
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 8 || fields[0] == Bridge {
			continue
		}
		destination, err1 := strconv.ParseUint(fields[1], 16, 32)
		mask, err2 := strconv.ParseUint(fields[7], 16, 32)
		if err1 != nil || err2 != nil {
			continue
		}
		var d [4]byte
		binary.LittleEndian.PutUint32(d[:], uint32(destination))
		if netip.PrefixFrom(netip.AddrFrom4(d), bits.OnesCount32(uint32(mask))) == subnet {
			return fields[0]
		}
	}
	return ""
}
