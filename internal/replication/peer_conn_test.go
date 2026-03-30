package replication

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/scrypster/muninndb/internal/transport/mbp"
)

// TestPeerConn_ConnectAndSendReceive verifies a full round-trip through a local
// TCP listener using the MBP frame format.
func TestPeerConn_ConnectAndSendReceive(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	wantType := mbp.TypePing
	wantPayload := []byte("hello-peer")

	var serverErr error
	var serverFrame *mbp.Frame
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			serverErr = err
			return
		}
		defer conn.Close()
		serverFrame, serverErr = mbp.ReadFrame(conn)
	}()

	pc := NewPeerConn("node-1", ln.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := pc.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if !pc.IsConnected() {
		t.Fatal("expected IsConnected() == true after Connect")
	}

	if err := pc.Send(uint8(wantType), wantPayload); err != nil {
		t.Fatalf("Send: %v", err)
	}

	wg.Wait()

	if serverErr != nil {
		t.Fatalf("server read error: %v", serverErr)
	}
	if serverFrame.Type != uint8(wantType) {
		t.Errorf("frame type: got %d, want %d", serverFrame.Type, wantType)
	}
	if string(serverFrame.Payload) != string(wantPayload) {
		t.Errorf("payload: got %q, want %q", serverFrame.Payload, wantPayload)
	}

	// Now test Receive from the other direction: server writes, client reads.
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen2: %v", err)
	}
	defer ln2.Close()

	wantType2 := mbp.TypePong
	wantPayload2 := []byte("pong-from-server")

	var wg2 sync.WaitGroup
	wg2.Add(1)
	go func() {
		defer wg2.Done()
		conn, err := ln2.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		f := &mbp.Frame{
			Version: 0x01,
			Type:    uint8(wantType2),
			Payload: wantPayload2,
		}
		_ = mbp.WriteFrame(conn, f)
	}()

	pc2 := NewPeerConn("node-2", ln2.Addr().String())
	ctx2, cancel2 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel2()
	if err := pc2.Connect(ctx2); err != nil {
		t.Fatalf("Connect2: %v", err)
	}

	wg2.Wait()

	ft, payload, err := pc2.Receive()
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if ft != uint8(wantType2) {
		t.Errorf("receive type: got %d, want %d", ft, wantType2)
	}
	if string(payload) != string(wantPayload2) {
		t.Errorf("receive payload: got %q, want %q", payload, wantPayload2)
	}
}

// TestPeerConn_Close_Idempotent verifies that calling Close twice does not panic
// or return an error on the second call.
func TestPeerConn_Close_Idempotent(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	pc := NewPeerConn("node-close", ln.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := pc.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if err := pc.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := pc.Close(); err != nil {
		t.Fatalf("second Close should be nil, got: %v", err)
	}
	if pc.IsConnected() {
		t.Error("expected IsConnected() == false after Close")
	}
}

// TestPeerConn_Send_NotConnected verifies that Send before Connect returns an error.
func TestPeerConn_Send_NotConnected(t *testing.T) {
	pc := NewPeerConn("node-no-conn", "127.0.0.1:9999")
	err := pc.Send(mbp.TypePing, []byte("data"))
	if err == nil {
		t.Fatal("expected error from Send on unconnected peer, got nil")
	}
	if err != ErrNotConnected {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
}

// TestPeerConn_ReceivePartialFrame simulates a server that writes the MBP frame
// header and payload in two separate small writes with a sleep in between,
// testing that ReadFrame handles partial reads correctly via io.ReadFull.
func TestPeerConn_ReceivePartialFrame(t *testing.T) {
	// Use net.Pipe for an in-process pipe with no buffering.
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	wantType := mbp.TypeReplEntry
	wantPayload := []byte("partial-write-test-payload")

	// Inject the server-side net.Conn into a PeerConn directly so we avoid a
	// real TCP dial.
	pc := &PeerConn{
		nodeID: "partial-node",
		addr:   "pipe",
		conn:   clientConn,
	}

	go func() {
		// Build the full 16-byte MBP prefix manually, then write it in two
		// pieces to exercise partial-read handling in io.ReadFull.
		f := &mbp.Frame{
			Version: 0x01,
			Type:    uint8(wantType),
			Payload: wantPayload,
		}
		// Serialize into a buffer via a pipe-backed writer using WriteFrame.
		pr, pw := io.Pipe()
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = mbp.WriteFrame(pw, f)
			pw.Close()
		}()

		// Read all bytes from the pipe writer, then drip them to serverConn.
		buf := make([]byte, 0, 512)
		tmp := make([]byte, 64)
		for {
			n, err := pr.Read(tmp)
			if n > 0 {
				buf = append(buf, tmp[:n]...)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
		}
		wg.Wait()

		// Write first byte, sleep, write the rest — simulating a slow writer.
		if len(buf) > 0 {
			_, _ = serverConn.Write(buf[:1])
			time.Sleep(10 * time.Millisecond)
			_, _ = serverConn.Write(buf[1:])
		}
		serverConn.Close()
	}()

	ft, payload, err := pc.Receive()
	if err != nil {
		t.Fatalf("Receive with partial write: %v", err)
	}
	if ft != uint8(wantType) {
		t.Errorf("frame type: got %d, want %d", ft, wantType)
	}
	if string(payload) != string(wantPayload) {
		t.Errorf("payload: got %q, want %q", payload, wantPayload)
	}
}
