package app

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// ctrlC is the key of a quit.
var ctrlC = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}

// quits runs cmd and what it batches, and tells if tea.Quit is among them.
// A command that blocks (a poll, a read) is skipped after a moment.
func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	out := make(chan tea.Msg, 1)
	go func() { out <- cmd() }()
	select {
	case msg := <-out:
		if _, ok := msg.(tea.QuitMsg); ok {
			return true
		}
		if b, ok := msg.(tea.BatchMsg); ok {
			return slices.ContainsFunc(b, quits)
		}
	case <-time.After(50 * time.Millisecond):
	}
	return false
}

// queueAgents is the queue machine with agents to start.
func queueAgents() proto.State {
	st := queueState()
	st.Agents = []string{"claude", "codex"}
	return st
}

// routeAt builds the state a routing row starts from.
func routeAt(t *testing.T, st proto.State, keys ...string) Model {
	t.Helper()
	var m tea.Model
	m, _ = loadedWith(t, 100, st)
	for _, k := range keys {
		m = sendKey(m, k)
	}
	return m.(Model)
}

// withRoute is a model with the layer open: sheet, form, peek, search, pair,
// held, pending, or none (main).
func withRoute(t *testing.T, layer string) Model {
	t.Helper()
	switch layer {
	case "main":
		return routeAt(t, queueState())
	case "sheet":
		m := routeAt(t, sheetState(proto.StokerView{PaneUp: true}, "matchblox"), "n")
		if m.sheet == nil {
			t.Fatal("n opened no sheet")
		}
		return m
	case "form":
		m := routeAt(t, queueAgents(), "N")
		if m.form == nil {
			t.Fatal("N opened no form")
		}
		return m
	case "peek":
		m := selectPane(t, routeAt(t, queueState()), "%2").(Model)
		next, _ := openedPeek(m)
		m = next.(Model)
		if m.peek == nil {
			t.Fatal("a opened no peek")
		}
		return m
	case "search":
		m := routeAt(t, queueState(), "/")
		if !m.searching {
			t.Fatal("/ did not start a search")
		}
		return m
	case "pair":
		_, m := openedPage(t)
		if !m.pair.owns() {
			t.Fatal("the pair page does not own the keys")
		}
		return m
	case "held":
		m := routeAt(t, nightState(), "ctrl+o")
		if m.held == nil {
			t.Fatal("enter at night held no key")
		}
		return m
	case "pending":
		m := selectPane(t, routeAt(t, queueState()), "%3").(Model)
		m = sendKey(m, "ctrl+o").(Model)
		if m.pending == nil {
			t.Fatal("ctrl+o set no pending")
		}
		return m
	}
	t.Fatalf("no layer %q", layer)
	return Model{}
}

func TestKeyRouting(t *testing.T) {
	esc := tea.KeyPressMsg{Code: tea.KeyEsc}
	text := func(s string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: []rune(s)[0], Text: s} }
	type row struct {
		name string
		from func(t *testing.T) Model
		key  tea.KeyPressMsg
		quit bool
		want func(t *testing.T, m Model, before Model)
	}
	rows := []row{
		{"splash: ctrl+c stops the start screen", func(t *testing.T) Model {
			m := New(Options{Conn: newFake()})
			next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
			m = next.(Model)
			if !m.splashing() {
				t.Fatal("no start screen")
			}
			return m
		}, ctrlC, false, func(t *testing.T, m, _ Model) {
			if !m.splashDone || m.quitting {
				t.Fatalf("splashDone %v, quitting %v", m.splashDone, m.quitting)
			}
		}},
		{"pair page owns: esc closes it", func(t *testing.T) Model { return withRoute(t, "pair") }, esc, false, func(t *testing.T, m, _ Model) {
			if m.pair != nil {
				t.Fatal("the pair page stayed")
			}
		}},
		{"sheet: esc closes it", func(t *testing.T) Model { return withRoute(t, "sheet") }, esc, false, func(t *testing.T, m, _ Model) {
			if m.sheet != nil {
				t.Fatal("the sheet stayed")
			}
		}},
		{"form: esc closes where's list, a second esc the form", func(t *testing.T) Model { return withRoute(t, "form") }, esc, false, func(t *testing.T, m, _ Model) {
			if m.form == nil || m.form.listOpen {
				t.Fatalf("the first esc: form %+v", m.form)
			}
			if next, _ := m.Update(esc); next.(Model).form != nil {
				t.Fatal("the form stayed")
			}
		}},
		{"peek: esc closes it", func(t *testing.T) Model { return withRoute(t, "peek") }, esc, false, func(t *testing.T, m, _ Model) {
			if m.peek != nil {
				t.Fatal("the peek stayed")
			}
		}},
		{"search: x is typed", func(t *testing.T) Model { return withRoute(t, "search") }, text("x"), false, func(t *testing.T, m, _ Model) {
			if m.filter != "x" || !m.searching {
				t.Fatalf("filter %q, searching %v", m.filter, m.searching)
			}
		}},
		{"held: n answers no", func(t *testing.T) Model { return withRoute(t, "held") }, text("n"), false, func(t *testing.T, m, _ Model) {
			if m.held != nil {
				t.Fatal("the held key stayed")
			}
		}},
		{"pending: esc cancels", func(t *testing.T) Model { return withRoute(t, "pending") }, esc, false, func(t *testing.T, m, _ Model) {
			if m.pending != nil || !strings.HasPrefix(m.flash, "cancelled: ") {
				t.Fatalf("pending %v, flash %q", m.pending, m.flash)
			}
		}},
		{"main: q quits", func(t *testing.T) Model { return withRoute(t, "main") }, text("q"), true, func(t *testing.T, m, _ Model) {
			if !m.quitting {
				t.Fatal("not quitting")
			}
		}},
		{"held: ctrl+c cancels as on main", func(t *testing.T) Model { return withRoute(t, "held") }, ctrlC, false, func(t *testing.T, m, _ Model) {
			if m.held != nil || m.quitting {
				t.Fatalf("held %v, quitting %v", m.held != nil, m.quitting)
			}
		}},
		{"pending: ctrl+c cancels", func(t *testing.T) Model { return withRoute(t, "pending") }, ctrlC, false, func(t *testing.T, m, _ Model) {
			if m.pending != nil || !strings.HasPrefix(m.flash, "cancelled: ") || m.quitting {
				t.Fatalf("pending %v, flash %q, quitting %v", m.pending, m.flash, m.quitting)
			}
		}},
		{"pending door, diff unread: down scrolls it", func(t *testing.T) Model {
			m := routeAt(t, doorState(longDoor()), "enter")
			if m.pending == nil || m.previewSeen() {
				t.Fatal("no unread diff")
			}
			return m
		}, text("j"), false, func(t *testing.T, m, before Model) {
			if m.pending == nil || m.previewTop != before.previewTop+1 {
				t.Fatalf("pending %v, previewTop %d after %d", m.pending, m.previewTop, before.previewTop)
			}
		}},
		{"pending door, diff unread: y waits", func(t *testing.T) Model {
			m := routeAt(t, doorState(longDoor()), "enter")
			if m.pending == nil || m.previewSeen() {
				t.Fatal("no unread diff")
			}
			return m
		}, text("y"), false, func(t *testing.T, m, before Model) {
			if m.pending == nil || m.previewTop != before.previewTop {
				t.Fatalf("pending %v, previewTop %d after %d", m.pending, m.previewTop, before.previewTop)
			}
		}},
	}
	for _, layer := range []string{"sheet", "form", "peek", "search", "pair", "main"} {
		rows = append(rows, row{layer + ": ctrl+c quits", func(t *testing.T) Model { return withRoute(t, layer) }, ctrlC, true, func(t *testing.T, m, _ Model) {
			if !m.quitting {
				t.Fatal("not quitting")
			}
		}})
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			before := r.from(t)
			var acts int
			if f, ok := before.conn.(*fakeConn); ok {
				acts = len(f.acts())
			}
			next, cmd := before.Update(r.key)
			if got := quits(cmd); got != r.quit {
				t.Fatalf("quit cmd %v, want %v", got, r.quit)
			}
			r.want(t, next.(Model), before)
			if f, ok := before.conn.(*fakeConn); ok && r.name == "pending door, diff unread: y waits" && len(f.acts()) != acts {
				t.Fatalf("y sent %+v before the diff was read", f.acts())
			}
		})
	}
}

// keyLineAt is the state a key line is read from.
type keyLineAt struct {
	name string
	m    tea.Model
}

// boxFeet are the feet of the boxes on screen, "╰─ …".
func boxFeet(m tea.Model) string {
	var feet []string
	for _, l := range strings.Split(screen(m), "\n") {
		if i, j := strings.Index(l, "╰"), strings.LastIndex(l, "╯"); i >= 0 && j > i {
			feet = append(feet, l[i:j+len("╯")])
		}
	}
	return strings.Join(feet, " | ")
}

// sampledAge is the clock at the right of the key line, not a key.
var sampledAge = regexp.MustCompile(`\s+sampled \S+ ago$`)

// keyLineOf is what the screen says to press: the key line, and the foot of
// a box when one is open.
func keyLineOf(m tea.Model) string {
	if p, ok := m.(pairScreen); ok {
		return p.lines()
	}
	line := sampledAge.ReplaceAllString(strings.TrimSpace(actionLine(m)), "")
	if feet := boxFeet(m); feet != "" {
		line += " || " + feet
	}
	return line
}

var letters = regexp.MustCompile(`[A-Za-z]`)

// pairScreen is the pair page, whose keys sit among its lines: all of them.
type pairScreen struct{ Model }

func (p pairScreen) lines() string {
	var out []string
	for _, l := range strings.Split(screen(p.Model), "\n") {
		if l = strings.TrimSpace(l); letters.MatchString(l) { // not the code's squares
			out = append(out, l)
		}
	}
	return strings.Join(out, " / ")
}

func keyLineStates(t *testing.T) []keyLineAt {
	t.Helper()
	var out []keyLineAt
	add := func(name string, m tea.Model) { out = append(out, keyLineAt{name, m}) }
	at := func(w int, st proto.State) tea.Model {
		m, _ := loadedWith(t, w, st)
		return m
	}
	tabAt := func(m tea.Model, tab int) tea.Model {
		return sendKey(m, string(rune('1'+tab)))
	}
	rowAt := func(m tea.Model, i int) tea.Model {
		for range m.(Model).rowCount() {
			m = sendKey(m, "up")
		}
		for range i {
			m = sendKey(m, "down")
		}
		return m
	}
	states := []struct {
		name  string
		st    proto.State
		width []int
	}{
		{"empty", proto.State{}, []int{100}},
		{"fixture", fixtureState(), []int{100}},
		{"queue", queueState(), []int{100, 50}},
		{"doors", doorsAndQueue(hooksDoor, wayBack), []int{100}},
		{"night", tabNight(queueState()), []int{100, 50}},
		{"morning", tabMorning(queueState()), []int{100}},
		{"stoker up", agentState(proto.StokerView{PaneUp: true}), []int{100}},
		{"stoker starting", agentState(proto.StokerView{Starting: true}), []int{100}},
		{"stoker run", stokerState(stokedRun(demoAt)), []int{100}},
		{"demo", screensState(t), []int{100, 50}},
		{"orphans", orphanState(), []int{100}},
	}
	for _, s := range states {
		for _, w := range s.width {
			for tab, name := range tabNames {
				base := tabAt(at(w, s.st), tab)
				n := min(base.(Model).rowCount(), 8)
				for i := range max(n, 1) {
					add(fmt.Sprintf("%s/%d/%s/row %d", s.name, w, name, i), rowAt(base, i))
				}
			}
		}
	}
	// What each overlay says: the box foot and the key line.
	for _, w := range []int{100, 50} {
		q := at(w, queueAgents())
		on := func(pane string) tea.Model { return selectPane(t, q, pane) }
		add(fmt.Sprintf("search/%d", w), typeText(sendKey(q, "/"), "app"))
		add(fmt.Sprintf("search kept/%d", w), sendKey(typeText(sendKey(q, "/"), "app"), "enter"))
		peek, _ := openedPeek(on("%2"))
		add(fmt.Sprintf("peek/%d", w), peek)
		add(fmt.Sprintf("peek typed/%d", w), typeText(peek, "yes"))
		add(fmt.Sprintf("peek question/%d", w), peekOn(on("%1")))
		add(fmt.Sprintf("form new/%d", w), sendKey(q, "N"))
		add(fmt.Sprintf("form rename/%d", w), sendKey(on("%2"), "r"))
		add(fmt.Sprintf("form switch/%d", w), sendKey(on("%2"), "A"))
		add(fmt.Sprintf("pending jump/%d", w), sendKey(on("%3"), "ctrl+o"))
		add(fmt.Sprintf("pending kill/%d", w), sendKey(on("%3"), "x"))
		add(fmt.Sprintf("pending cancelled/%d", w), sendKey(sendKey(on("%3"), "x"), "esc"))
		add(fmt.Sprintf("held/%d", w), sendKey(at(w, nightState()), "ctrl+o"))
		add(fmt.Sprintf("held x/%d", w), sendKey(at(w, nightState()), "x"))
		door := at(w, doorState(hooksDoor, wayBack))
		add(fmt.Sprintf("pending door/%d", w), sendKey(door, "enter"))
		add(fmt.Sprintf("pending door long/%d", w), sendKey(at(w, doorState(longDoor())), "enter"))
		add(fmt.Sprintf("pending door also/%d", w), sendKey(at(w, doorState(alsoDoor())), "enter"))
		add(fmt.Sprintf("sheet f/%d", w), sendKey(at(w, sheetState(proto.StokerView{}, "")), "f"))
		sh := at(w, sheetState(proto.StokerView{PaneUp: true}, "matchblox"))
		add(fmt.Sprintf("sheet n/%d", w), sendKey(sh, "n"))
		add(fmt.Sprintf("sheet chips/%d", w), sendKey(sendKey(sh, "n"), "tab"))
		add(fmt.Sprintf("stoker step ask/%d", w), sendKey(selectStep(t, tabAt(at(w, tabNight(queueState())), tabStoker).(Model), "g8"), "y"))
		add(fmt.Sprintf("stoker step look/%d", w), sendKey(selectStep(t, tabAt(at(w, tabMorning(queueState())), tabStoker).(Model), "p5"), "a"))
		add(fmt.Sprintf("stoker command/%d", w), sendKey(tabAt(at(w, tabNight(queueState())), tabStoker), "enter"))
		add(fmt.Sprintf("stoker quit agent/%d", w), sendKey(tabAt(at(w, agentState(proto.StokerView{PaneUp: true})), tabStoker), "Q"))
	}
	r, pm := newPairRig(t)
	_ = r
	pm.height = 40
	opened, cmd := pm.openPairPage()
	opened = offered(t, opened, cmd)
	add("pair page", pairScreen{opened})
	return out
}

// orphanState is the fixture machine with a stray process.
func orphanState() proto.State {
	st := fixtureState()
	st.Orphans = []sample.Orphan{{PID: 4242, Start: 7, Comm: "bash", Kill: []string{"kill", "4242"}}}
	return st
}

// alsoDoor is a door that offers a choice with its diff.
func alsoDoor() doors.Door {
	d := wayBack
	d.Also = &doors.Also{Label: "turn the mouse on", Preview: "+set -g mouse on"}
	return d
}

func TestKeyLinesUnchanged(t *testing.T) {
	states := keyLineStates(t)
	if len(states) != len(keyLines) {
		t.Errorf("%d states, %d pinned lines", len(states), len(keyLines))
	}
	for _, s := range states {
		want, ok := keyLines[s.name]
		if !ok {
			t.Errorf("%s: not pinned: %q", s.name, keyLineOf(s.m))
			continue
		}
		if got := keyLineOf(s.m); got != want {
			t.Errorf("%s:\n got %q\nwant %q", s.name, got, want)
		}
	}
}

// keyLines are the key lines and box feet of main, read from its output.
var keyLines = map[string]string{
	"empty/100/sessions/row 0":           "↑↓ move  ⏎ new session  p ships  f stoker  / find  q quit",
	"empty/100/ships/row 0":              "↑↓ move  1 sessions  f stoker  / find  q quit",
	"empty/100/machine/row 0":            "↑↓ move  p ships  f stoker  / find  q quit",
	"empty/100/git/row 0":                "↑↓ move  ⏎ shell  ␣ mark  X all safe  x remove  r rescan  / find  q quit",
	"empty/100/history/row 0":            "↑↓ move  p ships  f stoker  / find  q quit",
	"empty/100/panes/row 0":              "↑↓ move  ⏎ go  s sort  / find  q quit",
	"empty/100/stoker/row 0":             "↑↓ move  1 sessions  p ships  f stoker  / find  q quit",
	"fixture/100/sessions/row 0":         "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"fixture/100/sessions/row 1":         "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"fixture/100/sessions/row 2":         "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"fixture/100/sessions/row 3":         "↑↓ move  ⏎ new session  p ships  f stoker  / find  q quit",
	"fixture/100/sessions/row 4":         "↑↓ move  ⏎ do  x other  f stoker  / find  q quit",
	"fixture/100/sessions/row 5":         "↑↓ move  ⏎ do  x other  f stoker  / find  q quit",
	"fixture/100/ships/row 0":            "↑↓ move  1 sessions  f stoker  / find  q quit",
	"fixture/100/machine/row 0":          "↑↓ move  p ships  f stoker  / find  q quit",
	"fixture/100/git/row 0":              "↑↓ move  ⏎ shell  ␣ mark  X all safe  x remove  r rescan  / find  q quit",
	"fixture/100/history/row 0":          "↑↓ move  p ships  f stoker  / find  q quit",
	"fixture/100/panes/row 0":            "↑↓ move  ⏎ go  s sort  / find  q quit",
	"fixture/100/panes/row 1":            "↑↓ move  ⏎ go  s sort  / find  q quit",
	"fixture/100/panes/row 2":            "↑↓ move  ⏎ go  s sort  / find  q quit",
	"fixture/100/panes/row 3":            "↑↓ move  ⏎ go  s sort  / find  q quit",
	"fixture/100/stoker/row 0":           "↑↓ move  1 sessions  p ships  f stoker  / find  q quit",
	"queue/100/sessions/row 0":           "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"queue/100/sessions/row 1":           "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"queue/100/sessions/row 2":           "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"queue/100/sessions/row 3":           "↑↓ move  ⏎ new session  p ships  f stoker  / find  q quit",
	"queue/100/sessions/row 4":           "↑↓ move  ⏎ do  x other  f stoker  / find  q quit",
	"queue/100/sessions/row 5":           "↑↓ move  ⏎ do  x other  f stoker  / find  q quit",
	"queue/100/ships/row 0":              "↑↓ move  1 sessions  f stoker  / find  q quit",
	"queue/100/machine/row 0":            "↑↓ move  p ships  f stoker  / find  q quit",
	"queue/100/git/row 0":                "↑↓ move  ⏎ shell  ␣ mark  X all safe  x remove  r rescan  / find  q quit",
	"queue/100/history/row 0":            "↑↓ move  p ships  f stoker  / find  q quit",
	"queue/100/panes/row 0":              "↑↓ move  ⏎ go  s sort  / find  q quit",
	"queue/100/panes/row 1":              "↑↓ move  ⏎ go  s sort  / find  q quit",
	"queue/100/panes/row 2":              "↑↓ move  ⏎ go  s sort  / find  q quit",
	"queue/100/panes/row 3":              "↑↓ move  ⏎ go  s sort  / find  q quit",
	"queue/100/stoker/row 0":             "↑↓ move  1 sessions  p ships  f stoker  / find  q quit",
	"queue/50/sessions/row 0":            "⏎ chat  a chat  x kill  r rename  N new  f stoker",
	"queue/50/sessions/row 1":            "⏎ chat  a chat  x kill  r rename  N new  f stoker",
	"queue/50/sessions/row 2":            "⏎ chat  a chat  x kill  r rename  N new  f stoker",
	"queue/50/sessions/row 3":            "⏎ new session  p ships  f stoker",
	"queue/50/sessions/row 4":            "⏎ do  x other  f stoker",
	"queue/50/sessions/row 5":            "⏎ do  x other  f stoker",
	"queue/50/ships/row 0":               "1 sessions  f stoker",
	"queue/50/machine/row 0":             "p ships  f stoker",
	"queue/50/git/row 0":                 "⏎ shell  ␣ mark  X all safe  x remove  r rescan",
	"queue/50/history/row 0":             "p ships  f stoker",
	"queue/50/panes/row 0":               "⏎ go  s sort",
	"queue/50/panes/row 1":               "⏎ go  s sort",
	"queue/50/panes/row 2":               "⏎ go  s sort",
	"queue/50/panes/row 3":               "⏎ go  s sort",
	"queue/50/stoker/row 0":              "1 sessions  p ships  f stoker",
	"doors/100/sessions/row 0":           "↑↓ move  ⏎ open  x close  / find  q quit",
	"doors/100/sessions/row 1":           "↑↓ move  ⏎ open  x close  / find  q quit",
	"doors/100/sessions/row 2":           "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"doors/100/sessions/row 3":           "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"doors/100/sessions/row 4":           "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"doors/100/sessions/row 5":           "↑↓ move  ⏎ new session  p ships  f stoker  / find  q quit",
	"doors/100/sessions/row 6":           "↑↓ move  ⏎ do  x other  f stoker  / find  q quit",
	"doors/100/sessions/row 7":           "↑↓ move  ⏎ do  x other  f stoker  / find  q quit",
	"doors/100/ships/row 0":              "↑↓ move  1 sessions  f stoker  / find  q quit",
	"doors/100/machine/row 0":            "↑↓ move  p ships  f stoker  / find  q quit",
	"doors/100/git/row 0":                "↑↓ move  ⏎ shell  ␣ mark  X all safe  x remove  r rescan  / find  q quit",
	"doors/100/history/row 0":            "↑↓ move  p ships  f stoker  / find  q quit",
	"doors/100/panes/row 0":              "↑↓ move  ⏎ go  s sort  / find  q quit",
	"doors/100/panes/row 1":              "↑↓ move  ⏎ go  s sort  / find  q quit",
	"doors/100/panes/row 2":              "↑↓ move  ⏎ go  s sort  / find  q quit",
	"doors/100/panes/row 3":              "↑↓ move  ⏎ go  s sort  / find  q quit",
	"doors/100/stoker/row 0":             "↑↓ move  1 sessions  p ships  f stoker  / find  q quit",
	"night/100/sessions/row 0":           "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"night/100/sessions/row 1":           "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"night/100/sessions/row 2":           "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"night/100/sessions/row 3":           "↑↓ move  ⏎ new session  p ships  f stoker  / find  q quit",
	"night/100/sessions/row 4":           "↑↓ move  ⏎ do  x other  f stoker  / find  q quit",
	"night/100/sessions/row 5":           "↑↓ move  ⏎ do  x other  f stoker  / find  q quit",
	"night/100/ships/row 0":              "↑↓ move  1 sessions  f stoker  / find  q quit",
	"night/100/machine/row 0":            "↑↓ move  p ships  f stoker  / find  q quit",
	"night/100/git/row 0":                "↑↓ move  ⏎ shell  ␣ mark  X all safe  x remove  r rescan  / find  q quit",
	"night/100/history/row 0":            "↑↓ move  p ships  f stoker  / find  q quit",
	"night/100/panes/row 0":              "↑↓ move  ⏎ go  s sort  / find  q quit",
	"night/100/panes/row 1":              "↑↓ move  ⏎ go  s sort  / find  q quit",
	"night/100/panes/row 2":              "↑↓ move  ⏎ go  s sort  / find  q quit",
	"night/100/panes/row 3":              "↑↓ move  ⏎ go  s sort  / find  q quit",
	"night/100/stoker/row 0":             "↑↓ move  ⏎ command  1 sessions  p ships  n stop night  / find  q quit",
	"night/100/stoker/row 1":             "↑↓ move  ⏎ command  1 sessions  p ships  n stop night  / find  q quit",
	"night/100/stoker/row 2":             "↑↓ move  ⏎ command  1 sessions  p ships  n stop night  / find  q quit",
	"night/100/stoker/row 3":             "↑↓ move  ⏎ command  1 sessions  p ships  n stop night  / find  q quit",
	"night/100/stoker/row 4":             "↑↓ move  ⏎ command  1 sessions  p ships  n stop night  / find  q quit",
	"night/100/stoker/row 5":             "y give the go  ⏎ command  1 sessions  p ships  n stop night  / find  q quit",
	"night/100/stoker/row 6":             "↑↓ move  ⏎ command  1 sessions  p ships  n stop night  / find  q quit",
	"night/100/stoker/row 7":             "↑↓ move  ⏎ command  1 sessions  p ships  n stop night  / find  q quit",
	"night/50/sessions/row 0":            "⏎ chat  a chat  x kill  r rename  N new  f stoker",
	"night/50/sessions/row 1":            "⏎ chat  a chat  x kill  r rename  N new  f stoker",
	"night/50/sessions/row 2":            "⏎ chat  a chat  x kill  r rename  N new  f stoker",
	"night/50/sessions/row 3":            "⏎ new session  p ships  f stoker",
	"night/50/sessions/row 4":            "⏎ do  x other  f stoker",
	"night/50/sessions/row 5":            "⏎ do  x other  f stoker",
	"night/50/ships/row 0":               "1 sessions  f stoker",
	"night/50/machine/row 0":             "p ships  f stoker",
	"night/50/git/row 0":                 "⏎ shell  ␣ mark  X all safe  x remove  r rescan",
	"night/50/history/row 0":             "p ships  f stoker",
	"night/50/panes/row 0":               "⏎ go  s sort",
	"night/50/panes/row 1":               "⏎ go  s sort",
	"night/50/panes/row 2":               "⏎ go  s sort",
	"night/50/panes/row 3":               "⏎ go  s sort",
	"night/50/stoker/row 0":              "⏎ command  1 sessions  p ships  n stop night",
	"night/50/stoker/row 1":              "⏎ command  1 sessions  p ships  n stop night",
	"night/50/stoker/row 2":              "⏎ command  1 sessions  p ships  n stop night",
	"night/50/stoker/row 3":              "⏎ command  1 sessions  p ships  n stop night",
	"night/50/stoker/row 4":              "⏎ command  1 sessions  p ships  n stop night",
	"night/50/stoker/row 5":              "y give the go  ⏎ command  1 sessions  n stop nig…",
	"night/50/stoker/row 6":              "⏎ command  1 sessions  p ships  n stop night",
	"night/50/stoker/row 7":              "⏎ command  1 sessions  p ships  n stop night",
	"morning/100/sessions/row 0":         "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"morning/100/sessions/row 1":         "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"morning/100/sessions/row 2":         "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"morning/100/sessions/row 3":         "↑↓ move  ⏎ new session  p ships  f stoker  / find  q quit",
	"morning/100/sessions/row 4":         "↑↓ move  ⏎ do  x other  f stoker  / find  q quit",
	"morning/100/sessions/row 5":         "↑↓ move  ⏎ do  x other  f stoker  / find  q quit",
	"morning/100/ships/row 0":            "↑↓ move  1 sessions  f stoker  / find  q quit",
	"morning/100/machine/row 0":          "↑↓ move  p ships  f stoker  / find  q quit",
	"morning/100/git/row 0":              "↑↓ move  ⏎ shell  ␣ mark  X all safe  x remove  r rescan  / find  q quit",
	"morning/100/history/row 0":          "↑↓ move  p ships  f stoker  / find  q quit",
	"morning/100/panes/row 0":            "↑↓ move  ⏎ go  s sort  / find  q quit",
	"morning/100/panes/row 1":            "↑↓ move  ⏎ go  s sort  / find  q quit",
	"morning/100/panes/row 2":            "↑↓ move  ⏎ go  s sort  / find  q quit",
	"morning/100/panes/row 3":            "↑↓ move  ⏎ go  s sort  / find  q quit",
	"morning/100/stoker/row 0":           "↑↓ move  ⏎ command  1 sessions  p ships  f stoker  / find  q quit",
	"morning/100/stoker/row 1":           "↑↓ move  ⏎ command  1 sessions  p ships  f stoker  / find  q quit",
	"morning/100/stoker/row 2":           "↑↓ move  ⏎ command  1 sessions  p ships  f stoker  / find  q quit",
	"morning/100/stoker/row 3":           "↑↓ move  ⏎ command  1 sessions  p ships  f stoker  / find  q quit",
	"morning/100/stoker/row 4":           "↑↓ move  ⏎ command  1 sessions  p ships  f stoker  / find  q quit",
	"morning/100/stoker/row 5":           "↑↓ move  ⏎ command  1 sessions  p ships  f stoker  / find  q quit",
	"morning/100/stoker/row 6":           "↑↓ move  ⏎ command  1 sessions  p ships  f stoker  / find  q quit",
	"morning/100/stoker/row 7":           "↑↓ move  ⏎ command  1 sessions  p ships  f stoker  / find  q quit",
	"stoker up/100/sessions/row 0":       "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"stoker up/100/sessions/row 1":       "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"stoker up/100/sessions/row 2":       "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"stoker up/100/sessions/row 3":       "↑↓ move  ⏎ new session  p ships  f stoker  / find  q quit",
	"stoker up/100/sessions/row 4":       "↑↓ move  ⏎ do  x other  f stoker  / find  q quit",
	"stoker up/100/sessions/row 5":       "↑↓ move  ⏎ do  x other  f stoker  / find  q quit",
	"stoker up/100/ships/row 0":          "↑↓ move  1 sessions  f stoker  / find  q quit",
	"stoker up/100/machine/row 0":        "↑↓ move  p ships  f stoker  / find  q quit",
	"stoker up/100/git/row 0":            "↑↓ move  ⏎ shell  ␣ mark  X all safe  x remove  r rescan  / find  q quit",
	"stoker up/100/history/row 0":        "↑↓ move  p ships  f stoker  / find  q quit",
	"stoker up/100/panes/row 0":          "↑↓ move  ⏎ go  s sort  / find  q quit",
	"stoker up/100/panes/row 1":          "↑↓ move  ⏎ go  s sort  / find  q quit",
	"stoker up/100/panes/row 2":          "↑↓ move  ⏎ go  s sort  / find  q quit",
	"stoker up/100/panes/row 3":          "↑↓ move  ⏎ go  s sort  / find  q quit",
	"stoker up/100/stoker/row 0":         "⏎ open the stoker  Q quit it  1 sessions  p ships  f stoker  / find  q quit",
	"stoker starting/100/sessions/row 0": "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"stoker starting/100/sessions/row 1": "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"stoker starting/100/sessions/row 2": "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"stoker starting/100/sessions/row 3": "↑↓ move  ⏎ new session  p ships  f stoker  / find  q quit",
	"stoker starting/100/sessions/row 4": "↑↓ move  ⏎ do  x other  f stoker  / find  q quit",
	"stoker starting/100/sessions/row 5": "↑↓ move  ⏎ do  x other  f stoker  / find  q quit",
	"stoker starting/100/ships/row 0":    "↑↓ move  1 sessions  f stoker  / find  q quit",
	"stoker starting/100/machine/row 0":  "↑↓ move  p ships  f stoker  / find  q quit",
	"stoker starting/100/git/row 0":      "↑↓ move  ⏎ shell  ␣ mark  X all safe  x remove  r rescan  / find  q quit",
	"stoker starting/100/history/row 0":  "↑↓ move  p ships  f stoker  / find  q quit",
	"stoker starting/100/panes/row 0":    "↑↓ move  ⏎ go  s sort  / find  q quit",
	"stoker starting/100/panes/row 1":    "↑↓ move  ⏎ go  s sort  / find  q quit",
	"stoker starting/100/panes/row 2":    "↑↓ move  ⏎ go  s sort  / find  q quit",
	"stoker starting/100/panes/row 3":    "↑↓ move  ⏎ go  s sort  / find  q quit",
	"stoker starting/100/stoker/row 0":   "⏎ open the stoker  Q quit it  1 sessions  p ships  f stoker  / find  q quit",
	"stoker run/100/sessions/row 0":      "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"stoker run/100/sessions/row 1":      "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"stoker run/100/sessions/row 2":      "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"stoker run/100/sessions/row 3":      "↑↓ move  ⏎ new session  p ships  f stoker  / find  q quit",
	"stoker run/100/sessions/row 4":      "↑↓ move  ⏎ do  x other  f stoker  / find  q quit",
	"stoker run/100/sessions/row 5":      "↑↓ move  ⏎ do  x other  f stoker  / find  q quit",
	"stoker run/100/ships/row 0":         "↑↓ move  1 sessions  f stoker  / find  q quit",
	"stoker run/100/machine/row 0":       "↑↓ move  p ships  f stoker  / find  q quit",
	"stoker run/100/git/row 0":           "↑↓ move  ⏎ shell  ␣ mark  X all safe  x remove  r rescan  / find  q quit",
	"stoker run/100/history/row 0":       "↑↓ move  p ships  f stoker  / find  q quit",
	"stoker run/100/panes/row 0":         "↑↓ move  ⏎ go  s sort  / find  q quit",
	"stoker run/100/panes/row 1":         "↑↓ move  ⏎ go  s sort  / find  q quit",
	"stoker run/100/panes/row 2":         "↑↓ move  ⏎ go  s sort  / find  q quit",
	"stoker run/100/panes/row 3":         "↑↓ move  ⏎ go  s sort  / find  q quit",
	"stoker run/100/stoker/row 0":        "↑↓ move  ⏎ command  1 sessions  p ships  f stoker  / find  q quit",
	"stoker run/100/stoker/row 1":        "↑↓ move  ⏎ command  1 sessions  p ships  f stoker  / find  q quit",
	"demo/100/sessions/row 0":            "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"demo/100/sessions/row 1":            "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"demo/100/sessions/row 2":            "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"demo/100/sessions/row 3":            "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"demo/100/sessions/row 4":            "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"demo/100/sessions/row 5":            "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"demo/100/sessions/row 6":            "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"demo/100/sessions/row 7":            "↑↓ move  ⏎ new session  p ships  f stoker  / find  q quit",
	"demo/100/ships/row 0":               "↑↓ move  ⏎ session  f stoker  / find  q quit",
	"demo/100/ships/row 1":               "↑↓ move  ⏎ session  x merge  f stoker  / find  q quit",
	"demo/100/ships/row 2":               "↑↓ move  ⏎ session  a answer  f stoker  / find  q quit",
	"demo/100/ships/row 3":               "↑↓ move  ⏎ session  f stoker  / find  q quit",
	"demo/100/machine/row 0":             "↑↓ move  p ships  f stoker  / find  q quit",
	"demo/100/git/row 0":                 "↑↓ move  ⏎ shell  ␣ mark  X all safe  x remove  r rescan  / find  q quit",
	"demo/100/history/row 0":             "↑↓ move  p ships  f stoker  / find  q quit",
	"demo/100/panes/row 0":               "↑↓ move  ⏎ go  s sort  / find  q quit",
	"demo/100/panes/row 1":               "↑↓ move  ⏎ go  s sort  / find  q quit",
	"demo/100/panes/row 2":               "↑↓ move  ⏎ go  s sort  / find  q quit",
	"demo/100/panes/row 3":               "↑↓ move  ⏎ go  s sort  / find  q quit",
	"demo/100/panes/row 4":               "↑↓ move  ⏎ go  s sort  / find  q quit",
	"demo/100/panes/row 5":               "↑↓ move  ⏎ go  s sort  / find  q quit",
	"demo/100/panes/row 6":               "↑↓ move  ⏎ go  s sort  / find  q quit",
	"demo/100/panes/row 7":               "↑↓ move  ⏎ go  s sort  / find  q quit",
	"demo/100/stoker/row 0":              "↑↓ move  1 sessions  p ships  f stoker  / find  q quit",
	"demo/50/sessions/row 0":             "⏎ chat  a chat  x kill  r rename  N new  f stoker",
	"demo/50/sessions/row 1":             "⏎ chat  a chat  x kill  r rename  N new  f stoker",
	"demo/50/sessions/row 2":             "⏎ chat  a chat  x kill  r rename  N new  f stoker",
	"demo/50/sessions/row 3":             "⏎ chat  a chat  x kill  r rename  N new  f stoker",
	"demo/50/sessions/row 4":             "⏎ chat  a chat  x kill  r rename  N new  f stoker",
	"demo/50/sessions/row 5":             "⏎ chat  a chat  x kill  r rename  N new  f stoker",
	"demo/50/sessions/row 6":             "⏎ chat  a chat  x kill  r rename  N new  f stoker",
	"demo/50/sessions/row 7":             "⏎ new session  p ships  f stoker",
	"demo/50/ships/row 0":                "⏎ session  f stoker",
	"demo/50/ships/row 1":                "⏎ session  x merge  f stoker",
	"demo/50/ships/row 2":                "⏎ session  a answer  f stoker",
	"demo/50/ships/row 3":                "⏎ session  f stoker",
	"demo/50/machine/row 0":              "p ships  f stoker",
	"demo/50/git/row 0":                  "⏎ shell  ␣ mark  X all safe  x remove  r rescan",
	"demo/50/history/row 0":              "p ships  f stoker",
	"demo/50/panes/row 0":                "⏎ go  s sort",
	"demo/50/panes/row 1":                "⏎ go  s sort",
	"demo/50/panes/row 2":                "⏎ go  s sort",
	"demo/50/panes/row 3":                "⏎ go  s sort",
	"demo/50/panes/row 4":                "⏎ go  s sort",
	"demo/50/panes/row 5":                "⏎ go  s sort",
	"demo/50/panes/row 6":                "⏎ go  s sort",
	"demo/50/panes/row 7":                "⏎ go  s sort",
	"demo/50/stoker/row 0":               "1 sessions  p ships  f stoker",
	"orphans/100/sessions/row 0":         "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"orphans/100/sessions/row 1":         "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"orphans/100/sessions/row 2":         "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit",
	"orphans/100/sessions/row 3":         "↑↓ move  ⏎ new session  p ships  f stoker  / find  q quit",
	"orphans/100/sessions/row 4":         "↑↓ move  ⏎ do  x other  f stoker  / find  q quit",
	"orphans/100/sessions/row 5":         "↑↓ move  ⏎ do  x other  f stoker  / find  q quit",
	"orphans/100/ships/row 0":            "↑↓ move  1 sessions  f stoker  / find  q quit",
	"orphans/100/machine/row 0":          "↑↓ move  ⏎ jump  x kill  f stoker  / find  q quit",
	"orphans/100/git/row 0":              "↑↓ move  ⏎ shell  ␣ mark  X all safe  x remove  r rescan  / find  q quit",
	"orphans/100/history/row 0":          "↑↓ move  p ships  f stoker  / find  q quit",
	"orphans/100/panes/row 0":            "↑↓ move  ⏎ go  s sort  / find  q quit",
	"orphans/100/panes/row 1":            "↑↓ move  ⏎ go  s sort  / find  q quit",
	"orphans/100/panes/row 2":            "↑↓ move  ⏎ go  s sort  / find  q quit",
	"orphans/100/panes/row 3":            "↑↓ move  ⏎ go  s sort  / find  q quit",
	"orphans/100/stoker/row 0":           "↑↓ move  1 sessions  p ships  f stoker  / find  q quit",
	"search/100":                         "/ app▏  ⏎ keep  esc clear",
	"search kept/100":                    "/ app · esc clears  ⏎ chat  a chat  x kill  r rename  z park  N new  f stoker",
	"peek/100":                           "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit || ╰─ ⏎ send ─ ← prev  2/3  next → ─ esc ─ ^o jump ──────────╯",
	"peek typed/100":                     "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit || ╰─ ⏎ send ─ ← prev  2/3  next → ─ esc ─ ^o jump ──────────╯",
	"peek question/100":                  "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit || ╰─ ⏎ open ─ ← prev  1/3  next → ─ esc ─ ^o jump ──────────╯",
	"form new/100":                       "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit || ╰─ ⏎ pick · ↑↓ move · ←→ repo · tab next · esc keep the path ╯",
	"form rename/100":                    "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit || ╰─ ⏎ rename ─ esc ───────────────────────────────────────────╯",
	"form switch/100":                    "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit || ╰─ ⏎ plan ─ tab next ─ esc ──────────────────────────────────╯",
	"pending jump/100":                   "Jump: tmux switch-client -t %3 · ⏎ run ─ esc cancel || ╰─ ⏎ run ─ esc cancel ─────────────────────╯",
	"pending kill/100":                   "Kill tools: tmux kill-pane -t %3 then kill -TERM 400 · y run ─ any other key cancels || ╰─ y run ─ any other key cancels ──────────╯",
	"pending cancelled/100":              "cancelled: kill tools: tmux kill-pane -t %3, then kill -TERM 400",
	"held/100":                           "The stoker is on. Take over?: y  rests it while you work n  keeps it on · y take over ─ n keep it on || ╰─ y take over ─ n keep it on ─────────────────────╯",
	"held x/100":                         "The stoker is on. Take over?: y  rests it while you work n  keeps it on · y take over ─ n keep it on || ╰─ y take over ─ n keep it on ─────────────────────╯",
	"pending door/100":                   "RUN Add the queue hooks: write ~/.claude/settings.json   y run  any other key cancels",
	"pending door long/100":              "RUN Add the queue hooks: write ~/.claude/settings.json   ↓ the rest, then y",
	"pending door also/100":              "RUN Add the way back: write ~/.tmux.conf   y run  any other key cancels",
	"sheet f/100":                        "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit || ╰─ ⏎ start ─ tab chips ─ esc ────────────────────────────────╯",
	"sheet n/100":                        "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit || ╰─ ⏎ send ─ tab chips ─ esc ─────────────────────────────────╯",
	"sheet chips/100":                    "⏎ chat  a chat  x kill  r rename  z park  N new  p ships  f stoker  / find  q quit || ╰─ ⏎ send ─ space toggle ─ tab back ─────────────────────────╯",
	"stoker step ask/100":                "go given: gh release create v1.4",
	"stoker step look/100":               "kindle-web is gone: there is no pane to look at",
	"stoker command/100":                 "y give the go  ⏎ command  1 sessions  p ships  n stop night  / find  q quit",
	"stoker quit agent/100":              "Quit the stoker agent: tmux -L matchblox kill-session -t =stoker · y run ─ any other key cancels || ╰─ y run ─ any other key cancels ───────────────╯",
	"search/50":                          "/ app▏  ⏎ keep  esc clear",
	"search kept/50":                     "/ app · esc clears  ⏎ chat  a chat  x kill  r re…",
	"peek/50":                            "⏎ chat  a chat  x kill  r rename  N new  f stoker || ╰─ ⏎ send ─ ← prev  2/3  next → ─ esc ───────────╯",
	"peek typed/50":                      "⏎ chat  a chat  x kill  r rename  N new  f stoker || ╰─ ⏎ send ─ ← prev  2/3  next → ─ esc ───────────╯",
	"peek question/50":                   "⏎ chat  a chat  x kill  r rename  N new  f stoker || ╰─ ⏎ open ─ ← prev  1/3  next → ─ esc ───────────╯",
	"form new/50":                        "⏎ chat  a chat  x kill  r rename  N new  f stoker || ╰─ ⏎ pick · ↑↓ move · esc keep the path ─────────╯",
	"form rename/50":                     "⏎ chat  a chat  x kill  r rename  N new  f stoker || ╰─ ⏎ rename ─ esc ───────────────────────────────╯",
	"form switch/50":                     "⏎ chat  a chat  x kill  r rename  N new  f stoker || ╰─ ⏎ plan ─ tab next ─ esc ──────────────────────╯",
	"pending jump/50":                    "Jump: tmux switch-client -t %3 · ⏎ run ─ esc cancel || ╰─ ⏎ run ─ esc cancel ───────────────────────────╯",
	"pending kill/50":                    "Kill tools: tmux kill-pane -t %3 then kill -TERM 400 · y run ─ any other key cancels || ╰─ y run ─ any other key cancels ────────────────╯",
	"pending cancelled/50":               "cancelled: kill tools: tmux kill-pane -t %3, the…",
	"held/50":                            "The stoker is on. Take over?: y  rests it while you work n  keeps it on · y take over ─ n keep it on || ╰─ y take over ─ n keep it on ───────────────────╯",
	"held x/50":                          "The stoker is on. Take over?: y  rests it while you work n  keeps it on · y take over ─ n keep it on || ╰─ y take over ─ n keep it on ───────────────────╯",
	"pending door/50":                    "RUN Add the queue hooks: write ~/.claude/setting…",
	"pending door long/50":               "RUN Add the queue hooks: write ~/.claude/setting…",
	"pending door also/50":               "RUN Add the way back: write ~/.tmux.conf   y run…",
	"sheet f/50":                         "⏎ chat  a chat  x kill  r rename  N new  f stoker || ╰─ ⏎ start ─ tab chips ─ esc ────────────────────╯",
	"sheet n/50":                         "⏎ chat  a chat  x kill  r rename  N new  f stoker || ╰─ ⏎ send ─ tab chips ─ esc ─────────────────────╯",
	"sheet chips/50":                     "⏎ chat  a chat  x kill  r rename  N new  f stoker || ╰─ ⏎ send ─ space toggle ─ tab back ─────────────╯",
	"stoker step ask/50":                 "go given: gh release create v1.4",
	"stoker step look/50":                "kindle-web is gone: there is no pane to look at",
	"stoker command/50":                  "y give the go  ⏎ command  1 sessions  n stop nig…",
	"stoker quit agent/50":               "Quit the stoker agent: tmux -L matchblox kill-session -t =stoker · y run ─ any other key cancels || ╰─ y run ─ any other key cancels ────────────────╯",
	"pair page":                          "Scan with the matchblox app, or paste the link.   5:00 / c copies the link · esc stops / Port 22. For another port, run matchblox pair --port <n>.",
}
