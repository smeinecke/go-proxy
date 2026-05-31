package routing

import (
	"net/netip"
	"testing"
	"time"
)

func BenchmarkMakeSessionKey(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = MakeSessionKey("user", "uk", "yes", "abc123")
	}
}

func BenchmarkSessionStoreGet(b *testing.B) {
	store := NewSessionStore(1024)
	ip := netip.MustParseAddr("2001:db8::1")
	store.Set(MakeSessionKey("user", "uk", "yes", "abc123"), ip, time.Minute)

	key := MakeSessionKey("user", "uk", "yes", "abc123")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Get(key)
	}
}

func BenchmarkSessionStoreSet(b *testing.B) {
	store := NewSessionStore(1024)
	ip := netip.MustParseAddr("2001:db8::1")
	key := MakeSessionKey("user", "uk", "yes", "abc123")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Set(key, ip, time.Minute)
	}
}

func BenchmarkSessionStoreSetAndGet(b *testing.B) {
	store := NewSessionStore(1024)
	ip := netip.MustParseAddr("2001:db8::1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := MakeSessionKey("user", "uk", "yes", "abc123")
		store.Set(key, ip, time.Minute)
		_, _ = store.Get(key)
	}
}

func BenchmarkSessionStoreMiss(b *testing.B) {
	store := NewSessionStore(1024)
	key := MakeSessionKey("user", "uk", "yes", "notfound")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Get(key)
	}
}
