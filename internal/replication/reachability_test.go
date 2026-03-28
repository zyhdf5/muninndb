package replication_test

import (
	"context"
	"net"
	"testing"

	"github.com/scrypster/muninndb/internal/replication"
)

func TestTestNodeReachability_Success(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		conn.Close()
	}()

	reachable, err := replication.TestNodeReachability(context.Background(), ln.Addr().String())
	if !reachable || err != nil {
		t.Fatalf("expected reachable=true, got reachable=%v err=%v", reachable, err)
	}
}

func TestTestNodeReachability_Unreachable(t *testing.T) {
	// Use a port that is almost certainly not listening
	reachable, _ := replication.TestNodeReachability(context.Background(), "127.0.0.1:19999")
	if reachable {
		t.Fatal("expected unreachable")
	}
}
