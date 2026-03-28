package brief

import (
	"context"
	"math"
	"strings"
)

// EmbeddingModel is the interface for embedding text into vectors.
// Matches the existing embedder interface pattern in the codebase.
type EmbeddingModel interface {
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Dim() int
}

// ScoredSentence is a sentence with its cosine similarity score.
type ScoredSentence struct {
	Text       string
	Score      float32
	TokenCount int // approximate: len(strings.Fields(text))
}

// Scorer scores sentences by cosine similarity to a context embedding.
type Scorer struct {
	Model        EmbeddingModel
	Threshold    float32 // minimum cosine similarity (default 0.72)
	MaxSentences int     // max sentences to return (default 3)
	MaxSentLen   int     // max chars per sentence before truncation (default 512)
}

// DefaultScorer returns a Scorer with default settings.
func DefaultScorer(model EmbeddingModel) *Scorer {
	return &Scorer{
		Model:        model,
		Threshold:    0.72,
		MaxSentences: 3,
		MaxSentLen:   512,
	}
}

// Score returns the top sentences from content by cosine similarity to contextEmbedding.
// Falls back to first MaxSentences sentences if model is nil or embedding fails.
func (s *Scorer) Score(ctx context.Context, content string, contextEmbedding []float32) ([]ScoredSentence, error) {
	sentences := Split(content, s.MaxSentLen)
	if len(sentences) == 0 {
		return nil, nil
	}

	// Fallback: no model or no context embedding → return first MaxSentences
	if s.Model == nil || len(contextEmbedding) == 0 {
		return s.fallbackTopN(sentences), nil
	}

	// Embed all sentences in batch
	embeddings, err := s.Model.EmbedBatch(ctx, sentences)
	if err != nil {
		// Fallback on error
		return s.fallbackTopN(sentences), nil
	}

	// Score each sentence by cosine similarity
	type scoredSentence struct {
		text  string
		score float32
		idx   int
	}

	var scored []scoredSentence
	for i, embedding := range embeddings {
		if len(embedding) != len(contextEmbedding) {
			// Dimension mismatch; skip this sentence
			continue
		}
		cosine := cosineSimilarity(embedding, contextEmbedding)
		if cosine >= s.Threshold {
			scored = append(scored, scoredSentence{
				text:  sentences[i],
				score: cosine,
				idx:   i,
			})
		}
	}

	// If no sentences meet the threshold, fall back to top-N by cosine (even below threshold)
	if len(scored) == 0 {
		type allScored struct {
			text  string
			score float32
		}
		allSentences := make([]allScored, 0, len(embeddings))
		for i, embedding := range embeddings {
			if len(embedding) == len(contextEmbedding) {
				cosine := cosineSimilarity(embedding, contextEmbedding)
				allSentences = append(allSentences, allScored{
					text:  sentences[i],
					score: cosine,
				})
			}
		}

		// Sort by score descending (simple bubble sort or direct slice for small counts)
		for i := 0; i < len(allSentences)-1; i++ {
			for j := i + 1; j < len(allSentences); j++ {
				if allSentences[j].score > allSentences[i].score {
					allSentences[i], allSentences[j] = allSentences[j], allSentences[i]
				}
			}
		}

		// Return top-N
		maxN := s.MaxSentences
		if maxN > len(allSentences) {
			maxN = len(allSentences)
		}

		result := make([]ScoredSentence, maxN)
		for i := 0; i < maxN; i++ {
			result[i] = ScoredSentence{
				Text:       allSentences[i].text,
				Score:      allSentences[i].score,
				TokenCount: len(strings.Fields(allSentences[i].text)),
			}
		}
		return result, nil
	}

	// Sort scored sentences by score descending (simple bubble sort)
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	// Return top-N
	maxN := s.MaxSentences
	if maxN > len(scored) {
		maxN = len(scored)
	}

	result := make([]ScoredSentence, maxN)
	for i := 0; i < maxN; i++ {
		result[i] = ScoredSentence{
			Text:       scored[i].text,
			Score:      scored[i].score,
			TokenCount: len(strings.Fields(scored[i].text)),
		}
	}
	return result, nil
}

// fallbackTopN returns the first MaxSentences sentences with zero score.
func (s *Scorer) fallbackTopN(sentences []string) []ScoredSentence {
	maxN := s.MaxSentences
	if maxN > len(sentences) {
		maxN = len(sentences)
	}

	result := make([]ScoredSentence, maxN)
	for i := 0; i < maxN; i++ {
		result[i] = ScoredSentence{
			Text:       sentences[i],
			Score:      0,
			TokenCount: len(strings.Fields(sentences[i])),
		}
	}
	return result
}

// cosineSimilarity computes cosine similarity between two float32 vectors.
// Returns 0 if either vector is zero-magnitude.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, magA, magB float64
	for i := range a {
		fa := float64(a[i])
		fb := float64(b[i])
		dotProduct += fa * fb
		magA += fa * fa
		magB += fb * fb
	}

	magA = math.Sqrt(magA)
	magB = math.Sqrt(magB)

	if magA == 0 || magB == 0 {
		return 0
	}

	return float32(dotProduct / (magA * magB))
}
