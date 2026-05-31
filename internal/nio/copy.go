package nio

import (
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// CopyBidirectional copies data between two connections in both directions.
// It waits for both directions to finish, then closes both connections.
// When one direction hits EOF it performs a TCP half-close (CloseWrite) on
// the opposite side so the peer sees a clean EOF rather than a reset.
// The idleTimeout is a real idle timeout: it resets on every Read or Write,
// so active tunnels are not killed prematurely.
// It returns the total number of bytes transferred.
func CopyBidirectional(a, b net.Conn, idleTimeout time.Duration) int64 {
	var totalBytes int64
	var wg sync.WaitGroup
	wg.Add(2)

	var timer *time.Timer
	var mu sync.Mutex
	reset := func() {}

	if idleTimeout > 0 {
		timer = time.AfterFunc(idleTimeout, func() {
			a.Close()
			b.Close()
		})
		reset = func() {
			mu.Lock()
			if timer != nil {
				timer.Reset(idleTimeout)
			}
			mu.Unlock()
		}
	}

	copyAndClose := func(dst, src net.Conn) {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, err := src.Read(buf)
			if n > 0 {
				reset()
				nw, werr := dst.Write(buf[:n])
				if nw > 0 {
					reset()
					atomic.AddInt64(&totalBytes, int64(nw))
				}
				if werr != nil {
					return
				}
				if n != nw {
					return
				}
			}
			if err != nil {
				if tc, ok := dst.(interface{ CloseWrite() error }); ok {
					tc.CloseWrite()
				}
				return
			}
		}
	}

	go copyAndClose(a, b)
	go copyAndClose(b, a)

	wg.Wait()

	if timer != nil {
		mu.Lock()
		timer.Stop()
		timer = nil
		mu.Unlock()
	}

	a.Close()
	b.Close()

	return atomic.LoadInt64(&totalBytes)
}
