package replication

import (
	"context"
	"time"
)

// LeaseBackend is an abstraction for distributed lease management.
// Implementations can use etcd, Consul, or other consensus systems.
// In production, replace MemoryLeaseBackend with an etcd or Consul backend.
type LeaseBackend interface {
	// TryAcquire attempts to acquire a lease for nodeID with the given TTL.
	// Returns (true, nil) if the lease was acquired; (false, nil) if already held by another node.
	TryAcquire(ctx context.Context, nodeID string, ttl time.Duration) (bool, error)

	// Renew renews an existing lease for nodeID.
	Renew(ctx context.Context, nodeID string, ttl time.Duration) error

	// Release releases the lease held by nodeID.
	Release(ctx context.Context, nodeID string) error

	// CurrentHolder returns the nodeID that currently holds the lease, or empty string if no holder.
	CurrentHolder(ctx context.Context) (string, error)

	// Token returns a monotonically increasing fencing token.
	// Every time the lease changes hands, the token increments.
	// Used to fence off demoted primaries (split-brain prevention).
	Token(ctx context.Context) (uint64, error)
}
