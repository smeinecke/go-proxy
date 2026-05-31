package utils

import (
	"net"
	"net/netip"
	"testing"
)

func TestGenerateIPv4(t *testing.T) {
	cidrs := []string{
		"192.168.1.0/24",
		"10.0.0.0/8",
		"172.16.0.0/12",
	}

	for _, cidr := range cidrs {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			t.Errorf("net.ParseCIDR() returned error: %v", err)
		}

		ip, err := GenerateIP(*ipnet)
		if err != nil {
			t.Errorf("GenerateIP() returned error: %v", err)
		}

		if !ipnet.Contains(ip) {
			t.Errorf("Generated IP %v is not in network %v", ip, cidr)
		}

		t.Logf("Generated IP: %v", ip)
	}
}

func BenchmarkGenerateNetIPv6(b *testing.B) {
	prefix := netip.MustParsePrefix("2001:db8::/32")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = GenerateNetIP(prefix)
	}
}

func BenchmarkGenerateNetIPv4(b *testing.B) {
	prefix := netip.MustParsePrefix("192.168.0.0/16")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = GenerateNetIP(prefix)
	}
}

func TestGenerateIPv6(t *testing.T) {
	cidrs := []string{
		"2001:db8::/32",
		"2002:1234::/48",
		"2003:4567::/64",
		"2004:89ab::/72",
		"2005:cdef::/80",
		"2006:1234::/96",
		"2007:5678::/112",
		"2008:9abc::/120",
	}

	for _, cidr := range cidrs {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			t.Errorf("net.ParseCIDR() returned error: %v", err)
		}

		ip, err := GenerateIP(*ipnet)
		if err != nil {
			t.Errorf("GenerateIP() returned error: %v", err)
		}

		if !ipnet.Contains(ip) {
			t.Errorf("Generated IP %v is not in network %v", ip, cidr)
		}

		t.Logf("Generated IP: %v", ip)
	}
}
