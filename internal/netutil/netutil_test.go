package netutil

import (
	"net"
	"reflect"
	"testing"
)

func TestUsableLANIPv4(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"192.168.0.103", true},
		{"10.0.0.5", true},
		{"172.16.4.1", true},
		{"100.64.0.1", true},
		{"8.8.8.8", true},
		{"127.0.0.1", false},
		{"169.254.10.1", false},
		{"0.0.0.0", false},
		{"224.0.0.251", false},
		{"::1", false},
		{"fe80::1", false},
	}
	for _, c := range cases {
		if got := usableLANIPv4(net.ParseIP(c.ip)); got != c.want {
			t.Errorf("usableLANIPv4(%s) = %v, want %v", c.ip, got, c.want)
		}
	}
}

func TestSortLANIPv4s(t *testing.T) {
	got := sortLANIPv4s([]string{
		"8.8.8.8",
		"192.168.0.103",
		"10.0.0.5",
		"192.168.0.103",
		"172.16.4.1",
		"1.1.1.1",
	})
	want := []string{"10.0.0.5", "172.16.4.1", "192.168.0.103", "1.1.1.1", "8.8.8.8"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sortLANIPv4s = %v, want %v", got, want)
	}
}

func TestIsPrivateV4(t *testing.T) {
	for _, s := range []string{"10.1.2.3", "172.16.0.1", "192.168.1.1", "100.64.0.1"} {
		if !isPrivateV4(s) {
			t.Errorf("isPrivateV4(%s) = false, want true", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "172.32.0.1", "100.128.0.1", "not-an-ip"} {
		if isPrivateV4(s) {
			t.Errorf("isPrivateV4(%s) = true, want false", s)
		}
	}
}

func TestLANIPv4sNoLoopback(t *testing.T) {
	for _, s := range LANIPv4s() {
		ip := net.ParseIP(s)
		if ip == nil || ip.IsLoopback() {
			t.Fatalf("LANIPv4s returned a non-LAN address: %q", s)
		}
	}
}
