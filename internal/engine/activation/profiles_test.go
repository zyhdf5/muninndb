// internal/engine/activation/profiles_test.go
package activation

import (
	"testing"

	"github.com/scrypster/muninndb/internal/storage"
)

// --- Profile lookup ---

func TestGetProfile_KnownNames(t *testing.T) {
	for _, name := range []string{"default", "causal", "confirmatory", "adversarial", "structural"} {
		p := GetProfile(name)
		if p == nil {
			t.Errorf("GetProfile(%q) returned nil", name)
		}
	}
}

func TestGetProfile_UnknownFallsToDefault(t *testing.T) {
	p := GetProfile("nonexistent")
	if p == nil {
		t.Fatal("GetProfile(unknown) must not return nil")
	}
	def := GetProfile("default")
	if p != def {
		t.Error("GetProfile(unknown) should return the default profile pointer")
	}
}

// --- Include/Exclude semantics ---

func TestDefaultProfile_NoIncludes_NoExcludes(t *testing.T) {
	p := GetProfile("default")
	if len(p.Include) != 0 {
		t.Errorf("default profile should have empty Include, got %v", p.Include)
	}
	if len(p.Exclude) != 0 {
		t.Errorf("default profile should have empty Exclude, got %v", p.Exclude)
	}
}

func TestCausalProfile_IncludesCausalTypes(t *testing.T) {
	p := GetProfile("causal")
	expected := []storage.RelType{
		storage.RelCauses,
		storage.RelDependsOn,
		storage.RelBlocks,
		storage.RelPrecededBy,
		storage.RelFollowedBy,
	}
	for _, rel := range expected {
		if !p.Includes(rel) {
			t.Errorf("causal profile should include %v", rel)
		}
	}
}

func TestCausalProfile_ExcludesNonCausalTypes(t *testing.T) {
	p := GetProfile("causal")
	nonCausal := []storage.RelType{
		storage.RelSupports,
		storage.RelContradicts,
		storage.RelReferences,
		storage.RelBelongsToProject,
	}
	for _, rel := range nonCausal {
		if p.Includes(rel) {
			t.Errorf("causal profile should NOT include %v", rel)
		}
	}
}

func TestCausalProfile_NoExcludes(t *testing.T) {
	p := GetProfile("causal")
	if len(p.Exclude) != 0 {
		t.Errorf("causal profile should have no Exclude list, got %v", p.Exclude)
	}
}

func TestAdversarialProfile_IncludesConflictTypes(t *testing.T) {
	p := GetProfile("adversarial")
	if !p.Includes(storage.RelContradicts) {
		t.Error("adversarial profile must include RelContradicts")
	}
	if !p.Includes(storage.RelSupersedes) {
		t.Error("adversarial profile must include RelSupersedes")
	}
	if !p.Includes(storage.RelBlocks) {
		t.Error("adversarial profile must include RelBlocks")
	}
}

func TestConfirmatoryProfile_ExcludesContradicts(t *testing.T) {
	p := GetProfile("confirmatory")
	if !p.Excluded(storage.RelContradicts) {
		t.Error("confirmatory profile must exclude RelContradicts")
	}
}

func TestConfirmatoryProfile_IncludesSupports(t *testing.T) {
	p := GetProfile("confirmatory")
	if !p.Includes(storage.RelSupports) {
		t.Error("confirmatory profile must include RelSupports")
	}
}

func TestStructuralProfile_IncludesStructuralTypes(t *testing.T) {
	p := GetProfile("structural")
	for _, rel := range []storage.RelType{
		storage.RelIsPartOf,
		storage.RelBelongsToProject,
		storage.RelCreatedByPerson,
	} {
		if !p.Includes(rel) {
			t.Errorf("structural profile should include %v", rel)
		}
	}
}

// --- AllowsEdge ---

func TestAllowsEdge_DefaultAllowsAll(t *testing.T) {
	p := GetProfile("default")
	allTypes := []storage.RelType{
		storage.RelSupports, storage.RelContradicts, storage.RelDependsOn,
		storage.RelSupersedes, storage.RelRelatesTo, storage.RelIsPartOf,
		storage.RelCauses, storage.RelPrecededBy, storage.RelFollowedBy,
		storage.RelCreatedByPerson, storage.RelBelongsToProject, storage.RelReferences,
		storage.RelImplements, storage.RelBlocks, storage.RelResolves, storage.RelRefines,
	}
	for _, rel := range allTypes {
		if !p.AllowsEdge(rel) {
			t.Errorf("default profile should allow all edge types, blocked %v", rel)
		}
	}
}

func TestAllowsEdge_CausalBlocksNonCausal(t *testing.T) {
	p := GetProfile("causal")
	nonCausal := []storage.RelType{
		storage.RelSupports, storage.RelContradicts, storage.RelReferences,
		storage.RelIsPartOf, storage.RelBelongsToProject,
	}
	for _, rel := range nonCausal {
		if p.AllowsEdge(rel) {
			t.Errorf("causal profile should block %v", rel)
		}
	}
}

func TestAllowsEdge_ConfirmatoryBlocksContradicts(t *testing.T) {
	p := GetProfile("confirmatory")
	if p.AllowsEdge(storage.RelContradicts) {
		t.Error("confirmatory profile must not allow RelContradicts (excluded)")
	}
}

func TestAllowsEdge_ConfirmatoryAllowsSupports(t *testing.T) {
	p := GetProfile("confirmatory")
	if !p.AllowsEdge(storage.RelSupports) {
		t.Error("confirmatory profile must allow RelSupports")
	}
}

// --- Boost multipliers ---

func TestBoostFor_DefaultContradictsIsLow(t *testing.T) {
	p := GetProfile("default")
	boost := p.BoostFor(storage.RelContradicts)
	if boost >= 1.0 {
		t.Errorf("default profile RelContradicts boost should be < 1.0, got %f", boost)
	}
}

func TestBoostFor_DefaultUnknownTypeIsOne(t *testing.T) {
	p := GetProfile("default")
	boost := p.BoostFor(storage.RelRelatesTo)
	if boost != 1.0 {
		t.Errorf("default profile RelRelatesTo boost should be 1.0, got %f", boost)
	}
}

func TestBoostFor_AdversarialContradictsAboveOne(t *testing.T) {
	p := GetProfile("adversarial")
	boost := p.BoostFor(storage.RelContradicts)
	if boost <= 1.0 {
		t.Errorf("adversarial profile RelContradicts boost should be > 1.0, got %f", boost)
	}
}

func TestBoostFor_SameTypeDifferentProfilesDiffer(t *testing.T) {
	def := GetProfile("default").BoostFor(storage.RelContradicts)
	adv := GetProfile("adversarial").BoostFor(storage.RelContradicts)
	if def >= adv {
		t.Errorf("adversarial boost (%f) should exceed default boost (%f) for RelContradicts", adv, def)
	}
}

func TestBoostFor_NilBoostMapReturnsOne(t *testing.T) {
	p := GetProfile("structural")
	// structural has nil Boost map
	boost := p.BoostFor(storage.RelIsPartOf)
	if boost != 1.0 {
		t.Errorf("structural profile BoostFor should return 1.0 for all types, got %f", boost)
	}
}

// --- ValidProfileName ---

func TestValidProfileName_Known(t *testing.T) {
	for _, name := range []string{"default", "causal", "confirmatory", "adversarial", "structural"} {
		if !ValidProfileName(name) {
			t.Errorf("ValidProfileName(%q) should be true", name)
		}
	}
}

func TestValidProfileName_Unknown(t *testing.T) {
	if ValidProfileName("nonexistent") {
		t.Error("ValidProfileName(nonexistent) should be false")
	}
	if ValidProfileName("") {
		t.Error("ValidProfileName('') should be false")
	}
}

// --- InferProfile ---

func TestInferProfile_CausalQueries(t *testing.T) {
	cases := []string{
		"why did the deployment fail?",
		"what caused the outage?",
		"what led to this decision?",
		"root cause of the bug",
		"what is blocking the release?",
		"what depends on the auth service?",
		"what depends on this module?",
	}
	for _, q := range cases {
		got := InferProfile([]string{q}, "")
		if got != "causal" {
			t.Errorf("InferProfile(%q) = %q, want %q", q, got, "causal")
		}
	}
}

func TestInferProfile_AdversarialQueries(t *testing.T) {
	cases := []string{
		"what contradicts the user preference?",
		"find conflicts with this decision",
		"which memories are inconsistent?",
		"what disagrees with this claim?",
	}
	for _, q := range cases {
		got := InferProfile([]string{q}, "")
		if got != "adversarial" {
			t.Errorf("InferProfile(%q) = %q, want %q", q, got, "adversarial")
		}
	}
}

func TestInferProfile_ConfirmatoryQueries(t *testing.T) {
	cases := []string{
		"what validates this decision?",
		"find evidence for the hypothesis",
		"what confirms our approach?",
	}
	for _, q := range cases {
		got := InferProfile([]string{q}, "")
		if got != "confirmatory" {
			t.Errorf("InferProfile(%q) = %q, want %q", q, got, "confirmatory")
		}
	}
}

func TestInferProfile_StructuralQueries(t *testing.T) {
	cases := []string{
		"what is part of project alpha?",
		"what belongs to the auth module?",
	}
	for _, q := range cases {
		got := InferProfile([]string{q}, "")
		if got != "structural" {
			t.Errorf("InferProfile(%q) = %q, want %q", q, got, "structural")
		}
	}
}

func TestInferProfile_AmbiguousUsesVaultDefault(t *testing.T) {
	got := InferProfile([]string{"what does the user prefer?"}, "causal")
	if got != "causal" {
		t.Errorf("ambiguous query should fall through to vault default 'causal', got %q", got)
	}
}

func TestInferProfile_AmbiguousNoVaultDefaultUsesDefault(t *testing.T) {
	got := InferProfile([]string{"what does the user prefer?"}, "")
	if got != "default" {
		t.Errorf("ambiguous query with no vault default should return 'default', got %q", got)
	}
}

func TestInferProfile_EmptyContextUsesVaultDefault(t *testing.T) {
	got := InferProfile([]string{}, "structural")
	if got != "structural" {
		t.Errorf("empty context should use vault default 'structural', got %q", got)
	}
}

func TestInferProfile_EmptyContextNoVaultDefault(t *testing.T) {
	got := InferProfile([]string{}, "")
	if got != "default" {
		t.Errorf("empty context + no vault default should return 'default', got %q", got)
	}
}

func TestInferProfile_MultipleContextStrings(t *testing.T) {
	got := InferProfile([]string{"tell me about the system", "what caused the failure?"}, "")
	if got != "causal" {
		t.Errorf("multi-string context should infer causal, got %q", got)
	}
}

func TestInferProfile_InvalidVaultDefaultFallsToDefault(t *testing.T) {
	got := InferProfile([]string{"user preferences"}, "nonexistent")
	if got != "default" {
		t.Errorf("invalid vault default should fall through to 'default', got %q", got)
	}
}

func TestInferProfile_ReturnsValidProfileName(t *testing.T) {
	// Whatever InferProfile returns must always be a valid profile name
	queries := []string{
		"why does this work?",
		"here is why I chose Go",
		"user preferences",
		"what contradicts this?",
		"",
	}
	validProfiles := map[string]bool{
		"default": true, "causal": true, "confirmatory": true,
		"adversarial": true, "structural": true,
	}
	for _, q := range queries {
		got := InferProfile([]string{q}, "")
		if !validProfiles[got] {
			t.Errorf("InferProfile(%q) returned invalid profile name %q", q, got)
		}
	}
}

func TestInferProfile_TieBreaking_LexicographicWins(t *testing.T) {
	// "part of" scores structural=2; if another profile also hits 2, lexicographic wins.
	// In practice, only one profile tends to fire, but we test the tie-break is deterministic.
	// Call InferProfile twice with the same input — must return the same result every time.
	q := []string{"what is part of this project?"}
	first := InferProfile(q, "")
	for i := 0; i < 20; i++ {
		got := InferProfile(q, "")
		if got != first {
			t.Errorf("InferProfile is non-deterministic: got %q then %q", first, got)
		}
	}
	// Also verify it's a valid profile name
	if !ValidProfileName(first) {
		t.Errorf("InferProfile returned invalid profile %q", first)
	}
}

func TestInferProfile_WhyNotAtStart(t *testing.T) {
	got := InferProfile([]string{"some prefix text", "why did this happen?"}, "")
	if got != "causal" {
		t.Errorf("InferProfile with 'why' not at start should return 'causal', got %q", got)
	}
}

func TestAllowsEdge_UserDefinedRelType(t *testing.T) {
	const userDefined = storage.RelType(0x8001)

	// Default profile: empty Include — all types traverse including user-defined
	def := GetProfile("default")
	if !def.AllowsEdge(userDefined) {
		t.Error("default profile should allow user-defined RelType")
	}

	// Causal profile: explicit Include — user-defined type not in list, should be excluded
	causal := GetProfile("causal")
	if causal.AllowsEdge(userDefined) {
		t.Error("causal profile should exclude user-defined RelType")
	}
}

func TestAllowsEdge_ExcludeOverridesInclude(t *testing.T) {
	// Build a profile where a type is in both Include and Exclude.
	// Exclude must win — AllowsEdge must return false.
	p := newProfile(
		[]storage.RelType{storage.RelSupports, storage.RelContradicts},
		[]storage.RelType{storage.RelContradicts}, // contradicts is in both
		nil,
	)
	if p.AllowsEdge(storage.RelContradicts) {
		t.Error("Exclude must override Include: AllowsEdge should be false for RelContradicts")
	}
	if !p.AllowsEdge(storage.RelSupports) {
		t.Error("RelSupports should still be allowed (in Include, not in Exclude)")
	}
}
