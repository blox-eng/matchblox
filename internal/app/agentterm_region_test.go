package app

import (
	"image/color"
	"io"
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/replay"
	"github.com/blox-eng/matchblox/internal/sample"
)

func TestTheRegionSaysWhyItIsEmpty(t *testing.T) {
	m := boardModel(t, boardState())
	want := "No sessions yet. + new session starts one here (N). matchblox setup finds the agents you already run."
	if got := strings.Join(plainLines(m.agentLines(140, 10)), " "); !strings.Contains(strings.Join(strings.Fields(got), " "), want) {
		t.Fatalf("with no sessions the region says:\n%s", got)
	}

	m = mockupFrame(t, 160)
	i := slices.IndexFunc(m.all.sessions, func(s sample.Session) bool { return s.Name == "hearth-api" })
	m.sideAt = paneOf(m.all.sessions[i])
	if line := lineWith(m.agentLines(119, 30), "is not reached"); line != "" {
		t.Fatalf("with gpu-1 reached the region says %q", ansi.Strip(line))
	}
	m = upd(t, m, peerMsg{host: "gpu-1", conn: m.peers[m.peerAt("gpu-1")].conn, msg: lostMsg{err: io.EOF, conn: m.peers[m.peerAt("gpu-1")].conn}})
	line := lineWith(m.agentLines(119, 30), "gpu-1 is not reached · it redials")
	if line == "" {
		t.Fatalf("with gpu-1 not reached the region says:\n%s", strings.Join(plainLines(m.agentLines(119, 30)), "\n"))
	}
	wantInks(t, line, [][2]string{{"gpu-1 is not reached · it redials", "warn"}})

}

func TestTheAgentColoursFollowTheTheme(t *testing.T) {
	agentRed := lipgloss.Color("#D7005F")
	for _, c := range []struct {
		name   string
		st     styles
		tokens map[string]color.Color
		fg, bg string
	}{
		{"light", newStyles(false), Tokens(false), "fg", ""},
		{"dark", newStyles(true), Tokens(true), "fg", ""},
		{"night", nightStyles(), Tokens(true), "night-fg", "night-bg"},
	} {
		term := &agentTerm{scr: newAgentScreen(40, 2)}
		_, _ = term.scr.Write([]byte("plain \x1b[38;2;215;0;95mred\x1b[0m"))
		r := agentRegion{term: term, w: 40, h: 2}
		r.term.framed = true
		c.tokens["agent-red"] = agentRed
		lines, err := replay.Parse(strings.Join(r.lines(c.st, 40, 2), "\n")+"\n", c.tokens)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		for _, s := range lines[0] {
			want := map[string]string{"plain ": c.fg, "red": "agent-red"}[s.Text]
			if want == "" {
				continue
			}
			if s.FG != want || s.BG != c.bg {
				t.Errorf("%s: %q draws on %q in %q, want %q on %q", c.name, s.Text, s.BG, s.FG, want, c.bg)
			}
		}
		if !strings.Contains(ansi.Strip(lines[0][0].Text), "plain") {
			t.Errorf("%s: the line is %v", c.name, lines[0])
		}
	}
}

// TestTheAgentScreenKeepsNoScrollback: matchblox never reads the screen's
// scrollback, so it keeps none (the agent's tmux keeps its own).
func TestTheAgentScreenKeepsNoScrollback(t *testing.T) {
	scr := newAgentScreen(120, 30)
	_, _ = scr.Write([]byte(strings.Repeat("x\r\n", 20000)))
	if n := scr.(*vtScreen).e.ScrollbackLen(); n > 1 {
		t.Fatalf("the screen keeps %d lines of scrollback", n)
	}
}

// TestTheAgentScreenIsToldItDrawsTrueColour: the view's own tmux client,
// wherever ViewArgv puts its flags, is told the screen draws true colour.
func TestTheAgentScreenIsToldItDrawsTrueColour(t *testing.T) {
	j := remote.Jump{Pane: "%3", Prefix: "C-b", ID: "0123abcd", Outer: true}
	run := clientArgv(remote.ViewArgv(j, "flint-auth", "/tmp/tmux-1000/default"))
	i := slices.Index(run, "tmux")
	if i < 0 || !slices.Equal(run[i+1:i+3], []string{"-T", "RGB"}) {
		t.Fatalf("the view's client is not told RGB: %q", run)
	}
	if slices.Contains(run, `\;`) || !slices.Contains(run, ";") {
		t.Fatalf("the view's commands are not split by ';': %q", run)
	}
}
