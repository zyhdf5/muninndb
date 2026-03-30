package erf

import (
	"math"
	"testing"
	"time"
)

func TestPatchRelevance_RoundTrip(t *testing.T) {
	// Create a valid engram
	eng := &Engram{
		Concept:    "test-concept",
		Content:    "test content",
		Confidence: 0.9,
		Relevance:  0.5,
		Stability:  30.0,
		State:      0x01, // StateActive
		CreatedAt:  time.Now().Truncate(time.Nanosecond),
		UpdatedAt:  time.Now().Truncate(time.Nanosecond),
		LastAccess: time.Now().Truncate(time.Nanosecond),
	}
	copy(eng.ID[:], []byte("0123456789abcdef"))

	raw, err := Encode(eng)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Patch relevance/stability
	newRelevance := float32(0.8)
	newStability := float32(60.0)
	beforePatch := time.Now()
	if err := PatchRelevance(raw, time.Now(), newRelevance, newStability); err != nil {
		t.Fatalf("PatchRelevance: %v", err)
	}

	// Decode and verify
	decoded, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode after patch: %v (CRC32 should still be valid)", err)
	}

	if math.Abs(float64(decoded.Relevance-newRelevance)) > 1e-5 {
		t.Errorf("Relevance: expected %v, got %v", newRelevance, decoded.Relevance)
	}
	if math.Abs(float64(decoded.Stability-newStability)) > 1e-5 {
		t.Errorf("Stability: expected %v, got %v", newStability, decoded.Stability)
	}
	if decoded.UpdatedAt.Before(beforePatch) {
		t.Errorf("UpdatedAt should be after patch time")
	}
	// Concept should be unchanged
	if decoded.Concept != eng.Concept {
		t.Errorf("Concept should be unchanged: got %q", decoded.Concept)
	}
}

func TestPatchAllMeta_RoundTrip(t *testing.T) {
	eng := &Engram{
		Concept:    "all-meta-test",
		Content:    "some content here",
		Confidence: 0.7,
		Relevance:  0.4,
		Stability:  20.0,
		State:      0x01, // StateActive
		CreatedAt:  time.Now().Truncate(time.Nanosecond),
		UpdatedAt:  time.Now().Truncate(time.Nanosecond),
		LastAccess: time.Now().Truncate(time.Nanosecond),
	}
	copy(eng.ID[:], []byte("fedcba9876543210"))

	raw, err := Encode(eng)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	newConf := float32(0.95)
	newRel := float32(0.85)
	newStab := float32(90.0)
	newAcc := uint32(42)
	newState := uint8(0x04) // StateCompleted
	now := time.Now()

	if err := PatchAllMeta(raw, now, now, newConf, newRel, newStab, newAcc, newState); err != nil {
		t.Fatalf("PatchAllMeta: %v", err)
	}

	decoded, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode after PatchAllMeta: %v", err)
	}

	if math.Abs(float64(decoded.Confidence-newConf)) > 1e-5 {
		t.Errorf("Confidence: expected %v, got %v", newConf, decoded.Confidence)
	}
	if math.Abs(float64(decoded.Relevance-newRel)) > 1e-5 {
		t.Errorf("Relevance: expected %v, got %v", newRel, decoded.Relevance)
	}
	if math.Abs(float64(decoded.Stability-newStab)) > 1e-5 {
		t.Errorf("Stability: expected %v, got %v", newStab, decoded.Stability)
	}
	if decoded.AccessCount != newAcc {
		t.Errorf("AccessCount: expected %v, got %v", newAcc, decoded.AccessCount)
	}
	if decoded.State != newState {
		t.Errorf("State: expected %v, got %v", newState, decoded.State)
	}
	// Content should be unchanged
	if decoded.Content != eng.Content {
		t.Errorf("Content should be unchanged")
	}
}

func TestDecodeMetaConcept_ExtractsConcept(t *testing.T) {
	eng := &Engram{
		Concept:    "my-special-concept",
		Content:    "some very long content that we do not want to decompress",
		Confidence: 0.8,
		Relevance:  0.6,
		Stability:  45.0,
		State:      0x01, // StateActive
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		LastAccess: time.Now(),
	}
	copy(eng.ID[:], []byte("0123456789abcdef"))

	raw, err := Encode(eng)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	meta, concept, err := DecodeMetaConcept(raw)
	if err != nil {
		t.Fatalf("DecodeMetaConcept: %v", err)
	}
	if concept != eng.Concept {
		t.Errorf("expected concept %q, got %q", eng.Concept, concept)
	}
	if math.Abs(float64(meta.Relevance-eng.Relevance)) > 1e-5 {
		t.Errorf("meta.Relevance mismatch: expected %v, got %v", eng.Relevance, meta.Relevance)
	}
}
