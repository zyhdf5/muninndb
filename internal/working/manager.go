package working

import (
	"context"
	"sync"
	"time"
)

// Manager manages all active working memory sessions.
type Manager struct {
	sessions sync.Map // sessionID → *WorkingMemory
	HalfLife time.Duration
	MaxItems int
}

// NewManager creates a new Manager with default settings.
// Default half-life: 5 minutes, default max items: 50
func NewManager() *Manager {
	return &Manager{
		HalfLife: 5 * time.Minute,
		MaxItems: 50,
	}
}

// Create creates a new session and returns the working memory.
// If a session with the same ID already exists, it returns the existing one.
func (m *Manager) Create(sessionID string) *WorkingMemory {
	wm := &WorkingMemory{
		SessionID:  sessionID,
		Items:      make([]*WorkingItem, 0, m.MaxItems),
		MaxItems:   m.MaxItems,
		halfLife:   m.HalfLife,
		CreatedAt:  time.Now(),
		LastAccess: time.Now(),
	}

	// Load or store: if already exists, use the existing one
	actual, _ := m.sessions.LoadOrStore(sessionID, wm)
	return actual.(*WorkingMemory)
}

// Get retrieves an existing session.
// Returns nil if the session does not exist.
func (m *Manager) Get(sessionID string) (*WorkingMemory, bool) {
	val, ok := m.sessions.Load(sessionID)
	if !ok {
		return nil, false
	}
	return val.(*WorkingMemory), true
}

// Close closes a session and returns items eligible for promotion (attention > 0.6).
// Removes the session from the manager.
func (m *Manager) Close(sessionID string) ([]*WorkingItem, bool) {
	val, ok := m.sessions.LoadAndDelete(sessionID)
	if !ok {
		return nil, false
	}

	wm := val.(*WorkingMemory)
	// Return items with attention > 0.6 for promotion
	candidates := wm.Snapshot(m.HalfLife, 0.6)
	return candidates, true
}

// Delete removes a session without computing promotion candidates.
func (m *Manager) Delete(sessionID string) {
	m.sessions.Delete(sessionID)
}

// GC removes sessions idle for more than maxIdle duration.
// Returns the count of sessions removed.
func (m *Manager) GC(maxIdle time.Duration) int {
	count := 0
	m.sessions.Range(func(key, value interface{}) bool {
		sessionID := key.(string)
		wm := value.(*WorkingMemory)

		wm.mu.RLock()
		idle := time.Since(wm.LastAccess)
		wm.mu.RUnlock()

		if idle > maxIdle {
			m.sessions.Delete(sessionID)
			count++
		}
		return true
	})
	return count
}

// StartGC starts a background GC goroutine that runs every gcInterval.
// The goroutine removes sessions idle for more than maxIdle.
// The goroutine stops when ctx is cancelled.
func (m *Manager) StartGC(ctx context.Context, gcInterval, maxIdle time.Duration) {
	go func() {
		ticker := time.NewTicker(gcInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.GC(maxIdle)
			case <-ctx.Done():
				return
			}
		}
	}()
}
