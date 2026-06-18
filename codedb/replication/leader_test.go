package replication

import (
	"context"
	"testing"
	"time"
)

func TestLeaderElectorTick(t *testing.T) {
	b := NewMemoryLeaseBackend()
	e := NewLeaderElector("n1", b)
	e.RenewEvery = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = e.Run(ctx) }()
	time.Sleep(30 * time.Millisecond)
	if !e.IsLeader() {
		t.Fatal("not leader")
	}
	cancel()
}
