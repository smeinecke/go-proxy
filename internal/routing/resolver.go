package routing

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/phuslu/lru"
	"github.com/rs/zerolog/log"
)

const dnsCacheTTL = 3 * time.Minute

var (
	ErrNoLocalhost = errors.New("localhost is not allowed in non-debug mode")
	ErrNoIPFound   = errors.New("no IP address found")
	ErrBlocked     = errors.New("destination IP is blocked")
)

// ReplacementRule maps a CIDR prefix to a replacement IP.
type ReplacementRule struct {
	Prefix netip.Prefix
	IP     netip.Addr
}

// Resolver resolves hostnames to IPs with caching, blocking, and replacement.
type Resolver struct {
	preferGo     bool
	customServer string
	timeout      time.Duration
	debugMode    bool
	cache        *lru.TTLCache[string, string]
	blocked      []netip.Prefix
	replaces     []ReplacementRule
}

// NewResolver creates a Resolver with the given configuration.
func NewResolver(dnsType string, servers []string, timeout time.Duration, debugMode bool, blocked []netip.Prefix, replaces []ReplacementRule) *Resolver {
	var customServer string
	preferGo := false
	if dnsType == "custom" && len(servers) > 0 {
		preferGo = true
		customServer = servers[0]
		if _, _, err := net.SplitHostPort(customServer); err != nil {
			customServer = net.JoinHostPort(customServer, "53")
		}
	}

	return &Resolver{
		preferGo:     preferGo,
		customServer: customServer,
		timeout:      timeout,
		debugMode:    debugMode,
		cache:        lru.NewTTLCache[string, string](4096),
		blocked:      blocked,
		replaces:     replaces,
	}
}

// Resolve resolves a hostname to an IP address.
func (r *Resolver) Resolve(ctx context.Context, host string) (netip.Addr, error) {
	if isLocalhost(host) {
		if !r.debugMode {
			return netip.Addr{}, ErrNoLocalhost
		}
		return r.parseLocalhost(host)
	}

	if cached, ok := r.cache.Get(host); ok {
		if addr, err := netip.ParseAddr(cached); err == nil {
			return addr, nil
		}
	}

	addrs, err := r.lookupHost(ctx, host)
	if err != nil {
		return netip.Addr{}, err
	}

	var chosen netip.Addr
	for _, a := range addrs {
		addr, err := netip.ParseAddr(a)
		if err != nil {
			continue
		}
		if addr.Is6() {
			chosen = addr
			break
		}
		if !chosen.IsValid() {
			chosen = addr
		}
	}

	if !chosen.IsValid() {
		return netip.Addr{}, ErrNoIPFound
	}

	if r.IsBlocked(chosen) {
		return netip.Addr{}, ErrBlocked
	}

	// Apply replacements
	for _, rule := range r.replaces {
		if rule.Prefix.Contains(chosen) {
			if r.IsBlocked(rule.IP) {
				return netip.Addr{}, fmt.Errorf("replacement IP %s is blocked", rule.IP)
			}
			chosen = rule.IP
			log.Info().Str("ip", chosen.String()).Str("hostname", host).Msg("DNS override found")
			break
		}
	}

	r.cache.Set(host, chosen.String(), dnsCacheTTL)
	return chosen, nil
}

// lookupHost performs a DNS lookup using the configured resolver.
func (r *Resolver) lookupHost(ctx context.Context, host string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	if r.preferGo {
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: r.timeout}
				return d.DialContext(ctx, "udp", r.customServer)
			},
		}
		return resolver.LookupHost(ctx, host)
	}
	return net.DefaultResolver.LookupHost(ctx, host)
}

// IsBlocked checks if an IP is in any blocked prefix.
func (r *Resolver) IsBlocked(ip netip.Addr) bool {
	for _, p := range r.blocked {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

func isLocalhost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "[::1]"
}

func (r *Resolver) parseLocalhost(host string) (netip.Addr, error) {
	switch host {
	case "127.0.0.1":
		return netip.MustParseAddr("127.0.0.1"), nil
	case "::1", "[::1]":
		return netip.MustParseAddr("::1"), nil
	default:
		return netip.MustParseAddr("127.0.0.1"), nil
	}
}
