package replication

import (
	"testing"
	"time"
)

func TestJoinToken(t *testing.T) {
	m := NewJoinTokenManager("s", time.Minute)
	tok, err := m.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(tok); err != nil {
		t.Fatal(err)
	}
}
