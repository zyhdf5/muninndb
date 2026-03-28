package query

import (
	"errors"
	"time"

	"github.com/scrypster/muninndb/internal/storage"
)

// Filter is a post-retrieval predicate applied to activation results.
// All fields are optional (zero value = no constraint).
// Tags uses AND semantics (all listed tags must be present).
// States uses OR semantics (any listed state matches).
type Filter struct {
	// Temporal
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	UpdatedAfter  *time.Time

	// Metadata — use exact type names from types.go
	States  []storage.LifecycleState
	Tags    []string // AND: all must match
	Creator string

	// Score thresholds
	MinRelevance  float32
	MinConfidence float32
	MinStability  float32

	// Vault
	Vaults     []string
	CrossVault bool

	// Pagination
	Limit  int // default 20, max 200
	Offset int
}

// Validate returns an error if the filter has invalid values.
func (f *Filter) Validate() error {
	// Limit validation: 0-200, 0 treated as 20
	if f.Limit < 0 || f.Limit > 200 {
		return errors.New("limit must be between 0 and 200")
	}

	// Offset validation: must be non-negative
	if f.Offset < 0 {
		return errors.New("offset must be non-negative")
	}

	// Score thresholds must be in [0, 1]
	if f.MinRelevance < 0 || f.MinRelevance > 1 {
		return errors.New("min_relevance must be between 0.0 and 1.0")
	}
	if f.MinConfidence < 0 || f.MinConfidence > 1 {
		return errors.New("min_confidence must be between 0.0 and 1.0")
	}
	if f.MinStability < 0 {
		return errors.New("min_stability must be non-negative")
	}

	return nil
}

// Match returns true if the engram satisfies all filter constraints.
// A zero-value Filter matches everything.
func (f *Filter) Match(e *storage.Engram) bool {
	// Temporal constraints
	if f.CreatedAfter != nil && !e.CreatedAt.After(*f.CreatedAfter) {
		return false
	}
	if f.CreatedBefore != nil && !e.CreatedAt.Before(*f.CreatedBefore) {
		return false
	}
	if f.UpdatedAfter != nil && !e.UpdatedAt.After(*f.UpdatedAfter) {
		return false
	}

	// State constraint (OR semantics)
	if len(f.States) > 0 {
		stateMatches := false
		for _, s := range f.States {
			if e.State == s {
				stateMatches = true
				break
			}
		}
		if !stateMatches {
			return false
		}
	}

	// Tags constraint (AND semantics: all tags must be present in engram)
	if len(f.Tags) > 0 {
		engramTagMap := make(map[string]struct{}, len(e.Tags))
		for _, tag := range e.Tags {
			engramTagMap[tag] = struct{}{}
		}
		for _, requiredTag := range f.Tags {
			if _, found := engramTagMap[requiredTag]; !found {
				return false
			}
		}
	}

	// Creator constraint (exact match)
	if f.Creator != "" && e.CreatedBy != f.Creator {
		return false
	}

	// Score thresholds
	if f.MinRelevance > 0 && e.Relevance < f.MinRelevance {
		return false
	}
	if f.MinConfidence > 0 && e.Confidence < f.MinConfidence {
		return false
	}
	if f.MinStability > 0 && e.Stability < f.MinStability {
		return false
	}

	return true
}

// Apply filters and paginates a slice of engrams.
// Applies Match to each, then applies Offset and Limit.
func (f *Filter) Apply(engrams []*storage.Engram) []*storage.Engram {
	// First, filter all engrams with Match
	filtered := make([]*storage.Engram, 0, len(engrams))
	for _, e := range engrams {
		if e != nil && f.Match(e) {
			filtered = append(filtered, e)
		}
	}

	// Apply pagination
	limit := f.Limit
	if limit == 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	start := f.Offset
	if start >= len(filtered) {
		return []*storage.Engram{} // empty slice
	}

	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}

	return filtered[start:end]
}
