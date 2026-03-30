package cognitive

import (
	"sync"
	"time"
)

// ActivityTracker records the last time each vault received a write or activation.
// It is the shared signal that drives worker dormancy decisions.
type ActivityTracker struct {
	mu       sync.RWMutex
	lastSeen map[[8]byte]time.Time
}

func NewActivityTracker() *ActivityTracker {
	return &ActivityTracker{lastSeen: make(map[[8]byte]time.Time)}
}

// Record marks a vault as active right now.
func (t *ActivityTracker) Record(ws [8]byte) {
	t.mu.Lock()
	t.lastSeen[ws] = time.Now()
	t.mu.Unlock()
}

// IdleSince returns how long ago this vault was last active.
// Unknown vaults return 30 days (treated as very stale).
func (t *ActivityTracker) IdleSince(ws [8]byte) time.Duration {
	t.mu.RLock()
	last, ok := t.lastSeen[ws]
	t.mu.RUnlock()
	if !ok {
		return 30 * 24 * time.Hour
	}
	return time.Since(last)
}

// Evict removes the activity record for the given vault workspace prefix.
// After this call, IdleSince will return 30 days (treated as very stale) until
// a new write or activation is recorded for the vault.
func (t *ActivityTracker) Evict(ws [8]byte) {
	t.mu.Lock()
	delete(t.lastSeen, ws)
	t.mu.Unlock()
}

// ActiveVaults returns all vaults that have been active within the given window.
func (t *ActivityTracker) ActiveVaults(window time.Duration) [][8]byte {
	t.mu.RLock()
	defer t.mu.RUnlock()
	cutoff := time.Now().Add(-window)
	out := make([][8]byte, 0)
	for ws, last := range t.lastSeen {
		if last.After(cutoff) {
			out = append(out, ws)
		}
	}
	return out
}
