package routing

import (
	"net/netip"
	"time"

	"github.com/phuslu/lru"
)

// SessionKey uniquely identifies a session.
type SessionKey string

// MakeSessionKey builds a canonical session key from components.
func MakeSessionKey(username, location, fallback, session string) SessionKey {
	return SessionKey(username + ":" + location + ":" + fallback + ":" + session)
}

// SessionStore caches source IP addresses by session key.
type SessionStore interface {
	Get(key SessionKey) (netip.Addr, bool)
	Set(key SessionKey, ip netip.Addr, ttl time.Duration)
	Delete(key SessionKey)
}

// DefaultSessionStore is an LRU-backed TTL session cache.
type DefaultSessionStore struct {
	cache *lru.TTLCache[string, string]
}

// NewSessionStore creates a new LRU session cache with the given capacity.
func NewSessionStore(capacity int) *DefaultSessionStore {
	return &DefaultSessionStore{
		cache: lru.NewTTLCache[string, string](capacity),
	}
}

// Get retrieves a cached IP address for the given session key.
func (s *DefaultSessionStore) Get(key SessionKey) (netip.Addr, bool) {
	val, ok := s.cache.Get(string(key))
	if !ok {
		return netip.Addr{}, false
	}
	addr, err := netip.ParseAddr(val)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr, true
}

// Set stores an IP address for the given session key with a TTL.
func (s *DefaultSessionStore) Set(key SessionKey, ip netip.Addr, ttl time.Duration) {
	s.cache.Set(string(key), ip.String(), ttl)
}

// Delete removes a session from the cache.
func (s *DefaultSessionStore) Delete(key SessionKey) {
	s.cache.Delete(string(key))
}
