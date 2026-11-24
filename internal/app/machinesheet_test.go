package app

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
)

// agentRows is an agent's screen of n numbered rows.
func agentRows(n int) string {
	var b strings.Builder
	for y := range n {
		fmt.Fprintf(&b, "\x1b[%d;1H%02d agent row %02d", y+1, y, y)
	}
	return b.String()
}

// sheetTop is the first line of the content of the sheet whose top edge
// runs from x0 to x1 on line y0: what it opened on.
func sheetTop(cells [][]inked, x0, x1, y0 int) string {
	return strings.TrimSpace(textOf(cells[y0+1], x0+1, x1))
}

// burningDesk is the mockup's desk at 160×40 with the agent beside the
// list, a busy loop on ws-1 and one on gpu-1.
func burningDesk(t *testing.T) Model {
	t.Helper()
	home, gpu := mockupDesk()
	home.Orphans, gpu.Orphans = busyLoop(), busyLoop()
	gpu.Orphans[0].PID = 4343
	return withAgent(t, deskWith(t, 160, 40, true, home, gpu), agentRows(36), false)
}

// modeOf is the bar's mode word.
func modeOf(m Model) string {
	lines := strings.Split(ansi.Strip(m.render()), "\n")
	return strings.Fields(lines[len(lines)-1])[0]
}

func TestCommaOpensMachine(t *testing.T) {
	m := burningDesk(t)
	opened := pressOn(m, ",")
	cells := themedCells(t, opened.render(), true)
	const x0, y0, x1, y1 = 45, 3, 155, 36
	for _, c := range []struct {
		x, y int
		want string
	}{{x0, y0, "╭"}, {x1, y0, "╮"}, {x0, y1, "╰"}, {x1, y1, "╯"}} {
		if got := cells[c.y][c.x]; got.ch != c.want || got.fg != "faint" {
			t.Errorf("the sheet's corner at %d,%d is %q in %s, want %q in faint", c.x, c.y, got.ch, got.fg, c.want)
		}
	}
	if title := textOf(cells[y0], x0, x0+28); title != "╭─ ⚙ machine · ws-1 · gpu-1 " {
		t.Errorf("the sheet's top edge reads %q, want ⚙ machine and the hosts in it", title)
	}
	if top := sheetTop(cells, x0, x1, y0); top != "USAGE" {
		t.Errorf(", opened the sheet on %q, want USAGE", top)
	}
	if foot := strings.TrimSpace(textOf(cells[y1-1], x0+1, x1)); !strings.HasSuffix(foot, "esc back to flint-auth") || !strings.HasPrefix(foot, "↑↓") {
		t.Errorf("the sheet's keys read %q, want Machine's keys and esc back to flint-auth", foot)
	}
	bar := strings.Split(opened.render(), "\n")[39]
	if mode := modeOf(opened); mode != "MACHINE" {
		t.Errorf("with the sheet open the bar names %s, want MACHINE", mode)
	}
	if ink := inkOf(t, bar, "MACHINE"); ink != "accent bold" {
		t.Errorf("the mode word is %s, want accent bold", ink)
	}

	sc := m.draw(160)
	i := slices.IndexFunc(sc.keys, func(s span) bool { return s.press == "cog:loops:gpu-1" })
	if i < 0 {
		t.Fatalf("no busy-loop chip for gpu-1 on the bar: %q %+v", ansi.Strip(sc.lines[sc.keysAt]), sc.keys)
	}
	next, _ := tap(m, sc.keys[i].x0+1, sc.keysAt)
	cells = themedCells(t, next.(Model).render(), true)
	if top := sheetTop(cells, x0, x1, y0); top != "DETACHED BUSY LOOPS" {
		t.Errorf("a tap on gpu-1's chip opened the sheet on %q, want the busy loops", top)
	}
	if loop := textOf(cells[y0+2], x0+1, x1); !strings.Contains(loop, "4343 node") {
		t.Errorf("under the heading the sheet shows %q, want gpu-1's loop 4343", loop)
	}
}

// TestTheSheetHasMachinesKeys: in the sheet each of Machine's keys does
// what it does on the Machine tab, as the phone has it: ⏎ opens the row's
// view, x stops the chosen busy loop after a typed y, X marks every safe
// worktree, f lights the stoker.
func TestTheSheetHasMachinesKeys(t *testing.T) {
	home, gpu := mockupDesk()
	home.Orphans = busyLoop()
	home.Orphans[0].Kill = []string{"kill", "4242"}
	home.Git = gitState().Git
	now := boardAt
	m, f, conns := hostsAt(t, &now, "ws-1", 160, 40, "gpu-1")
	m = upd(t, m, stateMsg(home))
	m = upd(t, m, peerMsg{host: "gpu-1", conn: conns["gpu-1"], msg: stateMsg(gpu)})
	m.toList()
	sheet := pressOn(m, ",")
	if open := pressOn(sheet, "down", "enter"); open.tab != tabGit || !open.cogShown() {
		t.Errorf("⏎ on git: tab %d, in the sheet %v; want Git in the sheet", open.tab, open.cogShown())
	}
	if marked := pressOn(sheet, "down", "enter", "X"); len(marked.picked) != 2 {
		t.Errorf("X in Git marked %d worktrees, want the 2 safe ones", len(marked.picked))
	}
	if lit := pressOn(sheet, "f"); lit.sheet == nil || lit.sheet.night {
		t.Errorf("f with the stoker off: launch sheet %+v, want the day run's", lit.sheet)
	}
	asked := pressOn(sheet, "down", "down", "down", "down", "x")
	if asked.pending == nil || asked.pending.rec != "orphan:4242" {
		t.Fatalf("x on the busy loop asks %+v, want its stop", asked.pending)
	}
	pressOn(asked, "y")
	if !slices.ContainsFunc(f.acts(), func(a proto.Act) bool { return a.RecID == "orphan:4242" }) {
		t.Errorf("y sent %+v, want the stop of 4242", f.acts())
	}
}

// TestTheSheetKeysMatchThePhone: each of Machine's keys in the sheet
// leaves Machine as the same keys leave the phone's Machine tab.
func TestTheSheetKeysMatchThePhone(t *testing.T) {
	home, gpu := mockupDesk()
	home.Orphans = busyLoop()
	desk := withAgent(t, deskWith(t, 160, 40, true, home, gpu), agentRows(36), false)
	phone := deskWith(t, 44, 31, true, home, gpu)
	phone.setTab(tabMachine)
	did := func(m Model) string {
		p := ""
		if m.pending != nil {
			p = m.pending.String()
		}
		return fmt.Sprintf("tab %d · view %q · loop %d · confirm %q · launch %v", m.tab, m.viewPick, m.orphanPID, p, m.sheet != nil)
	}
	for _, c := range []struct {
		keys []string
		want func(Model) bool
	}{
		{[]string{"down"}, func(m Model) bool { return m.viewPick == "git" }},
		{[]string{"down", "down", "enter"}, func(m Model) bool { return m.tab == tabHistory }},
		{[]string{"enter"}, func(m Model) bool { return m.tab == tabStoker }},
		{[]string{"down", "down", "down", "down", "x"}, func(m Model) bool { return m.pending != nil }},
		{[]string{"X"}, func(Model) bool { return true }},
		{[]string{"f"}, func(m Model) bool { return m.sheet != nil || m.tab == tabStoker }},
	} {
		sheet := pressOn(pressOn(desk, ","), c.keys...)
		tab := pressOn(phone, c.keys...)
		if got, want := did(sheet), did(tab); got != want {
			t.Errorf("%v in the sheet: %s; on the Machine tab: %s", c.keys, got, want)
		}
		if !c.want(sheet) {
			t.Errorf("%v in the sheet did nothing: %s", c.keys, did(sheet))
		}
	}
}

// inTime is what run returns, or a failure when it takes 2 s: a broken
// way out fails the test instead of hanging it.
func inTime[T any](t *testing.T, what string, run func() T) T {
	t.Helper()
	got := make(chan T, 1)
	go func() { got <- run() }()
	select {
	case v := <-got:
		return v
	case <-time.After(2 * time.Second):
		t.Fatalf("%s did not end within 2s", what)
		var zero T
		return zero
	}
}

func TestCtrlQAlwaysLeavesTheSheet(t *testing.T) {
	m := burningDesk(t)
	for _, c := range []struct {
		name string
		keys []string
	}{
		{"the sheet", []string{","}},
		{"a confirm in it", []string{",", "down", "down", "down", "down", "x"}},
		{"history, which never answers", []string{",", "down", "down", "enter"}},
		{"the stoker's view", []string{",", "enter"}},
		{"the keys sheet", []string{"?"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := inTime(t, fmt.Sprint(c.keys), func() Model { return pressOn(m, c.keys...) })
			if !in.inSheets() {
				t.Fatalf("%v opened no sheet: tab %d", c.keys, in.tab)
			}
			// ctrl+q and all that reads the console after it: a key the
			// agent took blocks on its unread screen.
			type left struct {
				tab, mode, over             string
				pending, sheet, list, moved bool
			}
			out := inTime(t, "ctrl+q", func() left {
				out := pressOn(in, flipKey)
				return left{fmt.Sprint(out.tab), modeOf(out), out.over, out.pending != nil, out.cogShown(), out.inList(),
					pressOn(out, "down").chosenSide() != out.chosenSide()}
			})
			if want := (left{fmt.Sprint(tabSessions), "LIST", "", false, false, true, true}); out != want {
				t.Errorf("after ctrl+q: %+v, want %+v (the list's tab and keys, no sheet, ↓ moves the choice)", out, want)
			}
		})
	}
	t.Run("Hosts, opened from the sheet", func(t *testing.T) {
		r := releaseRig(t, "v0.42.0", "v0.42.0", nil)
		r.key(t, flipKey)
		if r.sh.layer != layerConsole || r.sh.cur.tab != tabSessions || modeOf(r.sh.cur) != "LIST" {
			t.Errorf("ctrl+q in Hosts: layer %v, tab %d, bar %s", r.sh.layer, r.sh.cur.tab, modeOf(r.sh.cur))
		}
	})
}

func TestEscStepsBackThenCloses(t *testing.T) {
	m := burningDesk(t)
	m = pressOn(m, ",", "down", "down", "enter")
	if m.tab != tabHistory {
		t.Fatalf("⏎ on history opened tab %d", m.tab)
	}
	if m = pressOn(m, "esc"); !m.cogShown() || modeOf(m) != "MACHINE" || m.viewPick != "history" {
		t.Errorf("esc from history: tab %d, bar %s, row %q; want the sheet on history", m.tab, modeOf(m), m.viewPick)
	}
	if m = pressOn(m, "esc"); m.tab != tabSessions || modeOf(m) != "LIST" {
		t.Errorf("the second esc: tab %d, bar %s; want the list", m.tab, modeOf(m))
	}
	t.Run("from Hosts", func(t *testing.T) {
		r := releaseRig(t, "v0.42.0", "v0.42.0", nil)
		r.key(t, "esc")
		if r.sh.layer != layerConsole || !r.sh.cur.cogShown() {
			t.Fatalf("esc in Hosts: layer %v, tab %d; want the sheet", r.sh.layer, r.sh.cur.tab)
		}
		r.key(t, "esc")
		if r.sh.cur.tab != tabSessions {
			t.Errorf("the second esc left tab %d", r.sh.cur.tab)
		}
	})
}

func TestEscReturnsToTheSameRow(t *testing.T) {
	m := withAgent(t, mockDesk(t, 160, 40, true), agentRows(36), false)
	m = pressOn(m, "down")
	before, chosen := m.render(), m.chosenSide()
	m = pressOn(m, ",", "down", "esc")
	if m.cogShown() || m.over != "" {
		t.Fatalf("esc left the sheet open: tab %d", m.tab)
	}
	if got := m.chosenSide(); got != chosen {
		t.Errorf("after the sheet the chosen row is %v, want %v", got, chosen)
	}
	if after := m.render(); after != before {
		t.Errorf("after the sheet the desk reads\n%s\nwant\n%s", ansi.Strip(after), ansi.Strip(before))
	}
}

func TestTabStillOpensMachineOnThePhone(t *testing.T) {
	phone, _ := loadedWith(t, 44, queueState())
	if next := pressOn(phone, "tab"); next.tab != tabMachine {
		t.Errorf("⇥ on the phone opens tab %d, want Machine", next.tab)
	}
	// The belt's `,` opens the Machine tab on the phone (no sheet; Task 16).
	if next := pressOn(phone, ","); next.tab != tabMachine || next.cogShown() {
		t.Errorf(", on the phone: tab %d, sheet %v; want Machine tab, no sheet", next.tab, next.cogShown())
	}
	t.Run("on the desk ⇥ is unbound and says nothing", func(t *testing.T) {
		m := withAgent(t, mockDesk(t, 160, 40, true), agentRows(36), false)
		next := pressOn(m, "tab")
		if next.tab != m.tab || next.over != "" {
			t.Errorf("⇥ on the desk opened tab %d, sheet %q", next.tab, next.over)
		}
		if got, want := ansi.Strip(next.render()), ansi.Strip(m.render()); got != want {
			t.Errorf("⇥ on the desk changed the screen:\n%s", got)
		}
	})
}

func TestCommaTypesInTheAgent(t *testing.T) {
	d := newScreenDesk(t, dragScreen, 40, 8)
	d.do(keyPress(flipKey))
	d.do(keyPress(","))
	if got := d.agentRead(); got != "," {
		t.Errorf("with the agent's keys , reached the agent as %q, want ,", got)
	}
	if d.m.tab != tabSessions {
		t.Errorf("with the agent's keys , opened tab %d", d.m.tab)
	}
}

func TestTheSheetFitsTheRegion(t *testing.T) {
	for _, w := range []int{160, 104} {
		m := withAgent(t, mockDesk(t, w, 40, true), agentRows(36), false)
		box := m.sideBoxAt(w)
		cells := themedCells(t, pressOn(m, ",").render(), true)
		x0, x1 := box.chatX+4, box.chatX+box.chatW-5
		if cells[3][x0].ch != "╭" || cells[3][x1].ch != "╮" || cells[36][x0].ch != "╰" {
			t.Errorf("at %d the sheet's edge is not 4 cells in: %q", w, textOf(cells[3], box.chatX, w))
		}
		if sw := x1 - x0 + 1; sw < 60 {
			t.Errorf("at %d the sheet is %d columns, want 60 or more", w, sw)
		}
	}
	const w, h = 79, 30
	m := withAgent(t, mockDesk(t, w, h, true), agentRows(h), false)
	box := m.sideBoxAt(w)
	if box.chatW != 50 {
		t.Fatalf("at %d by %d the agent region is %d columns, want 50", w, h, box.chatW)
	}
	cells := themedCells(t, pressOn(m, ",").render(), true)
	x0, x1, y1 := box.chatX, box.chatX+box.chatW-1, m.bodyRows()+1
	if cells[2][x0].ch != "╭" || cells[2][x1].ch != "╮" || cells[y1][x0].ch != "╰" || cells[y1][x1].ch != "╯" {
		t.Errorf("a 50-column region's sheet does not fill it:\n%s\n%s", textOf(cells[2], x0, x1+1), textOf(cells[y1], x0, x1+1))
	}
}

// sheetCell is the cell where Machine's row s starts in the ⚙ sheet.
func sheetCell(t *testing.T, m Model, s string) (x, y int) {
	t.Helper()
	for y, line := range strings.Split(ansi.Strip(m.render()), "\n") {
		if i := strings.Index(line, "│  "+s); i >= 0 {
			return ansi.StringWidth(line[:i]) + 3, y
		}
	}
	t.Fatalf("the sheet has no row %q:\n%s", s, ansi.Strip(m.render()))
	return -1, -1
}

// TestAViewOpensInsideTheSheet: on the desk git, history and panes open
// inside the ⚙ sheet, with no tab row; the stoker's view keeps the
// screen. From each esc goes back to the sheet on its row, and ctrl+q
// gives the keys to the list.
func TestAViewOpensInsideTheSheet(t *testing.T) {
	m := burningDesk(t)
	for i, name := range []string{"stoker", "git", "history", "panes"} {
		open := pressOn(m, append([]string{","}, append(slices.Repeat([]string{"down"}, i), "enter")...)...)
		if back := pressOn(open, "esc"); back.tab != tabMachine || !back.cogShown() || back.viewPick != name {
			t.Errorf("%s: esc gives tab %d on %q, want the sheet on its row", name, back.tab, back.viewPick)
		}
		if mode := modeOf(pressOn(open, flipKey)); mode != "LIST" {
			t.Errorf("%s: ctrl+q gives the keys to %s, want LIST", name, mode)
		}
		if name == "stoker" {
			if open.tab != tabStoker {
				t.Errorf("stoker: enter gives tab %d, want the stoker's view", open.tab)
			}
			continue
		}
		cells := themedCells(t, open.render(), true)
		lines := strings.Split(ansi.Strip(open.render()), "\n")
		if title := textOf(cells[3], 45, 100); textOf(cells[3], 45, 46) != "╭" || textOf(cells[36], 155, 156) != "╯" || !strings.Contains(title, "› "+name) {
			t.Errorf("%s: the sheet's top edge reads %q, want it named in the sheet", name, title)
		}
		if !strings.Contains(lines[1], "┬") || strings.Contains(strings.ToLower(lines[1]), "sessions") {
			t.Errorf("%s: line 1 is %q, want the desk's rule, no tab row", name, lines[1])
		}
		if mode := modeOf(open); mode != "MACHINE" {
			t.Errorf("%s: the bar names %s, want MACHINE", name, mode)
		}
		if foot := strings.TrimSpace(textOf(cells[35], 46, 155)); !strings.HasSuffix(foot, "esc machine") {
			t.Errorf("%s: the sheet's keys read %q, want esc machine last", name, foot)
		}
	}
}

// TestTheFixWrapsInTheSheet: a fix in Machine wraps inside the sheet, so
// none is cut, at a region the sheet fills and at one it sits 4 in.
func TestTheFixWrapsInTheSheet(t *testing.T) {
	const fix = "cannot read ~/.ssh/authorized_keys: permission denied. Check its owner and mode (600)."
	for _, w := range []int{100, 104} {
		_, m := devicesConsole(t, 0)
		host := *m.opt.Pair
		host.Devices = func() ([]PairDevice, error) { return nil, errors.New("permission denied") }
		m.opt.Pair = &host
		m = upd(t, m, tea.WindowSizeMsg{Width: w, Height: 40})
		m.setTab(tabSessions) // the console opened on Machine: , opens it again from the list, reading the devices
		m.toList()
		m = pressOn(m, ",")
		for i := 0; i < 20; i++ {
			if r, ok := m.selectedDevice(); ok && r.plus {
				break
			}
			m = pressOn(m, "down")
		}
		sc := m.draw(w)
		var words []string
		for _, l := range strings.Split(m.render(), "\n") {
			words = append(words, strings.Fields(ansi.Strip(ansi.Cut(l, sc.overX0+1, sc.overX1-1)))...)
		}
		if got := strings.Join(words, " "); !strings.Contains(got, fix) {
			t.Errorf("at %d the sheet cuts the fix:\n%s", w, ansi.Strip(m.render()))
		}
	}
}
