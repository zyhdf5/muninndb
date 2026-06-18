package replication

import "testing"

func TestTypesString(t *testing.T) {
	if RolePrimary.String() != "primary" {
		t.Fatal("role string")
	}
	if OpSet.String() != "set" {
		t.Fatal("op string")
	}
}
