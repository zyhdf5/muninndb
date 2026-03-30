package main

import (
	"os"
	"testing"
)

func TestPrintCompletion(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		// Redirect stdout to /dev/null to suppress output during test
		old := os.Stdout
		devnull, err := os.Open(os.DevNull)
		if err != nil {
			t.Fatalf("failed to open /dev/null: %v", err)
		}
		os.Stdout = devnull
		printCompletion(shell)
		os.Stdout = old
		devnull.Close()
	}
	// If we get here without panic, the test passes
}

func TestPrintCompletionUnknownShell(t *testing.T) {
	// Should not panic for unknown shell
	old := os.Stdout
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("failed to open /dev/null: %v", err)
	}
	os.Stdout = devnull
	printCompletion("powershell")
	os.Stdout = old
	devnull.Close()
}
