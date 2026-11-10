package main

import "testing"

// On 2026-11-10 a host ran at load 80 on 28 cores. The console ran at
// nice 19 and showed ↑ and ↓ half a second late over ssh. The console is
// the one program a person types into: it keeps the normal priority. The
// service and the one-shot commands stay at the lowest.
func TestOnlyTheConsoleKeepsItsPriority(t *testing.T) {
	if lowersPriority("console") {
		t.Error("the console lowers its priority: a key waits behind every busy process")
	}
	for _, cmd := range []string{"serve", "status", "sessions", "send", "scan", "new"} {
		if !lowersPriority(cmd) {
			t.Errorf("%s keeps the normal priority: it adds load", cmd)
		}
	}
}
