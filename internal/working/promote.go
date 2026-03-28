package working

import "github.com/scrypster/muninndb/internal/types"

// PromotionCandidate is an item from working memory eligible for engram relevance boost.
type PromotionCandidate struct {
	EngramID  types.ULID
	Attention float32
	Context   string
}

// PromotionDelta computes the relevance boost to apply.
// Formula: attention * 0.1, capped at 0.15
func PromotionDelta(attention float32) float32 {
	delta := attention * 0.1
	if delta > 0.15 {
		delta = 0.15
	}
	return delta
}
