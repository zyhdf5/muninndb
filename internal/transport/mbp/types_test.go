package mbp

import "testing"

func TestMBPActivateRequest_ProfileField(t *testing.T) {
	req := ActivateRequest{
		Vault:   "default",
		Context: []string{"why did this fail?"},
		Profile: "causal",
	}
	if req.Profile != "causal" {
		t.Errorf("expected Profile 'causal', got %q", req.Profile)
	}
}

func TestMBPActivateRequest_ProfileEmptyByDefault(t *testing.T) {
	req := ActivateRequest{
		Vault:   "default",
		Context: []string{"user preferences"},
	}
	if req.Profile != "" {
		t.Errorf("Profile should default to empty string, got %q", req.Profile)
	}
}
