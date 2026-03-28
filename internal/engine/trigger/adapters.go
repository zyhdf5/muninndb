package trigger

import (
	"context"

	"github.com/scrypster/muninndb/internal/index/fts"
	hnswpkg "github.com/scrypster/muninndb/internal/index/hnsw"
	"github.com/scrypster/muninndb/internal/storage"
)

// hnswTriggerAdapter adapts *hnswpkg.Registry to trigger.HNSWIndex.
type hnswTriggerAdapter struct{ reg *hnswpkg.Registry }

func (a *hnswTriggerAdapter) Search(ctx context.Context, ws [8]byte, vec []float32, topK int) ([]ScoredID, error) {
	results, err := a.reg.Search(ctx, ws, vec, topK)
	if err != nil {
		return nil, err
	}
	out := make([]ScoredID, len(results))
	for i, r := range results {
		out[i] = ScoredID{ID: storage.ULID(r.ID), Score: r.Score}
	}
	return out, nil
}

// NewHNSWAdapter returns an HNSWIndex that delegates to the given registry.
func NewHNSWAdapter(reg *hnswpkg.Registry) HNSWIndex {
	return &hnswTriggerAdapter{reg: reg}
}

// ftsTriggerAdapter adapts *fts.Index to trigger.FTSIndex.
type ftsTriggerAdapter struct{ idx *fts.Index }

func (a *ftsTriggerAdapter) Search(ctx context.Context, ws [8]byte, query string, topK int) ([]ScoredID, error) {
	results, err := a.idx.Search(ctx, ws, query, topK)
	if err != nil {
		return nil, err
	}
	out := make([]ScoredID, len(results))
	for i, r := range results {
		out[i] = ScoredID{ID: storage.ULID(r.ID), Score: r.Score}
	}
	return out, nil
}

// NewFTSAdapter returns an FTSIndex that delegates to the given index.
func NewFTSAdapter(idx *fts.Index) FTSIndex {
	return &ftsTriggerAdapter{idx: idx}
}
