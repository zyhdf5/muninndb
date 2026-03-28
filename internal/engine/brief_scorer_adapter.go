package engine

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/scrypster/muninndb/internal/engine/activation"
)

// embedderAdapter wraps an activation.Embedder into a brief.EmbeddingModel.
// It infers dimensionality from the first embedding if Dim() is not available.
//
// dim is written exactly once on the first successful embedding call and is
// thereafter read-only. dimOnce ensures the write happens only once and is
// visible to all subsequent readers without a data race.
type embedderAdapter struct {
	embedder activation.Embedder
	dimOnce  sync.Once
	dim      atomic.Int64 // read by Dim(); written once inside dimOnce.Do
}

// EmbedBatch implements brief.EmbeddingModel.EmbedBatch.
// It calls the underlying embedder's Embed method and reshapes the flat
// result into a slice of vectors.
func (a *embedderAdapter) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// Call the underlying embedder
	flat, err := a.embedder.Embed(ctx, texts)
	if err != nil {
		return nil, err
	}

	// Infer dimension from flat result once and cache atomically.
	if len(flat) > 0 {
		a.dimOnce.Do(func() {
			a.dim.Store(int64(len(flat) / len(texts)))
		})
	}

	dim := int(a.dim.Load())
	if dim == 0 || len(flat) != len(texts)*dim {
		// Dimension mismatch or couldn't infer; return nil
		return nil, nil
	}

	// Reshape flat vector into 2D
	result := make([][]float32, len(texts))
	for i := 0; i < len(texts); i++ {
		start := i * dim
		end := start + dim
		result[i] = flat[start:end]
	}

	return result, nil
}

// Dim implements brief.EmbeddingModel.Dim.
func (a *embedderAdapter) Dim() int {
	return int(a.dim.Load())
}

// newEmbedderAdapter wraps an activation.Embedder for use by the brief scorer.
func newEmbedderAdapter(embedder activation.Embedder) *embedderAdapter {
	return &embedderAdapter{embedder: embedder}
}
