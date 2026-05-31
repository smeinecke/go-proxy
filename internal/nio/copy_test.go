package nio

import (
	"net"
	"testing"
	"time"
)

func TestCopyBidirectionalTimeout(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()

	var server net.Conn
	connected := make(chan struct{})
	go func() {
		server, _ = l.Accept()
		close(connected)
	}()

	client, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	<-connected

	start := time.Now()
	CopyBidirectional(server, client, 50*time.Millisecond)
	elapsed := time.Since(start)

	if elapsed < 50*time.Millisecond || elapsed > 500*time.Millisecond {
		t.Fatalf("expected timeout around 50ms, got %v", elapsed)
	}
}

func TestCopyBidirectionalActiveTrafficResetsTimeout(t *testing.T) {
	// Two independent listener/dial pairs so CopyBidirectional can
	// copy in both directions without creating an echo loop.
	l1, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l1.Close()

	l2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l2.Close()

	var s1, s2 net.Conn
	connected := make(chan struct{}, 2)
	go func() {
		s1, _ = l1.Accept()
		connected <- struct{}{}
	}()
	go func() {
		s2, _ = l2.Accept()
		connected <- struct{}{}
	}()

	c1, err := net.Dial("tcp", l1.Addr().String())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	c2, err := net.Dial("tcp", l2.Addr().String())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	<-connected
	<-connected

	// Drain both server ends so CopyBidirectional writes never block.
	go func() {
		buf := make([]byte, 1024)
		for {
			if _, err := s1.Read(buf); err != nil {
				return
			}
		}
	}()
	go func() {
		buf := make([]byte, 1024)
		for {
			if _, err := s2.Read(buf); err != nil {
				return
			}
		}
	}()

	copyDone := make(chan struct{})
	go func() {
		CopyBidirectional(c1, c2, 200*time.Millisecond)
		close(copyDone)
	}()

	// Keep both directions alive by feeding data from the server ends.
	// 6 rounds at 50ms = ~300ms of active traffic.
	start := time.Now()
	for i := 0; i < 6; i++ {
		time.Sleep(50 * time.Millisecond)
		s1.Write([]byte("x"))
		s2.Write([]byte("y"))
	}

	// After last send, idle timeout should fire within ~200ms.
	<-copyDone
	elapsed := time.Since(start)

	if elapsed < 300*time.Millisecond {
		t.Fatalf("active traffic should have kept connection alive for at least 300ms, got %v", elapsed)
	}
	if elapsed > 800*time.Millisecond {
		t.Fatalf("idle timeout should have closed after ~200ms of idle, got %v", elapsed)
	}
}

func TestCopyBidirectionalClosesConnections(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()

	var server net.Conn
	connected := make(chan struct{})
	go func() {
		server, _ = l.Accept()
		close(connected)
	}()

	client, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	<-connected

	CopyBidirectional(server, client, 100*time.Millisecond)

	if _, err := server.Write([]byte("x")); err == nil {
		t.Fatalf("expected server conn to be closed")
	}
	if _, err := client.Write([]byte("x")); err == nil {
		t.Fatalf("expected client conn to be closed")
	}
}
