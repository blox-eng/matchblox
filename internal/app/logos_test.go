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
		next, _ := opened.Update(graphicsMsg{can: true, key: "/dev/ttys002"})
		if !next.(Shell).cur.logos {
			t.Fatalf("host %q: the shell does not pass graphicsMsg to its console", host)
		}
	}
}
