package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/logos"
)

// TestUsageDrawsLogosWhereEveryViewerCan: while every terminal that shows
// the console draws images, each account in USAGE starts with its
// provider's logo, sent once per set of viewers and background (#183).
func TestUsageDrawsLogosWhereEveryViewerCan(t *testing.T) {
	m, _ := loadedWith(t, 132, limitsState())
	next, send := m.Update(graphicsMsg{can: true, key: "/dev/ttys002"})
	m = next.(Model)
	if send == nil {
		t.Fatal("the logos are not sent to a new viewer")
	}
	claude := ansi.Strip(logos.Cells("anthropic"))
	out := screen(m)
	if !strings.Contains(out, " "+claude+" Claude builder") || !strings.Contains(out, ansi.Strip(logos.Cells("openai"))+" OpenAI Plus") {
		t.Fatalf("USAGE has no logos:\n%s", out)
	}
	if _, again := m.Update(graphicsMsg{can: true, key: "/dev/ttys002"}); again != nil {
		t.Error("the same viewers get the logos again")
	}
	if _, more := m.Update(graphicsMsg{can: true, key: "/dev/ttys002,/dev/ttys004"}); more == nil {
		t.Error("a viewer that attached gets no logos")
	}
	// A phone attaches: marks again, and no placeholder it would draw as boxes.
	next, _ = m.Update(graphicsMsg{})
	if out := screen(next); strings.Contains(out, claude) || !strings.Contains(out, "✻ Claude builder") {
		t.Fatalf("a viewer that cannot draw still gets logos:\n%s", out)
	}
}

// TestTheShellsConsoleAsksWhoShowsIt: the console the shell opens, here or
// on a host, keeps the base's Graphics and draws the logos it says it can.
func TestTheShellsConsoleAsksWhoShowsIt(t *testing.T) {
	asked := 0
	sh := NewShell(ShellOptions{
		Base: Options{NoMotion: true, Graphics: func() (bool, string) { asked++; return true, "/dev/ttys002" }},
		Open: func(string) Options { return Options{Conn: newFake()} },
	})
	for _, host := range []string{"", "ws-1"} {
		opened, _ := sh.open(host, false)
		if opened.cur.opt.Graphics == nil {
			t.Fatalf("host %q: the console has no Graphics", host)
		}
		next, _ := opened.Update(graphicsMsg{gen: opened.cur.opt.Gen, can: true, key: "/dev/ttys002"})
		if !next.(Shell).cur.logos {
			t.Fatalf("host %q: the shell does not pass graphicsMsg to its console", host)
		}
	}
}

// TestGraphicsAsksOnItsOwnClockOneAtATime: the console asks who shows it
// on a timer of its own, so a viewer that cannot draw turns the logos off
// even while no state arrives; a wedged tmux holds one question, not one
// more every tick; a closed console's timer is dropped.
func TestGraphicsAsksOnItsOwnClockOneAtATime(t *testing.T) {
	m, _ := loadedWith(t, 132, limitsState())
	m.opt.Graphics = func() (bool, string) { return true, "k" }
	next, cmd := m.Update(graphicsTickMsg{gen: m.opt.Gen})
	if cmd == nil || !next.(Model).gfxBusy {
		t.Fatal("a tick asks nothing")
	}
	m = next.(Model)
	next, _ = m.Update(graphicsTickMsg{gen: m.opt.Gen})
	if !next.(Model).gfxBusy || next.(Model).gfxAsks != m.gfxAsks {
		t.Fatal("a tick asks again while the last question waits")
	}
	next, _ = next.Update(graphicsMsg{gen: m.opt.Gen, can: true, key: "k"})
	if next.(Model).gfxBusy {
		t.Fatal("the answer does not free the next question")
	}
	if _, cmd := next.Update(graphicsTickMsg{gen: m.opt.Gen + 1}); cmd != nil {
		t.Fatal("another console's tick runs here")
	}
	if m.opt.Graphics = nil; m.Init() == nil {
		t.Fatal("no Init")
	}
}
