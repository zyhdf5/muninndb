package trigger

import (
	"testing"
)

func TestNewHNSWAdapterNotNil(t *testing.T) {
	// NewHNSWAdapter(nil) should still produce a non-nil interface value.
	// We don't call Search since that would panic on nil registry.
	adapter := NewHNSWAdapter(nil)
	if adapter == nil {
		t.Fatal("NewHNSWAdapter returned nil")
	}
}

func TestNewFTSAdapterNotNil(t *testing.T) {
	// NewFTSAdapter(nil) should still produce a non-nil interface value.
	adapter := NewFTSAdapter(nil)
	if adapter == nil {
		t.Fatal("NewFTSAdapter returned nil")
	}
}
