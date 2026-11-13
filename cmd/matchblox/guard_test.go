package main

import (
	"strings"
	"testing"
)

// TestGuardNoCommand: guard with nothing to run says what it needs.
func TestGuardNoCommand(t *testing.T) {
	for _, args := range [][]string{{"guard"}, {"guard", "--"}} {
		err := run(args)
		if err == nil || !strings.Contains(err.Error(), "guard needs a command: matchblox guard -- <command>") {
			t.Fatalf("matchblox %s: %v, want the words that name the command line", strings.Join(args, " "), err)
		}
	}
}
