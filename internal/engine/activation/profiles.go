// internal/engine/activation/profiles.go
package activation

import (
	"regexp"
	"strings"

	"github.com/scrypster/muninndb/internal/storage"
)

// minInferenceScore is the minimum score required for InferProfile to commit
// to a non-default profile. A single low-confidence rule match won't override Default.
const minInferenceScore = 2

// TraversalProfile controls how BFS Phase 5 traverses the association graph.
//
//   - Include: if non-empty, only traverse edges with RelType in this set.
//   - Exclude: skip edges with RelType in this set (applied after Include check).
//   - Boost: per-RelType score multiplier. Missing types default to 1.0.
//
// Note: user-defined RelTypes (values >= 0x8000) are only traversed by the default profile.
// Non-default profiles use explicit Include lists and will silently exclude any type not listed,
// including user-defined types.
type TraversalProfile struct {
	Include    []storage.RelType
	Exclude    []storage.RelType
	Boost      map[storage.RelType]float32
	includeSet map[storage.RelType]struct{}
	excludeSet map[storage.RelType]struct{}
}

// Includes reports whether rel passes the Include filter.
// If Include is empty, all types pass (traverse everything).
func (p *TraversalProfile) Includes(rel storage.RelType) bool {
	if len(p.includeSet) == 0 {
		return true
	}
	_, ok := p.includeSet[rel]
	return ok
}

// Excluded reports whether rel is in the Exclude list.
func (p *TraversalProfile) Excluded(rel storage.RelType) bool {
	_, ok := p.excludeSet[rel]
	return ok
}

// BoostFor returns the score multiplier for rel. Returns 1.0 if not configured.
func (p *TraversalProfile) BoostFor(rel storage.RelType) float32 {
	// Reading a nil map in Go returns the zero value (false ok), so this is safe
	// even when Boost is nil (e.g. structural profile).
	if v, ok := p.Boost[rel]; ok {
		return v
	}
	return 1.0
}

// AllowsEdge returns true if an edge with this RelType should be traversed.
func (p *TraversalProfile) AllowsEdge(rel storage.RelType) bool {
	return p.Includes(rel) && !p.Excluded(rel)
}

func newProfile(include, exclude []storage.RelType, boost map[storage.RelType]float32) *TraversalProfile {
	p := &TraversalProfile{
		Include:    include,
		Exclude:    exclude,
		Boost:      boost,
		includeSet: make(map[storage.RelType]struct{}, len(include)),
		excludeSet: make(map[storage.RelType]struct{}, len(exclude)),
	}
	for _, r := range include {
		p.includeSet[r] = struct{}{}
	}
	for _, r := range exclude {
		p.excludeSet[r] = struct{}{}
	}
	return p
}

// builtinProfiles are the five hardcoded traversal profiles.
// These are package-level constants — not configurable, not composable.
var builtinProfiles = map[string]*TraversalProfile{
	"default": newProfile(
		nil, // empty Include = traverse all edge types
		nil, // no exclusions
		map[storage.RelType]float32{
			storage.RelContradicts: 0.3, // dampen contradiction edges
			storage.RelSupersedes:  0.5, // dampen superseded edges
		},
	),
	"causal": newProfile(
		[]storage.RelType{
			storage.RelCauses,
			storage.RelDependsOn,
			storage.RelBlocks,
			storage.RelPrecededBy,
			storage.RelFollowedBy,
		},
		nil, // no additional exclusions beyond Include filtering
		map[storage.RelType]float32{
			storage.RelCauses: 1.3,
		},
	),
	"confirmatory": newProfile(
		[]storage.RelType{
			storage.RelSupports,
			storage.RelImplements,
			storage.RelRefines,
			storage.RelReferences,
		},
		[]storage.RelType{storage.RelContradicts}, // explicitly exclude contradictions
		map[storage.RelType]float32{
			storage.RelSupports: 1.2,
		},
	),
	"adversarial": newProfile(
		[]storage.RelType{
			storage.RelContradicts,
			storage.RelSupersedes,
			storage.RelBlocks,
		},
		nil,
		map[storage.RelType]float32{
			storage.RelContradicts: 1.5,
		},
	),
	"structural": newProfile(
		[]storage.RelType{
			storage.RelIsPartOf,
			storage.RelBelongsToProject,
			storage.RelCreatedByPerson,
		},
		nil,
		nil, // no boosts — structural traversal is topology-only
	),
}

// GetProfile returns the named built-in profile, or the default profile for unknown names.
// Never returns nil.
//
// WARNING: The returned pointer is shared with package-level state.
// Callers MUST NOT modify any fields of the returned value (Include, Exclude, Boost,
// or the unexported sets). Treat the return value as strictly read-only.
func GetProfile(name string) *TraversalProfile {
	if p, ok := builtinProfiles[name]; ok {
		return p
	}
	return builtinProfiles["default"]
}

// ValidProfileName reports whether name is a known built-in profile.
func ValidProfileName(name string) bool {
	_, ok := builtinProfiles[name]
	return ok
}

// --- Auto-Inference Engine ---

type profileRule struct {
	profile  string
	patterns []*regexp.Regexp
	score    int
}

var inferenceRules = []profileRule{
	{
		profile: "causal",
		score:   2,
		patterns: mustCompilePatterns([]string{
			`(?i)\bwhy\b`,
			`(?i)\bwhat\s+caused\b`,
			`(?i)\bwhat\s+led\s+to\b`,
			`(?i)\broot\s+cause\b`,
			`(?i)\bblocked\s+by\b`,
			`(?i)\bbecause\s+of\s+what\b`,
			`(?i)\bwhat\s+is\s+blocking\b`,
			`(?i)\bdepends\s+on\b.{0,40}\?`,
		}),
	},
	{
		profile: "adversarial",
		score:   3,
		patterns: mustCompilePatterns([]string{
			`(?i)\bcontradict`,
			`(?i)\bconflict(?:s|ing)?\s+(?:with|between)\b`,
			`(?i)\binconsisten`,
			`(?i)\bwrong\s+about\b`,
			`(?i)\bdisagree`,
		}),
	},
	{
		profile: "confirmatory",
		score:   2,
		patterns: mustCompilePatterns([]string{
			`(?i)\bvalidat`,
			`(?i)\bevidence\s+(?:for|that)\b`,
			`(?i)\bconfirm`,
			`(?i)\bsupports?\s+(?:the\s+)?(?:claim|idea|theory|decision|hypothesis)\b`,
		}),
	},
	{
		profile: "structural",
		score:   2,
		patterns: mustCompilePatterns([]string{
			`(?i)\bpart\s+of\b`,
			`(?i)\bbelongs\s+to\b`,
			`(?i)\bstructure\s+of\b`,
			`(?i)\bcomponents?\s+of\b`,
		}),
	},
}

// InferProfile selects a traversal profile from context phrases using pattern matching.
//
// Resolution order:
//  1. Pattern-match joined contexts; if max score >= 2, return that profile.
//  2. Fall through to vaultDefault if it is a valid profile name.
//  3. Fall through to "default".
func InferProfile(contexts []string, vaultDefault string) string {
	if len(contexts) == 0 {
		return fallbackProfile(vaultDefault)
	}

	joined := strings.Join(contexts, " ")
	scores := make(map[string]int, len(inferenceRules))

	for _, rule := range inferenceRules {
		for _, pat := range rule.patterns {
			if pat.MatchString(joined) {
				scores[rule.profile] += rule.score
				break // one match per rule is enough
			}
		}
	}

	// Find the highest-scoring profile. Lexicographic tie-break for determinism.
	type scored struct {
		profile string
		score   int
	}
	var winner scored
	first := true
	for profile, score := range scores {
		if first || score > winner.score || (score == winner.score && profile < winner.profile) {
			winner = scored{profile, score}
			first = false
		}
	}

	if winner.score >= minInferenceScore {
		return winner.profile
	}
	return fallbackProfile(vaultDefault)
}

func fallbackProfile(vaultDefault string) string {
	if vaultDefault != "" && ValidProfileName(vaultDefault) {
		return vaultDefault
	}
	return "default"
}

func mustCompilePatterns(patterns []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		out[i] = regexp.MustCompile(p)
	}
	return out
}
