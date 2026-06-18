package replication

import "testing"

func TestValidateFencingToken(t *testing.T) {
	if err := ValidateFencingToken(2, 2); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFencingToken(2, 1); err == nil {
		t.Fatal("want error")
	}
}
