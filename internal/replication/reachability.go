package replication

import (
	"context"
	"net"
	"time"
)

// TestNodeReachability attempts a TCP connection to addr with a 5-second timeout.
// Returns (true, nil) on success. Intentionally low-level — no MBP handshake —
// so it works even when the remote node is still starting up.
func TestNodeReachability(ctx context.Context, addr string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false, err
	}
	conn.Close()
	return true, nil
}
