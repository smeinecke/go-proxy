package stats

import "sync/atomic"

// Stats holds atomic counters for proxy operations.
type Stats struct {
	RequestsTotal     atomic.Uint64
	ConnectTotal      atomic.Uint64
	HTTPRequestsTotal atomic.Uint64
	AuthFailuresTotal atomic.Uint64
	DNSFailuresTotal  atomic.Uint64
	BlockedTotal      atomic.Uint64
	DialFailuresTotal atomic.Uint64
	BytesUp           atomic.Uint64
	BytesDown         atomic.Uint64
}

// Snapshot returns a non-atomic copy of the current stats.
func (s *Stats) Snapshot() map[string]uint64 {
	return map[string]uint64{
		"requests_total":      s.RequestsTotal.Load(),
		"connect_total":       s.ConnectTotal.Load(),
		"http_requests_total": s.HTTPRequestsTotal.Load(),
		"auth_failures_total": s.AuthFailuresTotal.Load(),
		"dns_failures_total":  s.DNSFailuresTotal.Load(),
		"blocked_total":       s.BlockedTotal.Load(),
		"dial_failures_total": s.DialFailuresTotal.Load(),
		"bytes_up":            s.BytesUp.Load(),
		"bytes_down":          s.BytesDown.Load(),
	}
}
