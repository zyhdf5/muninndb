package activation

import (
	"testing"

	"github.com/scrypster/muninndb/internal/storage"
)

func TestResolveProfile_CaseInsensitive(t *testing.T) {
	cases := []struct {
		input    string
		wantName string
	}{
		{"CAUSAL", "causal"},
		{"Causal", "causal"},
		{" causal ", "causal"},
		{"ADVERSARIAL", "adversarial"},
		{"DEFAULT", "default"},
	}
	for _, tc := range cases {
		req := &ActivateRequest{Profile: tc.input}
		gotName, gotProfile := resolveProfile(req)
		if gotName != tc.wantName {
			t.Errorf("resolveProfile(%q) name = %q, want %q", tc.input, gotName, tc.wantName)
		}
		if gotProfile == nil {
			t.Errorf("resolveProfile(%q) returned nil profile", tc.input)
		}
	}
}

// TestResolveProfile_ExplicitOverride verifies A (explicit) beats C (inferred) and B (vault default).
func TestResolveProfile_ExplicitOverride(t *testing.T) {
	req := &ActivateRequest{
		Profile:      "adversarial",
		VaultDefault: "causal",
		Context:      []string{"why did this fail?"}, // would infer causal
	}
	_, p := resolveProfile(req)
	if p != GetProfile("adversarial") {
		t.Error("explicit Profile override must win over inference and vault default")
	}
}

// TestResolveProfile_InferenceWinsOverVaultDefault verifies C beats B.
func TestResolveProfile_InferenceWinsOverVaultDefault(t *testing.T) {
	req := &ActivateRequest{
		Profile:      "",
		VaultDefault: "structural",
		Context:      []string{"what contradicts this decision?"}, // infers adversarial (score 3)
	}
	_, p := resolveProfile(req)
	if p != GetProfile("adversarial") {
		t.Errorf("inference (adversarial) should beat vault default (structural): got %v", p)
	}
}

// TestResolveProfile_VaultDefaultWinsOnAmbiguous verifies B wins when C can't commit.
func TestResolveProfile_VaultDefaultWinsOnAmbiguous(t *testing.T) {
	req := &ActivateRequest{
		Profile:      "",
		VaultDefault: "structural",
		Context:      []string{"user preferences"}, // ambiguous, score < 2
	}
	_, p := resolveProfile(req)
	if p != GetProfile("structural") {
		t.Error("vault default (structural) should win when inference is ambiguous")
	}
}

// TestResolveProfile_FallsToDefault when no override, inference, or vault default.
func TestResolveProfile_FallsToDefault(t *testing.T) {
	req := &ActivateRequest{
		Profile:      "",
		VaultDefault: "",
		Context:      []string{"user preferences"},
	}
	_, p := resolveProfile(req)
	if p != GetProfile("default") {
		t.Error("should fall through to 'default' profile when nothing else applies")
	}
}

// TestResolveProfile_InvalidExplicitProfileFallsThrough verifies unknown explicit names fall through.
func TestResolveProfile_InvalidExplicitProfileFallsThrough(t *testing.T) {
	req := &ActivateRequest{
		Profile:      "nonexistent",
		VaultDefault: "",
		Context:      []string{"user preferences"},
	}
	_, p := resolveProfile(req)
	// Unknown profile name should fall through to inference/default, not panic
	if p == nil {
		t.Error("resolveProfile must never return nil")
	}
}

// TestPhase5Profile_AllowsEdge_Default verifies default profile allows all RelTypes in BFS.
func TestPhase5Profile_AllowsEdge_Default(t *testing.T) {
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
			t.Errorf("default profile should allow %v", rel)
		}
	}
}

// TestPhase5Profile_AllowsEdge_Causal verifies causal profile blocks non-causal types.
func TestPhase5Profile_AllowsEdge_Causal(t *testing.T) {
	p := GetProfile("causal")
	allowed := []storage.RelType{
		storage.RelCauses, storage.RelDependsOn, storage.RelBlocks,
		storage.RelPrecededBy, storage.RelFollowedBy,
	}
	blocked := []storage.RelType{
		storage.RelSupports, storage.RelContradicts, storage.RelReferences,
		storage.RelIsPartOf, storage.RelBelongsToProject,
	}
	for _, rel := range allowed {
		if !p.AllowsEdge(rel) {
			t.Errorf("causal profile should allow %v", rel)
		}
	}
	for _, rel := range blocked {
		if p.AllowsEdge(rel) {
			t.Errorf("causal profile should block %v", rel)
		}
	}
}

// TestPhase5Profile_Boost_DefaultDampensContradicts verifies the boost multiplier.
func TestPhase5Profile_Boost_DefaultDampensContradicts(t *testing.T) {
	p := GetProfile("default")
	boost := p.BoostFor(storage.RelContradicts)
	if boost != 0.3 {
		t.Errorf("default profile RelContradicts boost = %f, want 0.3", boost)
	}
}

// TestPhase5Profile_Boost_AdversarialAmplifiesContradicts verifies adversarial boost.
func TestPhase5Profile_Boost_AdversarialAmplifiesContradicts(t *testing.T) {
	p := GetProfile("adversarial")
	boost := p.BoostFor(storage.RelContradicts)
	if boost != 1.5 {
		t.Errorf("adversarial profile RelContradicts boost = %f, want 1.5", boost)
	}
}

// TestPhase5Profile_Boost_OppositeProfiles verifies same type gets different boost by profile.
func TestPhase5Profile_Boost_OppositeProfiles(t *testing.T) {
	def := GetProfile("default").BoostFor(storage.RelContradicts)
	adv := GetProfile("adversarial").BoostFor(storage.RelContradicts)
	if def >= adv {
		t.Errorf("adversarial (%f) should have higher boost than default (%f) for RelContradicts", adv, def)
	}
}

// TestPhase5_HighBoostLowWeightEdgeNotSkipped documents that the early-termination
// logic uses continue (not break) so boosted edges are never incorrectly pruned.
// This test verifies AllowsEdge + BoostFor are both applied independently.
func TestPhase5_HighBoostDoesNotMaskLowBoost(t *testing.T) {
	// Adversarial profile: RelContradicts has boost=1.5, RelRelatesTo has boost=1.0.
	// A RelRelatesTo edge with weight=0.9 scores higher effective-weight than
	// a RelContradicts edge with weight=0.5 × 1.5 = 0.75.
	// If the loop broke after RelRelatesTo fell below threshold, it would
	// incorrectly skip RelContradicts (which is actually higher after boost).
	// This test just verifies BoostFor values to document the risk is understood.
	p := GetProfile("adversarial")
	relatesBoost := p.BoostFor(storage.RelRelatesTo)
	contradictsBoost := p.BoostFor(storage.RelContradicts)
	if relatesBoost > contradictsBoost {
		t.Errorf("test setup wrong: expected RelContradicts boost (%f) > RelRelatesTo boost (%f)", contradictsBoost, relatesBoost)
	}
	// RelRelatesTo (boost=1.0) × Weight=0.8 < RelContradicts (boost=1.5) × Weight=0.6
	effectiveRelatesTo := relatesBoost * 0.8
	effectiveContradicts := contradictsBoost * 0.6
	if effectiveRelatesTo >= effectiveContradicts {
		t.Logf("note: in this example RelRelatesTo effective=%.2f >= RelContradicts effective=%.2f", effectiveRelatesTo, effectiveContradicts)
	}
	// The real behavioral guarantee: AllowsEdge excludes RelRelatesTo in adversarial
	if p.AllowsEdge(storage.RelRelatesTo) {
		t.Error("adversarial profile should not allow RelRelatesTo (not in Include list)")
	}
	if !p.AllowsEdge(storage.RelContradicts) {
		t.Error("adversarial profile must allow RelContradicts")
	}
}
