package routing

import (
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/vlourme/go-proxy/internal/utils"
)

// RouteRequest holds all parameters needed to select a source IP and dialer.
type RouteRequest struct {
	Username string
	TargetIP netip.Addr
	Session  string
	Timeout  time.Duration
	Location string
	Fallback string
}

// RouteResult contains the outcome of a routing decision.
type RouteResult struct {
	SourceIP netip.Addr
	Mode     string // generated, session, fallback, explicit
	Dialer   *net.Dialer
}

// Router selects source IPs and creates dialers based on configuration.
type Router struct {
	sessions         SessionStore
	bindPrefixes     []netip.Prefix
	fallbackPrefixes []netip.Prefix
	locatedPrefixes  map[string][]netip.Prefix
	maxTimeout       time.Duration
	enableFallback   bool
}

// NewRouter creates a Router from validated prefix lists.
func NewRouter(
	sessions SessionStore,
	bindPrefixes, fallbackPrefixes []netip.Prefix,
	locatedPrefixes map[string][]netip.Prefix,
	maxTimeout time.Duration,
	enableFallback bool,
) *Router {
	return &Router{
		sessions:         sessions,
		bindPrefixes:     bindPrefixes,
		fallbackPrefixes: fallbackPrefixes,
		locatedPrefixes:  locatedPrefixes,
		maxTimeout:       maxTimeout,
		enableFallback:   enableFallback,
	}
}

// Route decides the source IP and builds a dialer for the given request.
func (r *Router) Route(req RouteRequest) (RouteResult, error) {
	prefix := r.selectPrefix(req.Location)

	cacheKey := SessionKey(req.Username + ":" + req.Location + ":" + req.Fallback + ":" + req.Session)

	var source netip.Addr
	var mode string

	if req.Session == "" {
		var err error
		source, err = utils.GenerateNetIP(prefix)
		if err != nil {
			return RouteResult{}, fmt.Errorf("generate IP: %w", err)
		}
		mode = "generated"
	} else {
		cached, ok := r.sessions.Get(cacheKey)
		if ok {
			source = cached
			mode = "session"
		} else {
			var err error
			source, err = utils.GenerateNetIP(prefix)
			if err != nil {
				return RouteResult{}, fmt.Errorf("generate IP: %w", err)
			}
			mode = "generated"

			ttl := req.Timeout
			if ttl <= 0 {
				ttl = 5 * time.Minute
			}
			if ttl > r.maxTimeout {
				ttl = r.maxTimeout
			}
			r.sessions.Set(cacheKey, source, ttl)
		}
	}

	// Fallback: if target family differs from source, use a fallback IPv4 prefix.
	if req.Fallback != "no" && r.enableFallback && req.TargetIP.Is4() != source.Is4() {
		if len(r.fallbackPrefixes) > 0 {
			fallbackPrefix := r.fallbackPrefixes[utils.RandomInt(len(r.fallbackPrefixes))]
			fallbackIP, err := utils.GenerateNetIP(fallbackPrefix)
			if err != nil {
				return RouteResult{}, fmt.Errorf("generate fallback IP: %w", err)
			}
			log.Warn().Str("prefix", fallbackPrefix.String()).Msg("IPv4 target, using fallback prefix")
			return RouteResult{
				SourceIP: fallbackIP,
				Mode:     "fallback",
				Dialer:   newDialer(fallbackIP.AsSlice()),
			}, nil
		}
	}

	return RouteResult{
		SourceIP: source,
		Mode:     mode,
		Dialer:   newDialer(source.AsSlice()),
	}, nil
}

// selectPrefix picks the appropriate CIDR prefix for a location.
func (r *Router) selectPrefix(location string) netip.Prefix {
	if location == "" {
		return r.bindPrefixes[utils.RandomInt(len(r.bindPrefixes))]
	}
	prefixes, ok := r.locatedPrefixes[location]
	if !ok || len(prefixes) == 0 {
		return r.bindPrefixes[utils.RandomInt(len(r.bindPrefixes))]
	}
	return prefixes[utils.RandomInt(len(prefixes))]
}

// newDialer creates a net.Dialer bound to the given local IP.
func newDialer(localIP net.IP) *net.Dialer {
	return &net.Dialer{
		LocalAddr:     &net.TCPAddr{IP: localIP},
		FallbackDelay: -1,
		Timeout:       5 * time.Second,
		KeepAlive:     -1,
	}
}
