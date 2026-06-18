package replication

import (
	"context"
	"net"
	"testing"
)

func TestNodeReachabilityFn(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if ok, err := TestNodeReachability(context.Background(), ln.Addr().String()); err != nil || !ok {
		t.Fatal("unreachable")
	}
}
