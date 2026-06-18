package replication

import (
	"context"
	"testing"
	"time"
)

func TestStreamer(t *testing.T) {
	l := NewReplicationLog(newMemKVStore())
	_, _ = l.Append(OpSet, []byte("k"), []byte("v"))
	s := NewStreamer(l)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = s.Stream(ctx, 0) }()
	select {
	case <-s.Entries():
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}
