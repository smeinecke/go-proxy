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
	BytesTotal        atomic.Uint64
}

// Container holds N independent Stats shards to avoid cache-line contention.
type Container struct {
	shards []Stats
}

// NewContainer creates a Container with the given number of shards.
func NewContainer(numShards int) *Container {
	return &Container{shards: make([]Stats, numShards)}
}

// Shard returns the stats shard for the given worker index.
// Returns nil when stats are disabled (c is nil or has no shards).
func (c *Container) Shard(idx int) *Stats {
	if c == nil || len(c.shards) == 0 {
		return nil
	}
	return &c.shards[idx%len(c.shards)]
}

// Snapshot returns the summed values of all shards.
func (c *Container) Snapshot() map[string]uint64 {
	var requests, connect, httpReqs, authFail, dnsFail, blocked, dialFail, bytesTotal uint64
	if c != nil {
		for i := range c.shards {
			s := &c.shards[i]
			requests += s.RequestsTotal.Load()
			connect += s.ConnectTotal.Load()
			httpReqs += s.HTTPRequestsTotal.Load()
			authFail += s.AuthFailuresTotal.Load()
			dnsFail += s.DNSFailuresTotal.Load()
			blocked += s.BlockedTotal.Load()
			dialFail += s.DialFailuresTotal.Load()
			bytesTotal += s.BytesTotal.Load()
		}
	}
	return map[string]uint64{
		"requests_total":      requests,
		"connect_total":       connect,
		"http_requests_total": httpReqs,
		"auth_failures_total": authFail,
		"dns_failures_total":  dnsFail,
		"blocked_total":       blocked,
		"dial_failures_total": dialFail,
		"bytes_total":         bytesTotal,
	}
}
