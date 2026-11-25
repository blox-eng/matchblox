package app

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/replay"
)

func TestTheHeaderCountsDone(t *testing.T) {
	ask, asks := boardMatch("flint-auth", time.Hour), boardMatch("ember-billing", time.Hour)
	waits := []queue.Item{waitFor(ask, queue.StateQuestion, time.Minute), waitFor(asks, queue.StatePermission, time.Minute)}
	st := boardState(ask, asks, doneMatch("kindle-web", time.Minute), doneMatch("ux-pass", 2*time.Minute), doneMatch("spark-rates", 3*time.Minute))
	st.Queue = waits
	line, hit, _ := boardModel(t, st).header(160)
	plain := ansi.Strip(line)
	at := strings.Index(plain, "● 2 need you · ✓ 3 done")
	if at < 0 {
		t.Fatalf("header %q, want ● 2 need you · ✓ 3 done", plain)
	}
	x := ansi.StringWidth(plain[:at])
	if hit.x0 != x || hit.x1 != x+len([]rune("● 2 need you")) {
		t.Errorf("a tap on who waits spans %d..%d, want %d..%d: only the waits", hit.x0, hit.x1, x, x+12)
	}
	cells := inkedCells(t, line)[0]
	for i, want := range map[int]string{x: "accent", x + 11: "accent", x + 13: "faint", x + 15: "match-ember", x + 19: "match-ember"} {
		if cells[i].fg != want {
			t.Errorf("cell %d %q is in %s, want %s", i, cells[i].ch, cells[i].fg, want)
		}
	}
	for _, w := range []int{44, 80} {
		line, _, _ := boardModel(t, st).header(w)
		if plain := ansi.Strip(line); !strings.Contains(plain, "● 2") || ansi.StringWidth(plain) > w {
			t.Errorf("at %d cells the header is %q (%d cells): want ● 2, and no wider", w, plain, ansi.StringWidth(plain))
		}
	}

	st = boardState(doneMatch("kindle-web", time.Minute), doneMatch("ux-pass", 2*time.Minute))
	line, _, _ = boardModel(t, st).header(160)
	if plain := ansi.Strip(line); !strings.Contains(plain, "✓ 2 done") || strings.Contains(plain, "need you") || strings.Contains(plain, "nothing") {
		t.Errorf("with nothing waiting and two done the header is %q: want ✓ 2 done alone", plain)
	}

	st = boardState(ask, asks, boardMatch("cinder-ci", time.Hour))
	st.Queue = waits
	line, _, _ = boardModel(t, st).header(160)
	if plain := ansi.Strip(line); !strings.Contains(plain, "● 2 need you") || strings.Contains(plain, "✓") {
		t.Fatalf("with nothing done the header is %q: want ● 2 need you and no ✓", plain)
	}
}

// phoneShots are the phone's screens TestThePhoneKeepsItsScreen holds: the
// list, a chat, a notice and the Machine tab, at 44×31.
func phoneShots(t *testing.T) map[string]Model {
	t.Helper()
	list, _, _ := sideShot(t, 44, 31)
	machine := list
	machine.setTab(tabMachine)
	home, gpu := mockupDesk()
	now := boardAt
	m, _, conns := hostsAt(t, &now, "ws-1", 44, 31, "gpu-1")
	m = upd(t, m, stateMsg(home))
	m = upd(t, m, peerMsg{host: "gpu-1", conn: conns["gpu-1"], msg: stateMsg(gpu)})
	m.notice("copied: 13 characters")
	return map[string]Model{"list": list, "chat": tapOn(t, list, listRow(t, list, "flint-auth")), "notice": m, "machine": machine}
}

// TestThePhoneKeepsItsScreen holds the phone's screens, ink and all, to
// the frames in testdata/phone, which the commit before the desk's frame
// drew. No flag writes them: the phone looks as it did (spec, "Non-goals").
func TestThePhoneKeepsItsScreen(t *testing.T) {
	for name, m := range phoneShots(t) {
		path := filepath.Join("testdata", "phone", name+".ans")
		got := m.render()
		want, err := os.ReadFile(path) //nolint:gosec // a golden
		if err != nil {
			t.Fatal(err)
		}
		if got != string(want) {
			t.Errorf("the phone's %s screen changed:\nnow:\n%s\nwas:\n%s", name, ansi.Strip(got), ansi.Strip(string(want)))
		}
	}
}

// mockDesk is the mockup's desk (mockupDesk) at w by h in the theme dark
// picks, its keys in the list.
func mockDesk(t *testing.T, w, h int, dark bool) Model {
	t.Helper()
	home, gpu := mockupDesk()
	return deskWith(t, w, h, dark, home, gpu)
}

// deskWith is mockDesk with home's and gpu-1's states.
func deskWith(t *testing.T, w, h int, dark bool, home, gpu proto.State) Model {
	t.Helper()
	now := boardAt
	m, _, conns := hostsAt(t, &now, "ws-1", w, h, "gpu-1")
	m = upd(t, m, stateMsg(home))
	m = upd(t, m, peerMsg{host: "gpu-1", conn: conns["gpu-1"], msg: stateMsg(gpu)})
	m.dark, m.st = dark, newStyles(dark)
	m.toList()
	return m
}

// withAgent is m on a desk that shows the chosen agent's screen, text,
// in the agent region, the keys with the agent when keys.
func withAgent(t *testing.T, m Model, text string, keys bool) Model {
	t.Helper()
	m.opt.Beside = true
	w := max(m.width, minWidth)
	box := m.sideBoxAt(w)
	scr := newAgentScreen(box.chatW, m.bodyRows())
	_, _ = scr.Write([]byte(text))
	t.Cleanup(func() { _ = scr.Close() })
	m.agent = agentRegion{term: &agentTerm{scr: scr, framed: true}, name: "flint-auth", keys: keys, x: box.chatX, y: 2, w: box.chatW, h: m.bodyRows()}
	return m
}

// themedCells are view's cells, each with its token in the theme dark
// picks; a wide glyph takes two.
func themedCells(t *testing.T, view string, dark bool) [][]inked {
	t.Helper()
	lines, err := replay.Parse(view, Tokens(dark))
	if err != nil {
		t.Fatal(err)
	}
	var out [][]inked
	for _, line := range lines {
		var row []inked
		for _, sp := range line {
			for _, r := range sp.Text {
				switch ansi.StringWidth(string(r)) {
				case 0:
					if len(row) > 0 {
						row[len(row)-1].ch += string(r)
					}
				case 2:
					row = append(row, inked{string(r), sp.FG, sp.BG}, inked{"", sp.FG, sp.BG})
				default:
					row = append(row, inked{string(r), sp.FG, sp.BG})
				}
			}
		}
		out = append(out, row)
	}
	return out
}

func TestTheDeskFrame(t *testing.T) {
	m := mockDesk(t, 160, 40, true)
	lines := plainLines(strings.Split(m.render(), "\n"))
	if len(lines) != 40 {
		t.Fatalf("the desk has %d lines, want 40:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	head, _ := m.frameHead(160)
	bar, _ := m.frameBar("LIST", 160)
	want := map[int]string{
		0:  ansi.Strip(head),
		1:  strings.Repeat("─", 40) + "┬" + strings.Repeat("─", 119),
		38: strings.Repeat("─", 40) + "┴" + strings.Repeat("─", 119),
		39: ansi.Strip(bar),
	}
	for y, l := range want {
		if lines[y] != l {
			t.Errorf("line %d reads\n%q, want\n%q", y, lines[y], l)
		}
	}
	for y := 2; y < 38; y++ {
		if edge := []rune(lines[y])[40]; edge != '│' && edge != '┤' {
			t.Errorf("line %d has %q at the edge: %q", y, edge, lines[y])
		}
	}
	if list := strings.Join(lines[2:38], "\n"); !strings.Contains(list, "flint-auth") || !strings.Contains(list, "NEEDS YOU") {
		t.Errorf("the body shows no list:\n%s", list)
	}
	for _, gone := range []string{"SESSIONS", "machine", "tab Machine", "esc list"} {
		if i := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, gone) }); i >= 0 {
			t.Errorf("line %d says %q: no tab row and no key line on the desk: %q", i, gone, lines[i])
		}
	}
	// The layout sets the region's place: beside the list, under the rule.
	if m = withAgent(t, m, "", false); m.agent.x != 41 || m.agent.y != 2 || m.agent.w != 119 || m.agent.h != 36 {
		t.Errorf("the region is %d×%d at %d,%d, want 119×36 at 41,2", m.agent.w, m.agent.h, m.agent.x, m.agent.y)
	}
	m = upd(t, m, tea.WindowSizeMsg{Width: 104, Height: 38})
	if m.agent.x != 33 || m.agent.y != 2 || m.agent.w != 71 || m.agent.h != 34 {
		t.Errorf("at 104×38 the region is %d×%d at %d,%d, want 71×34 at 33,2", m.agent.w, m.agent.h, m.agent.x, m.agent.y)
	}
}

// TestTheDeskFrameTicksOnlyForThePulse: the bar's Pulse moves, so a desk
// that shows one wakes to redraw it; with every value read nothing ticks.
func TestTheDeskFrameTicksOnlyForThePulse(t *testing.T) {
	calm := func(limits bool) Model {
		home, gpu := mockupDesk()
		home.Queue = nil // nothing asks: no match breathes
		if !limits {
			home.Limits[0].Windows = nil // the plan is not read yet: its value is the Pulse
		}
		m := deskWith(t, 160, 40, true, home, gpu)
		m.opt.NoMotion, m.splashDone = false, true
		return m
	}
	if m := calm(true); m.moving() {
		t.Fatalf("a desk with every value read moves: %q", plainLines(strings.Split(m.render(), "\n"))[39])
	}
	m := calm(false)
	if bar := plainLines(strings.Split(m.render(), "\n"))[39]; !strings.Contains(bar, "work …") {
		t.Fatalf("the bar shows no Pulse: %q", bar)
	}
	if !m.moving() {
		t.Error("the bar shows the Pulse, and nothing redraws it")
	}
}

func TestTheHalfEdge(t *testing.T) {
	for _, keys := range []string{"LIST", "AGENT"} {
		m := withAgent(t, mockDesk(t, 160, 40, true), "agent screen", keys == "AGENT")
		cells := themedCells(t, m.render(), true)
		if mode := strings.Fields(textOf(cells[39], 0, 10))[0]; mode != keys {
			t.Fatalf("the bar names %s, the keys are %s's", mode, keys)
		}
		for y := 2; y < 38; y++ {
			lit := y < 20 == (keys == "LIST")
			want := map[bool]string{true: "accent", false: "hair"}[lit]
			if c := cells[y][40]; c.fg != want {
				t.Errorf("with the %s's keys the edge at line %d (%q) is %s, want %s", keys, y, c.ch, c.fg, want)
			}
		}
	}
	// A click on the list gives it the keys back, and the edge says so.
	m := withAgent(t, mockDesk(t, 160, 40, true), "agent screen", true)
	m = mouse(t, m, tea.MouseClickMsg{X: 5, Y: 30, Button: tea.MouseLeft})
	m = mouse(t, m, tea.MouseReleaseMsg{X: 5, Y: 30, Button: tea.MouseLeft})
	if m.agent.mode() != "LIST" {
		t.Fatalf("a click on the list left the keys with %s", m.agent.mode())
	}
	if c := themedCells(t, m.render(), true)[2][40]; c.fg != "accent" {
		t.Errorf("after the click the edge's top half is %s", c.fg)
	}
}

func TestLinesJoin(t *testing.T) {
	for _, keys := range []string{"LIST", "AGENT"} {
		m := withAgent(t, mockDesk(t, 160, 40, true), "", keys == "AGENT")
		cells := themedCells(t, m.render(), true)
		top, bottom := cells[1][40], cells[38][40]
		if top.ch != "┬" || bottom.ch != "┴" {
			t.Fatalf("the rules meet the edge with %q and %q, want ┬ and ┴", top.ch, bottom.ch)
		}
		if want := map[bool]string{true: "accent", false: "hair"}[keys == "LIST"]; top.fg != want {
			t.Errorf("with the %s's keys ┬ is %s, want %s", keys, top.fg, want)
		}
		if want := map[bool]string{true: "accent", false: "hair"}[keys == "AGENT"]; bottom.fg != want {
			t.Errorf("with the %s's keys ┴ is %s, want %s", keys, bottom.fg, want)
		}
		joins := 0
		for y := 2; y < 38; y++ {
			if line := cells[y][39].ch == "─"; line != (cells[y][40].ch == "┤") {
				t.Errorf("line %d: the list's %q meets the edge with %q", y, cells[y][39].ch, cells[y][40].ch)
			} else if line {
				joins++
			}
		}
		if joins < 2 {
			t.Errorf("the list's group lines meet the edge %d times, want each of them", joins)
		}
	}
}

func TestASheetFloats(t *testing.T) {
	var screen strings.Builder
	for y := range 36 {
		fmt.Fprintf(&screen, "\x1b[%d;1H%02d agent row %02d", y+1, y, y)
	}
	m := withAgent(t, mockDesk(t, 160, 40, true), screen.String(), false)
	under := themedCells(t, m.render(), true)
	m = pressOn(m, "?")
	cells := themedCells(t, m.render(), true)
	const x0, x1, y0, y1 = 45, 155, 3, 36 // the region less 4 columns and a line on each side
	for _, c := range []struct {
		x, y int
		want string
	}{{x0, y0, "╭"}, {x1, y0, "╮"}, {x0, y1, "╰"}, {x1, y1, "╯"}, {x0, y0 + 1, "│"}, {x1, y1 - 1, "│"}, {x0 + 1, y0, "─"}} {
		if got := cells[c.y][c.x]; got.ch != c.want || got.fg != "faint" {
			t.Errorf("the sheet's edge at %d,%d is %q in %s, want %q in faint", c.x, c.y, got.ch, got.fg, c.want)
		}
	}
	if mode := strings.Fields(textOf(cells[39], 0, 10))[0]; mode != "KEYS" {
		t.Errorf("with the sheet open the bar names %s, want KEYS", mode)
	}
	if title := textOf(cells[y0], x0, x0+10); title != "╭─ ? keys " {
		t.Errorf("the sheet's top edge reads %q, want its title in it", title)
	}
	if c := cells[y0][x0+3]; c.fg != "fg" {
		t.Errorf("the title is %s, want fg", c.fg)
	}
	for y := y0 + 1; y < y1; y++ {
		if c := cells[y][x0+1]; c.ch != " " {
			t.Errorf("line %d has %q in the sheet's padding", y, c.ch)
		}
	}
	for y := 2; y < 38; y++ {
		for _, x := range []int{41, 42, 43, 44, 156, 157, 158, 159} {
			if y > y0 && y < y1 && x > x0 && x < x1 {
				continue
			}
			if cells[y][x] != under[y][x] {
				t.Errorf("around the sheet, %d,%d shows %+v, want the agent's %+v", x, y, cells[y][x], under[y][x])
			}
		}
	}
	if row := textOf(cells[2], 41, 60); !strings.HasPrefix(row, "00 agent row 00") {
		t.Errorf("over the sheet the agent's first line reads %q", row)
	}
	if m = pressOn(m, "esc"); strings.Contains(plainFrame(m), "╭─ ? keys") {
		t.Error("esc leaves the sheet open")
	}
}

func TestANoticeSitsAtTheListFoot(t *testing.T) {
	m := mockDesk(t, 160, 40, true)
	before := plainLines(strings.Split(m.render(), "\n"))
	m.notice("copied: 13 characters")
	cells := themedCells(t, m.render(), true)
	foot := textOf(cells[37], 0, 40)
	if !strings.HasPrefix(foot, " copied: 13 characters") {
		t.Fatalf("the list's last line reads %q, want the notice", foot)
	}
	if c := cells[37][1]; c.fg != "muted" {
		t.Errorf("the notice is in %s, want muted", c.fg)
	}
	after := plainLines(strings.Split(m.render(), "\n"))
	for y := range after {
		if y != 37 && after[y] != before[y] {
			t.Errorf("the notice changed line %d:\n%q\nwas\n%q", y, after[y], before[y])
		}
	}
}

// TestTheFrameInBothThemes draws the desk, a sheet open over the agent, in
// light and in dark: every part of the frame wears its token in both.
func TestTheFrameInBothThemes(t *testing.T) {
	for _, dark := range []bool{true, false} {
		m := pressOn(withAgent(t, mockDesk(t, 160, 40, dark), "agent", false), "?")
		cells := themedCells(t, m.render(), dark)
		for _, c := range []struct {
			x, y   int
			ch, fg string
		}{
			{1, 0, "▰", "accent"}, {3, 0, "m", "fg"}, {0, 1, "─", "hair"}, {40, 1, "┬", "accent"},
			{40, 2, "│", "accent"}, {40, 37, "│", "hair"}, {0, 38, "─", "hair"}, {40, 38, "┴", "hair"},
			{1, 39, "K", "accent"}, {45, 3, "╭", "faint"}, {48, 3, "?", "fg"},
		} {
			if got := cells[c.y][c.x]; got.ch != c.ch || got.fg != c.fg {
				t.Errorf("dark %v: %d,%d is %q in %q, want %q in %s", dark, c.x, c.y, got.ch, got.fg, c.ch, c.fg)
			}
		}
		head := textOf(cells[0], 0, 160)
		x := strings.Index(head, "✓ 3 done")
		if x < 0 {
			t.Fatalf("dark %v: the head row reads %q", dark, head)
		}
		if c := cells[0][ansi.StringWidth(head[:x])]; c.fg != "match-ember" {
			t.Errorf("dark %v: ✓ is %s, want match-ember", dark, c.fg)
		}
	}
}

// TestTheFrameMarksStandOut: the frame's marks, accent, faint and
// match-ember, stand 3:1 or more against bg in both themes, as DESIGN.md
// §2.1 gives them.
func TestTheFrameMarksStandOut(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "DESIGN.md"))
	if err != nil {
		t.Fatal(err)
	}
	row := regexp.MustCompile("(?m)^\\| `([a-z-]+)` \\| `(#[0-9A-F]{6})` \\| `(#[0-9A-F]{6})` \\|")
	tokens := map[string][2]string{}
	for _, r := range row.FindAllStringSubmatch(string(doc), -1) {
		tokens[r[1]] = [2]string{r[2], r[3]}
	}
	for i, theme := range []string{"dark", "light"} {
		for _, mark := range []string{"accent", "faint", "match-ember"} {
			fg, bg := tokens[mark][i], tokens["bg"][i]
			if fg == "" || bg == "" {
				t.Fatalf("DESIGN.md §2.1 gives no %s %s or bg", theme, mark)
			}
			if c := contrast(fg, bg); c < 3 {
				t.Errorf("in %s, %s %s on bg %s is %.2f:1, want 3:1 or more", theme, mark, fg, bg, c)
			}
		}
	}
}

// contrast is the WCAG contrast ratio of two #RRGGBB colours.
func contrast(a, b string) float64 {
	lum := func(hex string) float64 {
		var l float64
		for i, w := range []float64{0.2126, 0.7152, 0.0722} {
			v, _ := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
			c := float64(v) / 255
			if c <= 0.03928 {
				c /= 12.92
			} else {
				c = math.Pow((c+0.055)/1.055, 2.4)
			}
			l += w * c
		}
		return l
	}
	x, y := lum(a), lum(b)
	return (max(x, y) + 0.05) / (min(x, y) + 0.05)
}

func TestTheFrameAtEveryDeskWidth(t *testing.T) {
	for w, lw := range map[int]int{61: 28, 99: 28, 100: 32, 139: 32, 140: 40, 200: 40} {
		if got := deskListW(w); got != lw {
			t.Errorf("at %d columns the list is %d, want %d", w, got, lw)
		}
	}
	for _, size := range [][2]int{{60, 24}, {104, 38}, {132, 32}, {160, 40}} {
		w, h := size[0], size[1]
		m := mockDesk(t, w, h, true)
		lw := deskListW(w)
		lines := strings.Split(m.render(), "\n")
		plain := plainLines(lines)
		if len(plain) != h {
			t.Fatalf("at %d×%d the desk has %d lines", w, h, len(plain))
		}
		for y, l := range lines {
			if got := ansi.StringWidth(l); got != w || strings.ContainsRune(plain[y], '\uFFFD') {
				t.Errorf("at %d×%d line %d is %d cells, or cut mid-glyph: %q", w, h, y, got, plain[y])
			}
		}
		if r := []rune(plain[1]); r[lw] != '┬' || []rune(plain[h-2])[lw] != '┴' {
			t.Errorf("at %d×%d the rules read %q and %q, want ┬ and ┴ at %d", w, h, plain[1], plain[h-2], lw)
		}
		for y := 2; y < h-2; y++ {
			if e := []rune(plain[y])[lw]; e != '│' && e != '┤' {
				t.Errorf("at %d×%d line %d has %q at the edge", w, h, y, e)
			}
		}
		if !strings.HasPrefix(plain[0], " ▰ matchblox") || !strings.HasPrefix(plain[h-1], " LIST") {
			t.Errorf("at %d×%d the head row reads %q and the bar %q", w, h, plain[0], plain[h-1])
		}
		if w == 60 && (!strings.Contains(plain[h-1], "2 hosts s▸") || !strings.Contains(plain[0], " ▾")) {
			t.Errorf("at 60 the bar reads %q and the head row %q: want N hosts s▸ and +N ▾", plain[h-1], plain[0])
		}
		golden(t, m, "deskframe", w, h)
	}
}

// TestAPromptTakesTheBar: while a prompt waits for a key, the bar keeps
// its mode word first and says the prompt and its keys in place of the
// chips; once it is answered, the chips come back (owner, 2026-11-24).
func TestAPromptTakesTheBar(t *testing.T) {
	m := mockDesk(t, 160, 40, true)
	calm := plainLines(strings.Split(m.render(), "\n"))[39]
	m = pressOn(m, "x") // kill the chosen session: a confirm
	if m.pending == nil {
		t.Fatal("x asked nothing")
	}
	cells := themedCells(t, m.render(), true)
	bar := textOf(cells[39], 0, 160)
	if !strings.HasPrefix(bar, " CONFIRM  ") || !strings.Contains(bar, m.pending.String()) || !strings.Contains(bar, "y run") {
		t.Fatalf("with a confirm waiting the bar reads %q", bar)
	}
	if strings.Contains(bar, "cpu") || strings.Contains(bar, "? keys") {
		t.Errorf("the chips stay beside the prompt: %q", bar)
	}
	if c := cells[39][1]; c.fg != "accent" {
		t.Errorf("CONFIRM is %s, want accent", c.fg)
	}
	y := ansi.StringWidth(bar[:strings.Index(bar, "y run")])
	for x, want := range map[int]string{y: "fg", y + 2: "muted", y + 6: "faint", y + 8: "muted"} {
		if c := cells[39][x]; c.fg != want {
			t.Errorf("the confirm's %q is %s, want %s (key fg, word muted, · faint)", c.ch, c.fg, want)
		}
	}
	if m = pressOn(m, "esc"); plainLines(strings.Split(m.render(), "\n"))[39] != calm {
		t.Errorf("answered, the bar reads %q, want %q", plainLines(strings.Split(m.render(), "\n"))[39], calm)
	}
}

// TestTheHeadRowKeepsTheMarks: a standing error and data no longer fresh
// mark the head row before ⚙, in warn; a tap on ⚠ says the error again.
func TestTheHeadRowKeepsTheMarks(t *testing.T) {
	m := mockDesk(t, 160, 40, true)
	m.opt.Now = func() time.Time { return m.snap.At.Add(40 * time.Second) } // the service is slow
	cells := themedCells(t, m.render(), true)
	head := textOf(cells[0], 0, 160)
	if !strings.HasSuffix(strings.TrimRight(head, " "), "⚠ 1    ⚙") {
		t.Fatalf("the head row reads %q, want ⚠ 1 before ⚙", head)
	}
	x := ansi.StringWidth(head[:strings.Index(head, "⚠")])
	if c := cells[0][x]; c.fg != "warn" {
		t.Errorf("⚠ is %s, want warn", c.fg)
	}
	m.saying = nil
	m = mouse(t, m, tea.MouseClickMsg{X: x, Y: 0, Button: tea.MouseLeft})
	if said := m.sideSays(); !strings.Contains(said, "data is 40 s old") {
		t.Errorf("a tap on ⚠ says %q", said)
	}
}

// barLine is the last line of m's frame: the desk's bar.
func barLine(m Model) string {
	f := frameOf(m)
	return f[len(f)-1]
}

// firstChip is the bar's first chip after its mode word: the first host's.
func firstChip(m Model) string {
	f := strings.Fields(barLine(m))
	if len(f) < 2 {
		return ""
	}
	return f[1]
}
