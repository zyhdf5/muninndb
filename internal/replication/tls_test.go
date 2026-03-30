package replication

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/scrypster/muninndb/internal/config"
	"github.com/scrypster/muninndb/internal/transport/mbp"
)

func newTestTLSConfig(dir string) config.TLSConfig {
	return config.TLSConfig{
		Enabled:    true,
		AutoGenDir: dir,
	}
}

// TestClusterTLS_Bootstrap verifies that Bootstrap generates CA + node cert in
// a temp dir, verifies files exist, cert is signed by CA, and CN == nodeID.
func TestClusterTLS_Bootstrap(t *testing.T) {
	dir := t.TempDir()
	cfg := newTestTLSConfig(dir)
	ct := NewClusterTLS(cfg)

	if err := ct.Bootstrap("node-alpha", dir); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	// Verify files exist.
	for _, name := range []string{"ca.crt", "ca.key", "node.crt", "node.key"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}

	// Verify node cert is signed by the CA.
	caCert := ct.CACert()
	if caCert == nil {
		t.Fatal("expected non-nil CA cert")
	}

	nodeCert := ct.NodeCert()
	if nodeCert == nil {
		t.Fatal("expected non-nil node cert")
	}

	// Parse the leaf cert for inspection.
	leaf, err := x509.ParseCertificate(nodeCert.Certificate[0])
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}

	// Verify CN.
	if leaf.Subject.CommonName != "node-alpha" {
		t.Errorf("expected CN=node-alpha, got %q", leaf.Subject.CommonName)
	}

	// Verify signature chain.
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
		t.Errorf("cert not signed by CA: %v", err)
	}
}

// TestClusterTLS_ServerClientHandshake bootstraps two ClusterTLS instances and
// verifies that a TLS handshake succeeds between them.
func TestClusterTLS_ServerClientHandshake(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()

	ctA := NewClusterTLS(newTestTLSConfig(dirA))
	ctB := NewClusterTLS(newTestTLSConfig(dirB))

	if err := ctA.Bootstrap("node-a", dirA); err != nil {
		t.Fatalf("Bootstrap A: %v", err)
	}
	if err := ctB.Bootstrap("node-b", dirB); err != nil {
		t.Fatalf("Bootstrap B: %v", err)
	}

	// For mutual TLS to work between different CAs, each side must trust the
	// other's CA. In production, nodes sharing the same cluster secret act as
	// the trust anchor. For this test, we add each CA to the other's pool.
	ctA.caPool.AddCert(ctB.CACert())
	ctB.caPool.AddCert(ctA.CACert())

	serverCfg, err := ctA.ServerTLSConfig()
	if err != nil {
		t.Fatalf("ServerTLSConfig: %v", err)
	}
	// Server must also trust B's CA for client auth.
	serverCfg.ClientCAs = ctA.caPool

	clientCfg, err := ctB.ClientTLSConfig()
	if err != nil {
		t.Fatalf("ClientTLSConfig: %v", err)
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverCfg)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	var serverErr error
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
		// Force handshake completion on server side.
		if tlsConn, ok := conn.(*tls.Conn); ok {
			serverErr = tlsConn.Handshake()
		}
	}()

	conn, err := tls.Dial("tcp", ln.Addr().String(), clientCfg)
	if err != nil {
		t.Fatalf("TLS dial: %v", err)
	}
	conn.Close()

	wg.Wait()
	if serverErr != nil {
		t.Fatalf("server handshake error: %v", serverErr)
	}
}

// TestClusterTLS_InvalidCert_Rejected verifies that a node with a different CA
// cannot complete a TLS handshake.
func TestClusterTLS_InvalidCert_Rejected(t *testing.T) {
	dirA := t.TempDir()
	dirC := t.TempDir()

	ctA := NewClusterTLS(newTestTLSConfig(dirA))
	ctC := NewClusterTLS(newTestTLSConfig(dirC))

	if err := ctA.Bootstrap("node-a", dirA); err != nil {
		t.Fatalf("Bootstrap A: %v", err)
	}
	if err := ctC.Bootstrap("node-c", dirC); err != nil {
		t.Fatalf("Bootstrap C: %v", err)
	}

	// Do NOT add C's CA to A's pool -- they are from different clusters.
	serverCfg, err := ctA.ServerTLSConfig()
	if err != nil {
		t.Fatalf("ServerTLSConfig: %v", err)
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverCfg)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Force handshake -- expected to fail.
		if tlsConn, ok := conn.(*tls.Conn); ok {
			_ = tlsConn.Handshake()
		}
	}()

	// Client uses C's certs but does NOT trust A's CA for server verification,
	// and A does NOT trust C's CA for client verification.
	clientCfg, err := ctC.ClientTLSConfig()
	if err != nil {
		t.Fatalf("ClientTLSConfig: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	dialer := tls.Dialer{Config: clientCfg}
	conn, err := dialer.DialContext(ctx, "tcp", ln.Addr().String())
	if err == nil {
		conn.Close()
		t.Fatal("expected TLS handshake to fail with different CA, but it succeeded")
	}

	wg.Wait()
}

// TestClusterTLS_RotateCert verifies that RotateCert generates a new cert that
// is different from the old one but still signed by the same CA.
func TestClusterTLS_RotateCert(t *testing.T) {
	dir := t.TempDir()
	cfg := newTestTLSConfig(dir)
	ct := NewClusterTLS(cfg)

	if err := ct.Bootstrap("node-rotate", dir); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	oldCert := ct.NodeCert()
	oldLeaf, err := x509.ParseCertificate(oldCert.Certificate[0])
	if err != nil {
		t.Fatalf("parse old leaf: %v", err)
	}
	oldSerial := oldLeaf.SerialNumber

	if err := ct.RotateCert("node-rotate"); err != nil {
		t.Fatalf("RotateCert: %v", err)
	}

	newCert := ct.NodeCert()
	newLeaf, err := x509.ParseCertificate(newCert.Certificate[0])
	if err != nil {
		t.Fatalf("parse new leaf: %v", err)
	}

	// Serial numbers must differ.
	if oldSerial.Cmp(newLeaf.SerialNumber) == 0 {
		t.Error("expected different serial after rotation")
	}

	// New cert must still be signed by the same CA.
	pool := x509.NewCertPool()
	pool.AddCert(ct.CACert())
	if _, err := newLeaf.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
		t.Errorf("rotated cert not signed by same CA: %v", err)
	}

	// CN must still be correct.
	if newLeaf.Subject.CommonName != "node-rotate" {
		t.Errorf("expected CN=node-rotate, got %q", newLeaf.Subject.CommonName)
	}
}

// TestClusterTLS_ConnManager_TLS verifies that two nodes with TLS can exchange
// an MBP frame through a TLS-wrapped TCP connection.
func TestClusterTLS_ConnManager_TLS(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()

	ctA := NewClusterTLS(newTestTLSConfig(dirA))
	ctB := NewClusterTLS(newTestTLSConfig(dirB))

	if err := ctA.Bootstrap("node-a", dirA); err != nil {
		t.Fatalf("Bootstrap A: %v", err)
	}
	if err := ctB.Bootstrap("node-b", dirB); err != nil {
		t.Fatalf("Bootstrap B: %v", err)
	}

	// Cross-trust CAs (simulates shared cluster secret trust).
	ctA.caPool.AddCert(ctB.CACert())
	ctB.caPool.AddCert(ctA.CACert())

	// Set up TLS listener for node-a (server).
	serverCfg, err := ctA.ServerTLSConfig()
	if err != nil {
		t.Fatalf("ServerTLSConfig: %v", err)
	}

	tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("tcp listen: %v", err)
	}
	defer tcpLn.Close()
	tlsLn := tls.NewListener(tcpLn, serverCfg)

	wantType := mbp.TypePing
	wantPayload := []byte("tls-test-payload")

	var serverFrame *mbp.Frame
	var serverErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := tlsLn.Accept()
		if err != nil {
			serverErr = err
			return
		}
		defer conn.Close()
		serverFrame, serverErr = mbp.ReadFrame(conn)
	}()

	// Client side: node-b dials node-a with TLS.
	clientCfg, err := ctB.ClientTLSConfig()
	if err != nil {
		t.Fatalf("ClientTLSConfig: %v", err)
	}

	conn, err := tls.Dial("tcp", tcpLn.Addr().String(), clientCfg)
	if err != nil {
		t.Fatalf("TLS dial: %v", err)
	}
	defer conn.Close()

	// Write an MBP frame over the TLS connection.
	f := &mbp.Frame{
		Version: 0x01,
		Type:    uint8(wantType),
		Payload: wantPayload,
	}
	if err := mbp.WriteFrame(conn, f); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}

	wg.Wait()

	if serverErr != nil {
		t.Fatalf("server error: %v", serverErr)
	}
	if serverFrame.Type != uint8(wantType) {
		t.Errorf("frame type: got %d, want %d", serverFrame.Type, wantType)
	}
	if string(serverFrame.Payload) != string(wantPayload) {
		t.Errorf("payload: got %q, want %q", serverFrame.Payload, wantPayload)
	}
}
