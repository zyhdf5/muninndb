package erf

import (
	"testing"
	"time"
)

func makeTestID() [16]byte {
	var id [16]byte
	for i := range id {
		id[i] = byte(i + 1)
	}
	return id
}

func TestEncodeDecodeAssociation(t *testing.T) {
	targetID := makeTestID()
	now := time.Now().Truncate(time.Second)

	assoc := &Association{
		TargetID:      targetID,
		RelType:       0x0001, // RelSupports
		Weight:        0.75,
		Confidence:    0.85,
		CreatedAt:     now,
		LastActivated: int32(now.Unix()),
	}

	buf := make([]byte, AssocRecordSize)
	err := EncodeAssociation(buf, assoc)
	if err != nil {
		t.Fatalf("EncodeAssociation failed: %v", err)
	}

	if len(buf) != AssocRecordSize {
		t.Errorf("encoded association size wrong: %d != %d", len(buf), AssocRecordSize)
	}

	decoded, err := DecodeAssociation(buf)
	if err != nil {
		t.Fatalf("DecodeAssociation failed: %v", err)
	}

	if decoded.TargetID != targetID {
		t.Error("TargetID mismatch")
	}
	if decoded.RelType != 0x0001 {
		t.Error("RelType mismatch")
	}
	if decoded.Weight != assoc.Weight {
		t.Errorf("Weight mismatch: %f != %f", decoded.Weight, assoc.Weight)
	}
	if decoded.Confidence != assoc.Confidence {
		t.Errorf("Confidence mismatch: %f != %f", decoded.Confidence, assoc.Confidence)
	}
	if decoded.CreatedAt.UnixNano() != assoc.CreatedAt.UnixNano() {
		t.Error("CreatedAt mismatch")
	}
	if decoded.LastActivated != assoc.LastActivated {
		t.Errorf("LastActivated mismatch: %d != %d", decoded.LastActivated, assoc.LastActivated)
	}
}

func TestAssociationBufferSizeValidation(t *testing.T) {
	assoc := &Association{}

	buf := make([]byte, 39)
	err := EncodeAssociation(buf, assoc)
	if err == nil {
		t.Error("EncodeAssociation should reject buffer of size 39")
	}

	buf = make([]byte, AssocRecordSize)
	err = EncodeAssociation(buf, assoc)
	if err != nil {
		t.Errorf("EncodeAssociation failed with correct size: %v", err)
	}

	buf = make([]byte, 39)
	_, err = DecodeAssociation(buf)
	if err == nil {
		t.Error("DecodeAssociation should reject buffer of size 39")
	}
}

func TestAssociationExactSize(t *testing.T) {
	if AssocRecordSize != 40 {
		t.Errorf("AssocRecordSize should be 40, got %d", AssocRecordSize)
	}
}

func TestMultipleAssociations(t *testing.T) {
	id1 := makeTestID()
	id2 := [16]byte{16, 17, 18}
	id3 := [16]byte{32, 33, 34}

	assocs := []Association{
		{TargetID: id1, RelType: 0x0001, Weight: 0.9, Confidence: 0.95, LastActivated: 100},
		{TargetID: id2, RelType: 0x0002, Weight: 0.3, Confidence: 0.4, LastActivated: 200},
		{TargetID: id3, RelType: 0x0003, Weight: 0.6, Confidence: 0.7, LastActivated: 300},
	}

	encoded := make([][]byte, len(assocs))
	for i := range assocs {
		buf := make([]byte, AssocRecordSize)
		if err := EncodeAssociation(buf, &assocs[i]); err != nil {
			t.Fatalf("Encode assoc %d failed: %v", i, err)
		}
		encoded[i] = buf
	}

	for i, buf := range encoded {
		decoded, err := DecodeAssociation(buf)
		if err != nil {
			t.Fatalf("Decode assoc %d failed: %v", i, err)
		}
		if decoded.TargetID != assocs[i].TargetID {
			t.Errorf("Assoc %d TargetID mismatch", i)
		}
		if decoded.RelType != assocs[i].RelType {
			t.Errorf("Assoc %d RelType mismatch", i)
		}
		if decoded.Weight != assocs[i].Weight {
			t.Errorf("Assoc %d Weight mismatch", i)
		}
	}
}
