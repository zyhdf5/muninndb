package replication

import (
	"context"
	"time"
)

type LeaseBackend interface {
	TryAcquire(ctx context.Context, nodeID string, ttl time.Duration) (bool, error)
	Renew(ctx context.Context, nodeID string, ttl time.Duration) error
	Release(ctx context.Context, nodeID string) error
	CurrentHolder(ctx context.Context) (string, error)
	Token(ctx context.Context) (uint64, error)
}
