package provenance

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/scrypster/muninndb/internal/storage/keys"
	"github.com/scrypster/muninndb/internal/types"
)

// TestProvenance_AppendAndGet tests appending entries and retrieving them in order.
func TestProvenance_AppendAndGet(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "provenance-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := pebble.Open(tempDir, &pebble.Options{})
	if err != nil {
		t.Fatalf("Failed to open Pebble: %v", err)
	}
	defer db.Close()

	store := NewStore(db)
	ctx := context.Background()

	// Use a fixed vault prefix for testing
	vaultPrefix := keys.VaultPrefix("test-vault")
	id := types.NewULID()
	idBytes := [16]byte(id)

	// Append three entries
	entry1 := ProvenanceEntry{
		Timestamp: time.Now(),
		Source:    SourceHuman,
		AgentID:   "user:mj",
		Operation: "create",
		Note:      "Initial creation",
	}

	err = store.Append(ctx, vaultPrefix, idBytes, entry1)
	if err != nil {
		t.Fatalf("Append entry 1 failed: %v", err)
	}

	entry2 := ProvenanceEntry{
		Timestamp: time.Now().Add(1 * time.Second),
		Source:    SourceInferred,
		AgentID:   "relevance-worker",
		Operation: "update-relevance",
		Note:      "Relevance increased",
	}

	err = store.Append(ctx, vaultPrefix, idBytes, entry2)
	if err != nil {
		t.Fatalf("Append entry 2 failed: %v", err)
	}

	entry3 := ProvenanceEntry{
		Timestamp: time.Now().Add(2 * time.Second),
		Source:    SourceLLM,
		AgentID:   "ollama:llama3.2",
		Operation: "update-meta",
		Note:      "Enriched metadata",
	}

	err = store.Append(ctx, vaultPrefix, idBytes, entry3)
	if err != nil {
		t.Fatalf("Append entry 3 failed: %v", err)
	}

	// Retrieve and verify
	entries, err := store.Get(ctx, vaultPrefix, idBytes)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(entries))
	}

	// Verify order and content
	if entries[0].Source != SourceHuman {
		t.Errorf("Entry 0 source mismatch: got %v, want %v", entries[0].Source, SourceHuman)
	}
	if entries[0].Operation != "create" {
		t.Errorf("Entry 0 operation mismatch: got %q, want %q", entries[0].Operation, "create")
	}
	if entries[0].AgentID != "user:mj" {
		t.Errorf("Entry 0 agent ID mismatch: got %q, want %q", entries[0].AgentID, "user:mj")
	}

	if entries[1].Source != SourceInferred {
		t.Errorf("Entry 1 source mismatch: got %v, want %v", entries[1].Source, SourceInferred)
	}
	if entries[1].Operation != "update-relevance" {
		t.Errorf("Entry 1 operation mismatch: got %q, want %q", entries[1].Operation, "update-relevance")
	}

	if entries[2].Source != SourceLLM {
		t.Errorf("Entry 2 source mismatch: got %v, want %v", entries[2].Source, SourceLLM)
	}
	if entries[2].Operation != "update-meta" {
		t.Errorf("Entry 2 operation mismatch: got %q, want %q", entries[2].Operation, "update-meta")
	}
}

// TestProvenance_GetEmpty tests that Get returns empty slice for non-existent engram.
func TestProvenance_GetEmpty(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "provenance-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := pebble.Open(tempDir, &pebble.Options{})
	if err != nil {
		t.Fatalf("Failed to open Pebble: %v", err)
	}
	defer db.Close()

	store := NewStore(db)
	ctx := context.Background()

	vaultPrefix := keys.VaultPrefix("test-vault")
	unknownID := types.NewULID()
	unknownIDBytes := [16]byte(unknownID)

	// Get for non-existent engram
	entries, err := store.Get(ctx, vaultPrefix, unknownIDBytes)
	if err != nil {
		t.Fatalf("Get should not error on missing engram: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("Expected empty slice for missing engram, got %d entries", len(entries))
	}

	// Verify it's a zero-length slice, not nil
	if entries == nil {
		t.Error("Expected empty slice, got nil")
	}
}

// TestProvenance_MultipleEngrams tests that entries for different engrams don't bleed together.
func TestProvenance_MultipleEngrams(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "provenance-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := pebble.Open(tempDir, &pebble.Options{})
	if err != nil {
		t.Fatalf("Failed to open Pebble: %v", err)
	}
	defer db.Close()

	store := NewStore(db)
	ctx := context.Background()

	vaultPrefix := keys.VaultPrefix("test-vault")
	id1 := types.NewULID()
	id1Bytes := [16]byte(id1)
	id2 := types.NewULID()
	id2Bytes := [16]byte(id2)

	// Append entry to first engram
	entry1 := ProvenanceEntry{
		Timestamp: time.Now(),
		Source:    SourceHuman,
		AgentID:   "user:alice",
		Operation: "create",
		Note:      "Created by Alice",
	}
	err = store.Append(ctx, vaultPrefix, id1Bytes, entry1)
	if err != nil {
		t.Fatalf("Append to engram 1 failed: %v", err)
	}

	// Append entry to second engram
	entry2 := ProvenanceEntry{
		Timestamp: time.Now(),
		Source:    SourceHuman,
		AgentID:   "user:bob",
		Operation: "create",
		Note:      "Created by Bob",
	}
	err = store.Append(ctx, vaultPrefix, id2Bytes, entry2)
	if err != nil {
		t.Fatalf("Append to engram 2 failed: %v", err)
	}

	// Append another entry to first engram
	entry3 := ProvenanceEntry{
		Timestamp: time.Now().Add(1 * time.Second),
		Source:    SourceLLM,
		AgentID:   "enricher",
		Operation: "update-meta",
		Note:      "Enriched",
	}
	err = store.Append(ctx, vaultPrefix, id1Bytes, entry3)
	if err != nil {
		t.Fatalf("Append to engram 1 again failed: %v", err)
	}

	// Verify engram 1 has 2 entries
	entries1, err := store.Get(ctx, vaultPrefix, id1Bytes)
	if err != nil {
		t.Fatalf("Get engram 1 failed: %v", err)
	}
	if len(entries1) != 2 {
		t.Errorf("Engram 1 should have 2 entries, got %d", len(entries1))
	}
	if entries1[0].AgentID != "user:alice" {
		t.Errorf("Engram 1 entry 0 agent mismatch: got %q, want %q", entries1[0].AgentID, "user:alice")
	}
	if entries1[1].AgentID != "enricher" {
		t.Errorf("Engram 1 entry 1 agent mismatch: got %q, want %q", entries1[1].AgentID, "enricher")
	}

	// Verify engram 2 has 1 entry (not contaminated by engram 1)
	entries2, err := store.Get(ctx, vaultPrefix, id2Bytes)
	if err != nil {
		t.Fatalf("Get engram 2 failed: %v", err)
	}
	if len(entries2) != 1 {
		t.Errorf("Engram 2 should have 1 entry, got %d", len(entries2))
	}
	if entries2[0].AgentID != "user:bob" {
		t.Errorf("Engram 2 entry agent mismatch: got %q, want %q", entries2[0].AgentID, "user:bob")
	}
}
