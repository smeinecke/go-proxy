package routing

import (
	"io"
	"net/netip"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/vlourme/go-proxy/internal/utils"
)

func init() {
	log.Logger = zerolog.New(io.Discard)
}

func newTestRouter() *Router {
	bindPrefixes := []netip.Prefix{
		netip.MustParsePrefix("2001:db8::/32"),
		netip.MustParsePrefix("2002:1234::/48"),
	}
	fallbackPrefixes := []netip.Prefix{
		netip.MustParsePrefix("192.168.0.0/16"),
	}
	located := map[string][]netip.Prefix{
		"uk": {netip.MustParsePrefix("2003:abcd::/48")},
	}
	return NewRouter(
		NewSessionStore(1024),
		bindPrefixes,
		fallbackPrefixes,
		located,
		30*time.Minute,
		true,
	)
}

func BenchmarkRouterRouteNewIPv6(b *testing.B) {
	router := newTestRouter()
	req := RouteRequest{
		Username: "user",
		TargetIP: netip.MustParseAddr("2001:db8::1"),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = router.Route(req)
	}
}

func BenchmarkRouterRouteExistingSession(b *testing.B) {
	router := newTestRouter()
	req := RouteRequest{
		Username: "user",
		Session:  "abc123",
		TargetIP: netip.MustParseAddr("2001:db8::1"),
		Timeout:  time.Minute,
	}
	// Prime the session
	_, _ = router.Route(req)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = router.Route(req)
	}
}

func BenchmarkRouterRouteLocation(b *testing.B) {
	router := newTestRouter()
	req := RouteRequest{
		Username: "user",
		Location: "uk",
		TargetIP: netip.MustParseAddr("2001:db8::1"),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = router.Route(req)
	}
}

func BenchmarkRouterRouteFallback(b *testing.B) {
	router := newTestRouter()
	req := RouteRequest{
		Username: "user",
		TargetIP: netip.MustParseAddr("192.168.1.1"),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = router.Route(req)
	}
}

func BenchmarkUtilsGenerateNetIP(b *testing.B) {
	prefix := netip.MustParsePrefix("2001:db8::/32")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = utils.GenerateNetIP(prefix)
	}
}
