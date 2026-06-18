package replication

import (
	"context"
	"net"
	"time"
)

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
