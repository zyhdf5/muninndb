package episodic

import (
	"context"
	"os"
	"testing"

	"github.com/cockroachdb/pebble"
	"github.com/scrypster/muninndb/internal/storage/keys"
	"github.com/scrypster/muninndb/internal/types"
)

// TestEpisodic_CreateAndList tests creating an episode, appending frames, closing, and retrieving.
func TestEpisodic_CreateAndList(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "episodic-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := pebble.Open(tempDir, &pebble.Options{})
	if err != nil {
		t.Fatalf("Failed to open Pebble: %v", err)
	}
	defer db.Close()

	store := NewPebbleEpisodicStore(db)
	ctx := context.Background()

	// Use a fixed vault prefix for testing
	vaultPrefix := keys.VaultPrefix("test-vault")

	// Create an episode
	ep, err := store.CreateEpisode(ctx, vaultPrefix, "Test Episode")
	if err != nil {
		t.Fatalf("CreateEpisode failed: %v", err)
	}
	if ep == nil {
		t.Fatal("CreateEpisode returned nil")
	}
	if ep.Title != "Test Episode" {
		t.Errorf("Episode title mismatch: got %q, want %q", ep.Title, "Test Episode")
	}
	if ep.ClosedAt != nil {
		t.Error("Newly created episode should have ClosedAt=nil")
	}
	if ep.FrameCount != 0 {
		t.Errorf("New episode should have FrameCount=0, got %d", ep.FrameCount)
	}

	episodeID := ep.ID

	// Append three frames
	engram1 := types.NewULID()
	engram2 := types.NewULID()
	engram3 := types.NewULID()

	frame1, err := store.AppendFrame(ctx, vaultPrefix, episodeID, engram1, "First memory")
	if err != nil {
		t.Fatalf("AppendFrame 1 failed: %v", err)
	}
	if frame1.Position != 0 {
		t.Errorf("Frame 1 position mismatch: got %d, want 0", frame1.Position)
	}

	frame2, err := store.AppendFrame(ctx, vaultPrefix, episodeID, engram2, "Second memory")
	if err != nil {
		t.Fatalf("AppendFrame 2 failed: %v", err)
	}
	if frame2.Position != 1 {
		t.Errorf("Frame 2 position mismatch: got %d, want 1", frame2.Position)
	}

	frame3, err := store.AppendFrame(ctx, vaultPrefix, episodeID, engram3, "Third memory")
	if err != nil {
		t.Fatalf("AppendFrame 3 failed: %v", err)
	}
	if frame3.Position != 2 {
		t.Errorf("Frame 3 position mismatch: got %d, want 2", frame3.Position)
	}

	// ListFrames and verify order
	frames, err := store.ListFrames(ctx, vaultPrefix, episodeID)
	if err != nil {
		t.Fatalf("ListFrames failed: %v", err)
	}
	if len(frames) != 3 {
		t.Errorf("Expected 3 frames, got %d", len(frames))
	}

	for i, f := range frames {
		if f.Position != uint32(i) {
			t.Errorf("Frame %d position mismatch: got %d, want %d", i, f.Position, i)
		}
	}

	// Verify engram IDs match
	if frames[0].EngramID != engram1 {
		t.Error("Frame 0 engram ID mismatch")
	}
	if frames[1].EngramID != engram2 {
		t.Error("Frame 1 engram ID mismatch")
	}
	if frames[2].EngramID != engram3 {
		t.Error("Frame 2 engram ID mismatch")
	}

	// Close the episode
	err = store.CloseEpisode(ctx, vaultPrefix, episodeID)
	if err != nil {
		t.Fatalf("CloseEpisode failed: %v", err)
	}

	// Retrieve and verify
	ep2, err := store.GetEpisode(ctx, vaultPrefix, episodeID)
	if err != nil {
		t.Fatalf("GetEpisode failed: %v", err)
	}
	if ep2 == nil {
		t.Fatal("GetEpisode returned nil")
	}
	if ep2.ClosedAt == nil {
		t.Error("Closed episode should have ClosedAt set")
	}
	if ep2.FrameCount != 3 {
		t.Errorf("Episode FrameCount mismatch: got %d, want 3", ep2.FrameCount)
	}

	// Verify idempotency: closing again should succeed
	err = store.CloseEpisode(ctx, vaultPrefix, episodeID)
	if err != nil {
		t.Fatalf("CloseEpisode (second call) failed: %v", err)
	}
}

// TestEpisodic_MultiEpisode tests multiple episodes with frame isolation.
func TestEpisodic_MultiEpisode(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "episodic-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := pebble.Open(tempDir, &pebble.Options{})
	if err != nil {
		t.Fatalf("Failed to open Pebble: %v", err)
	}
	defer db.Close()

	store := NewPebbleEpisodicStore(db)
	ctx := context.Background()

	vaultPrefix := keys.VaultPrefix("test-vault")

	// Create two episodes
	ep1, err := store.CreateEpisode(ctx, vaultPrefix, "Episode 1")
	if err != nil {
		t.Fatalf("CreateEpisode 1 failed: %v", err)
	}

	ep2, err := store.CreateEpisode(ctx, vaultPrefix, "Episode 2")
	if err != nil {
		t.Fatalf("CreateEpisode 2 failed: %v", err)
	}

	// Add frames to ep1
	engram1a := types.NewULID()
	engram1b := types.NewULID()
	_, err = store.AppendFrame(ctx, vaultPrefix, ep1.ID, engram1a, "ep1 frame 1")
	if err != nil {
		t.Fatalf("AppendFrame to ep1 failed: %v", err)
	}
	_, err = store.AppendFrame(ctx, vaultPrefix, ep1.ID, engram1b, "ep1 frame 2")
	if err != nil {
		t.Fatalf("AppendFrame to ep1 failed: %v", err)
	}

	// Add frames to ep2
	engram2a := types.NewULID()
	engram2b := types.NewULID()
	engram2c := types.NewULID()
	_, err = store.AppendFrame(ctx, vaultPrefix, ep2.ID, engram2a, "ep2 frame 1")
	if err != nil {
		t.Fatalf("AppendFrame to ep2 failed: %v", err)
	}
	_, err = store.AppendFrame(ctx, vaultPrefix, ep2.ID, engram2b, "ep2 frame 2")
	if err != nil {
		t.Fatalf("AppendFrame to ep2 failed: %v", err)
	}
	_, err = store.AppendFrame(ctx, vaultPrefix, ep2.ID, engram2c, "ep2 frame 3")
	if err != nil {
		t.Fatalf("AppendFrame to ep2 failed: %v", err)
	}

	// ListEpisodes
	episodes, err := store.ListEpisodes(ctx, vaultPrefix, 10)
	if err != nil {
		t.Fatalf("ListEpisodes failed: %v", err)
	}
	if len(episodes) != 2 {
		t.Errorf("Expected 2 episodes, got %d", len(episodes))
	}

	// Verify titles
	found1 := false
	found2 := false
	for _, e := range episodes {
		if e.Title == "Episode 1" {
			found1 = true
		}
		if e.Title == "Episode 2" {
			found2 = true
		}
	}
	if !found1 || !found2 {
		t.Error("Not all episodes found in ListEpisodes")
	}

	// ListFrames per episode — verify isolation
	frames1, err := store.ListFrames(ctx, vaultPrefix, ep1.ID)
	if err != nil {
		t.Fatalf("ListFrames ep1 failed: %v", err)
	}
	if len(frames1) != 2 {
		t.Errorf("Expected 2 frames in ep1, got %d", len(frames1))
	}

	frames2, err := store.ListFrames(ctx, vaultPrefix, ep2.ID)
	if err != nil {
		t.Fatalf("ListFrames ep2 failed: %v", err)
	}
	if len(frames2) != 3 {
		t.Errorf("Expected 3 frames in ep2, got %d", len(frames2))
	}

	// Verify frame isolation: frames1 should not contain frames from ep2
	for _, f := range frames1 {
		if f.EpisodeID != ep1.ID {
			t.Error("Frame from ep1 has wrong episode ID")
		}
	}
	for _, f := range frames2 {
		if f.EpisodeID != ep2.ID {
			t.Error("Frame from ep2 has wrong episode ID")
		}
	}
}

// TestEpisodic_ListEpisodesLimit tests the limit parameter in ListEpisodes.
func TestEpisodic_ListEpisodesLimit(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "episodic-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := pebble.Open(tempDir, &pebble.Options{})
	if err != nil {
		t.Fatalf("Failed to open Pebble: %v", err)
	}
	defer db.Close()

	store := NewPebbleEpisodicStore(db)
	ctx := context.Background()

	vaultPrefix := keys.VaultPrefix("test-vault")

	// Create 5 episodes
	for i := 0; i < 5; i++ {
		_, err := store.CreateEpisode(ctx, vaultPrefix, "Episode")
		if err != nil {
			t.Fatalf("CreateEpisode failed: %v", err)
		}
	}

	// ListEpisodes with limit 3
	episodes, err := store.ListEpisodes(ctx, vaultPrefix, 3)
	if err != nil {
		t.Fatalf("ListEpisodes failed: %v", err)
	}
	if len(episodes) != 3 {
		t.Errorf("Expected 3 episodes with limit 3, got %d", len(episodes))
	}

	// ListEpisodes with limit 10 (more than available)
	episodes, err = store.ListEpisodes(ctx, vaultPrefix, 10)
	if err != nil {
		t.Fatalf("ListEpisodes failed: %v", err)
	}
	if len(episodes) != 5 {
		t.Errorf("Expected 5 episodes, got %d", len(episodes))
	}
}

// TestEpisodic_NotFound tests retrieving non-existent episodes.
func TestEpisodic_NotFound(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "episodic-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := pebble.Open(tempDir, &pebble.Options{})
	if err != nil {
		t.Fatalf("Failed to open Pebble: %v", err)
	}
	defer db.Close()

	store := NewPebbleEpisodicStore(db)
	ctx := context.Background()

	vaultPrefix := keys.VaultPrefix("test-vault")
	fakeID := types.NewULID()

	// GetEpisode for non-existent ID should return nil, no error
	ep, err := store.GetEpisode(ctx, vaultPrefix, fakeID)
	if err != nil {
		t.Fatalf("GetEpisode failed: %v", err)
	}
	if ep != nil {
		t.Error("GetEpisode for non-existent ID should return nil")
	}

	// ListFrames for non-existent episode should return empty list
	frames, err := store.ListFrames(ctx, vaultPrefix, fakeID)
	if err != nil {
		t.Fatalf("ListFrames failed: %v", err)
	}
	if len(frames) != 0 {
		t.Errorf("ListFrames for non-existent episode should return empty, got %d", len(frames))
	}

	// AppendFrame to non-existent episode should fail
	engram := types.NewULID()
	_, err = store.AppendFrame(ctx, vaultPrefix, fakeID, engram, "test")
	if err == nil {
		t.Error("AppendFrame to non-existent episode should fail")
	}
}
