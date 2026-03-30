package working

import (
	"math"
	"sync"
	"time"

	"github.com/scrypster/muninndb/internal/types"
)

// WorkingMemory represents a session-scoped working memory with attention decay.
type WorkingMemory struct {
	SessionID  string
	Items      []*WorkingItem
	mu         sync.RWMutex
	MaxItems   int
	halfLife   time.Duration
	CreatedAt  time.Time
	LastAccess time.Time
}

// WorkingItem represents an engram in working memory with decaying attention.
type WorkingItem struct {
	EngramID  types.ULID
	Attention float32
	AddedAt   time.Time
	Context   string
}

// CurrentAttention computes decayed attention at the current time.
// Formula: attention₀ × 2^(-elapsed / halfLife)
func (item *WorkingItem) CurrentAttention(halfLife time.Duration) float32 {
	elapsed := time.Since(item.AddedAt)
	exponent := -float64(elapsed) / float64(halfLife)
	decayFactor := math.Pow(2, exponent)
	return float32(float64(item.Attention) * decayFactor)
}

// Add adds an engram to working memory, evicting the lowest-attention item if at capacity.
func (wm *WorkingMemory) Add(engramID types.ULID, initialAttention float32, context string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	// Clamp attention to [0.0, 1.0]
	if initialAttention < 0 {
		initialAttention = 0
	} else if initialAttention > 1 {
		initialAttention = 1
	}

	item := &WorkingItem{
		EngramID:  engramID,
		Attention: initialAttention,
		AddedAt:   time.Now(),
		Context:   context,
	}

	// If at capacity, evict lowest-attention item
	if len(wm.Items) >= wm.MaxItems {
		minIdx := wm.findLowestAttentionIndex(wm.halfLife)
		if minIdx >= 0 {
			wm.Items = append(wm.Items[:minIdx], wm.Items[minIdx+1:]...)
		}
	}

	wm.Items = append(wm.Items, item)
	wm.LastAccess = time.Now()
}

// findLowestAttentionIndex finds the index of the item with the lowest current (decayed) attention.
// Must be called with lock held.
// When halfLife is zero, falls back to comparing the initial stored Attention values.
func (wm *WorkingMemory) findLowestAttentionIndex(halfLife time.Duration) int {
	if len(wm.Items) == 0 {
		return -1
	}

	attOf := func(item *WorkingItem) float32 {
		if halfLife <= 0 {
			return item.Attention
		}
		return item.CurrentAttention(halfLife)
	}

	minIdx := 0
	minAttention := attOf(wm.Items[0])
	for i := 1; i < len(wm.Items); i++ {
		att := attOf(wm.Items[i])
		if att < minAttention {
			minAttention = att
			minIdx = i
		}
	}
	return minIdx
}

// Get returns all items with their current (decayed) attention values.
// Attention values are computed at call time.
func (wm *WorkingMemory) Get(halfLife time.Duration) []*WorkingItem {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	result := make([]*WorkingItem, len(wm.Items))
	for i, item := range wm.Items {
		// Create a copy with current attention
		copy := &WorkingItem{
			EngramID:  item.EngramID,
			Attention: item.CurrentAttention(halfLife),
			AddedAt:   item.AddedAt,
			Context:   item.Context,
		}
		result[i] = copy
	}
	return result
}

// Remove removes an engram from working memory by ID.
// Returns true if the item was found and removed.
func (wm *WorkingMemory) Remove(engramID types.ULID) bool {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	for i, item := range wm.Items {
		if item.EngramID == engramID {
			wm.Items = append(wm.Items[:i], wm.Items[i+1:]...)
			return true
		}
	}
	return false
}

// Snapshot returns items with current attention > minAttention threshold.
func (wm *WorkingMemory) Snapshot(halfLife time.Duration, minAttention float32) []*WorkingItem {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	var result []*WorkingItem
	for _, item := range wm.Items {
		current := item.CurrentAttention(halfLife)
		if current > minAttention {
			copy := &WorkingItem{
				EngramID:  item.EngramID,
				Attention: current,
				AddedAt:   item.AddedAt,
				Context:   item.Context,
			}
			result = append(result, copy)
		}
	}
	return result
}
