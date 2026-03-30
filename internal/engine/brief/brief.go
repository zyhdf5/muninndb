package brief

import (
	"regexp"
	"sort"
	"strings"

	"github.com/scrypster/muninndb/internal/index/fts"
)

const (
	MaxSentencesPerEngram = 20 // cap sentences processed per engram
	MinSentenceLen        = 10 // minimum characters to consider a sentence
	BriefSize             = 5  // number of sentences in the brief
)

// Sentence is a scored sentence from an engram.
type Sentence struct {
	EngramID string
	Text     string
	Score    float64
}

// EngramContent holds the content of a single engram for brief computation.
type EngramContent struct {
	ID      string
	Content string
}

// sentenceBoundaryRegex matches sentence endings: punctuation followed by
// whitespace. We'll manually check for capital letter after matching.
// This regex matches ". " or "? " or "! " followed by whitespace.
var sentenceBoundaryRegex = regexp.MustCompile(`[.?!]\s+`)

// splitSentences splits text into sentences using the regex heuristic.
// Returns at most MaxSentencesPerEngram sentences of at least MinSentenceLen chars.
// First splits on newlines, then on sentence boundaries, filtering and capping.
func splitSentences(text string) []string {
	// First split on newlines to handle structured entries
	lines := strings.Split(text, "\n")
	var sentences []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < MinSentenceLen {
			continue
		}

		// Split line on sentence boundaries using regex
		// FindAllStringIndex returns slice of [start, end] pairs for matches
		matches := sentenceBoundaryRegex.FindAllStringIndex(line, -1)

		if len(matches) == 0 {
			// No sentence boundaries found; treat entire line as one sentence
			if len(line) >= MinSentenceLen {
				sentences = append(sentences, line)
			}
		} else {
			// Split at match positions, keeping punctuation with left part
			lastEnd := 0
			for i, match := range matches {
				// match[0] is the start of the punctuation, match[1] is after the whitespace
				// We want the sentence to include the punctuation but not the whitespace
				splitPos := match[0] + 1 // include the punctuation character
				sentence := line[lastEnd:splitPos]
				sentence = strings.TrimSpace(sentence)

				// Check if next character is uppercase (actual sentence boundary)
				nextStart := match[1]
				if nextStart < len(line) && len(sentence) >= MinSentenceLen {
					// Verify the character after whitespace is uppercase or is a letter
					// This filters out abbreviated matches like "Dr. smith"
					nextRune := rune(line[nextStart])
					if nextStart < len(line) && (nextRune >= 'A' && nextRune <= 'Z') {
						sentences = append(sentences, sentence)
					} else if i == len(matches)-1 {
						// Last match; include it anyway
						sentences = append(sentences, sentence)
					}
				}
				// Next sentence starts after the whitespace
				lastEnd = match[1]
			}
			// Add remainder after last sentence boundary
			remainder := line[lastEnd:]
			remainder = strings.TrimSpace(remainder)
			if len(remainder) >= MinSentenceLen {
				sentences = append(sentences, remainder)
			}
		}

		if len(sentences) >= MaxSentencesPerEngram {
			break
		}
	}

	// Cap at MaxSentencesPerEngram
	if len(sentences) > MaxSentencesPerEngram {
		return sentences[:MaxSentencesPerEngram]
	}
	return sentences
}

// scoreText scores a piece of text against a set of query terms.
// Score = sum of (term frequency in text) for each query term.
// Uses fts.Tokenize to normalize and filter text.
// Returns 0 if no query terms match.
func scoreText(text string, queryTerms map[string]bool) float64 {
	tokens := fts.Tokenize(text)
	score := 0.0
	for _, tok := range tokens {
		if queryTerms[tok] {
			score++
		}
	}
	return score
}

// Compute generates an ActivationBrief from a set of engrams and a query.
// engrams is a slice of (engramID, content) pairs.
// query is the list of query context strings.
// Returns the top BriefSize sentences sorted by score descending.
// Returns nil (not an error) if there are no engrams, no query terms, or no matching sentences.
func Compute(engrams []EngramContent, query []string) []Sentence {
	if len(engrams) == 0 || len(query) == 0 {
		return nil
	}

	// Build query term set
	queryTerms := make(map[string]bool)
	for _, q := range query {
		for _, tok := range fts.Tokenize(q) {
			queryTerms[tok] = true
		}
	}

	if len(queryTerms) == 0 {
		return nil
	}

	// Score all sentences from all engrams
	var all []Sentence
	for _, eng := range engrams {
		sentences := splitSentences(eng.Content)
		for _, s := range sentences {
			score := scoreText(s, queryTerms)
			if score > 0 {
				all = append(all, Sentence{
					EngramID: eng.ID,
					Text:     strings.TrimSpace(s),
					Score:    score,
				})
			}
		}
	}

	// Sort by score descending
	sort.Slice(all, func(i, j int) bool {
		return all[i].Score > all[j].Score
	})

	// Return top BriefSize
	if len(all) > BriefSize {
		return all[:BriefSize]
	}
	return all
}
