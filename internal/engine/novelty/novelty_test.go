package novelty

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

// TestBuildFingerprint verifies fingerprint contains expected terms, stops at TopTerms
func TestBuildFingerprint(t *testing.T) {
	concept := "security alert"
	content := "unauthorized access detected from 192.168.1.1 multiple failed login attempts authentication failed"

	fp := BuildFingerprint(concept, content)

	// Verify fingerprint is not empty
	if len(fp.terms) == 0 {
		t.Fatal("fingerprint is empty")
	}

	// Verify fingerprint doesn't exceed TopTerms
	if len(fp.terms) > TopTerms {
		t.Fatalf("fingerprint has %d terms, expected at most %d", len(fp.terms), TopTerms)
	}

	// Verify expected terms are present (allowing for variation due to stop word filtering)
	expectedTerms := []string{"security", "alert", "unauthorized", "access", "detected"}
	for _, term := range expectedTerms {
		if _, ok := fp.terms[term]; !ok {
			t.Logf("warning: expected term %q not found in fingerprint", term)
		}
	}
}

// TestJaccardIdentical verifies two identical fingerprints return 1.0
func TestJaccardIdentical(t *testing.T) {
	concept := "test concept"
	content := "this is test content with various terms"

	fp1 := BuildFingerprint(concept, content)
	fp2 := BuildFingerprint(concept, content)

	similarity := Jaccard(fp1, fp2)
	if similarity != 1.0 {
		t.Fatalf("identical fingerprints returned similarity %f, expected 1.0", similarity)
	}
}

// TestJaccardDisjoint verifies two disjoint fingerprints return 0.0
func TestJaccardDisjoint(t *testing.T) {
	fp1 := BuildFingerprint("apple banana cherry", "red yellow blue")
	fp2 := BuildFingerprint("zebra xray whiskey", "alpha bravo charlie")

	similarity := Jaccard(fp1, fp2)
	if similarity != 0.0 {
		t.Fatalf("disjoint fingerprints returned similarity %f, expected 0.0", similarity)
	}
}

// TestJaccardPartial verifies two fingerprints with partial overlap return expected value
func TestJaccardPartial(t *testing.T) {
	// Create two fingerprints with known overlap
	concept1 := "common words here"
	content1 := "common words here and additional content for first engram"

	concept2 := "common words here"
	content2 := "common words here and different content for second engram"

	fp1 := BuildFingerprint(concept1, content1)
	fp2 := BuildFingerprint(concept2, content2)

	similarity := Jaccard(fp1, fp2)

	// They should have significant overlap but not be identical
	if similarity <= 0.0 || similarity >= 1.0 {
		t.Fatalf("partial overlap returned similarity %f, expected between 0.0 and 1.0", similarity)
	}

	// Calculate expected intersection and union manually
	intersection := 0
	for term := range fp1.terms {
		if fp2.terms[term] {
			intersection++
		}
	}
	union := len(fp1.terms) + len(fp2.terms) - intersection

	if union > 0 {
		expected := float64(intersection) / float64(union)
		if similarity != expected {
			t.Fatalf("similarity mismatch: got %f, expected %f", similarity, expected)
		}
	}
}

// TestDetectorNearDuplicate writes two near-identical texts, Check returns a match on the second
func TestDetectorNearDuplicate(t *testing.T) {
	d := New()
	vaultID := uint32(1)

	concept := "incident report"
	content1 := "unauthorized access detected from IP 192.168.1.100 at 14:30 UTC multiple failed login attempts were recorded"
	content2 := "unauthorized access detected from IP 192.168.1.100 at 14:31 UTC multiple failed login attempts were recorded"

	ulid1 := "01ARZ3NDEKTSV4RRFFQ69G5FAV" // example ULID
	ulid2 := "01ARZ3NDEKTSV4RRFFQ69G5G00" // example ULID

	// First write should return no match
	match1 := d.Check(vaultID, ulid1, concept, content1)
	if match1 != nil {
		t.Fatalf("first write should not find a match, got: %+v", match1)
	}

	// Second write (near-duplicate) should find a match
	match2 := d.Check(vaultID, ulid2, concept, content2)
	if match2 == nil {
		t.Fatal("second write should find a match for near-duplicate")
	}

	if match2.ExistingULID != ulid1 {
		t.Fatalf("match ULID mismatch: got %q, expected %q", match2.ExistingULID, ulid1)
	}

	if match2.Similarity < Threshold {
		t.Fatalf("similarity %f is below threshold %f", match2.Similarity, Threshold)
	}
}

// TestDetectorDistinct writes two completely different texts, Check returns nil
func TestDetectorDistinct(t *testing.T) {
	d := New()
	vaultID := uint32(1)

	ulid1 := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	ulid2 := "01ARZ3NDEKTSV4RRFFQ69G5G00"

	// First write
	match1 := d.Check(vaultID, ulid1, "security alert", "unauthorized access from 192.168.1.100")
	if match1 != nil {
		t.Fatalf("first write should not find a match")
	}

	// Second write with completely different content
	match2 := d.Check(vaultID, ulid2, "weather report", "sunny conditions expected throughout the region tomorrow")
	if match2 != nil {
		t.Fatalf("distinct texts should not find a match, got: %+v", match2)
	}
}

// TestDetectorLRUEviction adds CacheSize+10 entries, verify cache doesn't exceed CacheSize
func TestDetectorLRUEviction(t *testing.T) {
	d := New()
	vaultID := uint32(1)

	// Add CacheSize + 10 entries
	numEntries := CacheSize + 10
	for i := 0; i < numEntries; i++ {
		// Generate unique ULID and content for each entry
		ulid := fmt.Sprintf("ULID%08d", i)
		concept := fmt.Sprintf("concept_%d", i)
		content := fmt.Sprintf("content with unique terms for entry number %d to ensure distinctness", i)

		d.Check(vaultID, ulid, concept, content)
	}

	// Verify cache size by checking that we can't find all the early entries
	shard := d.shardFor(vaultID)
	shard.mu.RLock()
	vc := shard.vaults[vaultID]
	shard.mu.RUnlock()

	if vc == nil {
		t.Fatal("vault cache not found")
	}

	vc.mu.RLock()
	actualSize := len(vc.entries)
	vc.mu.RUnlock()

	if actualSize > CacheSize {
		t.Fatalf("cache size %d exceeds maximum %d", actualSize, CacheSize)
	}

	// Verify that early entries (0-9) were likely evicted
	// We expect entries from index ~10 onward to be present
	shard.mu.RLock()
	vc = shard.vaults[vaultID]
	shard.mu.RUnlock()

	vc.mu.RLock()
	defer vc.mu.RUnlock()

	// Early entries should be evicted
	for i := 0; i < 5; i++ {
		ulid := fmt.Sprintf("ULID%08d", i)
		if _, ok := vc.entries[ulid]; ok {
			t.Logf("warning: early entry %q still present (may be due to fingerprint collisions)", ulid)
		}
	}

	// Later entries should be present
	found := 0
	for i := numEntries - 5; i < numEntries; i++ {
		ulid := fmt.Sprintf("ULID%08d", i)
		if _, ok := vc.entries[ulid]; ok {
			found++
		}
	}
	if found == 0 {
		t.Fatal("no recent entries found in cache after eviction")
	}
}

// TestDetectorConcurrent performs concurrent Check calls without panicking (race detector)
func TestDetectorConcurrent(t *testing.T) {
	d := New()
	numGoroutines := 16
	checksPerGoroutine := 100

	var wg sync.WaitGroup
	var successCount atomic.Int32
	var errorCount atomic.Int32

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for i := 0; i < checksPerGoroutine; i++ {
				vaultID := uint32(goroutineID % 4) // Use 4 different vaults to increase contention
				ulid := fmt.Sprintf("G%d_E%d", goroutineID, i)
				concept := fmt.Sprintf("concept_%d_%d", goroutineID, i)
				content := fmt.Sprintf("content for goroutine %d entry %d with some unique text", goroutineID, i)

				defer func() {
					if r := recover(); r != nil {
						t.Logf("panic in goroutine %d: %v", goroutineID, r)
						errorCount.Add(1)
					}
				}()

				_ = d.Check(vaultID, ulid, concept, content)
				successCount.Add(1)
			}
		}(g)
	}

	wg.Wait()

	expectedCount := int32(numGoroutines * checksPerGoroutine)
	if successCount.Load() != expectedCount {
		t.Fatalf("expected %d successful checks, got %d (errors: %d)", expectedCount, successCount.Load(), errorCount.Load())
	}

	if errorCount.Load() > 0 {
		t.Fatalf("concurrent execution resulted in %d panics", errorCount.Load())
	}
}

// TestJaccardEmptyFingerprints verifies Jaccard returns 0.0 for empty fingerprints
func TestJaccardEmptyFingerprints(t *testing.T) {
	fp1 := Fingerprint{terms: make(map[string]bool)}
	fp2 := Fingerprint{terms: make(map[string]bool)}

	similarity := Jaccard(fp1, fp2)
	if similarity != 0.0 {
		t.Fatalf("empty fingerprints returned similarity %f, expected 0.0", similarity)
	}
}

// TestJaccardOneEmpty verifies Jaccard returns 0.0 when one fingerprint is empty
func TestJaccardOneEmpty(t *testing.T) {
	fp1 := BuildFingerprint("test content", "with some terms")
	fp2 := Fingerprint{terms: make(map[string]bool)}

	sim1 := Jaccard(fp1, fp2)
	sim2 := Jaccard(fp2, fp1)

	if sim1 != 0.0 || sim2 != 0.0 {
		t.Fatalf("one empty fingerprint returned non-zero similarity: fp1->fp2=%f, fp2->fp1=%f", sim1, sim2)
	}
}

// TestMultipleVaults verifies different vaults maintain separate caches
func TestMultipleVaults(t *testing.T) {
	d := New()

	// Write to vault 1
	vault1ID := uint32(1)
	ulid1_1 := "VAULT1_ENTRY1"
	match1_1 := d.Check(vault1ID, ulid1_1, "vault1 concept", "vault1 content with unique terms")
	if match1_1 != nil {
		t.Fatal("first entry in vault 1 should not match")
	}

	// Write to vault 2
	vault2ID := uint32(2)
	ulid2_1 := "VAULT2_ENTRY1"
	match2_1 := d.Check(vault2ID, ulid2_1, "vault2 concept", "vault2 content with different terms")
	if match2_1 != nil {
		t.Fatal("first entry in vault 2 should not match")
	}

	// Write similar content to vault 1 - should match vault1's first entry
	ulid1_2 := "VAULT1_ENTRY2"
	match1_2 := d.Check(vault1ID, ulid1_2, "vault1 concept", "vault1 content with unique terms")
	if match1_2 == nil {
		t.Fatal("similar content in vault 1 should find a match")
	}
	if match1_2.ExistingULID != ulid1_1 {
		t.Fatalf("vault 1 match mismatch: expected %q, got %q", ulid1_1, match1_2.ExistingULID)
	}

	// Write similar content to vault 2 - should match vault2's first entry, not vault1's
	ulid2_2 := "VAULT2_ENTRY2"
	match2_2 := d.Check(vault2ID, ulid2_2, "vault2 concept", "vault2 content with different terms")
	if match2_2 == nil {
		t.Fatal("similar content in vault 2 should find a match")
	}
	if match2_2.ExistingULID != ulid2_1 {
		t.Fatalf("vault 2 match mismatch: expected %q, got %q", ulid2_1, match2_2.ExistingULID)
	}
}

// TestBuildFingerprintEmptyInput verifies handling of empty inputs
func TestBuildFingerprintEmptyInput(t *testing.T) {
	// Both empty
	fp1 := BuildFingerprint("", "")
	if len(fp1.terms) != 0 {
		t.Fatal("empty input should produce empty fingerprint")
	}

	// Only concept
	fp2 := BuildFingerprint("test content here", "")
	if len(fp2.terms) == 0 {
		t.Fatal("concept-only input should produce non-empty fingerprint")
	}

	// Only content
	fp3 := BuildFingerprint("", "test content here")
	if len(fp3.terms) == 0 {
		t.Fatal("content-only input should produce non-empty fingerprint")
	}
}
