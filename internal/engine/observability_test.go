package engine

import (
	"context"
	"testing"
)

func TestObservability_BasicSnapshot(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()

	ctx := context.Background()
	snap, err := eng.Observability(ctx, "test", 100)
	if err != nil {
		t.Fatalf("Observability returned error: %v", err)
	}

	// Verify System stats
	if snap.System.Version != "test" {
		t.Errorf("Version = %q, want %q", snap.System.Version, "test")
	}
	if snap.System.UptimeSeconds != 100 {
		t.Errorf("UptimeSeconds = %d, want 100", snap.System.UptimeSeconds)
	}

	// Verify Vaults map is non-nil
	if snap.Vaults == nil {
		t.Error("Vaults map is nil, want non-nil")
	}

	// Verify Storage stats have non-negative DiskBytes
	if snap.Storage.DiskBytes < 0 {
		t.Errorf("DiskBytes = %d, want >= 0", snap.Storage.DiskBytes)
	}
}

func TestObservability_NilLatencyTracker(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()

	// The default testEnv does not set a latency tracker, so this tests nil-safety.
	if eng.LatencyTracker() != nil {
		t.Skip("testEnv sets a latency tracker; nil-safety test not applicable")
	}

	ctx := context.Background()
	snap, err := eng.Observability(ctx, "v0", 0)
	if err != nil {
		t.Fatalf("Observability with nil latency tracker returned error: %v", err)
	}
	if snap == nil {
		t.Fatal("snapshot is nil")
	}
}

func TestObservability_NoProcessors(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()

	// testEnv does not register retroactive processors, so Processors should be empty.
	ctx := context.Background()
	snap, err := eng.Observability(ctx, "v0", 0)
	if err != nil {
		t.Fatalf("Observability returned error: %v", err)
	}
	if len(snap.Processors) != 0 {
		t.Errorf("Processors length = %d, want 0", len(snap.Processors))
	}
}
