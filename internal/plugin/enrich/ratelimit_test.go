package enrich

import (
	"context"
	"testing"
	"time"
)

// TestTokenBucketLimiter_Basic tests that a token is acquired immediately when available.
func TestTokenBucketLimiter_Basic(t *testing.T) {
	limiter := NewTokenBucketLimiter(10.0, 10.0)
	ctx := context.Background()

	// First token should be acquired immediately
	start := time.Now()
	err := limiter.Wait(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}

	if elapsed > 100*time.Millisecond {
		t.Fatalf("Wait took too long: %v", elapsed)
	}
}

// TestTokenBucketLimiter_ContextCancel tests that context cancellation is respected.
func TestTokenBucketLimiter_ContextCancel(t *testing.T) {
	// Create a limiter with moderate rate
	limiter := NewTokenBucketLimiter(1.0, 1.0)

	// Consume the initial bucket
	ctx := context.Background()
	if err := limiter.Wait(ctx); err != nil {
		t.Fatalf("First Wait failed: %v", err)
	}

	// Now create a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This should timeout
	err := limiter.Wait(ctx)
	if err != context.DeadlineExceeded {
		t.Fatalf("Expected context.DeadlineExceeded, got: %v", err)
	}
}
