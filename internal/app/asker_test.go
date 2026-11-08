package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/blox-eng/matchblox/internal/keycmd"
)

const console = "%99"

func clients(lines ...string) func() (string, error) {
	return func() (string, error) { return strings.Join(lines, "\n"), nil }
}

// marked is the marker the tmux binding hands the console for the client
// on tty, as the app's next key arrives.
func marked(m tea.Model, tty string) tea.Model {
	next, _ := m.Update(uv.UnknownApcEvent(keycmd.MarkedPrefix + tty + "\x1b\\"))
	return next
}

func TestDeskIsTheLastActiveClientThatIsNoDevice(t *testing.T) {
	cs := clientsOf("1794089228 /dev/pts/7 %99\n1794089220 /dev/pts/2 %99\n1794089210 /dev/pts/1 %99\n1794089229 /dev/pts/3 %4", console)
	if got := deskOf(cs, map[string]string{"/dev/pts/7": "iPad-id"}); got != "/dev/pts/2" {
		t.Errorf("desk = %q, want /dev/pts/2 (pts/7 is the iPad, pts/3 is not on the console)", got)
	}
}

func TestADeviceAloneIsItsOwnDesk(t *testing.T) {
	cs := clientsOf("1794089228 /dev/pts/7 %99", console)
	if got := deskOf(cs, map[string]string{"/dev/pts/7": "iPad-id"}); got != "/dev/pts/7" {
		t.Errorf("desk = %q", got)
	}
}

func TestNoClientOnTheConsoleIsNoDesk(t *testing.T) {
	cs := clientsOf("1794089228 /dev/pts/3 %4\nnot a client\n12 pts/5 %99\nx /dev/pts/6 %99", console)
	if got := deskOf(cs, nil); got != "" {
		t.Errorf("desk = %q, want none", got)
	}
}

// A desk attached to another session of the console's window is still on
// the console: clients match by the pane they show.
func TestADeskInAnotherSessionIsStillTheDesk(t *testing.T) {
	cs := clientsOf("1794089228 /dev/pts/1 %99", console)
	if !cs[0].here {
		t.Error("the client on the console's pane is not here")
	}
}

func TestAMarkIsSpentByTheNextEvent(t *testing.T) {
	m := loaded(t, 100)
	next := marked(m, "/dev/pts/7")
	next, _ = next.Update(keyPress("down"))
	if got := next.(Model).askedBy; got != "/dev/pts/7" {
		t.Errorf("the marked event asked as %q", got)
	}
	next, _ = next.Update(keyPress("up"))
	if got := next.(Model).askedBy; got != "" {
		t.Errorf("the next event still asked as %q", got)
	}
}

func TestAMarkLapsesAfter300ms(t *testing.T) {
	t0 := time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)
	if got := (clientMark{"/dev/pts/7", t0}).spend(t0.Add(301 * time.Millisecond)); got != "" {
		t.Errorf("a lapsed mark gave %q", got)
	}
	if got := (clientMark{"/dev/pts/7", t0}).spend(t0.Add(300 * time.Millisecond)); got != "/dev/pts/7" {
		t.Errorf("a fresh mark gave %q", got)
	}
}

func TestAMarkOfNoDeviceIsDropped(t *testing.T) {
	m := loaded(t, 100)
	next := marked(m, "/dev/pts/7;x")
	if got := next.(Model).marked.tty; got != "" {
		t.Errorf("marked %q", got)
	}
}

// Only a whole mark of a tty marks: a cut, empty or foreign sequence leaves
// no mark.
func TestAMarkThatIsNotWholeIsDropped(t *testing.T) {
	m := loaded(t, 100)
	for _, apc := range []string{
		keycmd.MarkedPrefix + "/dev/pts/7",           // no ST
		keycmd.MarkedPrefix + "\x1b\\",               // no tty
		keycmd.MarkedPrefix + "/dev/pts/7\x07\x1b\\", // a control byte
		"\x1b_Gi=31;OK\x1b\\",                        // kitty's answer
	} {
		next, cmd := m.Update(uv.UnknownApcEvent(apc))
		if got := next.(Model).marked.tty; got != "" || cmd != nil {
			t.Errorf("%q marked %q, cmd %v", apc, got, cmd != nil)
		}
	}
}
