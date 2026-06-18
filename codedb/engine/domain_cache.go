package engine

import (
	"container/heap"
	"math"
	"sync"
	"time"
)

// DomainCache 基于相关性加权最小堆淘汰策略的缓存。
//
//	score = relevance × confidence × log₂(1 + access_count) × (0.5 + 0.5×stability/365)
type DomainCache struct {
	mu       sync.RWMutex
	items    map[ULID]*domainCacheEntry
	heap     scoreHeap
	maxItems int
}

type domainCacheEntry struct {
	engram  *Engram
	score   float64
	heapIdx int
}

func NewDomainCache(maxSize int) *DomainCache {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &DomainCache{
		items:    make(map[ULID]*domainCacheEntry),
		heap:     make(scoreHeap, 0),
		maxItems: maxSize,
	}
}

func computeDomainScore(eng *Engram) float64 {
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

	logFactor := math.Log2(1.0 + float64(eng.AccessCount))

	stab := float64(eng.Stability) / 365.0
	if stab < 0 {
		stab = 0
	} else if stab > 1 {
		stab = 1
	}
	stabilityWeight := 0.5 + 0.5*stab

	return relevance * confidence * logFactor * stabilityWeight
}

func (c *DomainCache) Get(id ULID) (*Engram, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.items[id]
	if !ok {
		return nil, false
	}

	entry.engram.AccessCount++
	entry.engram.LastAccess = time.Now()
	entry.score = computeDomainScore(entry.engram)

	if entry.heapIdx >= 0 && entry.heapIdx < len(c.heap) {
		heap.Fix(&c.heap, entry.heapIdx)
	}

	return entry.engram, true
}

func (c *DomainCache) Set(id ULID, eng *Engram) {
	entry := &domainCacheEntry{
		engram:  eng,
		score:   computeDomainScore(eng),
		heapIdx: -1,
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if oldEntry, exists := c.items[id]; exists {
		if oldEntry.heapIdx >= 0 && oldEntry.heapIdx < len(c.heap) {
			heap.Remove(&c.heap, oldEntry.heapIdx)
		}
		delete(c.items, id)
	}

	heap.Push(&c.heap, entry)
	c.items[id] = entry

	for len(c.items) > c.maxItems {
		c.evictOne()
	}
}

func (c *DomainCache) Delete(id ULID) {
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

func (c *DomainCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

func (c *DomainCache) evictOne() {
	if len(c.heap) == 0 {
		return
	}
	entry := heap.Pop(&c.heap).(*domainCacheEntry)
	delete(c.items, entry.engram.ID)
}

type scoreHeap []*domainCacheEntry

func (h scoreHeap) Len() int           { return len(h) }
func (h scoreHeap) Less(i, j int) bool { return h[i].score < h[j].score }
func (h scoreHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].heapIdx = i
	h[j].heapIdx = j
}

func (h *scoreHeap) Push(x any) {
	entry := x.(*domainCacheEntry)
	entry.heapIdx = len(*h)
	*h = append(*h, entry)
}

func (h *scoreHeap) Pop() any {
	old := *h
	n := len(old)
	entry := old[n-1]
	entry.heapIdx = -1
	*h = old[0 : n-1]
	return entry
}
