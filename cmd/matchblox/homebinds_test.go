package main

import (
	"slices"
	"strings"
	"testing"
)

// TestHomeBindsTakeOnlyAPaneID: the console's own pane comes from the
// environment; a value that is not a tmux pane id never reaches a tmux
// command, and a pane id marks and sizes that pane's session.
func TestHomeBindsTakeOnlyAPaneID(t *testing.T) {
	has := func(binds [][]string, word string) bool {
		return slices.ContainsFunc(binds, func(a []string) bool { return strings.Contains(strings.Join(a, " "), word) })
	}
	if b := homeBinds("%1;kill-server"); has(b, "kill-server") || has(b, "%1") {
		t.Errorf("a bad pane reached tmux: %q", b)
	}
	if b := homeBinds("%7"); !has(b, "-t %7 @matchblox_home 1") || !has(b, "-t %7 window-size") {
		t.Errorf("the console's pane is not marked and sized: %q", b)
	}
}
