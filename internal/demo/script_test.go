package demo_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/demo"
)

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
	term.waitFor(t, "BOARD 7", "flint-auth")
	time.Sleep(500 * time.Millisecond) // more samples
	if m := oldAge.FindString(term.text()); m != "" {
		t.Errorf("an age reads %q:\n%s", m, term.text())
	}
}

func TestTheAgentWaitsFirst(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	_, term := start(t)
	term.waitFor(t, "NEEDS YOU", "flint-auth")
	_, after, _ := strings.Cut(term.text(), "NEEDS YOU")
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

const flintRow = 7 // flint-auth's card on the board's first screen at 44x31, under the doors line and the page marks

// openQuestion opens the stand-in for flint-auth's pane, which asks.
func openQuestion(t *testing.T) (*demo.Demo, *terminal) {
	t.Helper()
	t.Setenv("TMPDIR", t.TempDir())
	d, term := start(t)
	term.waitFor(t, "NEEDS YOU", "flint-auth")
	term.send(t, "\r")
	term.waitFor(t, "Rotate it", "type 2 to answer")
	return d, term
}

// TestTheStandInAnswersWithADigit: T8.
func TestTheStandInAnswersWithADigit(t *testing.T) {
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

// TestTheStandInAnswersWithArrowsThenEnter: T8.
func TestTheStandInAnswersWithArrowsThenEnter(t *testing.T) {
	d, term := openQuestion(t)
	term.send(t, "\x1b[B") // Down: the cursor moves to 2. Rotate, keep
	term.send(t, "\r")     // at once: the keys still reach the world in order (#342)
	term.waitFor(t, "I rotate the token on each refresh")
	if got := transcript(t); !strings.Contains(got, "The user picked option 2") {
		t.Errorf("Down then Enter did not pick option 2:\n%s", got)
	}
	eventually(t, 6*time.Second, "Done", d.Done)
}

// TestTheStandInGoesBackToSessions: T8, the app's home key.
func TestTheStandInGoesBackToSessions(t *testing.T) {
	d, term := openQuestion(t)
	term.reset()
	term.send(t, "\x1b_matchblox\x1b\\")
	term.waitFor(t, "NEEDS YOU")
	if d.Done() {
		t.Fatal("going back answered the question")
	}
}

// TestALineFeedGoesBackToTheFirstColumn: no terminal driver stands between the
// console and the app's terminal, whose line feed only goes down.
func TestALineFeedGoesBackToTheFirstColumn(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	_, term := startAt(t, 44, 31)
	term.waitFor(t, "flint-auth")
	raw := term.raw()
	if !strings.Contains(raw, "\n") {
		t.Fatal("the first frame has no line feed to check")
	}
	if strings.Contains(strings.ReplaceAll(raw, "\r\n", ""), "\n") {
		t.Errorf("a line feed with no carriage return:\n%q", raw)
	}
}

var cursorMove = regexp.MustCompile(`\x1b\[(\d+);(\d+)H`)

// TestAResizeAtOnceIsKept: the app starts the demo before its terminal has its
// size and resizes at once. The size it starts with must not arrive after that.
func TestAResizeAtOnceIsKept(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	d, term := startAt(t, 80, 25)
	d.Resize(50, 44)
	drawnAtTheNewSize := func() bool {
		raw := term.raw()
		rows, cols := 0, 0
		for _, m := range cursorMove.FindAllStringSubmatch(raw[max(strings.LastIndex(raw, "\x1b[2J"), 0):], -1) {
			row, _ := strconv.Atoi(m[1])
			col, _ := strconv.Atoi(m[2])
			rows, cols = max(rows, row), max(cols, col)
		}
		return rows > 25 && cols <= 50
	}
	eventually(t, 3*time.Second, "a frame of 50x44", drawnAtTheNewSize)
	time.Sleep(settled)
	if !drawnAtTheNewSize() {
		t.Error("the size the demo started with came back")
	}
}

func TestTheDemoSurvivesATurn(t *testing.T) {
	d, term := openQuestion(t)
	sentKeys := func(from int) [][]string {
		var keys [][]string
		for _, argv := range d.Ran()[from:] {
			if len(argv) > 1 && argv[1] == "send-keys" {
				keys = append(keys, argv)
			}
		}
		return keys
	}

	// Too short for the question: the stand-in closes, and the keys are
	// the console's, never a menu's the person cannot see.
	term.reset()
	d.Resize(106, 10)
	term.waitFor(t, "NEEDS YOU", "go in") // ten rows leave the board no room for a card
	from := len(d.Ran())
	term.send(t, "2")
	term.send(t, "1")
	time.Sleep(settled)
	if keys := sentKeys(from); len(keys) != 0 {
		t.Fatalf("keys reached the world while the question was not drawn: %q", keys)
	}

	term.reset()
	d.Resize(44, 31)
	term.waitFor(t, "NEEDS YOU", "flint-auth")
	time.Sleep(settled)
	if d.Done() {
		t.Fatal("a resize answered the question")
	}
	term.send(t, "\r") // a new Enter opens the stand-in again
	term.waitFor(t, "Rotate it", "type 2 to answer")
	term.send(t, "2")
	term.waitFor(t, "I rotate the token on each refresh")
	eventually(t, 6*time.Second, "Done", d.Done)
}

// TestTapsOpenTheStandIn: on a phone's one column a tap on the agent's card
// chooses it and a second goes into the stand-in, as Enter does, and the
// belt's 2 answers (#351).
func TestTapsOpenTheStandIn(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	d, term := start(t)
	term.waitFor(t, "NEEDS YOU", "flint-auth")
	term.tap(t, flintRow)
	term.tap(t, flintRow)
	term.waitFor(t, "type 2 to answer")
	if d.Done() {
		t.Fatal("Done before an answer")
	}
	term.send(t, "2")
	term.waitFor(t, "I rotate the token on each refresh")
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
