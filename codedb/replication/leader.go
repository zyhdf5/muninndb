package replication

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type LeaderElector struct {
	NodeID     string
	Backend    LeaseBackend
	LeaseTTL   time.Duration
	RenewEvery time.Duration

	OnPromote func()
	OnDemote  func()

	isLeader atomic.Bool
	token    atomic.Uint64

	mu     sync.Mutex
	runCtx context.Context
	cancel context.CancelFunc
}

func NewLeaderElector(nodeID string, backend LeaseBackend) *LeaderElector {
	return &LeaderElector{
		NodeID:     nodeID,
		Backend:    backend,
		LeaseTTL:   10 * time.Second,
		RenewEvery: 3 * time.Second,
		OnPromote:  func() {},
		OnDemote:   func() {},
	}
}

func (e *LeaderElector) Run(ctx context.Context) error {
	e.mu.Lock()
	if e.runCtx != nil {
		e.mu.Unlock()
		return ErrAlreadyRunning
	}
	runCtx, cancel := context.WithCancel(ctx)
	e.runCtx = runCtx
	e.cancel = cancel
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		e.runCtx = nil
		e.cancel = nil
		e.mu.Unlock()
		_ = e.Backend.Release(context.Background(), e.NodeID)
		e.isLeader.Store(false)
	}()

	ticker := time.NewTicker(e.RenewEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			e.tick(ctx)
		}
	}
}

func (e *LeaderElector) tick(ctx context.Context) {
	wasLeader := e.isLeader.Load()
	acquired, err := e.Backend.TryAcquire(ctx, e.NodeID, e.LeaseTTL)
	if err != nil {
		return
	}
	if token, err := e.Backend.Token(ctx); err == nil {
		e.token.Store(token)
	}
	if acquired && !wasLeader {
		e.isLeader.Store(true)
		e.OnPromote()
	} else if !acquired && wasLeader {
		e.isLeader.Store(false)
		e.OnDemote()
	}
}

func (e *LeaderElector) IsLeader() bool       { return e.isLeader.Load() }
func (e *LeaderElector) FencingToken() uint64 { return e.token.Load() }

type electorErr struct{ msg string }

func (e *electorErr) Error() string { return e.msg }

var ErrAlreadyRunning = &electorErr{msg: "leader elector already running"}
