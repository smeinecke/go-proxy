package nio

import (
	"net"
	"testing"
)

func TestIsBlocked(t *testing.T) {
	tests := []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true},
		{"127.255.255.255", true},
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.0.1", true},
		{"192.168.255.255", true},
		{"169.254.0.1", true},
		{"169.254.169.254", true},
		{"100.64.0.1", true},
		{"100.127.255.255", true},
		{"0.0.0.0", true},
		{"0.255.255.255", true},
		{"::1", true},
		{"fc00::1", true},
		{"fe80::1", true},
		{"ff02::1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"2001:db8::1", false},
	}

	for _, tc := range tests {
		parsed := net.ParseIP(tc.ip)
		if parsed == nil {
			t.Fatalf("failed to parse IP %q", tc.ip)
		}
		got := IsBlockedDefault(parsed)
		if got != tc.blocked {
			t.Errorf("IsBlockedDefault(%q) = %v, want %v", tc.ip, got, tc.blocked)
		}
	}
}
