package replication

import (
	"context"
	"sync"
	"time"
)

type MemoryLeaseBackend struct {
	mu      sync.Mutex
	holder  string
	expires time.Time
	token   uint64
	genLock map[string]uint64
}

func NewMemoryLeaseBackend() *MemoryLeaseBackend {
	return &MemoryLeaseBackend{
		holder:  "",
		expires: time.Time{},
		token:   0,
		genLock: make(map[string]uint64),
	}
}

func (b *MemoryLeaseBackend) TryAcquire(ctx context.Context, nodeID string, ttl time.Duration) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if b.expires.Before(now) || b.holder == "" {
		b.holder = nodeID
		b.expires = now.Add(ttl)
		b.token++
		b.genLock[nodeID] = b.token
		return true, nil
	}

	if b.holder == nodeID {
		b.expires = now.Add(ttl)
		return true, nil
	}

	return false, nil
}

func (b *MemoryLeaseBackend) Renew(ctx context.Context, nodeID string, ttl time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.holder != nodeID {
		return ErrNotLeaseHolder
	}
	b.expires = time.Now().Add(ttl)
	return nil
}

func (b *MemoryLeaseBackend) Release(ctx context.Context, nodeID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.holder == nodeID {
		b.holder = ""
		b.expires = time.Time{}
	}
	return nil
}

func (b *MemoryLeaseBackend) CurrentHolder(ctx context.Context) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.expires.Before(time.Now()) {
		return "", nil
	}
	return b.holder, nil
}

func (b *MemoryLeaseBackend) Token(ctx context.Context) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.token, nil
}

type leaseErr struct{ msg string }

func (e *leaseErr) Error() string { return e.msg }

var ErrNotLeaseHolder = &leaseErr{msg: "not lease holder"}
