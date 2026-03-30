package circuit

import (
	"errors"
	"testing"
	"time"
)

var errFail = errors.New("simulated failure")

func TestClosedAllowsAll(t *testing.T) {
	b := New(3, time.Second)
	for i := 0; i < 10; i++ {
		if err := b.Allow(); err != nil {
			t.Fatalf("closed breaker rejected call %d: %v", i, err)
		}
		b.RecordSuccess()
	}
}

func TestOpensAfterConsecutiveFailures(t *testing.T) {
	b := New(3, time.Hour) // long reset so it stays open
	for i := 0; i < 3; i++ {
		_ = b.Allow()
		b.RecordFailure()
	}
	if b.State() != StateOpen {
		t.Fatalf("expected StateOpen after 3 failures, got %v", b.StateString())
	}
	if err := b.Allow(); err != ErrOpen {
		t.Fatalf("expected ErrOpen from open circuit, got %v", err)
	}
}

func TestHalfOpenAfterReset(t *testing.T) {
	b := New(2, 10*time.Millisecond)
	_ = b.Allow()
	b.RecordFailure()
	_ = b.Allow()
	b.RecordFailure()
	// Circuit is open
	if b.State() != StateOpen {
		t.Fatal("expected open")
	}
	// Wait for reset interval
	time.Sleep(20 * time.Millisecond)
	// Allow should succeed (probe)
	if err := b.Allow(); err != nil {
		t.Fatalf("expected allow after reset, got %v", err)
	}
	if b.State() != StateHalfOpen {
		t.Fatalf("expected half-open, got %v", b.StateString())
	}
}

func TestHalfOpenOnlyAllowsOneProbe(t *testing.T) {
	b := New(1, 10*time.Millisecond)
	_ = b.Allow()
	b.RecordFailure()
	time.Sleep(20 * time.Millisecond)
	_ = b.Allow() // probe
	// Second call should be rejected
	if err := b.Allow(); err != ErrOpen {
		t.Fatalf("expected ErrOpen for second half-open call, got %v", err)
	}
}

func TestSuccessClosesFromHalfOpen(t *testing.T) {
	b := New(1, 10*time.Millisecond)
	_ = b.Allow()
	b.RecordFailure()
	time.Sleep(20 * time.Millisecond)
	_ = b.Allow() // probe
	b.RecordSuccess()
	if b.State() != StateClosed {
		t.Fatalf("expected closed after half-open success, got %v", b.StateString())
	}
}

func TestDoWrapsSuccessAndFailure(t *testing.T) {
	b := New(2, time.Hour)

	// Two failures should open
	_ = b.Do(func() error { return errFail })
	_ = b.Do(func() error { return errFail })
	if b.State() != StateOpen {
		t.Fatal("expected open after 2 Do failures")
	}

	// Open circuit returns ErrOpen without calling fn
	called := false
	err := b.Do(func() error { called = true; return nil })
	if !errors.Is(err, ErrOpen) {
		t.Fatalf("expected ErrOpen, got %v", err)
	}
	if called {
		t.Fatal("fn should not be called when circuit is open")
	}
}

func TestSuccessResetsFailureCount(t *testing.T) {
	b := New(3, time.Hour)
	_ = b.Allow()
	b.RecordFailure()
	_ = b.Allow()
	b.RecordFailure()
	_ = b.Allow()
	b.RecordSuccess() // resets count
	// Two more failures (below threshold again)
	_ = b.Allow()
	b.RecordFailure()
	_ = b.Allow()
	b.RecordFailure()
	if b.State() != StateClosed {
		t.Fatal("expected closed: success reset failure count")
	}
}
