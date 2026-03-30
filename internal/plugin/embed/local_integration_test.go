//go:build integration

package embed

import (
	"context"
	"math"
	"testing"
)

// TestLocalProviderIntegration is a full end-to-end test that requires model assets
// to be embedded in the binary.
//
// Build and run with:
//
//	go test -tags integration ./internal/plugin/embed/ -run TestLocalProvider -v
func TestLocalProviderIntegration(t *testing.T) {
	if len(embeddedModel) == 0 {
		t.Skip("embeddedModel is empty — run `make fetch-assets` and rebuild with -tags integration")
	}

	ctx := context.Background()
	dir := t.TempDir()

	p := &LocalProvider{}
	dim, err := p.Init(ctx, ProviderHTTPConfig{DataDir: dir})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if dim != localModelDim {
		t.Errorf("dim: want %d, got %d", localModelDim, dim)
	}
	defer p.Close()

	// Verify embeddings have the correct shape and unit norm.
	sentences := []string{
		"The quick brown fox jumps over the lazy dog",
		"Machine learning and artificial intelligence",
		"The cat sat on the mat",
	}
	for _, text := range sentences {
		vecs, err := p.EmbedBatch(ctx, []string{text})
		if err != nil {
			t.Fatalf("EmbedBatch(%q): %v", text, err)
		}
		if len(vecs) != localModelDim {
			t.Errorf("text %q: expected %d floats, got %d", text, localModelDim, len(vecs))
		}
		norm := computeNorm(vecs)
		if diff := math.Abs(norm - 1.0); diff > 1e-4 {
			t.Errorf("text %q: expected unit norm, got %.6f", text, norm)
		}
	}

	// Semantically related pair should score higher than unrelated pair.
	vFox, _ := p.EmbedBatch(ctx, []string{"the fox jumps"})
	vDog, _ := p.EmbedBatch(ctx, []string{"the dog runs"})
	vRocket, _ := p.EmbedBatch(ctx, []string{"NASA launches rocket"})

	simFoxDog := cosineSim(vFox, vDog)
	simFoxRocket := cosineSim(vFox, vRocket)
	t.Logf("cosine sim fox-dog=%.4f  fox-rocket=%.4f", simFoxDog, simFoxRocket)

	if simFoxDog <= simFoxRocket {
		t.Errorf("expected related pair (fox-dog=%.3f) > unrelated pair (fox-rocket=%.3f)",
			simFoxDog, simFoxRocket)
	}
}

func cosineSim(a, b []float32) float64 {
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot // vectors are already L2-normalized
}
