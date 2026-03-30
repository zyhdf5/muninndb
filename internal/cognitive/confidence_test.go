package cognitive

import (
	"context"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// mockConfidenceStore for testing the ConfidenceWorker in isolation.
// ---------------------------------------------------------------------------

type mockConfidenceStore struct {
	mu          sync.Mutex
	confidences map[[16]byte]float32
}

func newMockConfidenceStore() *mockConfidenceStore {
	return &mockConfidenceStore{confidences: make(map[[16]byte]float32)}
}

func (m *mockConfidenceStore) GetConfidence(_ context.Context, _ [8]byte, id [16]byte) (float32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.confidences[id]
	if !ok {
		return 0.8, nil // default prior
	}
	return v, nil
}

func (m *mockConfidenceStore) UpdateConfidence(_ context.Context, _ [8]byte, id [16]byte, c float32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.confidences[id] = c
	return nil
}

func (m *mockConfidenceStore) get(id [16]byte) (float32, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.confidences[id]
	return v, ok
}

// ---------------------------------------------------------------------------
// Test 1: BayesianUpdate with contradiction evidence lowers confidence
// ---------------------------------------------------------------------------

// TestBayesianUpdateContradictionLowersConfidence verifies that applying the
// "contradiction_detected" evidence source via BayesianUpdate produces a
// posterior that is lower than the prior (since EvidenceContradiction < 0.5).
func TestBayesianUpdateContradictionLowersConfidence(t *testing.T) {
	prior := 0.8
	evidence := EvidenceStrength("contradiction_detected") // EvidenceContradiction = 0.1

	posterior := BayesianUpdate(prior, evidence)

	if posterior >= prior {
		t.Errorf("BayesianUpdate with contradiction evidence should lower confidence: prior=%v posterior=%v",
			prior, posterior)
	}
	// Laplace smoothing prevents posterior from hitting 0.
	if posterior <= 0 {
		t.Errorf("posterior should be > 0 due to Laplace smoothing, got %v", posterior)
	}
}

// ---------------------------------------------------------------------------
// Test 2: BayesianUpdate with co-activation evidence raises confidence
// ---------------------------------------------------------------------------

// TestBayesianUpdateCoActivationRaisesConfidence verifies that applying the
// "co_activation" evidence source via BayesianUpdate produces a posterior that
// is greater than the prior (since EvidenceCoActivation > 0.5).
func TestBayesianUpdateCoActivationRaisesConfidence(t *testing.T) {
	prior := 0.5
	evidence := EvidenceStrength("co_activation") // EvidenceCoActivation = 0.65

	posterior := BayesianUpdate(prior, evidence)

	if posterior <= prior {
		t.Errorf("BayesianUpdate with co_activation evidence should raise confidence: prior=%v posterior=%v",
			prior, posterior)
	}
}

// ---------------------------------------------------------------------------
// Test 3: ConfidenceWorker lowers confidence after a contradiction event
// ---------------------------------------------------------------------------

// TestConfidenceWorkerLowersConfidenceAfterContradiction verifies that submitting
// a ConfidenceUpdate with "contradiction_detected" source causes the worker to
// write a lower confidence value to the store than the starting prior.
func TestConfidenceWorkerLowersConfidenceAfterContradiction(t *testing.T) {
	store := newMockConfidenceStore()
	cw := NewConfidenceWorker(store)

	engramID := [16]byte{1}
	ws := [8]byte{0, 0, 0, 1}

	// Pre-seed a high prior.
	store.mu.Lock()
	store.confidences[engramID] = 0.9
	store.mu.Unlock()

	// Submit contradiction evidence directly to processBatch.
	ctx := context.Background()
	err := cw.processBatch(ctx, []ConfidenceUpdate{
		{
			WS:       ws,
			EngramID: engramID,
			Evidence: EvidenceContradiction,
			Source:   "contradiction_detected",
		},
	})
	if err != nil {
		t.Fatalf("processBatch: %v", err)
	}

	got, ok := store.get(engramID)
	if !ok {
		t.Fatal("UpdateConfidence was never called — engram confidence not updated")
	}
	if got >= 0.9 {
		t.Errorf("expected confidence to decrease from 0.9 after contradiction, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Test 4: ConfidenceWorker end-to-end via Submit
// ---------------------------------------------------------------------------

// TestConfidenceWorkerSubmitAndProcess verifies that events submitted via
// Submit() are eventually processed and the store is updated.
func TestConfidenceWorkerSubmitAndProcess(t *testing.T) {
	store := newMockConfidenceStore()
	cw := NewConfidenceWorker(store)

	ctx, cancel := context.WithCancel(context.Background())

	// Start the worker in a goroutine (it blocks until ctx is cancelled).
	done := make(chan struct{})
	go func() {
		defer close(done)
		cw.Worker.Run(ctx) //nolint:errcheck
	}()

	engramID := [16]byte{42}
	ws := [8]byte{0, 0, 0, 2}

	// Pre-seed prior.
	store.mu.Lock()
	store.confidences[engramID] = 0.8
	store.mu.Unlock()

	// Submit a contradiction confidence update.
	cw.Worker.Submit(ConfidenceUpdate{
		WS:       ws,
		EngramID: engramID,
		Evidence: EvidenceContradiction,
		Source:   "contradiction_detected",
	})

	// Give the worker time to process (tick interval is 30s in production;
	// the batch fills when batchSize=100 is reached OR the ticker fires.
	// We cancel context to force a final flush via shutdown path).
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	// After shutdown flush the store should have been updated.
	got, ok := store.get(engramID)
	if !ok {
		t.Fatal("UpdateConfidence was never called after Submit+shutdown")
	}
	if got >= 0.8 {
		t.Errorf("expected confidence < 0.8 after contradiction event, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Test 5: Multiple contradictions chain-lower confidence
// ---------------------------------------------------------------------------

// TestConfidenceWorkerMultipleContradictions verifies that chained contradictions
// on the same engram progressively lower its confidence (each update is applied
// sequentially within a single batch using grouped evidence).
func TestConfidenceWorkerMultipleContradictions(t *testing.T) {
	store := newMockConfidenceStore()
	cw := NewConfidenceWorker(store)

	engramID := [16]byte{99}
	ws := [8]byte{0, 0, 0, 3}

	store.mu.Lock()
	store.confidences[engramID] = 0.95
	store.mu.Unlock()

	ctx := context.Background()

	// Three contradiction events in one batch.
	batch := []ConfidenceUpdate{
		{WS: ws, EngramID: engramID, Evidence: EvidenceContradiction, Source: "contradiction_detected"},
		{WS: ws, EngramID: engramID, Evidence: EvidenceContradiction, Source: "contradiction_detected"},
		{WS: ws, EngramID: engramID, Evidence: EvidenceContradiction, Source: "contradiction_detected"},
	}
	if err := cw.processBatch(ctx, batch); err != nil {
		t.Fatalf("processBatch: %v", err)
	}

	got, ok := store.get(engramID)
	if !ok {
		t.Fatal("confidence not updated after multiple contradictions")
	}
	if got >= 0.95 {
		t.Errorf("expected confidence well below 0.95 after 3 contradictions, got %v", got)
	}
}
