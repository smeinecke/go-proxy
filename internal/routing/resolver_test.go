package routing

import (
	"context"
	"net/netip"
	"testing"
	"time"
)

func newTestResolver() *Resolver {
	blocked := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("192.168.0.0/16"),
	}
	return NewResolver("", nil, 5*time.Second, false, blocked, nil)
}

func BenchmarkResolverCachedLookup(b *testing.B) {
	resolver := newTestResolver()
	// Prime the cache
	resolver.cache.Set("example.com", "2001:db8::1", dnsCacheTTL)

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = resolver.Resolve(ctx, "example.com")
	}
}

func BenchmarkResolverBlockedCheck(b *testing.B) {
	resolver := newTestResolver()
	ip := netip.MustParseAddr("10.0.0.1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = resolver.isBlocked(ip)
	}
}

func BenchmarkResolverBlockedCheckMiss(b *testing.B) {
	resolver := newTestResolver()
	ip := netip.MustParseAddr("1.2.3.4")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = resolver.isBlocked(ip)
	}
}

func BenchmarkResolverLocalhost(b *testing.B) {
	resolver := newTestResolver()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = resolver.Resolve(ctx, "localhost")
	}
}
