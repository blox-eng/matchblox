package app

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/logos"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

// pressOn is pressAll on the board's console.
func pressOn(m Model, keys ...string) Model { return pressAll(m, keys...).(Model) }

func boardFake(m Model) *fakeConn { return m.conn.(*fakeConn) }

func chosenName(t *testing.T, m Model) string {
	t.Helper()
	s, ok := m.sideChosen()
	if !ok {
		t.Fatal("no session is chosen")
	}
	return s.Name
}

// newBoardState gives the console a new state of this machine: st's
// matches, queue and ships that are not gpu-1's, as boardShot splits them.
func newBoardState(t *testing.T, m Model, st proto.State) Model {
	t.Helper()
	home := st
	home.Sessions = slices.DeleteFunc(slices.Clone(st.Sessions), func(s sample.Session) bool { return s.Host == "gpu-1" })
	home.Queue = slices.DeleteFunc(slices.Clone(st.Queue), func(it queue.Item) bool { return it.Host == "gpu-1" })
	home.Ships = &proto.Ships{At: demoAt}
	if st.Ships != nil {
		home.Ships.Ships = slices.DeleteFunc(slices.Clone(st.Ships.Ships), func(sh proto.Ship) bool { return sh.Host == "gpu-1" })
	}
	next, _ := m.Update(stateMsg(home))
	return next.(Model)
}

// noticeRow is what the console says now, on one line.
func noticeRow(m Model) string {
	return strings.Join(saidRows(m, max(m.width, minWidth)), " ")
}

func mergeOf(t *testing.T, st proto.State) proto.Ship {
	t.Helper()
	for _, sh := range st.Ships.Ships {
		if sh.Number == 4556 {
			return sh
		}
	}
	t.Fatal("no ship #4556")
	return proto.Ship{}
}

// TestAKeyOnAnEmptySearchSaysEscClears: a card key with no card says the
// fix that is true: esc clears a search, and s turns a host.
func TestAKeyOnAnEmptySearchSaysEscClears(t *testing.T) {
	st := boardFixture(t)
	m := typeText(pressOn(boardShot(t, st, 104, 38, nil), "/"), "zzzz").(Model)
	m = pressOn(m, "enter")
	for _, k := range []string{"x", "p", "m"} {
		if got := noticeRow(pressOn(m, k)); got != `nothing matches "zzzz" · esc clears` {
			t.Errorf("%s on an empty search says %q", k, got)
		}
	}
	m = boardShot(t, st, 104, 38, func(m *Model) { m.boardHost = "macbook" })
	if got := noticeRow(pressOn(m, "x")); got != "no match on macbook · s turns the host" {
		t.Errorf("x on a host with no match says %q", got)
	}
}

func TestPMergesThroughTheConsentBox(t *testing.T) {
	st := boardFixture(t)
	sh := mergeOf(t, st)
	m := pressOn(boardShot(t, st, 80, 31, chooseCard("kindle-web")), "p")
	if !m.consenting() || !strings.Contains(consentText(m), strings.Join(sh.Merge, " ")) {
		t.Fatalf("p does not show the merge: %q", consentText(m))
	}
	m = pressOn(m, "enter")
	if m.pending != nil || len(boardFake(m).acts()) != 0 {
		t.Fatalf("⏎ ran the merge: %+v", boardFake(m).acts())
	}
	m = pressOn(m, "p", "y")
	want := proto.Act{RecID: "ship:" + sh.Repo + "#4556", Which: "secondary", Confirm: "y"}
	if acts := boardFake(m).acts(); len(acts) != 1 || acts[0] != want {
		t.Fatalf("y sent %+v, want %+v", acts, want)
	}

	m = pressOn(boardShot(t, st, 80, 31, chooseCard("spark-search")), "p", "p")
	if m.pending != nil || len(boardFake(m).acts()) != 0 {
		t.Errorf("p on a stuck ship acted: %+v", boardFake(m).acts())
	}
	if got := noticeRow(m); got != "no ship of spark-search waits for a merge" {
		t.Errorf("the notice: %q", got)
	}
}

func TestXOnlyEverKills(t *testing.T) {
	st := boardFixture(t)
	m := boardShot(t, st, 80, 31, chooseCard("kindle-web"))
	pane := paneNamed(st, "kindle-web")
	m = pressOn(m, "x")
	box := consentText(m)
	if !strings.Contains(box, "tmux kill-pane -t "+pane) || strings.Contains(box, "merge") {
		t.Fatalf("x shows %q", box)
	}
	m = pressOn(m, "y", "y")
	if acts := boardFake(m).acts(); len(acts) != 1 || acts[0].RecID != "kill:"+pane {
		t.Fatalf("x sent %+v", acts)
	}
}

func TestTheBoxKeepsWhatItShowed(t *testing.T) {
	st := boardFixture(t)
	sh := mergeOf(t, st)
	m := pressOn(boardShot(t, st, 80, 31, chooseCard("kindle-web")), "p")
	shown := consentText(m)

	merged := st
	merged.Ships = &proto.Ships{At: demoAt}
	for _, s := range st.Ships.Ships {
		if s.Number == 4556 {
			s.Kind, s.Note, s.Stage, s.Merge = shipRunning, "merged · deploying", 3, nil
		}
		merged.Ships.Ships = append(merged.Ships.Ships, s)
	}
	m = newBoardState(t, m, merged)
	if got := consentText(m); got != shown {
		t.Fatalf("the box changed with the state:\n%q\n%q", shown, got)
	}
	m = pressOn(m, "y")
	want := proto.Act{RecID: "ship:" + sh.Repo + "#4556", Which: "secondary", Confirm: "y"}
	if acts := boardFake(m).acts(); len(acts) != 1 || acts[0] != want {
		t.Fatalf("y sent %+v, want %+v", acts, want)
	}

	two := st
	two.Ships = &proto.Ships{At: demoAt, Ships: slices.Clone(st.Ships.Ships)}
	kindle := st.Sessions[slices.IndexFunc(st.Sessions, func(s sample.Session) bool { return s.Name == "kindle-web" })]
	stuck := shipFor(kindle, 4557, shipStuck, "CI failed · tests", 1, time.Hour)
	two.Ships.Ships = append(two.Ships.Ships, stuck)
	m = boardShot(t, two, 80, 31, chooseCard("kindle-web"))
	if lead := m.shipsOf(kindle)[0]; lead.Number != 4557 {
		t.Fatalf("the leading ship is #%d", lead.Number)
	}
	if m = pressOn(m, "p"); m.pending != nil {
		t.Errorf("p merged a ship that does not lead: %v", m.pending)
	}
}

func TestAChangedKeySaysSoOnce(t *testing.T) {
	dir := t.TempDir()
	var clipped []string
	now := demoAt
	open := func() Model {
		return boardShot(t, boardFixture(t), 80, 31, func(m *Model) {
			m.opt.StateDir, m.viewPrefs = dir, loadViewPrefs(dir)
			m.opt.Clip = func(s string) { clipped = append(clipped, s) }
			m.opt.Now = func() time.Time { return now }
		})
	}
	m := open()
	for _, c := range []struct{ key, once, then string }{
		{"c", "c copies now: the last message, whole.", "copied: last message of spark-search · not checked here"},
		{"g", "g shows one chain now. esc shows all.", "spark-search is in no chain"},
		{"p", "p merges now: the pull request that waits for you.", "no ship of spark-search waits for a merge"},
	} {
		m = pressOn(m, c.key)
		if got := noticeRow(m); got != c.once {
			t.Errorf("the first %s says %q, want %q", c.key, got, c.once)
		}
		now = now.Add(noticeFor) // the first notice ends
		m = pressOn(m, c.key)
		if got := noticeRow(m); got == c.once || c.then != "" && got != c.then {
			t.Errorf("the second %s says %q, want %q", c.key, got, c.then)
		}
	}
	if want := "Lint fails on a file I did not change. I tried twice and stopped."; len(clipped) != 2 || clipped[0] != want {
		t.Errorf("c copied %q", clipped)
	}

	m = open()
	for _, k := range []string{"c", "g", "p"} {
		m = pressOn(m, k)
		if got := noticeRow(m); strings.Contains(got, " now") {
			t.Errorf("a new console says %q again on %s", got, k)
		}
	}

	m = pressOn(open(), "down", "x")
	if got := noticeRow(m); got != "x kills. p merges #4556." {
		t.Errorf("the first x on a card whose ship waits says %q", got)
	}
}

func TestEscNeverLeavesTheConsole(t *testing.T) {
	m := boardShot(t, boardFixture(t), 80, 31, func(m *Model) {
		m.opt.Layered = true
		m.filter = "flint"
		m.refresh()
	})
	next, cmd := key(m, "esc")
	if m = next.(Model); m.filter != "" || slices.ContainsFunc(cmdMsgs(cmd), func(msg tea.Msg) bool { _, ok := msg.(layerMsg); return ok }) {
		t.Fatalf("esc does not clear the search first: %q", m.filter)
	}
	next, cmd = key(m, "esc")
	if slices.ContainsFunc(cmdMsgs(cmd), func(msg tea.Msg) bool { _, ok := msg.(layerMsg); return ok }) || !next.(Model).boardShown() {
		t.Error("esc left the board")
	}
}

func TestTheNoticeIsGoneAfterItsTime(t *testing.T) {
	now := demoAt
	m := boardShot(t, boardFixture(t), 40, 34, func(m *Model) { m.opt.Now = func() time.Time { return now } })
	tick := m.notice("not copied: tmux drops it · tmux set -s set-clipboard on")
	if two := noticeRow(m); two != "not copied: tmux drops it · tmux set -s set-clipboard on" {
		t.Errorf("a long notice on a phone is not two whole lines: %q", two)
	}
	if !strings.Contains(ansi.Strip(m.render()), "● 4 need you") {
		t.Error("the header changed for a notice")
	}
	msgs := cmdMsgs(tick) // the tick ends the notice; it waits noticeFor
	if !slices.ContainsFunc(msgs, func(msg tea.Msg) bool { _, ok := msg.(noticeMsg); return ok }) {
		t.Fatalf("the notice's ticks are %T", msgs)
	}
	now = now.Add(noticeFor)
	next, _ := m.Update(noticeMsg{})
	if f := plainFrame(next.(Model)); strings.Contains(f, "not copied") {
		t.Errorf("the notice stays after %v:\n%s", noticeFor, f)
	}
}

func TestARefusedMergeKeepsItsReason(t *testing.T) {
	m := boardShot(t, boardFixture(t), 80, 31, func(m *Model) {
		m.opt.StateDir, m.viewPrefs = t.TempDir(), viewPrefs{} // the first p ever
		chooseCard("kindle-web")(m)
	})
	next, _ := m.Update(lostMsg{err: io.EOF, conn: m.conn})
	m = pressOn(next.(Model), "p")
	if m.pending != nil || !strings.Contains(m.flash, "is not connected") || !strings.Contains(noticeRow(m), "is not connected") {
		t.Errorf("the first p on a lost host: pending %v, flash %q, notice row %q", m.pending, m.flash, noticeRow(m))
	}
}

// goBoard is the board with a stoker step that waits for a go on
// hearth-api's pane.
func goBoard(t *testing.T, now *time.Time, edit func(*Model)) Model {
	t.Helper()
	st := boardFixture(t)
	hearth := st.Sessions[slices.IndexFunc(st.Sessions, func(s sample.Session) bool { return s.Name == "hearth-api" })]
	st.Stoker = &proto.StokerView{Mode: proto.StokerMode{On: true}, Run: []proto.StokerEntry{goFor(hearth, 2*time.Minute)}}
	return boardShot(t, st, 80, 31, func(m *Model) {
		m.opt.Now = func() time.Time { return *now }
		if edit != nil {
			edit(m)
		}
	})
}

var grantAct = proto.Act{RecID: "stoker:grant", Which: "primary", Confirm: "y", Text: "go-hearth-api"}

// TestTheBeltsGoArmsThenGives (owner, 2026-11-13: "Arm then send"): on
// Pocket the belt's y is a tap, so the first y arms the go and the card
// says what it does; a second y gives it, and any other key between them
// disarms it.
func TestTheBeltsGoArmsThenGives(t *testing.T) {
	now := demoAt
	pocket := func(m *Model) { chooseCard("hearth-api")(m); pocketOnly(m) }

	m := pressOn(goBoard(t, &now, pocket), "y")
	if !m.onPocket() {
		t.Fatal("the board is not on Pocket")
	}
	if len(boardFake(m).acts()) != 0 || m.pending != nil {
		t.Fatalf("one belt y gave the go: acts %+v, pending %v", boardFake(m).acts(), m.pending)
	}
	if f := noticeRow(m); !strings.Contains(f, "y again gives the go: merge #4556") {
		t.Errorf("the armed go does not say what it does: %q", f)
	}
	if v := m.beltValue(); !strings.Contains(v, "y*") {
		t.Errorf("the belt does not light the armed y: %q", v)
	}
	if m = pressOn(m, "y"); !slices.Equal(boardFake(m).acts(), []proto.Act{grantAct}) {
		t.Fatalf("the second y sent %+v", boardFake(m).acts())
	}

	answered := func(m Model, k string) Model { // the app answers each new board= value
		m = pressOn(m, k)
		m.belt.pocket[0].n = m.belt.n
		return m
	}
	for _, between := range []string{"c", "g", "esc"} {
		now = demoAt
		m = answered(answered(answered(goBoard(t, &now, pocket), "y"), between), "y")
		if !m.onPocket() {
			t.Fatalf("y %s y: the board left Pocket", between)
		}
		if len(boardFake(m).acts()) != 0 {
			t.Errorf("y %s y gave the go: %+v", between, boardFake(m).acts())
		}
	}
}

// TestAnyTapDisarmsTheBeltsGo: on Pocket a tap anywhere disarms an armed
// go, not only a tap on a card; and y on a card with no go arms nothing.
func TestAnyTapDisarmsTheBeltsGo(t *testing.T) {
	now := demoAt
	pocket := func(m *Model) { chooseCard("hearth-api")(m); pocketOnly(m) }
	for name, tapAway := range map[string]func(Model) Model{
		"the header":  func(m Model) Model { next, _ := tap(m, m.width-1, 0); return next.(Model) },
		"a blank row": func(m Model) Model { next, _ := tap(m, m.width-1, m.height-2); return next.(Model) },
	} {
		now = demoAt
		m := pressOn(goBoard(t, &now, pocket), "y")
		if m.goArmed == (paneKey{}) {
			t.Fatalf("%s: the first y armed nothing", name)
		}
		m = tapAway(m)
		m.belt.pocket[0].n = m.belt.n // the app answers the new board= value
		if m.goArmed != (paneKey{}) {
			t.Errorf("a tap on %s kept the go armed", name)
		}
		if m = pressOn(m, "y"); len(boardFake(m).acts()) != 0 {
			t.Errorf("y, a tap on %s, y gave the go: %+v", name, boardFake(m).acts())
		}
	}

	now = demoAt
	m := pressOn(goBoard(t, &now, func(m *Model) { chooseCard("kindle-web")(m); pocketOnly(m) }), "y")
	if m.goArmed != (paneKey{}) || m.noticeText != "no step waits for your go on kindle-web" {
		t.Errorf("y on a card with no go: armed %v, notice %q", m.goArmed, m.noticeText)
	}
}

func TestCtrlOGoesIntoTheChosenCard(t *testing.T) {
	st := boardFixture(t)
	var ran []string
	m := boardShot(t, st, 80, 31, func(m *Model) {
		chooseCard("hearth-api")(m)
		m.opt.Run = func(argv []string) error { ran = argv; return nil }
	})
	next, cmd := key(m, "ctrl+o")
	fed(next, cmd)
	if want := "tmux switch-client -t " + paneNamed(st, "hearth-api"); strings.Join(ran, " ") != want {
		t.Errorf("ctrl+o ran %q, want %q", ran, want)
	}
}

// pocketOnly makes the app the only viewer, one that answered the board's
// keys with a mark.
func pocketOnly(m *Model) {
	m.viewers, m.belt.value, m.belt.n = "/dev/pts/7:105@here", ";", 1
	m.belt.pocket = []pocketClient{{"/dev/pts/7:105", 1}}
}

// TestPocketSeesANewerService: Pocket has no key line, so the notice row
// says that the service is newer, and a tap on it starts the console again.
func TestPocketSeesANewerService(t *testing.T) {
	st := boardFixture(t)
	for _, w := range []int{40, 44} {
		m := boardShot(t, st, w, 34, func(m *Model) {
			pocketOnly(m)
			m.newer, m.startsAgain = "R start again · service v0.43.0, console v0.42.0", true
			m.opt.Restart = func() error { return nil }
		})
		if !m.onPocket() {
			t.Fatal("the board is not on Pocket")
		}
		if f := strings.Join(strings.Fields(plainFrame(m)), " "); !strings.Contains(f, "R start again · service v0.43.0, console v0.42.0") || strings.Contains(f, "esc R") {
			t.Fatalf("%d: Pocket in the list does not say R starts again:\n%s", w, plainFrame(m))
		}
		next := tapText(t, m, "R start again")
		if !next.restarting {
			t.Errorf("%d: a tap on the notice did not start the console again", w)
		}
		m.toLine() // R types in the line: the notice names esc first
		if f := strings.Join(strings.Fields(plainFrame(m)), " "); !strings.Contains(f, "esc R start again · service v0.43.0, console v0.42.0") {
			t.Fatalf("%d: Pocket in the line does not name esc:\n%s", w, plainFrame(m))
		}
		if next = tapText(t, m, "esc R start again"); !next.restarting {
			t.Errorf("%d: a tap on the notice in the line did not start the console again", w)
		}
	}
}

// boardSaid are the board= values in what cmd writes to the terminal.
func boardSaid(cmd tea.Cmd) []string {
	var got []string
	for _, r := range raws(cmd) {
		if _, v, ok := strings.Cut(r, "matchblox;board="); ok {
			got = append(got, v[:strings.Index(v, "\x1b")])
		}
	}
	return got
}

// TestPocketHasNoKeyLine (spec "The belt on Pocket"): on Pocket the cards
// take the key line's two rows; a frame
// that is not Pocket's, the menu and the consent box keep the key line.
func TestPocketHasNoKeyLine(t *testing.T) {
	st := boardFixture(t)
	desk := boardShot(t, st, 40, 34, chooseCard("kindle-web"))
	pocket := boardShot(t, st, 40, 34, func(m *Model) { chooseCard("kindle-web")(m); pocketOnly(m) })
	if desk.onPocket() || !pocket.onPocket() {
		t.Fatalf("onPocket: desk %v, pocket %v", desk.onPocket(), pocket.onPocket())
	}
	ds, ps := desk.draw(40), pocket.draw(40)
	if ds.keysAt != len(ds.lines)-1 || !strings.Contains(ansi.Strip(ds.lines[ds.keysAt]), "in the list") {
		t.Errorf("the desk's last line is %q, want the key line", ansi.Strip(ds.lines[len(ds.lines)-1]))
	}
	if ps.keysAt != -1 {
		t.Errorf("Pocket: key line at %d of %d lines; want no key line", ps.keysAt, len(ps.lines))
	}
	if strings.Contains(ansi.Strip(ps.lines[len(ps.lines)-2]), "─────") {
		t.Errorf("Pocket keeps the key line's hairline: %q", ansi.Strip(ps.lines[len(ps.lines)-2]))
	}
	if pocket.bodyRows() != desk.bodyRows()+2 {
		t.Errorf("Pocket's body has %d rows, the desk's %d: want two more", pocket.bodyRows(), desk.bodyRows())
	}
	if n := len(frameLines(pocket)); n != 34 {
		t.Errorf("Pocket's frame has %d lines, want 34", n)
	}
	for name, m := range map[string]Model{"the menu": pressOn(pocket, "m"), "the consent box": pressOn(pocket, "p")} {
		if sc := m.draw(40); sc.keysAt != len(sc.lines)-1 || m.bodyRows() != desk.bodyRows() {
			t.Errorf("%s on Pocket: key line at %d of %d lines, body %d rows; want the key line kept", name, sc.keysAt, len(sc.lines), m.bodyRows())
		}
	}
	both := pocket
	both.viewers = "/dev/pts/2:90@here,/dev/pts/7:105@here"
	if both.onPocket() || both.draw(40).keysAt < 0 {
		t.Error("with a desk also viewing, the key line goes")
	}
	off := pocket
	off.tab = tabHistory
	if off.draw(40).keysAt < 0 {
		t.Error("with the board off, Pocket loses the key line")
	}
}

// TestPocketIsTheAppThatAnswered (controller ruling 21): onPocket holds
// while every viewer of the session answered the board's keys with the
// belt's answer (BeltKey, which tmux hands on with its tty) since it attached. Any other mark
// is no answer: an old app keeps its key line. A desk that joins, a
// terminal that comes, a new client on the same tty, or a value left
// unanswered ends it.
func TestPocketIsTheAppThatAnswered(t *testing.T) {
	st := boardFixture(t)
	clock := demoAt
	m := boardShot(t, st, 40, 34, func(m *Model) { m.opt.Now = func() time.Time { return clock } })
	update := func(msgs ...tea.Msg) {
		t.Helper()
		for _, msg := range msgs {
			next, _ := m.Update(msg)
			m = next.(Model)
		}
	}
	mark := func(tty string) tea.Msg { return uv.UnknownApcEvent(keycmd.MarkedPrefix + tty + "\x1b\\") }
	answer := uv.UnknownApcEvent(keycmd.BeltFromPrefix + "/dev/pts/7\x1b\\")
	viewers := func(key string) tea.Msg { return graphicsMsg{gen: m.opt.Gen, watched: true, key: key} }

	update(viewers("/dev/pts/7:105@here"))
	if m.onPocket() {
		t.Fatal("Pocket before the app answered")
	}
	update(mark("/dev/pts/7"), keyPress("down"))
	if m.onPocket() {
		t.Fatal("a mark alone (an old app's key) made the console Pocket")
	}
	update(uv.UnknownApcEvent(keycmd.BeltKey))
	if m.onPocket() {
		t.Fatal("the app's answer with no client named made the console Pocket")
	}
	update(answer)
	if !m.onPocket() {
		t.Fatal("the app answered the board's keys: not on Pocket")
	}

	// A new value: the answer to the one before holds for beltAnswerWait.
	update(keyPress("m"))
	if m.belt.value != "menu" || !m.onPocket() {
		t.Fatalf("value %q: the key line comes back while the app's answer is on its way", m.belt.value)
	}
	clock = clock.Add(beltAnswerWait)
	if m.onPocket() {
		t.Error("the app did not answer the menu's value, yet the console stays on Pocket")
	}
	update(answer)
	if !m.onPocket() {
		t.Error("the app answered again: not on Pocket")
	}
	update(keyPress("esc"), keyPress("m"))
	if m.onPocket() {
		t.Error("two values went unanswered, yet the console stays on Pocket")
	}
	update(answer)

	update(uv.UnknownApcEvent(keycmd.ViewerKey))
	if m.onPocket() {
		t.Error("a terminal came, yet the console stays on Pocket before the app answers again")
	}
	update(answer)
	if !m.onPocket() {
		t.Error("the app answered after a terminal came: not on Pocket")
	}
	update(viewers("/dev/pts/2:90@here,/dev/pts/7:105@here"), answer) // the app answers the value sent again
	if m.onPocket() {
		t.Error("a desk joined, yet the console is on Pocket")
	}
	update(viewers("/dev/pts/7:105@away"))
	if !m.onPocket() {
		t.Error("the desk left and the app stays: not on Pocket")
	}
	update(viewers("/dev/pts/7:200@here"))
	if m.onPocket() {
		t.Error("a new client on the app's tty is on Pocket before it answered")
	}

	off := boardShot(t, st, 40, 34, func(m *Model) { m.tab = tabHistory })
	next, _ := off.Update(viewers("/dev/pts/7:105@here"))
	next, _ = next.Update(answer)
	if next.(Model).onPocket() {
		t.Error("an answer while the board is off answered no board signal")
	}

	old := boardShot(t, st, 40, 34, func(m *Model) { m.opt.LocalTmux = "3.2" })
	next, _ = old.Update(viewers("/dev/pts/7:105@here"))
	next, _ = next.Update(answer)
	if next.(Model).onPocket() {
		t.Error("below tmux 3.3 the signal does not pass, yet the console is on Pocket")
	}
}

func beltFaces(ks []beltKey) []string {
	var out []string
	for _, k := range ks {
		out = append(out, k.Face)
	}
	return out
}

func beltKeyOf(t *testing.T, ks []beltKey, face string) beltKey {
	t.Helper()
	for _, k := range ks {
		if k.Face == face {
			return k
		}
	}
	t.Fatalf("no %s on the belt: %v", face, beltFaces(ks))
	return beltKey{}
}

// TestBeltKeysFollowTheChosenCard (design decision 10): home is ◂, the
// chosen session's keys, ↑ ↓ ⏎; the tail has the keys of the list.
func TestBeltKeysFollowTheChosenCard(t *testing.T) {
	tail := []string{"⌃o", "s", "g", "/", "m", "c", "z", "N", "⇥", "x", "?", ",", "t", "D"}
	st := boardFixture(t)

	m, _ := flintBoard(t, 80, 34)
	ks := m.beltKeys()
	if want := append([]string{"◂", "1", "2", "↑", "↓", "⏎"}, tail...); !slices.Equal(beltFaces(ks), want) {
		t.Fatalf("flint-auth's belt is %v, want %v", beltFaces(ks), want)
	}
	for i, n := range []string{"1", "2"} {
		if k := ks[1+i]; k != (beltKey{Key: n, Face: n, Word: "answer", On: true, Act: true}) {
			t.Errorf("key %d is %+v, want %s answer in the accent", 1+i, k, n)
		}
	}
	if g := beltKeyOf(t, ks, "g"); !g.On || g.Word != "chain" {
		t.Errorf("flint-auth is in chain login, yet g is %+v", g)
	}
	for _, k := range ks[3:] {
		if k.Act {
			t.Errorf("%s is in the accent: only the card's keys are", k.Face)
		}
	}

	kindle := boardShot(t, st, 80, 34, chooseCard("kindle-web"))
	ks = kindle.beltKeys()
	if want := append([]string{"◂", "p", "↑", "↓", "⏎"}, tail...); !slices.Equal(beltFaces(ks), want) {
		t.Fatalf("kindle-web's belt is %v, want %v", beltFaces(ks), want)
	}
	if p := ks[1]; p != (beltKey{Key: "p", Face: "p", Word: "merge", On: true, Act: true}) {
		t.Errorf("kindle-web's ship waits, yet its key is %+v", p)
	}
	if g := beltKeyOf(t, ks, "g"); g.On {
		t.Errorf("kindle-web is in no chain, yet g is %+v", g)
	}
	words := map[string]string{"⌃o": "expand", "c": "copy", "m": "menu", "g": "chain", "s": "hosts", "⇥": "machine", "x": "kill", "z": "park", "N": "new", "?": "keys", ",": "machine", "t": "shell", "D": "diff"}
	for face, word := range words {
		if k := beltKeyOf(t, ks, face); k.Word != word {
			t.Errorf("%s says %q, want %q", face, k.Word, word)
		}
	}

	box := pressOn(kindle, "p")
	want := []beltKey{{Face: "◂", On: true}, {Key: "y", Face: "y", Word: "runs", On: true, Act: true}, {Key: "esc", Face: "esc", Word: "cancels", On: true}}
	if got := box.beltKeys(); !slices.Equal(got, want) {
		t.Errorf("with the consent box up the belt is %+v, want %+v", got, want)
	}
}

// TestBeltFollowsTheFocus: the belt shows what acts. With the line empty
// the digits answer and ↑ ↓ choose, in the line and in the list. While
// the line holds text a digit types, so the digits are off; in the line
// ↑ ↓ are off too, and in the list they choose.
func TestBeltFollowsTheFocus(t *testing.T) {
	m, _ := flintBoard(t, 80, 34)
	m.toLine()
	if got := m.beltValue(); got != "1,2;" {
		t.Errorf("the line, empty: %q, want 1,2;", got)
	}
	typed := pressOn(m, "h")
	if typed.lines[typed.chosenSide()] != "h" {
		t.Fatalf("the line holds %q, want h", typed.lines[typed.chosenSide()])
	}
	if got := typed.beltValue(); got != ";1,2,up,down" {
		t.Errorf("the line holds text: %q, want ;1,2,up,down", got)
	}
	list := typed
	list.toList()
	if got := list.beltValue(); got != ";1,2" {
		t.Errorf("the list, a draft in the line: %q, want ;1,2", got)
	}
	m.toList()
	if got := m.beltValue(); got != "1,2;" {
		t.Errorf("the list, the line empty: %q, want 1,2;", got)
	}
}

// TestTheBeltSaysWhenTheChatTakesTheScreen: where one pane shows, the
// value ends in ;chat while the chat takes the screen, so Pocket's hold
// selects text there and opens the row's menu only on the list (#485).
// Side by side the list always shows: no flag.
func TestTheBeltSaysWhenTheChatTakesTheScreen(t *testing.T) {
	phone, _ := flintBoard(t, 40, 34)
	phone.toLine()
	if got := phone.beltValue(); got != "1,2;;chat" {
		t.Errorf("one pane, the chat: %q, want 1,2;;chat", got)
	}
	machine := phone
	machine.setTab(tabMachine)
	if got := machine.beltValue(); strings.HasSuffix(got, ";chat") {
		t.Errorf("Machine after the chat: %q, want no ;chat: Machine shows its rows", got)
	}
	phone.toList()
	if got := phone.beltValue(); got != "1,2;" {
		t.Errorf("one pane, the list: %q, want 1,2;", got)
	}
	desk, _ := flintBoard(t, 80, 34)
	desk.toLine()
	if got := desk.beltValue(); got != "1,2;" {
		t.Errorf("side by side, the line: %q, want 1,2;", got)
	}
}

// TestTheBeltSignalSaysTheBoardsKeys: the console tells the app the
// chosen card's keys, an armed one with *, and the dim keys after ;; box
// and menu while they show; "" when it leaves the board. It says a value
// once, and again to new viewers.
func TestTheBeltSignalSaysTheBoardsKeys(t *testing.T) {
	st := boardFixture(t)
	value := func(m Model) string { t.Helper(); return m.beltValue() }

	empty := st
	empty.Sessions, empty.Queue, empty.Ships, empty.Chains = nil, nil, &proto.Ships{At: demoAt}, nil
	if got := value(boardShot(t, empty, 80, 34, nil)); got != ";ctrl+o,s,g,m,c,z,x,t,D" {
		t.Errorf("no card chosen: %q", got)
	}

	m, clock := flintBoard(t, 80, 34)
	if got := value(m); got != "1,2;" {
		t.Errorf("flint-auth: %q, want %q", got, "1,2;")
	}
	m = pressOn(m, "2")
	*clock = clock.Add(time.Millisecond)
	if got := value(m); got != "1,2*;" {
		t.Errorf("flint-auth with 2 armed: %q, want %q", got, "1,2*;")
	}

	kindle := boardShot(t, st, 80, 34, chooseCard("kindle-web"))
	if got := value(kindle); got != "p;g" {
		t.Errorf("kindle-web, its ship waits and it is in no chain: %q", got)
	}
	if got := value(pressOn(kindle, "m")); got != "menu" {
		t.Errorf("the menu open: %q", got)
	}
	if got := value(pressOn(kindle, "p")); got != "box" {
		t.Errorf("the consent box up: %q", got)
	}
	// Only the box that takes a typed y and esc is box; the take-over
	// question and the scan's consent are home with no card keys, and keep
	// the key line on Pocket.
	held := kindle
	held.held = func(m Model) (tea.Model, tea.Cmd) { return m, nil }
	scan := kindle
	scan.scanAsk, scan.found = &scanAsk{}, &proto.Found{}
	for name, c := range map[string]Model{"the take-over question": held, "the scan's consent": scan} {
		if !c.consenting() {
			t.Fatalf("%s does not show", name)
		}
		if got := value(c); got != ";ctrl+o,s,g,/,m,c,z,N,tab,x,?,comma,t,D" {
			t.Errorf("%s: %q, want the home with no card keys", name, got)
		}
		if ks := c.beltKeys(); slices.ContainsFunc(ks, func(k beltKey) bool { return k.Act }) {
			t.Errorf("%s: card keys on the belt: %v", name, beltFaces(ks))
		}
		pocketOnly(&c)
		if c.draw(80).keysAt < 0 {
			t.Errorf("%s on Pocket: no key line", name)
		}
	}

	// A card on another host takes its digits too: the pick goes on the
	// host's own conn.
	far := st
	far.Sessions = slices.Clone(st.Sessions)
	far.Queue = slices.Clone(st.Queue)
	for i := range far.Sessions {
		if far.Sessions[i].Name == "flint-auth" {
			far.Sessions[i].Host = "gpu-1"
		}
	}
	for i := range far.Queue {
		if far.Queue[i].Name == "flint-auth" {
			far.Queue[i].Host = "gpu-1"
		}
	}
	if got := value(boardShot(t, far, 80, 34, chooseCard("flint-auth"))); !strings.HasPrefix(got, "1,2;") {
		t.Errorf("flint-auth on gpu-1: %q, want its digits live", got)
	}

	off := kindle
	off.tab = tabHistory
	if got := value(off); got != "" {
		t.Errorf("the board off: %q", got)
	}

	// Sent once for each change, through the agent signal's way.
	next, cmd := kindle.Update(noticeMsg{})
	if got := raws(cmd); !slices.Contains(got, logos.Passthrough(keycmd.BoardSignal("p;g"))) {
		t.Fatalf("the first frame wrote %q, want board=p;g through tmux", got)
	}
	next, cmd = next.Update(noticeMsg{})
	if got := boardSaid(cmd); len(got) != 0 {
		t.Errorf("an unchanged value went again: %q", got)
	}
	next, cmd = next.Update(graphicsMsg{gen: kindle.opt.Gen, watched: true, key: "/dev/pts/7:105@here"})
	if got := boardSaid(cmd); !slices.Equal(got, []string{"p;g"}) {
		t.Errorf("new viewers got %q, want p;g", got)
	}
	next, cmd = next.Update(uv.UnknownApcEvent(keycmd.ViewerKey))
	if got := boardSaid(cmd); !slices.Equal(got, []string{"p;g"}) {
		t.Errorf("a terminal that came got %q, want p;g", got)
	}
	nm := next.(Model)
	nm.tab = tabHistory
	next, cmd = nm.Update(noticeMsg{})
	if got := boardSaid(cmd); !slices.Equal(got, []string{""}) {
		t.Errorf("leaving the board wrote %q, want one empty value", got)
	}
	for range 2 {
		if next, cmd = next.Update(noticeMsg{}); len(boardSaid(cmd)) != 0 {
			t.Errorf("the board is off, yet the console wrote %q", boardSaid(cmd))
		}
	}
	// Every console opens on the board now (Task 12 step 1): none is
	// without one.
}

// TestTheBeltValueKeepsItsLimit: a value never passes beltMax bytes; the
// dim keys leave whole, from the right, and the card's keys stay.
func TestTheBeltValueKeepsItsLimit(t *testing.T) {
	ks := []beltKey{{Face: "◂", On: true}, {Key: "1", Act: true, On: true}, {Key: "2", Act: true, On: true}, {Key: "3", Act: true, On: true}}
	for i := range 20 {
		ks = append(ks, beltKey{Key: fmt.Sprintf("long%02d", i)})
	}
	got := beltWord(ks, "3", false)
	if len(got) > beltMax {
		t.Fatalf("%q has %d bytes, more than %d", got, len(got), beltMax)
	}
	card, dims, ok := strings.Cut(got, ";")
	if !ok || card != "1,2,3*" {
		t.Fatalf("%q: want the card's keys 1,2,3* before ;", got)
	}
	for i, d := range strings.Split(dims, ",") {
		if d != fmt.Sprintf("long%02d", i) {
			t.Fatalf("dim key %d is %q in %q: want whole keys, in order", i, d, got)
		}
	}
	if chat := beltWord(ks, "3", true); len(chat) > beltMax || !strings.HasSuffix(chat, ";chat") {
		t.Errorf("%q: want at most %d bytes, ending in ;chat", chat, beltMax)
	}
	if beltWord(ks, "", false) == beltWord(ks, "3", false) {
		t.Error("the armed key does not show")
	}
}

// TestPocketExpiresWhenTheBeltLeavesBoard (controller ruling 22): while
// on Pocket, each viewers poll sends the value again as a new value; an
// app that answers keeps Pocket, one whose belt left Board stops
// answering and loses it within one poll plus beltAnswerWait, with no
// state change. With a desk viewing nothing is sent again.
func TestPocketExpiresWhenTheBeltLeavesBoard(t *testing.T) {
	st := boardFixture(t)
	clock := demoAt
	m := boardShot(t, st, 40, 34, func(m *Model) { m.opt.Now = func() time.Time { return clock } })
	var cmd tea.Cmd
	update := func(msgs ...tea.Msg) {
		t.Helper()
		for _, msg := range msgs {
			var next tea.Model
			next, cmd = m.Update(msg)
			m = next.(Model)
		}
	}
	phone := "/dev/pts/7:105@here"
	poll := func(key string) tea.Msg { return graphicsMsg{gen: m.opt.Gen, watched: true, key: key} }
	answer := uv.UnknownApcEvent(keycmd.BeltFromPrefix + "/dev/pts/7\x1b\\")
	update(poll(phone), answer)
	if !m.onPocket() {
		t.Fatal("the app answered: not on Pocket")
	}
	value := m.belt.value

	for range 3 { // an app on Board answers every poll's value
		clock = clock.Add(graphicsEvery)
		update(poll(phone))
		if got := boardSaid(cmd); !slices.Equal(got, []string{value}) {
			t.Fatalf("a poll on Pocket wrote %q, want %q again", got, value)
		}
		update(answer)
		if !m.onPocket() {
			t.Fatal("the app answers each value, yet the console left Pocket")
		}
	}

	// The belt leaves Board: no answer, and nothing on the board changes.
	clock = clock.Add(graphicsEvery)
	update(poll(phone))
	if len(boardSaid(cmd)) != 1 || !m.onPocket() {
		t.Fatalf("the poll wrote %q; on Pocket %v while the answer may come", boardSaid(cmd), m.onPocket())
	}
	clock = clock.Add(beltAnswerWait)
	if m.onPocket() {
		t.Fatal("no answer for a poll plus the grace, yet the console stays on Pocket")
	}
	clock = clock.Add(graphicsEvery)
	if update(poll(phone)); len(boardSaid(cmd)) != 0 {
		t.Errorf("off Pocket, a poll sent the unchanged value: %q", boardSaid(cmd))
	}

	// A late answer to a value counts for the next, but not past one poll.
	update(answer)
	if !m.onPocket() {
		t.Fatal("the app answered again: not on Pocket")
	}
	clock = clock.Add(graphicsEvery)
	update(poll(phone)) // sent again: the app is gone from Board and says nothing
	clock = clock.Add(beltAnswerWait / 2)
	update(answer) // late: it answers the value before, and counts for this one
	clock = clock.Add(graphicsEvery)
	update(poll(phone))
	clock = clock.Add(beltAnswerWait)
	if m.onPocket() {
		t.Error("a late answer held Pocket past the next poll")
	}

	desk := boardShot(t, st, 40, 34, func(m *Model) { m.opt.Now = func() time.Time { return clock } })
	next, _ := desk.Update(poll("/dev/pts/2:90@here," + phone))
	next, _ = next.Update(answer)
	clock = clock.Add(graphicsEvery)
	if _, c := next.Update(poll("/dev/pts/2:90@here," + phone)); len(boardSaid(c)) != 0 {
		t.Errorf("with a desk viewing, a poll sent the unchanged value: %q", boardSaid(c))
	}
}

// TestATerminalThatComesWaitsForTheViewers: on ViewerKey the console asks
// who views it at once, and counts no answer until that reply came; then
// every viewer must answer.
func TestATerminalThatComesWaitsForTheViewers(t *testing.T) {
	st := boardFixture(t)
	viewers := "/dev/pts/7:105@here"
	m := boardShot(t, st, 40, 34, func(m *Model) {
		m.opt.Graphics = func() (bool, string, bool) { return false, viewers, true }
	})
	answer := uv.UnknownApcEvent(keycmd.BeltFromPrefix + "/dev/pts/7\x1b\\")
	update := func(msg tea.Msg) tea.Cmd {
		t.Helper()
		next, cmd := m.Update(msg)
		m = next.(Model)
		return cmd
	}
	update(graphicsMsg{gen: m.opt.Gen, watched: true, key: viewers})
	update(answer)
	if !m.onPocket() {
		t.Fatal("the app answered: not on Pocket")
	}

	viewers = "/dev/pts/2:90@here,/dev/pts/7:105@here" // a desk attaches
	cmd := update(uv.UnknownApcEvent(keycmd.ViewerKey))
	var asked []graphicsMsg
	for _, msg := range cmdMsgs(cmd) {
		if g, ok := msg.(graphicsMsg); ok {
			asked = append(asked, g)
		}
	}
	if len(asked) != 1 {
		t.Fatalf("a terminal came, yet the console did not ask who views it: %v", cmdMsgs(cmd))
	}
	update(answer) // the app answers at once, before the viewers are known
	if m.onPocket() {
		t.Fatal("the app answered before the viewers were known: on Pocket with a desk there")
	}
	update(asked[0])
	update(answer)
	if m.onPocket() || m.draw(40).keysAt < 0 {
		t.Fatal("the desk views the console, yet the key line is gone")
	}

	viewers = "/dev/pts/7:105@here" // the desk leaves; the app came again
	cmd = update(uv.UnknownApcEvent(keycmd.ViewerKey))
	for _, msg := range cmdMsgs(cmd) {
		if g, ok := msg.(graphicsMsg); ok {
			update(g)
		}
	}
	update(answer)
	if !m.onPocket() {
		t.Error("only the app views the console and it answered: not on Pocket")
	}
}

// TestATapOnPocketChoosesTheCard: Pocket is worked by touch alone, so a
// tap on a card chooses it there as on a desk.
func TestATapOnPocketChoosesTheCard(t *testing.T) {
	for _, w := range []int{40, 132} {
		m := boardShot(t, boardFixture(t), w, 34, pocketOnly)
		if !m.beltOnly() {
			t.Fatalf("%d: the fixture is not on Pocket", w)
		}
		lines := strings.Split(plainFrame(m), "\n")
		y := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, " flint-auth ") })
		if y < 0 || chosenName(t, m) == "flint-auth" {
			t.Fatalf("%d: flint-auth is not an unchosen card on the frame:\n%s", w, strings.Join(lines, "\n"))
		}
		x := strings.Index(lines[y], "flint-auth")
		next, cmd := tap(m, len([]rune(lines[y][:x]))+1, y)
		drain(cmd)
		if got := chosenName(t, next.(Model)); got != "flint-auth" {
			t.Errorf("%d: a tap on flint-auth chose %s", w, got)
		}
	}
}

// TestATabsSpanPressesNoKey: a tap on a tab's name opens the tab, and the
// span says so without a key: digits answer a card's question on the
// board (ruling 37), so no tab span may press one.
func TestATabsSpanPressesNoKey(t *testing.T) {
	m := boardShot(t, boardFixture(t), 132, 34, nil)
	sc := m.draw(132)
	machine := -1
	for i, s := range sc.tabs {
		if len(s.press) == 1 && s.press >= "0" && s.press <= "9" {
			t.Errorf("tab span %d presses the digit %q", i, s.press)
		}
		if s.press == tabPress(tabMachine) {
			machine = i
		}
	}
	if machine < 0 {
		t.Fatalf("no span opens machine: %+v", sc.tabs)
	}
	next, cmd := tap(m, sc.tabs[machine].x0, 1)
	drain(cmd)
	if nm := next.(Model); nm.tab != tabMachine || len(boardFake(nm).acts()) != 0 {
		t.Errorf("a tap on machine: tab %d, sent %+v", nm.tab, boardFake(nm).acts())
	}
}
