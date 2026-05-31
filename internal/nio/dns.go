package nio

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/phuslu/lru"
	"github.com/rs/zerolog/log"
	"github.com/vlourme/go-proxy/internal/config"
)

var (
	ErrNoLocalhost = errors.New("localhost is not allowed in non-debug mode")
	ErrNoIPFound   = errors.New("no IP address found")
	ErrBlocked     = errors.New("destination IP is blocked")
)

const CACHE_TTL = 3 * time.Minute

var dnsCache = lru.NewTTLCache[string, string](4096)

func newResolver() *net.Resolver {
	cfg := config.Get()
	if cfg.DNS.Type == "custom" {
		return &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: config.DNSTimeout(),
				}
				// Pick first custom DNS server
				server := cfg.DNS.Servers[0]
				if _, _, err := net.SplitHostPort(server); err != nil {
					server = net.JoinHostPort(server, "53")
				}
				return d.DialContext(ctx, "udp", server)
			},
		}
	}
	return net.DefaultResolver
}

// ResolveHostname resolves the hostname to an IP address
// based on the network type.
func ResolveHostname(hostname string) (string, error) {
	cfg := config.Get()
	if isLocalhost(hostname) {
		if !cfg.DebugMode {
			return "", ErrNoLocalhost
		}
		return hostname, nil
	}

	ip, ok := dnsCache.Get(hostname)
	if ok {
		return ip, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.DNSTimeout())
	defer cancel()

	resolver := newResolver()
	addrs, err := resolver.LookupHost(ctx, hostname)
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if IsIPv6(addr) { // IPv6
			ip = addr
			break
		} else { // IPv4
			ip = addr
		}
	}

	if ip == "" {
		return "", ErrNoIPFound
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return "", errors.New("failed to parse resolved IP")
	}

	if IsBlocked(parsedIP) {
		return "", ErrBlocked
	}

	if len(config.GetReplaceIPs()) > 0 {
		for cidr, replacement := range config.GetReplaceIPs() {
			if cidr.Contains(parsedIP) {
				replacementIP := net.ParseIP(replacement)
				if replacementIP == nil {
					return "", errors.New("replace_ips contains invalid replacement IP")
				}
				if IsBlocked(replacementIP) {
					return "", fmt.Errorf("replacement IP %s is blocked", replacement)
				}
				ip = replacement
				log.Info().Str("ip", ip).Str("hostname", hostname).Msg("DNS override found")
				break
			}
		}
	}

	if IsIPv6(ip) {
		ip = "[" + ip + "]"
	}

	dnsCache.Set(hostname, ip, CACHE_TTL)
	return ip, nil
}

func isLocalhost(hostname string) bool {
	return hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" || hostname == "[::1]"
}
