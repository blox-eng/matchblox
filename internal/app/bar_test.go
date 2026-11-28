package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/stoker"
)

func negPrefix(m Model) string {
	x := m.st.neg.Render("x")
	i := strings.Index(x, "x")
	if i < 0 {
		return x
	}
	return x[:i]
}

// modeTmux records @matchblox_mode and answers show-option with it.
func modeTmux(mode *string) func(args ...string) (string, error) {
	return func(args ...string) (string, error) {
		if len(args) == 0 {
			return "", nil
		}
		switch args[0] {
		case "set-option":
			for i, a := range args {
				if a == sample.ModeOption && i+1 < len(args) {
					*mode = args[i+1]
				}
			}
		case "show-option":
			if slices.Contains(args, sample.ModeOption) {
				return *mode, nil
			}
		}
		return "", nil
	}
}

func barFromOption(mode string) frameModel {
	held := mode
	f := newFrame("bar", New(Options{NoMotion: true, Conn: newFake(), Pane: "%1", Tmux: modeTmux(&held), Now: func() time.Time { return demoAt }}))
	f.width = 160
	next, _ := f.Update(frameTick{})
	return next.(frameModel)
}

func TestBarShowsBusyLoopOnThatHostOnly(t *testing.T) {
	now := lostAt
	gpu := healthy()
	gpu.Orphans = []sample.Orphan{{PID: 4242, Start: 7, Comm: "bash", CPU: 99, HotFor: time.Hour, Parent: "init", Kill: []string{"kill", "4242"}}}
	m, _, conns := hostsAt(t, &now, "", 160, 30, "gpu-1", "ws-1")
	m = upd(t, m, stateMsg(stateAt(healthy(), now)))
	m = upd(t, m, peerMsg{host: "gpu-1", conn: conns["gpu-1"], msg: stateMsg(stateAt(gpu, now))})
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(stateAt(healthy(), now))})
	m.deskFrame = true

	raw := newFrame("bar", m).View().Content
	plain := ansi.Strip(raw)
	if !strings.Contains(plain, "⚠ busy loop") || !strings.Contains(plain, "gpu-1") {
		t.Fatalf("bar %q, want ⚠ busy loop on gpu-1", plain)
	}
	neg := negPrefix(m)
	if neg == "" {
		t.Fatal("neg draws no colour")
	}
	for _, part := range strings.Split(raw, "● ") {
		text := ansi.Strip(part)
		switch {
		case strings.HasPrefix(text, "gpu-1"):
			if !strings.Contains(text, "⚠ busy loop") || !strings.Contains(part, neg) {
				t.Fatalf("gpu-1 chip %q", part)
			}
		case strings.Contains(part, neg):
			t.Fatalf("neg on a chip that is not gpu-1: %q", text)
		}
	}
	head := ansi.Strip(newFrame("header", m).View().Content)
	if !strings.Contains(head, "⚙ !") {
		t.Fatalf("⚙ does not show !: %q", head)
	}
}

func TestBarShowsThePlanAndTheStoker(t *testing.T) {
	now := lostAt
	st := healthy()
	st.Limits = []limits.Account{{
		Provider: "anthropic", Account: "a@b · work", State: limits.Measured, At: now,
		Windows: []limits.Window{
			{UsedPct: 62, Minutes: limits.BoxMinutes, ResetsAt: now.Add(2*time.Hour + 48*time.Minute)},
			{UsedPct: 40, Minutes: limits.WeekMinutes, ResetsAt: now.Add(4 * 24 * time.Hour)},
		},
	}}
	st.Stoker = &stoker.View{Mode: stoker.Mode{On: true}}
	m, _, _ := hostsAt(t, &now, "ws-1", 160, 30)
	m = upd(t, m, stateMsg(stateAt(st, now)))
	m.deskFrame = true

	got := ansi.Strip(newFrame("bar", m).View().Content)
	for _, want := range []string{"✻ work", "5h 62% ↻2h48m", "wk 40% ↻4d", "≋ stoker", "? keys"} {
		if !strings.Contains(got, want) {
			t.Errorf("bar %q, missing %q", got, want)
		}
	}
}

// deskModeTmux is a window whose current pane and @matchblox_mode the test moves.
func deskModeTmux(mode, current *string) func(args ...string) (string, error) {
	return func(args ...string) (string, error) {
		if len(args) == 0 {
			return "", nil
		}
		switch args[0] {
		case "set-option":
			for i, a := range args {
				if a == sample.ModeOption && i+1 < len(args) {
					*mode = args[i+1]
				}
			}
		case "show-option":
			switch {
			case slices.Contains(args, sample.ModeOption):
				return *mode, nil
			case slices.Contains(args, sample.ConsoleOption):
				return "%1", nil
			case slices.Contains(args, sample.HeaderOption):
				return "%2", nil
			case slices.Contains(args, sample.BarOption):
				return "%3", nil
			}
		case "list-panes":
			return *current, nil
		}
		return "", nil
	}
}

func TestBarWritesListWhenTheListPaneIsCurrent(t *testing.T) {
	mode, current := "AGENT", "%9"
	run := deskModeTmux(&mode, &current)
	f := newFrame("bar", New(Options{NoMotion: true, Conn: newFake(), Pane: "%1", Tmux: run, Now: func() time.Time { return demoAt }}))
	f.width = 160
	next, _ := f.Update(frameTick{})
	f = next.(frameModel)
	got := ansi.Strip(f.View().Content)
	if !strings.Contains(got, "AGENT") || !strings.Contains(got, "ctrl+q list") {
		t.Fatalf("bar %q, want AGENT and ctrl+q list", got)
	}

	current = "%1"
	m := New(Options{NoMotion: true, Conn: newFake(), Pane: "%1", Tmux: run, Now: func() time.Time { return demoAt }})
	m.deskFrame = true
	m.barMode = "LIST" // the list last published LIST; the agent pane then wrote AGENT
	cmd := m.watchDeskMode()
	if cmd == nil {
		t.Fatal("the list does not watch which pane is current")
	}
	msg := cmd()
	updated, follow := m.Update(msg)
	m = updated.(Model)
	drain(follow)
	if mode != "LIST" {
		t.Fatalf("mode %q, want LIST once the list pane is current", mode)
	}
	shown, _ := f.Update(frameTick{})
	got = ansi.Strip(shown.(frameModel).View().Content)
	if !strings.Contains(got, "LIST") || strings.Contains(got, "ctrl+q") {
		t.Fatalf("bar %q, want LIST and no ctrl+q list", got)
	}
}

func TestBarShowsCtrlQWhenTheAgentHasTheKeys(t *testing.T) {
	for _, mode := range []string{"AGENT", "SHELL"} {
		got := ansi.Strip(barFromOption(mode).View().Content)
		q, keys := strings.Index(got, "ctrl+q list"), strings.Index(got, "? keys")
		if q < 0 || keys < 0 || q > keys {
			t.Errorf("%s bar %q, want ctrl+q list before ? keys", mode, got)
		}
		if !strings.Contains(got, mode) {
			t.Errorf("%s bar %q, missing the mode word", mode, got)
		}
	}
	if got := ansi.Strip(barFromOption("LIST").View().Content); strings.Contains(got, "ctrl+q") {
		t.Fatalf("LIST bar shows ctrl+q: %q", got)
	}
}

func TestConfirmUsesTheBar(t *testing.T) {
	var mode string
	m := listShot(t, boardFixture(t), 160, 38, "flint-auth")
	m.deskFrame = true
	m.opt.Pane = "%1"
	m.opt.Tmux = modeTmux(&mode)
	m = pressOn(m, "x")
	if m.pending == nil {
		t.Fatalf("x left no confirm, notice %q", m.noticeText)
	}

	f := newFrame("bar", New(Options{NoMotion: true, Conn: newFake(), Pane: "%1", Tmux: modeTmux(&mode), Now: func() time.Time { return demoAt }}))
	f.width = 160
	next, _ := f.Update(frameTick{})
	f = next.(frameModel)
	got := ansi.Strip(f.View().Content)
	const want = "CONFIRM  kill flint-auth?  y kill · esc keep"
	if !strings.Contains(got, want) {
		t.Fatalf("bar %q, want %q", got, want)
	}
	held, _ := f.Update(keyPress("y"))
	if ansi.Strip(held.(frameModel).View().Content) != got {
		t.Fatal("y on the bar changed it; the list has the keys")
	}

	kept := pressOn(m, "esc")
	if kept.pending != nil || len(boardFake(kept).acts()) != 0 {
		t.Fatalf("esc did not keep flint-auth: pending %v acts %+v", kept.pending != nil, boardFake(kept).acts())
	}

	m = pressOn(m, "y")
	if len(killActs(m)) == 0 {
		m = pressOn(m, "y")
	}
	if acts := killActs(m); len(acts) != 1 {
		t.Fatalf("y on the list sent %+v", boardFake(m).acts())
	}
}

func killActs(m Model) []proto.Act {
	var out []proto.Act
	for _, a := range boardFake(m).acts() {
		if strings.HasPrefix(a.RecID, "kill:") {
			out = append(out, a)
		}
	}
	return out
}

func TestDeskHasNoKeyLine(t *testing.T) {
	m := boardShot(t, boardFixture(t), 160, 34, func(m *Model) { m.deskFrame = true })
	last := ansi.Strip(newFrame("bar", m).View().Content)
	if strings.Contains(last, "c v s") || !strings.Contains(last, "? keys") {
		t.Fatalf("the window's last row is %q, want the bar", last)
	}
	sc := m.draw(160)
	if sc.keysAt >= 0 {
		t.Fatalf("the list still draws a key line: %q", ansi.Strip(sc.lines[sc.keysAt]))
	}
	for _, l := range sc.lines {
		if strings.Contains(ansi.Strip(l), "c v s") {
			t.Fatalf("key line %q", ansi.Strip(l))
		}
	}
	side := boardShot(t, boardFixture(t), 160, 34, nil)
	if side.draw(160).keysAt < 0 {
		t.Fatal("the sidebar lost its key line")
	}

	m.notice("saved the list")
	if !m.noticeUntil.Equal(m.now().Add(noticeFor)) {
		t.Fatalf("notice stays %v, want %v", m.noticeUntil.Sub(m.now()), noticeFor)
	}
	if body := ansi.Strip(m.render()); !strings.Contains(body, "saved the list") {
		t.Fatalf("notice left the list:\n%s", body)
	}
	if strings.Contains(ansi.Strip(newFrame("bar", m).View().Content), "saved the list") {
		t.Fatal("the notice moved into the bar")
	}
}

func TestTabSilentOnDesk(t *testing.T) {
	m := boardShot(t, boardFixture(t), 160, 34, func(m *Model) { m.deskFrame = true })
	m.toList()
	next := pressOn(m, "tab")
	if next.tab != tabSessions {
		t.Fatalf("tab opened tab %d", next.tab)
	}
	if next.noticeText != "" || next.flash != "" {
		t.Fatalf("tab printed notice %q flash %q", next.noticeText, next.flash)
	}
	if strings.Contains(ansi.Strip(next.render()), "MACHINE") {
		t.Fatal("tab opened Machine")
	}

	side := boardShot(t, boardFixture(t), 80, 34, nil)
	side.toList()
	if got := pressOn(side, "tab"); got.tab != tabMachine {
		t.Fatalf("tab on the sidebar opened tab %d, want Machine", got.tab)
	}
}
