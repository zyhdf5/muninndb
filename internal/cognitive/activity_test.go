package cognitive_test

import (
	"testing"
	"time"

	"github.com/scrypster/muninndb/internal/cognitive"
)

func TestActivityTracker_RecordAndQuery(t *testing.T) {
	at := cognitive.NewActivityTracker()

	vault1 := [8]byte{1}
	vault2 := [8]byte{2}

	at.Record(vault1)
	time.Sleep(10 * time.Millisecond)
	at.Record(vault2)

	since1 := at.IdleSince(vault1)
	since2 := at.IdleSince(vault2)

	if since1 < since2 {
		t.Fatalf("vault1 is older: idle since %v, vault2 %v", since1, since2)
	}
}

func TestActivityTracker_UnknownVault(t *testing.T) {
	at := cognitive.NewActivityTracker()
	unknown := [8]byte{99}
	idle := at.IdleSince(unknown)
	if idle < 24*time.Hour {
		t.Fatalf("unknown vault should appear very idle, got %v", idle)
	}
}

func TestActivityTracker_ActiveVaults(t *testing.T) {
	at := cognitive.NewActivityTracker()
	v1, v2 := [8]byte{1}, [8]byte{2}
	at.Record(v1)
	at.Record(v2)
	vaults := at.ActiveVaults(10 * time.Minute)
	if len(vaults) != 2 {
		t.Fatalf("expected 2 active vaults, got %d", len(vaults))
	}
}
