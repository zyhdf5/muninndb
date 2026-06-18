package replication

import (
	"context"
	"net"
	"testing"
)

func TestPeerConnLifecycle(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, _ := ln.Accept()
		if c != nil {
			defer c.Close()
		}
	}()
	p := NewPeerConn("n", ln.Addr().String())
	if err := p.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
}
