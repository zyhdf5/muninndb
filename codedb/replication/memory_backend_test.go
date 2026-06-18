package replication

import (
	"context"
	"testing"
	"time"
)

func TestMemoryLeaseBackend(t *testing.T) {
	b := NewMemoryLeaseBackend()
	if ok, _ := b.TryAcquire(context.Background(), "a", time.Second); !ok {
		t.Fatal("acquire")
	}
	if err := b.Renew(context.Background(), "a", time.Second); err != nil {
		t.Fatal(err)
	}
	if err := b.Release(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
}
