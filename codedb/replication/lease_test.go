package replication

import (
	"context"
	"testing"
	"time"
)

func TestLeaseBackendContract(t *testing.T) {
	b := NewMemoryLeaseBackend()
	ok, err := b.TryAcquire(context.Background(), "n1", time.Second)
	if err != nil || !ok {
		t.Fatal("acquire")
	}
	h, _ := b.CurrentHolder(context.Background())
	if h != "n1" {
		t.Fatal("holder")
	}
}
