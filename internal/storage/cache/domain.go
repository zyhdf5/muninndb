package cache

import (
	"container/heap"
	"math"
	"sync"
	"time"

	"github.com/scrypster/muninndb/internal/storage"
)

// DomainCache is a relevance-weighted min-heap eviction policy cache.
// It replaces the LRU cache with domain-aware scoring based on engram metadata.
//
// Eviction score formula:
//
//	score = relevance × confidence × log₂(1 + access_count) × stability_weight
//	where stability_weight = 0.5 + 0.5 × stability
//
// Higher score = more important = keep longer.
// Evict the LOWEST score first.
type DomainCache struct {
	mu       sync.RWMutex
	items    map[storage.ULID]*cacheEntry
	heap     scoreHeap
	maxItems int
}

type cacheEntry struct {
	engram  *storage.Engram
	score   float64
	heapIdx int
}

// NewDomainCache creates a new domain-aware cache with specified max capacity.
func NewDomainCache(maxSize int) *DomainCache {
	if maxSize <= 0 {
		maxSize = 10000 // default
	}
	return &DomainCache{
		items:    make(map[storage.ULID]*cacheEntry),
		heap:     make(scoreHeap, 0),
		maxItems: maxSize,
	}
}

// computeScore calculates the eviction score for an engram.
// Higher score = more important = keep in cache.
//
// Formula: score = relevance × confidence × log₂(1 + access_count) × stability_weight
// where stability_weight = 0.5 + 0.5 × stability
func computeScore(eng *storage.Engram) float64 {
	// Clamp relevance and confidence to [0, 1]
	relevance := float64(eng.Relevance)
	if relevance < 0 {
		relevance = 0
	} else if relevance > 1 {
		relevance = 1
	}

	confidence := float64(eng.Confidence)
	if confidence < 0 {
		confidence = 0
	} else if confidence > 1 {
		confidence = 1
	}

	// Log factor: log₂(1 + access_count)
	// Ensures even high-frequency items have bounded contribution
	logFactor := math.Log2(1.0 + float64(eng.AccessCount))

	// Stability weight: 0.5 + 0.5 × stability (normalized)
	// Stability is in days; normalize to [0, 1] by dividing by a typical max (e.g., 365 days)
	// For simplicity, we use stability directly but clamp to [0, 1] after normalization
	stab := float64(eng.Stability) / 365.0 // normalize to 1 year as baseline
	if stab < 0 {
		stab = 0
	} else if stab > 1 {
		stab = 1
	}
	stabilityWeight := 0.5 + 0.5*stab

	return relevance * confidence * logFactor * stabilityWeight
}

// Get retrieves an engram from the cache and updates its access count and score.
// A full write lock is held for the entire operation to prevent a data race
// between the map lookup and the subsequent field writes on the entry.
func (c *DomainCache) Get(id storage.ULID) (*storage.Engram, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.items[id]
	if !ok {
		return nil, false
	}

	// Update access count and recalculate score
	entry.engram.AccessCount++
	entry.engram.LastAccess = time.Now()
	entry.score = computeScore(entry.engram)

	// Re-heapify at the entry's position
	if entry.heapIdx >= 0 && entry.heapIdx < len(c.heap) {
		heap.Fix(&c.heap, entry.heapIdx)
	}

	return entry.engram, true
}

// Set stores an engram in the cache, computing its eviction score.
// If cache is full, evicts the lowest-score item.
func (c *DomainCache) Set(id storage.ULID, eng *storage.Engram) {
	entry := &cacheEntry{
		engram:  eng,
		score:   computeScore(eng),
		heapIdx: -1,
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// If ID already exists, remove the old entry first
	if oldEntry, exists := c.items[id]; exists {
		if oldEntry.heapIdx >= 0 && oldEntry.heapIdx < len(c.heap) {
			heap.Remove(&c.heap, oldEntry.heapIdx)
		}
		delete(c.items, id)
	}

	// Add new entry to heap
	heap.Push(&c.heap, entry)
	c.items[id] = entry

	// Evict if over capacity
	for len(c.items) > c.maxItems {
		c.evictOne()
	}
}

// Delete removes an engram from the cache.
func (c *DomainCache) Delete(id storage.ULID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.items[id]
	if !ok {
		return
	}

	if entry.heapIdx >= 0 && entry.heapIdx < len(c.heap) {
		heap.Remove(&c.heap, entry.heapIdx)
	}
	delete(c.items, id)
}

// Len returns the number of entries in the cache.
func (c *DomainCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// evictOne removes the lowest-score item from the cache.
// Must be called with mu locked.
func (c *DomainCache) evictOne() {
	if len(c.heap) == 0 {
		return
	}

	entry := heap.Pop(&c.heap).(*cacheEntry)
	delete(c.items, entry.engram.ID)
}

// scoreHeap is a min-heap of *cacheEntry sorted by score ascending.
// Lowest score = most evictable = at top.
type scoreHeap []*cacheEntry

func (h scoreHeap) Len() int           { return len(h) }
func (h scoreHeap) Less(i, j int) bool { return h[i].score < h[j].score }
func (h scoreHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].heapIdx = i
	h[j].heapIdx = j
}

func (h *scoreHeap) Push(x interface{}) {
	entry := x.(*cacheEntry)
	entry.heapIdx = len(*h)
	*h = append(*h, entry)
}

func (h *scoreHeap) Pop() interface{} {
	old := *h
	n := len(old)
	entry := old[n-1]
	entry.heapIdx = -1
	*h = old[0 : n-1]
	return entry
}
