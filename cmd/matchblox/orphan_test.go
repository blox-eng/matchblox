package main

import (
	"testing"
	"time"
)

// TestTheWatchQuitsAfterTwoOrphanedChecks: one orphaned check is a terminal
// between two sessions; two in a row quit the console, once.
func TestTheWatchQuitsAfterTwoOrphanedChecks(t *testing.T) {
	shown, alone := "/dev/pts/2\tmatchblox-2\n", "/dev/pts/1\tmatchblox\n"
	listings := []string{shown, alone, shown, alone, alone, alone}
	checks, quits := 0, 0
	list := func() (string, error) {
		l := listings[min(checks, len(listings)-1)]
		checks++
		return l, nil
	}
	done := make(chan struct{})
	go func() {
		watchOrphan("matchblox-2", list, func() { quits++ }, time.Millisecond, done)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the watch did not end")
	}
	if checks != 5 || quits != 1 {
		t.Errorf("checks %d, quits %d: want the quit at the 5th check, once", checks, quits)
	}
}
