package mbp

import (
	"sync"
	"testing"
	"time"
)

func TestPendingMapRegisterAndComplete(t *testing.T) {
	pm := NewPendingMap()

	// Register a request
	corrID := uint64(12345)
	ch := pm.Register(corrID)

	// Send a response frame
	responseFrame := &Frame{
		Type:          TypeWriteOK,
		CorrelationID: corrID,
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		pm.Complete(corrID, responseFrame)
	}()

	// Wait for the response
	select {
	case resp := <-ch:
		if resp.CorrelationID != corrID {
			t.Errorf("response correlation ID mismatch: got %d, want %d", resp.CorrelationID, corrID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for response")
	}
}

func TestPendingMapMultipleConcurrent(t *testing.T) {
	pm := NewPendingMap()
	numRequests := 100

	var wg sync.WaitGroup
	responses := make(map[uint64]bool)
	var mu sync.Mutex

	// Register multiple requests
	channels := make(map[uint64]<-chan *Frame)
	for i := 0; i < numRequests; i++ {
		corrID := uint64(i)
		channels[corrID] = pm.Register(corrID)
	}

	// Send responses in random order (simulating out-of-order responses)
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			corrID := uint64(idx)
			// Stagger the responses slightly
			time.Sleep(time.Duration(100-idx) * time.Microsecond)
			pm.Complete(corrID, &Frame{CorrelationID: corrID})
		}(i)
	}

	// Wait for all responses
	for corrID, ch := range channels {
		wg.Add(1)
		go func(id uint64, c <-chan *Frame) {
			defer wg.Done()
			select {
			case <-c:
				mu.Lock()
				responses[id] = true
				mu.Unlock()
			case <-time.After(500 * time.Millisecond):
				t.Errorf("timeout waiting for response %d", id)
			}
		}(corrID, ch)
	}

	wg.Wait()

	// Check all responses received
	if len(responses) != numRequests {
		t.Errorf("got %d responses, expected %d", len(responses), numRequests)
	}
}

func TestPendingMapCancel(t *testing.T) {
	pm := NewPendingMap()

	corrID := uint64(999)
	ch := pm.Register(corrID)

	// Cancel the request
	pm.Cancel(corrID)

	// Channel should be closed
	select {
	case <-ch:
		// Expected: channel is closed
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected channel to be closed")
	}
}

func TestPendingMapCancelNonexistent(t *testing.T) {
	pm := NewPendingMap()

	// Cancelling a non-existent request should not panic
	pm.Cancel(99999)

	// Should complete without error
}

func TestPendingMapCompleteAfterCancel(t *testing.T) {
	pm := NewPendingMap()

	corrID := uint64(555)
	_ = pm.Register(corrID)

	// Cancel first
	pm.Cancel(corrID)

	// Then try to complete (should be safe)
	pm.Complete(corrID, &Frame{CorrelationID: corrID})

	// Should complete without panicking
}

func TestPendingMapNoReceiver(t *testing.T) {
	pm := NewPendingMap()

	corrID := uint64(777)
	_ = pm.Register(corrID)

	// Don't receive on the channel, just complete
	// This should not panic or hang
	pm.Complete(corrID, &Frame{CorrelationID: corrID})
}
