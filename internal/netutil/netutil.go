// Package netutil discovers the machine's own LAN addresses so the CLI and
// the dashboard can show other devices (and a tunnel) where to connect.
package netutil

import (
	"net"
	"sort"
)

// LANIPv4s returns the IPv4 addresses of up, non-loopback interfaces.
// Point-to-point links are skipped so VPN/tunnel interfaces (utun*) are not
// advertised as LAN addresses. Private addresses come first, then the rest;
// both groups are sorted so the output is stable across calls.
func LANIPv4s() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return []string{}
	}
	var raw []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 ||
			iface.Flags&net.FlagLoopback != 0 ||
			iface.Flags&net.FlagPointToPoint != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if ip4 := ipnet.IP.To4(); ip4 != nil && usableLANIPv4(ip4) {
				raw = append(raw, ip4.String())
			}
		}
	}
	return sortLANIPv4s(raw)
}

// usableLANIPv4 reports whether ip is an address another device could use
// to reach this machine.
func usableLANIPv4(ip net.IP) bool {
	if ip == nil || ip.To4() == nil {
		return false
	}
	return !ip.IsLoopback() && !ip.IsLinkLocalUnicast() &&
		!ip.IsUnspecified() && !ip.IsMulticast()
}

// sortLANIPv4s deduplicates and orders addresses: private first (the ones
// devices on the same network normally reach), then public, each sorted.
func sortLANIPv4s(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		pi, pj := isPrivateV4(out[i]), isPrivateV4(out[j])
		if pi != pj {
			return pi
		}
		return out[i] < out[j]
	})
	return out
}

// isPrivateV4 reports whether s is an RFC 1918 address or a CGNAT address.
func isPrivateV4(s string) bool {
	ip := net.ParseIP(s).To4()
	if ip == nil {
		return false
	}
	switch {
	case ip[0] == 10:
		return true
	case ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31:
		return true
	case ip[0] == 192 && ip[1] == 168:
		return true
	case ip[0] == 100 && ip[1] >= 64 && ip[1] <= 127:
		return true
	}
	return false
}
