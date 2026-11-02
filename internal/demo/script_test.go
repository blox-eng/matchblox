package demo_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/demo"
)

// quiet is longer than the console's quiet after a menu comes to a chat.
const quiet = 700 * time.Millisecond

// reset forgets what was drawn, so the next waitFor sees only what comes after.
func (term *terminal) reset() {
	term.mu.Lock()
	defer term.mu.Unlock()
	term.drawn.Reset()
}

func (term *terminal) send(t *testing.T, s string) {
	t.Helper()
	if _, err := term.in.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
}

func eventually(t *testing.T, within time.Duration, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(within); !ok(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%s did not happen within %s", what, within)
		}
	}
}

func TestTheHostIsDemo(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	_, term := start(t)
	term.waitFor(t, "matchblox · demo")

	if b, err := os.ReadFile(filepath.Join(tree, "proc", "sys", "kernel", "hostname")); err != nil || strings.TrimSpace(string(b)) != "ws-1" {
		t.Errorf("the fixture's own name changed: %q %v", b, err)
	}
}

var oldAge = regexp.MustCompile(`stale|\b31d\b`)

func TestAgesAreRecent(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	_, term := start(t)
	term.waitFor(t, "SESSIONS 7", "hearth-api")
	time.Sleep(500 * time.Millisecond) // more samples
	if m := oldAge.FindString(term.text()); m != "" {
		t.Errorf("an age reads %q:\n%s", m, term.text())
	}
}

func TestTheAgentWaitsFirst(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	_, term := start(t)
	term.waitFor(t, "WAITS FOR YOU", "flint-auth")
	_, after, _ := strings.Cut(term.text(), "WAITS FOR YOU")
	if i, j := strings.Index(after, "flint-auth"), strings.Index(after, "kindle-"); i < 0 || j >= 0 && j < i {
		t.Errorf("flint-auth is not the first that waits:\n%s", after)
	}
	// The selected row is the one the chat opens on.
	term.reset()
	term.send(t, "a")
	term.waitFor(t, "Rotate it", "Keep the token")
}

func openQuestion(t *testing.T) (*demo.Demo, *terminal) {
	t.Helper()
	t.Setenv("TMPDIR", t.TempDir())
	d, term := start(t)
	term.waitFor(t, "WAITS FOR YOU", "flint-auth")
	term.send(t, "a")
	term.waitFor(t, "Rotate it")
	time.Sleep(quiet)
	return d, term
}

func TestAnAnswerEndsInDone(t *testing.T) {
	d, term := openQuestion(t)
	done := d.Done
	if done() {
		t.Fatal("Done before any answer")
	}
	term.send(t, "2")
	if done() {
		t.Fatal("Done at once: the turn has not ended")
	}
	term.waitFor(t, "I rotate the token on each refresh")
	eventually(t, 6*time.Second, "Done", done)
}

func TestTheDemoSurvivesATurn(t *testing.T) {
	d, term := openQuestion(t)
	done := d.Done

	d.Resize(106, 10)
	term.waitFor(t, "too short")

	term.reset()
	d.Resize(44, 31)
	term.waitFor(t, "flint-auth")
	time.Sleep(1200 * time.Millisecond) // the console holds keys for a second after it shut the chat itself
	term.send(t, "a")                   // it took the chat's keys back while the screen was too short
	term.waitFor(t, "Rotate it", "Keep the token")
	time.Sleep(quiet)
	if done() {
		t.Fatal("a resize answered the question")
	}
	term.send(t, "2")
	eventually(t, 6*time.Second, "Done", done)
}

// TestATapEndsInDone: a person taps an option, there is no digit key. The
// console's tap goes where a key goes, so the same answer ends the turn.
func TestATapEndsInDone(t *testing.T) {
	d, term := openQuestion(t)
	done := d.Done
	// An SGR press and release over the option rows, from the top: the first
	// row that is an option answers.
	for y := 3; y <= 24 && !done(); y++ {
		term.send(t, fmt.Sprintf("\x1b[<0;20;%dM\x1b[<0;20;%dm", y, y))
		time.Sleep(60 * time.Millisecond)
		if strings.Contains(term.text(), "I rotate the token on each refresh") {
			break
		}
	}
	term.waitFor(t, "I rotate the token on each refresh")
	eventually(t, 6*time.Second, "Done", done)
}
