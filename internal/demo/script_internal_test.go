package demo

import (
	"os"
	"runtime"
	"testing"
	"time"
)

func scriptOn(t *testing.T, lasts time.Duration) (*script, *World) {
	t.Helper()
	old := turnLasts
	turnLasts = lasts
	t.Cleanup(func() { turnLasts = old })
	root, err := copyTree("../../testdata/demo")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	w, err := NewWorld(root, time.Now, func(int) {})
	if err != nil {
		t.Fatal(err)
	}
	return &script{world: w}, w
}

func TestOnlyTheAnswerToTheWaitingAgentSetsDone(t *testing.T) {
	s, _ := scriptOn(t, 20*time.Millisecond)

	s.answered("%1") // hearth-api's menu is not the question
	time.Sleep(150 * time.Millisecond)
	if s.done.Load() {
		t.Fatal("an answer to another agent set Done")
	}
	s.answered(askPane)
	s.answered(askPane)
	for deadline := time.Now().Add(time.Second); !s.done.Load(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the answer to the waiting agent never set Done")
		}
	}
	s.stop()
}

// TestAStopBeforeTheTurnEndsLeavesNothing: the timer goes with the demo, so
// nothing writes to the machine after Stop, and Done stays false.
func TestAStopBeforeTheTurnEndsLeavesNothing(t *testing.T) {
	s, w := scriptOn(t, 50*time.Millisecond)
	registry := w.registry(1310)
	before, err := os.ReadFile(registry)
	if err != nil {
		t.Fatal(err)
	}
	goroutines := runtime.NumGoroutine()

	s.answered(askPane)
	s.stop()
	time.Sleep(200 * time.Millisecond)

	after, err := os.ReadFile(registry)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("the turn ended after Stop: %s", after)
	}
	if s.done.Load() {
		t.Error("Done after a Stop that came before the turn ended")
	}
	if n := runtime.NumGoroutine(); n > goroutines {
		t.Errorf("goroutines: %d before, %d after", goroutines, n)
	}
}
