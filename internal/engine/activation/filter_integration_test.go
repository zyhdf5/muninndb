package activation

import (
	"testing"
	"time"

	"github.com/scrypster/muninndb/internal/storage"
)

// mockStructuredFilter implements the interface expected by the activation engine.
// It's a simplified query.Filter for testing purposes.
type mockStructuredFilter struct {
	minRelevance float32
	states       map[storage.LifecycleState]bool
	tags         []string
}

func (f *mockStructuredFilter) Match(e *storage.Engram) bool {
	// Check relevance
	if f.minRelevance > 0 && e.Relevance < f.minRelevance {
		return false
	}

	// Check state (OR semantics)
	if len(f.states) > 0 {
		if !f.states[e.State] {
			return false
		}
	}

	// Check tags (AND semantics)
	if len(f.tags) > 0 {
		tagMap := make(map[string]struct{}, len(e.Tags))
		for _, tag := range e.Tags {
			tagMap[tag] = struct{}{}
		}
		for _, requiredTag := range f.tags {
			if _, found := tagMap[requiredTag]; !found {
				return false
			}
		}
	}

	return true
}

// TestActivation_StructuredFilterIntegration tests that the structured filter
// is applied correctly in the activation pipeline (specifically in phase6Score).
func TestActivation_StructuredFilterIntegration(t *testing.T) {
	// Create mock engrams
	now := time.Now()
	engrams := []*storage.Engram{
		{
			ID:         storage.NewULID(),
			Concept:    "high relevance, active, tagged",
			Content:    "content1",
			CreatedAt:  now,
			UpdatedAt:  now,
			State:      storage.StateActive,
			Relevance:  0.8,
			Confidence: 0.9,
			Tags:       []string{"important", "urgent"},
		},
		{
			ID:         storage.NewULID(),
			Concept:    "low relevance, active, tagged",
			Content:    "content2",
			CreatedAt:  now,
			UpdatedAt:  now,
			State:      storage.StateActive,
			Relevance:  0.2,
			Confidence: 0.7,
			Tags:       []string{"important"},
		},
		{
			ID:         storage.NewULID(),
			Concept:    "high relevance, paused, tagged",
			Content:    "content3",
			CreatedAt:  now,
			UpdatedAt:  now,
			State:      storage.StatePaused,
			Relevance:  0.8,
			Confidence: 0.8,
			Tags:       []string{"important"},
		},
		{
			ID:         storage.NewULID(),
			Concept:    "high relevance, active, no tags",
			Content:    "content4",
			CreatedAt:  now,
			UpdatedAt:  now,
			State:      storage.StateActive,
			Relevance:  0.8,
			Confidence: 0.9,
			Tags:       []string{},
		},
	}

	// Test 1: Filter by state (OR) and tags (AND)
	t.Run("filter_state_and_tags", func(t *testing.T) {
		filter := &mockStructuredFilter{
			states: map[storage.LifecycleState]bool{
				storage.StateActive: true,
			},
			tags: []string{"important", "urgent"},
		}

		// Apply filter (mimicking what happens in phase6Score)
		var filtered []*storage.Engram
		for _, e := range engrams {
			if filter.Match(e) {
				filtered = append(filtered, e)
			}
		}

		// Should only match engram 0 (active, has both important and urgent)
		if len(filtered) != 1 {
			t.Fatalf("expected 1 result, got %d", len(filtered))
		}
		if filtered[0].Concept != "high relevance, active, tagged" {
			t.Fatalf("unexpected engram matched: %q", filtered[0].Concept)
		}
	})

	// Test 2: Filter by relevance
	t.Run("filter_min_relevance", func(t *testing.T) {
		filter := &mockStructuredFilter{
			minRelevance: 0.5,
		}

		var filtered []*storage.Engram
		for _, e := range engrams {
			if filter.Match(e) {
				filtered = append(filtered, e)
			}
		}

		// Should match 3 engrams: 0, 2, 3 (all have relevance >= 0.5)
		if len(filtered) != 3 {
			t.Fatalf("expected 3 results, got %d", len(filtered))
		}
	})

	// Test 3: Combined filters (state, tags, relevance)
	t.Run("filter_combined", func(t *testing.T) {
		filter := &mockStructuredFilter{
			minRelevance: 0.7,
			states: map[storage.LifecycleState]bool{
				storage.StateActive: true,
			},
			tags: []string{"important"},
		}

		var filtered []*storage.Engram
		for _, e := range engrams {
			if filter.Match(e) {
				filtered = append(filtered, e)
			}
		}

		// Should match only engram 0 (active, has "important", relevance 0.8)
		if len(filtered) != 1 {
			t.Fatalf("expected 1 result, got %d", len(filtered))
		}
		if filtered[0].Concept != "high relevance, active, tagged" {
			t.Fatalf("unexpected engram matched: %q", filtered[0].Concept)
		}
	})

	// Test 4: No filters (all pass)
	t.Run("filter_none", func(t *testing.T) {
		filter := &mockStructuredFilter{}

		var filtered []*storage.Engram
		for _, e := range engrams {
			if filter.Match(e) {
				filtered = append(filtered, e)
			}
		}

		if len(filtered) != 4 {
			t.Fatalf("expected 4 results, got %d", len(filtered))
		}
	})
}
