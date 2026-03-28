package trigger

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// TestSubscriptionRegistryAddAndGet — Add a subscription, Get returns it
// ---------------------------------------------------------------------------

func TestSubscriptionRegistryAddAndGet(t *testing.T) {
	reg := newRegistry()

	sub := newMinimalSub("get-test-1", 10, 0)
	reg.Add(sub)

	got, ok := reg.Get("get-test-1")
	if !ok {
		t.Fatal("Get returned false for a subscription that was just Added")
	}
	if got == nil {
		t.Fatal("Get returned nil subscription")
	}
	if got.ID != "get-test-1" {
		t.Errorf("Get returned subscription with ID %q, want %q", got.ID, "get-test-1")
	}
	if got.VaultID != 10 {
		t.Errorf("Get returned subscription with VaultID %d, want 10", got.VaultID)
	}
}

// ---------------------------------------------------------------------------
// TestSubscriptionRegistryRemove — Remove deletes the subscription
// ---------------------------------------------------------------------------

func TestSubscriptionRegistryRemove(t *testing.T) {
	reg := newRegistry()

	sub := newMinimalSub("remove-test-1", 20, 0)
	reg.Add(sub)

	// Confirm it exists first.
	if _, ok := reg.Get("remove-test-1"); !ok {
		t.Fatal("subscription not found before Remove — test setup error")
	}

	reg.Remove("remove-test-1")

	// After Remove, Get must return false.
	if _, ok := reg.Get("remove-test-1"); ok {
		t.Error("Get returned true after Remove — subscription was not deleted")
	}

	// ForVault must also return an empty slice.
	subs := reg.ForVault(20)
	for _, s := range subs {
		if s.ID == "remove-test-1" {
			t.Error("removed subscription still present in ForVault result")
		}
	}

	// Counts must be back to zero.
	if reg.CountForVault(20) != 0 {
		t.Errorf("CountForVault after Remove = %d, want 0", reg.CountForVault(20))
	}
	if reg.CountTotal() != 0 {
		t.Errorf("CountTotal after Remove = %d, want 0", reg.CountTotal())
	}
}

// ---------------------------------------------------------------------------
// TestSubscriptionRegistryForVault — ForVault returns only the matching vault's subs
// ---------------------------------------------------------------------------

func TestSubscriptionRegistryForVault(t *testing.T) {
	reg := newRegistry()

	// Add two subscriptions to vault 100 and one to vault 200.
	a := newMinimalSub("vault-a1", 100, 0)
	b := newMinimalSub("vault-a2", 100, 0)
	c := newMinimalSub("vault-b1", 200, 0)

	reg.Add(a)
	reg.Add(b)
	reg.Add(c)

	// ForVault(100) must return exactly the two vault-100 subs.
	subs100 := reg.ForVault(100)
	if len(subs100) != 2 {
		t.Fatalf("ForVault(100) returned %d subs, want 2", len(subs100))
	}
	ids100 := map[string]bool{}
	for _, s := range subs100 {
		ids100[s.ID] = true
	}
	if !ids100["vault-a1"] {
		t.Error("vault-a1 missing from ForVault(100)")
	}
	if !ids100["vault-a2"] {
		t.Error("vault-a2 missing from ForVault(100)")
	}
	if ids100["vault-b1"] {
		t.Error("vault-b1 from vault 200 incorrectly appeared in ForVault(100)")
	}

	// ForVault(200) must return exactly the one vault-200 sub.
	subs200 := reg.ForVault(200)
	if len(subs200) != 1 {
		t.Fatalf("ForVault(200) returned %d subs, want 1", len(subs200))
	}
	if subs200[0].ID != "vault-b1" {
		t.Errorf("ForVault(200) returned sub with ID %q, want 'vault-b1'", subs200[0].ID)
	}

	// ForVault on an unknown vault must return nil/empty.
	subs999 := reg.ForVault(999)
	if len(subs999) != 0 {
		t.Errorf("ForVault(999) returned %d subs for unknown vault, want 0", len(subs999))
	}
}

// ---------------------------------------------------------------------------
// TestSubscriptionRegistryPruneExpiredDetailed — expired subs are removed;
// non-expired and TTL=0 subs survive; counts are accurate after pruning.
// (trigger_test.go covers the basic case; this test adds count assertions
// and a third long-lived subscription to prove selective pruning.)
// ---------------------------------------------------------------------------

func TestSubscriptionRegistryPruneExpiredDetailed(t *testing.T) {
	reg := newRegistry()

	// 1ms TTL — expires almost immediately.
	shortLived := newMinimalSub("prune-short", 30, 1*time.Millisecond)
	// No TTL — should survive forever.
	immortal := newMinimalSub("prune-immortal", 30, 0)
	// Longer TTL — should still be alive when we prune.
	longLived := newMinimalSub("prune-long", 30, 10*time.Second)

	reg.Add(shortLived)
	reg.Add(immortal)
	reg.Add(longLived)

	// Wait for the short TTL to elapse.
	time.Sleep(10 * time.Millisecond)

	pruned := reg.PruneExpired()
	if pruned != 1 {
		t.Errorf("PruneExpired removed %d subscriptions, want 1", pruned)
	}

	// The short-lived sub must be gone.
	if _, ok := reg.Get("prune-short"); ok {
		t.Error("expired subscription 'prune-short' still present after PruneExpired")
	}

	// The immortal sub must still be present.
	if _, ok := reg.Get("prune-immortal"); !ok {
		t.Error("TTL=0 subscription 'prune-immortal' was incorrectly pruned")
	}

	// The long-lived sub must still be present.
	if _, ok := reg.Get("prune-long"); !ok {
		t.Error("long-lived subscription 'prune-long' was incorrectly pruned")
	}

	// Counts must reflect the two remaining subs.
	if reg.CountForVault(30) != 2 {
		t.Errorf("CountForVault(30) after prune = %d, want 2", reg.CountForVault(30))
	}
	if reg.CountTotal() != 2 {
		t.Errorf("CountTotal after prune = %d, want 2", reg.CountTotal())
	}
}
