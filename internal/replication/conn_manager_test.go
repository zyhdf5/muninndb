package replication

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/scrypster/muninndb/internal/transport/mbp"
)

// startEchoServer starts a TCP listener that accepts one connection, reads one
// MBP frame and echoes it back, then closes. It returns the listener address.
func startEchoServer(t *testing.T) (addr string, done <-chan struct{}) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ch := make(chan struct{})
	go func() {
		defer close(ch)
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		f, err := mbp.ReadFrame(conn)
		if err != nil {
			return
		}
		_ = mbp.WriteFrame(conn, f)
	}()
	return ln.Addr().String(), ch
}

// startSinkServer accepts connections and discards all incoming frames.
func startSinkServer(t *testing.T, wg *sync.WaitGroup, count *int, mu *sync.Mutex) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				for {
					f, err := mbp.ReadFrame(c)
					if err != nil {
						return
					}
					_ = f
					mu.Lock()
					*count++
					mu.Unlock()
					wg.Done()
				}
			}(conn)
		}
	}()
	return ln.Addr().String()
}

// TestConnManager_AddRemovePeer verifies add, get, and remove operations.
func TestConnManager_AddRemovePeer(t *testing.T) {
	m := NewConnManager("local-node")

	m.AddPeer("node-a", "127.0.0.1:9001")
	p, ok := m.GetPeer("node-a")
	if !ok {
		t.Fatal("expected GetPeer to return peer after AddPeer")
	}
	if p.NodeID() != "node-a" {
		t.Errorf("node ID: got %q, want %q", p.NodeID(), "node-a")
	}

	m.RemovePeer("node-a")
	_, ok = m.GetPeer("node-a")
	if ok {
		t.Fatal("expected GetPeer to return false after RemovePeer")
	}

	// Remove non-existent peer should not panic.
	m.RemovePeer("no-such-node")
}

// TestConnManager_Broadcast verifies that Broadcast delivers a frame to all
// connected peers and returns an empty error map on success.
func TestConnManager_Broadcast(t *testing.T) {
	m := NewConnManager("local-node")

	const peerCount = 3
	var mu sync.Mutex
	var wg sync.WaitGroup
	received := 0

	// Start peerCount sink servers and add connected peers.
	for i := 0; i < peerCount; i++ {
		wg.Add(1)
		addr := startSinkServer(t, &wg, &received, &mu)
		nodeID := "node-" + string(rune('a'+i))
		m.AddPeer(nodeID, addr)

		p, _ := m.GetPeer(nodeID)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := p.Connect(ctx); err != nil {
			t.Fatalf("Connect %s: %v", nodeID, err)
		}
	}

	errs := m.Broadcast(mbp.TypePing, []byte("broadcast-test"))
	if len(errs) != 0 {
		t.Errorf("Broadcast returned errors: %v", errs)
	}

	// Wait for all peers to receive the frame.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for all peers to receive broadcast frame")
	}

	mu.Lock()
	got := received
	mu.Unlock()
	if got != peerCount {
		t.Errorf("received count: got %d, want %d", got, peerCount)
	}
}

// TestConnManager_Broadcast_PartialFailure verifies that when one peer is
// disconnected, Broadcast still delivers to the others and returns an error
// only for the disconnected peer.
func TestConnManager_Broadcast_PartialFailure(t *testing.T) {
	m := NewConnManager("local-node")

	var mu sync.Mutex
	var wg sync.WaitGroup
	received := 0

	// Two healthy peers.
	for i := 0; i < 2; i++ {
		wg.Add(1)
		addr := startSinkServer(t, &wg, &received, &mu)
		nodeID := "healthy-" + string(rune('a'+i))
		m.AddPeer(nodeID, addr)
		p, _ := m.GetPeer(nodeID)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := p.Connect(ctx); err != nil {
			t.Fatalf("Connect %s: %v", nodeID, err)
		}
	}

	// One disconnected peer: use net.Pipe to get a valid conn, then close it
	// so that the connection is in a broken state.
	serverConn, clientConn := net.Pipe()
	serverConn.Close() // close server side immediately
	brokenPeer := &PeerConn{
		nodeID: "broken-peer",
		addr:   "pipe",
		conn:   clientConn, // client side is open but server side is gone
	}
	m.mu.Lock()
	m.peers["broken-peer"] = brokenPeer
	m.mu.Unlock()

	errs := m.Broadcast(mbp.TypePing, []byte("partial-broadcast"))

	// Wait for healthy peers to receive the frame.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for healthy peers to receive frame")
	}

	if _, hasErr := errs["broken-peer"]; !hasErr {
		t.Error("expected error for broken-peer, got none")
	}
	if len(errs) != 1 {
		t.Errorf("expected exactly 1 error entry, got %d: %v", len(errs), errs)
	}
}

// TestConnManager_RegisterHandler_Dispatch verifies that a registered handler
// is called with the correct arguments when Dispatch is invoked.
func TestConnManager_RegisterHandler_Dispatch(t *testing.T) {
	m := NewConnManager("local-node")

	type call struct {
		fromNodeID string
		payload    []byte
	}
	var calls []call
	var mu sync.Mutex

	m.RegisterHandler(mbp.TypeCogForward, func(fromNodeID string, payload []byte) error {
		mu.Lock()
		calls = append(calls, call{fromNodeID: fromNodeID, payload: payload})
		mu.Unlock()
		return nil
	})

	wantFrom := "node-sender"
	wantPayload := []byte("cog-side-effect-data")
	if err := m.Dispatch(wantFrom, mbp.TypeCogForward, wantPayload); err != nil {
		t.Fatalf("Dispatch returned unexpected error: %v", err)
	}

	mu.Lock()
	n := len(calls)
	mu.Unlock()
	if n != 1 {
		t.Fatalf("expected 1 handler call, got %d", n)
	}
	if calls[0].fromNodeID != wantFrom {
		t.Errorf("fromNodeID: got %q, want %q", calls[0].fromNodeID, wantFrom)
	}
	if string(calls[0].payload) != string(wantPayload) {
		t.Errorf("payload: got %q, want %q", calls[0].payload, wantPayload)
	}
}

// TestConnManager_Dispatch_NoHandler verifies that Dispatch returns nil when
// no handler is registered for the given frame type.
func TestConnManager_Dispatch_NoHandler(t *testing.T) {
	m := NewConnManager("local-node")

	if err := m.Dispatch("some-node", mbp.TypeCogForward, []byte("data")); err != nil {
		t.Fatalf("Dispatch with no handler returned unexpected error: %v", err)
	}
}

// TestConnManager_Close_All verifies that Close() shuts down all managed peers.
func TestConnManager_Close_All(t *testing.T) {
	m := NewConnManager("local-node")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	for i := 0; i < 3; i++ {
		nodeID := "close-node-" + string(rune('a'+i))
		m.AddPeer(nodeID, ln.Addr().String())
		p, _ := m.GetPeer(nodeID)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := p.Connect(ctx); err != nil {
			t.Fatalf("Connect %s: %v", nodeID, err)
		}
	}

	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// After Close, the peers map should be empty.
	peers := m.Peers()
	if len(peers) != 0 {
		t.Errorf("expected 0 peers after Close, got %d", len(peers))
	}

	// Calling Close again should be safe.
	if err := m.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
