package replication

import "testing"

func TestSchemaVersion(t *testing.T) {
	db := newMemKVStore()
	if err := CheckAndSetSchemaVersion(db); err != nil {
		t.Fatal(err)
	}
}
