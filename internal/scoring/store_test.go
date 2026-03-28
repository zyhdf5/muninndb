package scoring

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/cockroachdb/pebble"
)

func setupTestDB(t *testing.T) *pebble.DB {
	tmpDir := t.TempDir()
	db, err := pebble.Open(tmpDir, &pebble.Options{})
	if err != nil {
		t.Fatalf("failed to open pebble: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_ = os.RemoveAll(tmpDir)
	})
	return db
}

func TestStore_GetDefault(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	ws := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	vw, err := store.Get(ctx, ws)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if vw == nil {
		t.Fatal("expected non-nil VaultWeights")
	}

	if vw.VaultPrefix != ws {
		t.Errorf("vault prefix mismatch: got %v, want %v", vw.VaultPrefix, ws)
	}

	sum := 0.0
	for _, w := range vw.Weights {
		sum += w
	}
	if sum < 0.99 || sum > 1.01 {
		t.Errorf("default weights sum = %v, want ~1.0", sum)
	}
}

func TestStore_SaveAndGet(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	ws := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	vw := &VaultWeights{
		VaultPrefix:  ws,
		Weights:      [NumDims]float64{0.2, 0.2, 0.2, 0.2, 0.1, 0.1},
		LearningRate: 0.15,
		UpdateCount:  5,
		UpdatedAt:    time.Now(),
	}

	if err := store.Save(ctx, vw); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	retrieved, err := store.Get(ctx, ws)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected non-nil VaultWeights")
	}

	for i := 0; i < NumDims; i++ {
		if retrieved.Weights[i] != vw.Weights[i] {
			t.Errorf("weight[%d] mismatch: got %v, want %v", i, retrieved.Weights[i], vw.Weights[i])
		}
	}

	if retrieved.LearningRate != vw.LearningRate {
		t.Errorf("learning rate mismatch: got %v, want %v", retrieved.LearningRate, vw.LearningRate)
	}

	if retrieved.UpdateCount != vw.UpdateCount {
		t.Errorf("update count mismatch: got %d, want %d", retrieved.UpdateCount, vw.UpdateCount)
	}
}

func TestStore_RecordFeedback_Throttled(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	ws := [8]byte{0x01}
	engramID := [16]byte{0x01}

	// First feedback should be recorded
	now := time.Now()
	signal1 := FeedbackSignal{
		EngramID:    engramID,
		Accessed:    true,
		Timestamp:   now,
		ScoreVector: [NumDims]float64{0.8, 0.1, 0.05, 0.03, 0.01, 0.01},
	}
	store.RecordFeedback(ctx, ws, signal1)

	vw1, err := store.Get(ctx, ws)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	weights1 := vw1.Weights

	// Second feedback within 30 minutes should be throttled (same engram ID)
	signal2 := FeedbackSignal{
		EngramID:    engramID,
		Accessed:    false, // opposite signal
		Timestamp:   now.Add(1 * time.Minute),
		ScoreVector: [NumDims]float64{0.8, 0.1, 0.05, 0.03, 0.01, 0.01},
	}
	store.RecordFeedback(ctx, ws, signal2)

	// Weights should be unchanged (second signal was throttled)
	vw2, _ := store.Get(ctx, ws)
	for i := 0; i < NumDims; i++ {
		if weights1[i] != vw2.Weights[i] {
			t.Errorf("weight[%d] changed when it should have been throttled: %v -> %v",
				i, weights1[i], vw2.Weights[i])
		}
	}

	// But a feedback for a DIFFERENT engram should NOT be throttled (different tracking key)
	differentEngramID := [16]byte{0x02}
	signal3 := FeedbackSignal{
		EngramID:    differentEngramID,
		Accessed:    false,
		Timestamp:   now.Add(1 * time.Minute),
		ScoreVector: [NumDims]float64{0.8, 0.1, 0.05, 0.03, 0.01, 0.01},
	}
	store.RecordFeedback(ctx, ws, signal3)

	vw3, _ := store.Get(ctx, ws)
	different := false
	for i := 0; i < NumDims; i++ {
		if weights1[i] != vw3.Weights[i] {
			different = true
			break
		}
	}
	if !different {
		t.Error("weights should have changed for different engram ID (not throttled)")
	}
}

func TestStore_CacheInvalidation(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	ws := [8]byte{0x01}

	// Get default weights (populates cache)
	vw1, err := store.Get(ctx, ws)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Save custom weights
	vw1.Weights = [NumDims]float64{0.1, 0.1, 0.1, 0.1, 0.3, 0.3}
	if err := store.Save(ctx, vw1); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Invalidate cache and fetch again
	store.InvalidateCache()
	vw2, err := store.Get(ctx, ws)
	if err != nil {
		t.Fatalf("Get after invalidation failed: %v", err)
	}

	// Weights should reflect what was saved
	for i := 0; i < NumDims; i++ {
		if vw2.Weights[i] != vw1.Weights[i] {
			t.Errorf("weight[%d] mismatch after cache invalidation: got %v, want %v",
				i, vw2.Weights[i], vw1.Weights[i])
		}
	}
}
