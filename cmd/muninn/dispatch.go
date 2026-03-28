package main

import "strings"

// parseSubcommand converts os.Args[1:] into a canonical subcommand token.
// Returns "help" for no args (show help screen).
func parseSubcommand(args []string) string {
	if len(args) == 0 {
		return ""
	}
	first := strings.ToLower(args[0])
	// Only combine two-word commands when the second word is not a flag.
	if len(args) >= 2 && !strings.HasPrefix(args[1], "-") {
		second := strings.ToLower(args[1])
		return first + ":" + second
	}
	return first
}
