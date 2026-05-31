package nio

import (
	"net"

	"github.com/vlourme/go-proxy/internal/config"
)

var defaultBlockedCIDRs = []string{
	"127.0.0.0/8",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"169.254.0.0/16",
	"100.64.0.0/10",
	"0.0.0.0/8",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
	"ff00::/8",
}

var blockedNets []*net.IPNet

func init() {
	for _, cidr := range defaultBlockedCIDRs {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		blockedNets = append(blockedNets, ipnet)
	}
}

// IsBlockedDefault checks only the built-in private/reserved ranges.
// It does not depend on config and is safe to use in tests.
func IsBlockedDefault(ip net.IP) bool {
	for _, n := range blockedNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// IsBlocked checks if the given IP is in a blocked private, reserved, or multicast range,
// including any user-configured blocked_cidrs.
func IsBlocked(ip net.IP) bool {
	if IsBlockedDefault(ip) {
		return true
	}
	for _, n := range config.GetBlockedNets() {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
