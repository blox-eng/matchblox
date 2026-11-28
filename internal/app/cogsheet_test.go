package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
)

// cogTmux stores @matchblox_cog and forwards every other call.
func cogTmux(base func(args ...string) (string, error), cog *string) func(args ...string) (string, error) {
	return func(args ...string) (string, error) {
		if len(args) > 0 && slices.Contains(args, sample.CogOption) {
			switch args[0] {
			case "set-option":
				if slices.Contains(args, "-wu") {
					*cog = ""
					return "", nil
				}
				*cog = args[len(args)-1]
				return "", nil
			case "show-option":
				return *cog, nil
			}
		}
		if base == nil {
			return "", nil
		}
		return base(args...)
	}
}

func plainBody(m Model) []string {
	lines := strings.Split(ansi.Strip(m.render()), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) <= 3 {
		return nil
	}
	out := make([]string, len(lines)-3)
	for i, l := range lines[3:] {
		out[i] = strings.TrimRight(l, " ")
	}
	return out
}

func machinePlain(m Model) []string {
	w := max(m.width, minWidth)
	lines := m.machinePanel(w).lines
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimRight(ansi.Strip(l), " ")
	}
	return out
}

func TestCommaOpensCogOverTheList(t *testing.T) {
	m, tm := keysDesk(t)
	before, err := tm.run("capture-pane", "-p", "-t", "%9")
	if err != nil {
		t.Fatal(err)
	}
	want := machinePlain(m)
	m = pressOn(m, ",")
	if m.cogSheet == nil {
		t.Fatal(", did not open ⚙ over the list")
	}
	got := plainBody(m)
	n := min(len(want), m.bodyRows())
	if n == 0 || len(got) < n {
		t.Fatalf("sheet shows %d lines, machine starts with %d", len(got), n)
	}
	for i := 0; i < n; i++ {
		if got[i] != want[i] {
			t.Fatalf("line %d\n got %q\n want %q\n(machine from dock.go)", i, got[i], want[i])
		}
	}
	after, _ := tm.run("capture-pane", "-p", "-t", "%9")
	if after != before || after != "agent ready\n" {
		t.Fatalf("agent capture changed %q -> %q", before, after)
	}
	if len(tm.sent) != 0 {
		t.Fatalf(", sent keys into a pane: %q", tm.sent)
	}
	if tm.mode != "MACHINE" || m.barMode != "MACHINE" {
		t.Fatalf("mode %q, option %q, want MACHINE", m.barMode, tm.mode)
	}
}

func TestEscReturnsToTheSameRow(t *testing.T) {
	m, _ := keysDesk(t)
	start := m.sideAt
	m = pressOn(m, "down")
	if m.sideAt == start || m.sideAt == (paneKey{}) {
		t.Fatalf("down left the row %v", m.sideAt)
	}
	row := m.sideAt
	m = pressOn(m, ",")
	if m.cogSheet == nil {
		t.Fatal(", did not open ⚙")
	}
	m = pressOn(m, "esc")
	if m.cogSheet != nil {
		t.Fatal("esc left ⚙ open")
	}
	if m.sideAt != row {
		t.Fatalf("after esc the row is %v, want %v", m.sideAt, row)
	}
}

func TestCtrlQClosesCog(t *testing.T) {
	m, tm := keysDesk(t)
	m = pressOn(m, ",")
	if m.cogSheet == nil || tm.mode != "MACHINE" {
		t.Fatalf("⚙ did not open: sheet %v mode %q", m.cogSheet != nil, tm.mode)
	}
	m = pressOn(m, "ctrl+q")
	if m.cogSheet != nil {
		t.Fatal("ctrl+q left ⚙ open")
	}
	if strings.Contains(ansi.Strip(m.render()), "USAGE") {
		t.Fatal("ctrl+q left Machine on the list")
	}
	if m.barMode != "LIST" || tm.mode != "LIST" {
		t.Fatalf("after close, mode %q option %q, want LIST", m.barMode, tm.mode)
	}
}

func TestHeaderCogOpensMachine(t *testing.T) {
	m, _ := keysDesk(t)
	byComma := pressOn(m, ",")
	if byComma.cogSheet == nil {
		t.Fatal(", did not open ⚙")
	}

	var cog string
	m, tm := keysDesk(t)
	m.opt.Tmux = cogTmux(tm.run, &cog)
	h := headerWithTmux(m)
	text := headerText(h)
	x := cellOf(text, "⚙")
	if x < 0 {
		t.Fatalf("no ⚙ on the header pane: %q", text)
	}
	_, cmd := h.Update(tea.MouseClickMsg{X: x, Y: 0, Button: tea.MouseLeft})
	drain(cmd)
	if cog != "open" {
		t.Fatalf("header wrote %q, want open", cog)
	}
	look := m.watchDeskMode()
	if look == nil {
		t.Fatal("the list does not read the header's tap")
	}
	next, cmd := m.Update(look())
	drain(cmd)
	m = next.(Model)
	if m.cogSheet == nil {
		t.Fatal("a tap on ⚙ did not open Machine")
	}
	if ansi.Strip(m.render()) != ansi.Strip(byComma.render()) {
		t.Fatal("header ⚙ did not open the same Machine as ,")
	}
	if tm.mode != "MACHINE" || m.barMode != "MACHINE" {
		t.Fatalf("mode %q option %q, want MACHINE", m.barMode, tm.mode)
	}
}

func TestBarViewTracksMouse(t *testing.T) {
	m := New(Options{NoMotion: true, Conn: newFake()})
	if got := newFrame("bar", m).View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("bar MouseMode %v, want cell motion", got)
	}
}

func TestBarChipOpensCogOnThatLoop(t *testing.T) {
	now := lostAt
	gpu := healthy()
	gpu.Orphans = []sample.Orphan{{PID: 4242, Start: 7, Comm: "bash", CPU: 99, HotFor: time.Hour, Parent: "init", Kill: []string{"kill", "4242"}}}
	m, _, conns := hostsAt(t, &now, "", 160, 30, "gpu-1", "ws-1")
	m = upd(t, m, stateMsg(stateAt(healthy(), now)))
	m = upd(t, m, peerMsg{host: "gpu-1", conn: conns["gpu-1"], msg: stateMsg(stateAt(gpu, now))})
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(stateAt(healthy(), now))})

	var cog, mode string
	m.deskFrame = true
	m.opt.Pane = "%1"
	m.opt.Tmux = cogTmux(modeTmux(&mode), &cog)
	m.toList()
	start := m.sideAt
	m = pressOn(m, "down")
	if m.sideAt == start || m.sideAt == (paneKey{}) {
		t.Fatalf("down left the row %v", m.sideAt)
	}
	row := m.sideAt

	bar := newFrame("bar", m)
	text := ansi.Strip(bar.View().Content)
	x := cellOf(text, "⚠ busy loop")
	if x < 0 {
		t.Fatalf("no busy loop chip: %q", text)
	}
	_, cmd := bar.Update(tea.MouseClickMsg{X: x, Y: 0, Button: tea.MouseLeft})
	drain(cmd)
	if cog != "loop:gpu-1" {
		t.Fatalf("bar wrote %q, want loop:gpu-1", cog)
	}
	look := m.watchDeskMode()
	if look == nil {
		t.Fatal("the list does not read the bar's tap")
	}
	next, cmd := m.Update(look())
	drain(cmd)
	m = next.(Model)
	if m.cogSheet == nil {
		t.Fatal("the busy loop chip did not open ⚙")
	}
	body := strings.Join(plainBody(m), "\n")
	if !strings.Contains(body, "4242") || !strings.Contains(body, "bash") {
		t.Fatalf("⚙ did not open on gpu-1's loop:\n%s", body)
	}
	m = pressOn(m, "esc")
	if m.cogSheet != nil || m.sideAt != row {
		t.Fatalf("esc: sheet open %v, row %v, want %v", m.cogSheet != nil, m.sideAt, row)
	}
}
