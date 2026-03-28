package storage

import (
	"sync"
	"testing"
)

func TestL1CacheDeleteByVault_RemovesAllEntries(t *testing.T) {
	c := NewL1Cache(100)
	var wsA [8]byte
	wsA[0] = 0xAA
	var wsB [8]byte
	wsB[0] = 0xBB

	for i := 0; i < 5; i++ {
		id := NewULID()
		c.Set(wsA, id, &Engram{ID: id, Concept: "a", Content: "a"})
	}
	for i := 0; i < 3; i++ {
		id := NewULID()
		c.Set(wsB, id, &Engram{ID: id, Concept: "b", Content: "b"})
	}

	c.DeleteByVault(wsA)

	if got := c.Len(); got != 3 {
		t.Errorf("expected 3 entries (vault-B only), got %d", got)
	}
}

func TestL1CacheDeleteByVault_NegativeCountPrevented(t *testing.T) {
	c := NewL1Cache(100)
	var ws [8]byte
	ws[0] = 0xCC
	ids := make([]ULID, 10)
	for i := range ids {
		ids[i] = NewULID()
		c.Set(ws, ids[i], &Engram{ID: ids[i], Concept: "x", Content: "y"})
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		c.DeleteByVault(ws)
	}()
	go func() {
		defer wg.Done()
		for _, id := range ids {
			c.Delete(ws, id)
		}
	}()
	wg.Wait()

	if got := c.Len(); got < 0 {
		t.Errorf("cache count went negative: %d", got)
	}
	// All entries deleted by exactly one goroutine — final count must be 0
	if got := c.Len(); got != 0 {
		t.Errorf("expected Len=0 after concurrent deletes, got %d", got)
	}
}
