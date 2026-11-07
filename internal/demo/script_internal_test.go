package demo

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/history"
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

// TestEachSessionsAnswerEndsItsOwnTurn: an answer typed to another session
// does not use up flint-auth's question. A pick on its menu afterwards still
// answers it, and each transcript closes with its own line.
func TestEachSessionsAnswerEndsItsOwnTurn(t *testing.T) {
	s, w := scriptOn(t, 20*time.Millisecond)
	w.Picked = s.answered
	other := "%3"

	for _, argv := range [][]string{
		{"tmux", "send-keys", "-t", other, "-l", "--", "use the second one"},
		{"tmux", "send-keys", "-t", other, "Enter"},
	} {
		if err := w.Run(argv); err != nil {
			t.Fatal(err)
		}
	}
	read := func(pane string) string {
		w.mu.Lock()
		defer w.mu.Unlock()
		sess := w.sessions[pane]
		b, _ := os.ReadFile(w.transcript(sess.name, sess.id))
		return string(b)
	}
	eventually := func(what string, ok func() bool) {
		t.Helper()
		for deadline := time.Now().Add(2 * time.Second); !ok(); time.Sleep(5 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("%s did not happen", what)
			}
		}
	}
	eventually("the first turn's end", func() bool { return strings.Contains(read(other), otherText) })
	eventually("Done", s.done.Load)
	if strings.Contains(read(askPane), askText) {
		t.Fatal("flint-auth finished on another session's answer")
	}

	if err := w.Run([]string{"tmux", "send-keys", "-t", askPane, "-l", "--", "2"}); err != nil {
		t.Fatal(err)
	}
	eventually("flint-auth's own close", func() bool { return strings.Contains(read(askPane), askText) })
	s.stop()
	if strings.Contains(read(askPane), otherText) || strings.Contains(read(other), askText) {
		t.Error("a session closed with another session's line")
	}
}

// TestATypedReplyToAnySessionIsAnAnswer: a reply typed to a session whose
// agent waits, or works, ends in Done as a pick does; a slash command, a
// rename and an empty Enter do not.
func TestATypedReplyToAnySessionIsAnAnswer(t *testing.T) {
	typed := func(t *testing.T, pane string, text string) bool {
		s, w := scriptOn(t, 10*time.Millisecond)
		w.Picked = s.answered
		argv := []string{"tmux", "send-keys", "-t", pane, "-l", "--", text}
		if text != "" {
			if err := w.Run(argv); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Run([]string{"tmux", "send-keys", "-t", pane, "Enter"}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(150 * time.Millisecond)
		s.stop()
		return s.done.Load()
	}
	for _, pane := range []string{"%1", "%3", "%5", "%8"} {
		if !typed(t, pane, "use the second one") {
			t.Errorf("a reply typed to %s did not answer", pane)
		}
	}
	for _, text := range []string{"/compact", "/rename other", ""} {
		if typed(t, "%3", text) {
			t.Errorf("%q answered", text)
		}
	}
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

func TestOnlyAnOptionOrEnterAnswersTheMenu(t *testing.T) {
	send := func(t *testing.T, w *World, keys ...string) {
		t.Helper()
		for _, k := range keys {
			argv := []string{"tmux", "send-keys", "-t", askPane, "-l", "--", k}
			if k == "Enter" {
				argv = []string{"tmux", "send-keys", "-t", askPane, "Enter"}
			}
			if err := w.Run(argv); err != nil {
				t.Fatal(err)
			}
		}
	}
	answers := func(keys ...string) bool {
		s, w := scriptOn(t, 10*time.Millisecond)
		w.Picked = s.answered
		send(t, w, keys...)
		time.Sleep(150 * time.Millisecond)
		s.stop()
		return s.done.Load()
	}
	for _, k := range []string{"x", "9", "0", "6", ""} {
		if answers(k) {
			t.Errorf("%q answered the question", k)
		}
	}
	for _, k := range []string{"1", "2", "5", "Enter"} {
		if !answers(k) {
			t.Errorf("%q did not answer the question", k)
		}
	}
}

// The seed alone spans the two hours the History tab names: a console
// that has one snapshot draws "last 2h", not "last 1h59m" (#310).
func TestTheSeededHistorySpansTwoHours(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.csv")
	if err := writeHistory(path); err != nil {
		t.Fatal(err)
	}
	h, err := history.Load(fixtureNow, path)
	if err != nil {
		t.Fatal(err)
	}
	pts := h.Points
	if len(pts) < 2 {
		t.Fatalf("%d points", len(pts))
	}
	if span := pts[len(pts)-1].At.Sub(pts[0].At); span != historySpan {
		t.Errorf("the seed spans %v, want %v", span, historySpan)
	}
	if last := pts[len(pts)-1].At; !last.Before(fixtureNow) {
		t.Errorf("the last seeded point is at %v, not before the demo's clock %v", last, fixtureNow)
	}
}
