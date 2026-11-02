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

// settled is how long after the chat shows again at a new size that a tap
// goes through: the screen at that size arrives and starts the console's
// quiet once more.
const settled = 1500 * time.Millisecond

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
	for _, size := range [][2]int{{44, 31}, {132, 32}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Setenv("TMPDIR", t.TempDir())
			_, term := startAt(t, size[0], size[1])
			term.waitFor(t, "matchblox · demo")
		})
	}

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

// tap is a press and a release at row y (counted from 1) of the screen.
func (term *terminal) tap(t *testing.T, y int) {
	t.Helper()
	term.send(t, fmt.Sprintf("\x1b[<0;20;%dM\x1b[<0;20;%dm", y, y))
}

// transcript is what flint-auth's transcript holds in the one temp copy.
func transcript(t *testing.T) string {
	t.Helper()
	found, err := filepath.Glob(filepath.Join(tempCopies(t)[0], "home", ".claude", "projects", "-work-flint-auth", "*.jsonl"))
	if err != nil || len(found) != 1 {
		t.Fatalf("flint-auth's transcript: %v %v", found, err)
	}
	b, err := os.ReadFile(found[0])
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const (
	flintRow  = 6  // flint-auth's row on the first screen at 44x31
	optionRow = 18 // an option of its menu, once the chat shows
)

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

	d.Resize(106, 10)
	term.waitFor(t, "too short")

	term.reset()
	d.Resize(44, 31)
	term.waitFor(t, "Rotate it", "Keep the token")
	if text := term.text(); strings.Contains(text, "too short") {
		t.Errorf("a false line after turning back:\n%s", text)
	}
	time.Sleep(settled)
	if d.Done() {
		t.Fatal("a resize answered the question")
	}
	term.tap(t, optionRow)
	term.waitFor(t, "I rotate the token on each refresh")
	eventually(t, 6*time.Second, "Done", d.Done)
}

// TestTapsAloneEndInDone: one tap on the agent's row opens its question, one
// tap on an option answers it. No key is pressed.
func TestTapsAloneEndInDone(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	d, term := start(t)
	term.waitFor(t, "WAITS FOR YOU", "flint-auth")
	term.tap(t, flintRow)
	term.waitFor(t, "Rotate it", "Keep the token")
	time.Sleep(quiet)
	if d.Done() {
		t.Fatal("Done before an answer")
	}
	term.tap(t, optionRow)
	term.waitFor(t, "I rotate the token on each refresh")
	if got := transcript(t); !strings.Contains(got, "The user picked option 1") {
		t.Errorf("the tap did not pick option 1:\n%s", got)
	}
	eventually(t, 6*time.Second, "Done", d.Done)
}

// TestEnterAnswersWithTheHighlightedOption: Enter at the menu picks option 1.
func TestEnterAnswersWithTheHighlightedOption(t *testing.T) {
	d, term := openQuestion(t)
	term.send(t, "\r")
	term.waitFor(t, "I rotate the token on each refresh")
	if got := transcript(t); !strings.Contains(got, "The user picked option 1") {
		t.Errorf("Enter did not pick option 1:\n%s", got)
	}
	eventually(t, 6*time.Second, "Done", d.Done)
}

// TestKeysThatAreNoOptionDoNothing: a letter or a digit past the menu is not
// an answer.
func TestKeysThatAreNoOptionDoNothing(t *testing.T) {
	d, term := openQuestion(t)
	term.send(t, "x")
	time.Sleep(100 * time.Millisecond)
	term.send(t, "9")
	time.Sleep(4 * time.Second)
	if d.Done() || strings.Contains(transcript(t), "picked option") {
		t.Error("a key that is no option answered the question")
	}
}
