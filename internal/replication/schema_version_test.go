package replication

import (
	"strings"
	"testing"

	"github.com/cockroachdb/pebble"
)

func openSchemaTestDB(t *testing.T) *pebble.DB {
	t.Helper()
	db, err := pebble.Open(t.TempDir(), &pebble.Options{})
	if err != nil {
		t.Fatalf("pebble.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCheckAndSetSchemaVersion_FreshDB(t *testing.T) {
	db := openSchemaTestDB(t)
	if err := CheckAndSetSchemaVersion(db); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v, err := readSchemaVersion(db)
	if err != nil {
		t.Fatalf("readSchemaVersion: %v", err)
	}
	if v != CurrentSchemaVersion {
		t.Errorf("stored=%d, want %d", v, CurrentSchemaVersion)
	}
}

func TestCheckAndSetSchemaVersion_SameVersion(t *testing.T) {
	db := openSchemaTestDB(t)
	if err := writeSchemaVersion(db, CurrentSchemaVersion); err != nil {
		t.Fatalf("writeSchemaVersion: %v", err)
	}
	if err := CheckAndSetSchemaVersion(db); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckAndSetSchemaVersion_Downgrade_Blocked(t *testing.T) {
	db := openSchemaTestDB(t)
	if err := writeSchemaVersion(db, 999); err != nil {
		t.Fatalf("writeSchemaVersion: %v", err)
	}
	err := CheckAndSetSchemaVersion(db)
	if err == nil {
		t.Fatal("expected error for downgrade, got nil")
	}
	if !strings.Contains(err.Error(), "newer binary") {
		t.Errorf("error = %q, want it to contain 'newer binary'", err.Error())
	}
}
