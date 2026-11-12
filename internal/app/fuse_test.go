package app

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// fuseState is the demo machine as the mockup draws it: kindle-web asks,
// hearth-api and flint-auth work, ember-billing and spark-docs are quiet.
// Each list of names is one chain.
func fuseState(t *testing.T, chained ...[]string) proto.State {
	t.Helper()
	st := screensState(t)
	keep := []string{"kindle-web", "hearth-api", "ember-billing", "flint-auth", "spark-docs"}
	st.Sessions = slices.DeleteFunc(st.Sessions, func(s sample.Session) bool { return !slices.Contains(keep, s.Name) })
	pane := map[string]string{}
	for i := range st.Sessions {
		s := &st.Sessions[i]
		pane[s.Name] = s.Pane
		s.Do, s.Shells, s.Agents, s.Progress, s.Ask = "", nil, 0, nil, nil
		switch s.Name {
		case "hearth-api", "flint-auth":
			s.Busy, s.Status, s.Idle = true, "busy", 0
		case "ember-billing":
			s.Busy, s.Status, s.Idle = false, "idle", 3*time.Minute
		case "spark-docs":
			s.Busy, s.Status, s.Idle = false, "idle", 9*time.Minute
		}
	}
	st.Queue = slices.DeleteFunc(st.Queue, func(it queue.Item) bool { return it.Pane != pane["kindle-web"] })
	st.Ships, st.Children = nil, nil
	st.Chains = &state.ChainsView{}
	for i, names := range chained {
		c := chains.Chain{ID: "c" + string(rune('1'+i)), Name: fuseTeams[i], Repo: "/home/user/src/hearth"}
		for _, n := range names {
			for _, s := range st.Sessions {
				if s.Name == n {
					c.Members = append(c.Members, memberOf(s))
				}
			}
		}
		st.Chains.Chains = append(st.Chains.Chains, c)
	}
	return st
}

// fuseTeams name the chains of fuseState, in order.
var fuseTeams = []string{"hearth", "web", "docs"}

// listLines are the lines of the Sessions list w wide, plain, each row cut
// to n cells: the mark, the match, the fuse and the name. A group's label
// is whole.
func listLines(m Model, w, n int) []string {
	var b body
	m.sessionRows(&b, w)
	var out []string
	for _, l := range b.lines {
		l = ansi.Strip(l)
		if r := []rune(l); len(r) < 2 || !unicode.IsLetter(r[1]) && !unicode.IsDigit(r[1]) {
			l = ansi.Truncate(l, n, "")
		}
		out = append(out, strings.TrimRight(l, " "))
	}
	return out
}

// fuseModel is the console on fuseState, on the demo's still clock, with
// the session named pick picked.
func fuseModel(t *testing.T, width int, st proto.State, pick string) Model {
	t.Helper()
	var next tea.Model = New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }})
	next, _ = next.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	next, _ = next.Update(stateMsg(st))
	m := next.(Model)
	m.tab = tabSessions
	if pick != "" {
		for _, s := range m.snap.Sessions {
			if s.Name == pick {
				choose(&m, s.PID)
			}
		}
	}
	return m
}

func sameLines(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) < len(want) || !slices.Equal(got[:len(want)], want) {
		t.Fatalf("rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

var mockupChain = []string{"kindle-web", "hearth-api", "ember-billing"}

// TestFuseJoinsAChainUnderNeed: kindle-web waits, so its whole chain moves
// up with it; the label still says one waits.
func TestFuseJoinsAChainUnderNeed(t *testing.T) {
	m := fuseModel(t, 160, fuseState(t, mockupChain), "")
	sameLines(t, listLines(m, 100, 20), []string{
		" WAITS FOR YOU · 1 · chain of 3",
		"▌╿─╮ kindle-web",
		" ✦─┤ hearth-api",
		" ╿─╯ ember-billing",
		" WORKING · 1",
		" ✦   flint-auth",
		" QUIET · 1",
		" │   spark-docs",
	})
}

// TestFuseStubsInStatus: status splits the chain; each match keeps a stub,
// and selecting one lights its chain-mates'.
func TestFuseStubsInStatus(t *testing.T) {
	t.Skip("dropped (Task 12 step 4): the groupings go (intended change: the columns are the grouping)")
}

func pidOf(m Model, name string) int {
	for _, s := range m.snap.Sessions {
		if s.Name == name {
			return s.PID
		}
	}
	return 0
}

// TestFuseIsWholeInRepo: a chain in one repo has its matches side by side
// there, whatever the sort puts between them.
func TestFuseIsWholeInRepo(t *testing.T) {
	t.Skip("dropped (Task 12 step 4): the groupings go (intended change: the columns are the grouping)")
}

// TestFuseSplitsAcrossRepos: by repo, a chain across two repos keeps each
// match in its own repo's group, with a stub that lights with the selected
// chain-mate, on a desk and on a phone.
func TestFuseSplitsAcrossRepos(t *testing.T) {
	t.Skip("dropped (Task 12 step 4): the groupings go (intended change: the columns are the grouping)")
}

// TestTwoChainsNeverTouch: two chains next to each other each open with ╮
// and close with ╯.
func TestTwoChainsNeverTouch(t *testing.T) {
	m := fuseModel(t, 160, fuseState(t, []string{"kindle-web", "hearth-api"}, []string{"ember-billing", "spark-docs"}), "")
	sameLines(t, listLines(m, 100, 20), []string{
		" WAITS FOR YOU · 1 · chain of 2",
		"▌╿─╮ kindle-web",
		" ✦─╯ hearth-api",
		" WORKING · 1",
		" ✦   flint-auth",
		" QUIET · 2 · chain of 2",
		" ╿─╮ ember-billing",
		" │─╯ spark-docs",
	})
}

// TestAFilterSaysHowManyAreHidden: the fuse joins the matches the filter
// leaves, and the label counts the rest.
func TestAFilterSaysHowManyAreHidden(t *testing.T) {
	t.Skip("replaced by TestTheCountSeesThroughEveryFilter (Task 12 step 1)")
	st := fuseState(t, mockupChain)
	for i := range st.Sessions {
		if st.Sessions[i].Name != "hearth-api" {
			st.Sessions[i].Tab = "chainlook"
		}
	}
	m := fuseModel(t, 160, st, "")
	m.filter = "chainlook"
	m.refresh()
	sameLines(t, listLines(m, 100, 20)[:3], []string{
		" WAITS FOR YOU · 1 · chain of 3 · 1 hidden",
		"▌╿─╮ kindle-web",
		" ╿─╯ ember-billing",
	})
}

// TestAFilterSaysHowManyAreHiddenInEveryGrouping: status and repo add no
// chain count, but a filter that hides chain-mates still says how many, on
// the group of the chain's first match in view.
func TestAFilterSaysHowManyAreHiddenInEveryGrouping(t *testing.T) {
	t.Skip("dropped (Task 12 step 1): the groupings go (step 4)")
}

// TestFuseOnAPhone: one column; the opened block carries the fuse and its
// hairlines start after it; the row that waits is whole.
func TestFuseOnAPhone(t *testing.T) {
	m := fuseModel(t, 44, fuseState(t, mockupChain), "hearth-api")
	got := listLines(m, 44, 44)
	if got[0] != " WAITS FOR YOU · 1 · chain of 3" || !strings.HasPrefix(got[1], " ╿╮ kindle-web ") {
		t.Fatalf("rows:\n%s", strings.Join(got, "\n"))
	}
	if !strings.HasPrefix(got[2], "  │──") || !strings.HasPrefix(got[3], "▌✦┤ hearth-api ") {
		t.Fatalf("the top hairline starts after the fuse:\n%s", strings.Join(got, "\n"))
	}
	i := 4
	for ; !strings.HasPrefix(got[i], "  │──"); i++ {
		if !strings.HasPrefix(got[i], "  │") {
			t.Fatalf("an opened line without the fuse: %q\n%s", got[i], strings.Join(got, "\n"))
		}
	}
	if i == 4 || !strings.HasPrefix(got[i+1], " ╿╯ ember-billing ") {
		t.Fatalf("rows:\n%s", strings.Join(got, "\n"))
	}
	row := got[1]
	for _, part := range []string{"kindle-web", "asks", "1m"} {
		if !strings.Contains(row, part) {
			t.Fatalf("the row that waits lacks %q: %q", part, row)
		}
	}
	for _, l := range got {
		if w := len([]rune(l)); w > 44 {
			t.Fatalf("a line is %d wide: %q", w, l)
		}
	}
}

// TestFuseRunsPastChildRows: the processes under a chain's match keep the
// fuse going to its next match.
func TestFuseRunsPastChildRows(t *testing.T) {
	st := fuseState(t, mockupChain)
	for _, s := range st.Sessions {
		if s.Name == "kindle-web" {
			parent := sample.ProcRef{PID: s.PID, Start: s.Start}
			st.Children = []sample.Session{{Host: s.Host, Name: "go test ./...", PID: 9001, Parent: &parent}}
		}
	}
	m := fuseModel(t, 160, st, "")
	sameLines(t, listLines(m, 100, 24), []string{
		" WAITS FOR YOU · 1 · chain of 3",
		"▌╿─╮ kindle-web",
		"   │ └─ go test ./...",
		" ✦─┤ hearth-api",
		" ╿─╯ ember-billing",
	})
}

// TestSameRepoHint: two matches in one repo that are not chained say so
// while the name has room; a chained row never does.
func TestSameRepoHint(t *testing.T) {
	st := fuseState(t)
	st.Git = &proto.GitReport{Repos: []gitscan.Repo{{Path: "/home/user/src/hearth"}}}
	for i := range st.Sessions {
		switch st.Sessions[i].Name {
		case "kindle-web", "hearth-api":
			st.Sessions[i].Cwd = "/home/user/src/hearth"
		default:
			st.Sessions[i].Cwd = "/home/user/src/" + st.Sessions[i].Name
		}
	}
	const hint = " · same repo · chain?"
	m := fuseModel(t, 160, st, "")
	var b body
	m.sessionRows(&b, 100)
	for _, name := range []string{"kindle-web", "hearth-api"} {
		if l := ansi.Strip(lineWith(b.lines, name)); !strings.Contains(l, name+hint) {
			t.Errorf("%s lacks the hint: %q", name, l)
		}
	}
	if l := ansi.Strip(lineWith(b.lines, "flint-auth")); strings.Contains(l, hint) {
		t.Errorf("flint-auth is alone in its repo: %q", l)
	}
	roomy := fuseModel(t, 56, st, "")
	if roomy.layoutAt(56) != Narrow {
		t.Fatalf("56 columns is not a phone")
	}
	b = body{}
	roomy.sessionRows(&b, 56)
	if l := ansi.Strip(lineWith(b.lines, "kindle-web")); !strings.Contains(l, "kindle-web"+hint) {
		t.Errorf("a phone with room for the name and the hint lacks it: %q", l)
	}
	phone := fuseModel(t, 44, st, "")
	b = body{}
	phone.sessionRows(&b, 44)
	if l := ansi.Strip(lineWith(b.lines, "kindle-web")); strings.Contains(l, "same repo") || !strings.Contains(l, "kindle-web ") {
		t.Errorf("no room, no hint, the name whole: %q", l)
	}
	st.Chains = fuseState(t, []string{"kindle-web", "hearth-api"}).Chains
	m = fuseModel(t, 160, st, "")
	b = body{}
	m.sessionRows(&b, 100)
	if l := ansi.Strip(lineWith(b.lines, "kindle-web")); strings.Contains(l, hint) {
		t.Errorf("a chained row has no hint: %q", l)
	}
	st.Chains = fuseState(t, []string{"kindle-web", "flint-auth"}).Chains
	m = fuseModel(t, 160, st, "")
	b = body{}
	m.sessionRows(&b, 100)
	for _, name := range []string{"kindle-web", "hearth-api"} {
		if l := ansi.Strip(lineWith(b.lines, name)); !strings.Contains(l, name+hint) {
			t.Errorf("chained in another repo, %s lacks the hint for its repo mate: %q", name, l)
		}
	}
}

// TestNoChainNoFuse: with no chain in the view the list is today's, byte
// for byte, wide and on a phone.
func TestNoChainNoFuse(t *testing.T) {
	rows := func(width, list int, cv *state.ChainsView) []string {
		st := fuseState(t)
		st.Chains = cv
		m := fuseModel(t, width, st, "hearth-api")
		var b body
		m.sessionRows(&b, list)
		return b.lines
	}
	got := append(rows(160, 100, nil), rows(44, 44, nil)...)
	path := filepath.Join("..", "..", "testdata", "screens", "nofuse.ansi")
	if *update {
		if err := os.WriteFile(path, []byte(strings.Join(got, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n"); !slices.Equal(got, want) {
		t.Fatalf("the list changed with no chain in it:\n%s", ansi.Strip(strings.Join(got, "\n")))
	}
	if empty := rows(160, 100, &state.ChainsView{}); !slices.Equal(empty, got[:len(empty)]) {
		t.Fatalf("an empty chains view draws a fuse:\n%s", ansi.Strip(strings.Join(empty, "\n")))
	}
}

// TestSparkPlaysOnceAndNotWithNoMotion: when a chain's spark moves, one
// bright cell runs down its fuse once; with NoMotion it never does.
func TestSparkPlaysOnceAndNotWithNoMotion(t *testing.T) {
	for _, still := range []bool{false, true} {
		now := demoAt
		var m tea.Model = New(Options{NoMotion: still, Conn: newFake(), Now: func() time.Time { return now }})
		m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
		send := func(spark time.Time) {
			st := fuseState(t, mockupChain)
			st.Chains.Chains[0].Spark = spark
			m, _ = m.Update(stateMsg(st))
		}
		send(now.Add(-time.Minute)) // the first state shows what was, and plays nothing
		lit := func() string {
			mm := m.(Model)
			var b body
			mm.sessionRows(&b, 100)
			var out []string
			for _, name := range mockupChain {
				l := lineWith(b.lines, name)
				for _, g := range []string{"─╮", "─┤", "─╯"} {
					if strings.Contains(l, mm.sparkStyle().Render(g)) {
						out = append(out, name)
					}
				}
			}
			return strings.Join(out, " ")
		}
		if got := lit(); got != "" {
			t.Fatalf("still %v: a spark on the first state: %q", still, got)
		}
		send(now)
		want := []string{"kindle-web", "hearth-api", "ember-billing", ""}
		if still {
			want = []string{"", "", "", ""}
		}
		for i, at := range []time.Duration{0, 250 * time.Millisecond, 450 * time.Millisecond, 700 * time.Millisecond} {
			now = demoAt.Add(at)
			if got := lit(); got != want[i] {
				t.Fatalf("still %v, at %v: the spark is on %q, want %q", still, at, got, want[i])
			}
		}
		now = demoAt.Add(800 * time.Millisecond)
		send(demoAt) // the same spark again plays nothing
		if got := lit(); got != "" {
			t.Fatalf("still %v: the spark played twice: %q", still, got)
		}
	}
}

// TestAChainWithOneMatchHereIsNoChain: a chain that names three sessions of
// which one is left in Sessions draws no fuse and no count; with two left
// it is a chain of those two.
func TestAChainWithOneMatchHereIsNoChain(t *testing.T) {
	st := fuseState(t, mockupChain)
	left := func(gone ...string) proto.State {
		out := st
		out.Sessions = slices.DeleteFunc(slices.Clone(st.Sessions), func(s sample.Session) bool { return slices.Contains(gone, s.Name) })
		return out
	}
	m := fuseModel(t, 160, left("hearth-api", "ember-billing"), "")
	got := listLines(m, 100, 20)
	sameLines(t, got, []string{" WAITS FOR YOU · 1", "▌╿ kindle-web"})
	for _, l := range got {
		if strings.Contains(l, "chain of") || strings.ContainsAny(l, "╮┤╯─") {
			t.Errorf("a chain of one is drawn: %q", l)
		}
	}
	m = fuseModel(t, 160, left("ember-billing"), "")
	sameLines(t, listLines(m, 100, 20), []string{
		" WAITS FOR YOU · 1 · chain of 2",
		"▌╿─╮ kindle-web",
		" ✦─╯ hearth-api",
	})
}

// TestOpenedLinesStartWithTwoBlanks: fused takes the first two cells of an
// opened line for the fuse, so every line opened makes starts with two
// blanks; and a tap on the opened keys of a chained row on a phone presses
// the key it shows.
func TestOpenedLinesStartWithTwoBlanks(t *testing.T) {
	t.Skip("dropped (Task 12 step 1; deleted at step 7, listed at step 8): the fuse's two cells go")
	m := fuseModel(t, 44, fuseState(t, mockupChain), "hearth-api")
	s, _ := m.selectedSession()
	for _, l := range m.opened(s, 44) {
		if !strings.HasPrefix(ansi.Strip(l), "  ") {
			t.Errorf("an opened line without its two blanks: %q", ansi.Strip(l))
		}
	}
	sc := m.draw(44)
	if sc.actKeysAt <= 0 {
		t.Fatalf("the opened keys are not marked")
	}
	line := ansi.Strip(sc.lines[sc.actKeysAt])
	if !strings.HasPrefix(line, "  │") {
		t.Fatalf("the opened keys' line lacks the fuse: %q", line)
	}
	for _, sp := range sc.actKeys {
		cells := []rune(line)[sp.x0:sp.x1]
		want := map[string]string{"A": "A switch", "enter": "⏎ go in"}[sp.press]
		if string(cells) != want {
			t.Errorf("the span for %q covers %q, want %q", sp.press, string(cells), want)
		}
	}
	ran := recordRuns(&m)
	next, cmd := m.Update(tea.MouseClickMsg{X: strings.Index(line, "⏎") + 1, Y: sc.actKeysAt, Button: tea.MouseLeft})
	drain(cmd)
	if len(*ran) != 1 || next.(Model).standIn {
		t.Errorf("a tap on ⏎ go in ran %q, stand-in %v", *ran, next.(Model).standIn)
	}
}

// TestSharedMatchDrawsInItsFirstTeam: kindle-web is in hearth and web; it
// draws in hearth's fuse and says it is also in web, and web's other match
// keeps a stub.
func TestSharedMatchDrawsInItsFirstTeam(t *testing.T) {
	m := fuseModel(t, 160, fuseState(t, mockupChain, []string{"kindle-web", "flint-auth"}), "")
	got := listLines(m, 100, 20)
	sameLines(t, got, []string{
		" WAITS FOR YOU · 1 · chain of 3",
		"▌╿─╮ kindle-web · al",
		" ✦─┤ hearth-api",
		" ╿─╯ ember-billing",
		" WORKING · 1 · chain of 2",
		" ✦─╴ flint-auth",
	})
	var b body
	m.sessionRows(&b, 100)
	if l := lineWith(b.lines, "kindle-web"); !strings.Contains(l, m.st.faint.Render(" · also in web")) {
		t.Errorf("kindle-web does not say it is also in web: %q", l)
	}
	if l := ansi.Strip(lineWith(b.lines, "flint-auth")); !strings.Contains(l, "─╴ flint-auth") {
		t.Errorf("web's other match has no stub: %q", l)
	}
	if l := ansi.Strip(lineWith(b.lines, "hearth-api")); strings.Contains(l, "also in") {
		t.Errorf("a match in one team says also in: %q", l)
	}
}

// TestSharedMatchRowSaysAlsoInOnlyWithRoom: on a phone with no room the
// name stays whole and the suffix goes.
func TestSharedMatchRowSaysAlsoInOnlyWithRoom(t *testing.T) {
	m := fuseModel(t, 44, fuseState(t, mockupChain, []string{"kindle-web", "flint-auth"}), "")
	var b body
	m.sessionRows(&b, 44)
	if l := ansi.Strip(lineWith(b.lines, "kindle-web")); strings.Contains(l, "also in") || !strings.Contains(l, "kindle-web") {
		t.Errorf("no room: %q", l)
	}
}

// joinModel is the console after a first state with hearth of two, then a
// state where state has the given chains.
func joinModel(t *testing.T, width int, next ...[]string) Model {
	t.Helper()
	var m tea.Model = New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	m, _ = m.Update(stateMsg(fuseState(t, mockupChain[:2])))
	m, _ = m.Update(stateMsg(fuseState(t, next...)))
	return m.(Model)
}

func TestJoinToastAndSpark(t *testing.T) {
	if m := joinModel(t, 160, mockupChain); saidNow(m) != "ember-billing joined hearth · u in its room takes it out" { // u moves to the menu at step 6d
		t.Fatalf("toast = %q", saidNow(m))
	}
}

func TestJoinToastNarrow(t *testing.T) {
	if m := joinModel(t, 40, mockupChain); saidNow(m) != "ember-billing joined hearth · u: out" {
		t.Fatalf("toast = %q", saidNow(m))
	}
}

// saidNow is the news the console said last: the header's toast, or on the
// board the notice in the strip's row.
func saidNow(m Model) string { return cmp.Or(m.toast, m.noticeText) }

func TestNewTeamToast(t *testing.T) {
	if m := joinModel(t, 160, mockupChain[:2], []string{"flint-auth", "spark-docs"}); saidNow(m) != "web formed: flint-auth, spark-docs" {
		t.Fatalf("toast = %q", saidNow(m))
	}
}

func TestNoJoinNoToast(t *testing.T) {
	if m := joinModel(t, 160, mockupChain[:2]); m.toast != "" {
		t.Fatalf("toast = %q", m.toast)
	}
}

func TestOutOfTeamAsks(t *testing.T) {
	m := fuseModel(t, 160, fuseState(t, mockupChain), "")
	var s sample.Session
	for _, x := range m.snap.Sessions {
		if x.Name == "ember-billing" {
			s = x
		}
	}
	a := m.outOfTeam(s, "hearth")
	if a.label != "take ember-billing out of hearth" || a.rec != "chain:out:"+s.Pane+"@hearth" || !a.destructive {
		t.Fatalf("%+v", a)
	}
	if got := a.String(); got != "take ember-billing out of hearth" { // the confirm shows it: no steps
		t.Fatalf("confirm says %q", got)
	}
}

// TestAClearIsNoJoin: a member renamed by a /clear (a new session id in
// the same pane) is not a match that joined.
func TestAClearIsNoJoin(t *testing.T) {
	var m tea.Model = New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	m, _ = m.Update(stateMsg(fuseState(t, mockupChain[:2])))
	st := fuseState(t, mockupChain[:2])
	for i := range st.Sessions {
		if st.Sessions[i].Name == "hearth-api" {
			st.Sessions[i].SessionID = "cleared"
			st.Chains.Chains[0].Members[1] = memberOf(st.Sessions[i])
		}
	}
	m, _ = m.Update(stateMsg(st))
	if got := m.(Model).toast; got != "" {
		t.Fatalf("toast = %q", got)
	}
}

// TestASharedMatchFusesWithTheTeamItCanShow: kindle-web's first team has
// no other match here, so it draws in web's fuse with flint-auth.
func TestASharedMatchFusesWithTheTeamItCanShow(t *testing.T) {
	st := fuseState(t, []string{"kindle-web"}, []string{"kindle-web", "flint-auth"})
	st.Chains.Chains[0].Members = append(st.Chains.Chains[0].Members, chains.Member{SessionID: "gone", Pane: "%99"})
	m := fuseModel(t, 160, st, "")
	var b body
	m.sessionRows(&b, 100)
	k, f := ansi.Strip(lineWith(b.lines, "kindle-web")), ansi.Strip(lineWith(b.lines, "flint-auth"))
	if !strings.Contains(k, "─╮ kindle-web") || !strings.Contains(f, "─╯ flint-auth") {
		t.Fatalf("rows:\n%s\n%s", k, f)
	}
}

// stokerPane is the pane of the chain stoker s1's agent on its own server.
const stokerPane = "%9@matchblox"

// headState is fuseState with the mockup's chain (c1) given chain stoker
// s1 in v; with agent, its agent runs in tmux session stoker-s1 on the
// stoker's server.
func headState(t *testing.T, v proto.StokerView, agent bool, chained ...[]string) proto.State {
	t.Helper()
	if len(chained) == 0 {
		chained = [][]string{mockupChain}
	}
	st := fuseState(t, chained...)
	st.StokerServer = "matchblox"
	if agent {
		st.Sessions = append(st.Sessions, sample.Session{Host: st.Sessions[0].Host, Name: "stoker-s1", PID: 9001, Pane: stokerPane,
			Target: "stoker-s1:0.0", Status: "busy", Busy: true, Cwd: "/home/user"})
	}
	st.Stokers = []state.ChainStoker{{ID: "s1", Chain: "c1", Name: "kindle-web", View: v}}
	return st
}

// runningPlan is a run on, with two steps of five of its plan done.
func runningPlan() proto.StokerView {
	v := proto.StokerView{PaneUp: true}
	v.On = true
	o := proto.StokerOrder{State: "running", Words: "ship the chain"}
	for n := 1; n <= 5; n++ {
		o.Plan = append(o.Plan, proto.StokerPlan{N: n, Text: "step"})
	}
	v.Order = &o
	for n := 1; n <= 2; n++ {
		v.Run = append(v.Run, proto.StokerEntry{ID: "p" + string(rune('0'+n)), Step: n, State: "done", At: demoAt.Add(-time.Hour)})
	}
	return v
}

// headModel is the console on st, on the demo's still clock, with its
// conn.
func headModel(t *testing.T, width int, st proto.State) (Model, *fakeConn) {
	t.Helper()
	f := newFake()
	var next tea.Model = New(Options{NoMotion: true, Conn: f, Now: func() time.Time { return demoAt }})
	next, _ = next.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	next, _ = next.Update(stateMsg(st))
	m := next.(Model)
	m.tab = tabSessions
	return m, f
}

// TestHeadRowLeadsItsChain: a chain stoker whose run is on heads its
// chain's fuse with its name and its plan; the label still counts the
// matches.
func TestHeadRowLeadsItsChain(t *testing.T) {
	m, _ := headModel(t, 160, headState(t, runningPlan(), true))
	sameLines(t, listLines(m, 100, 40), []string{
		" WAITS FOR YOU · 1 · chain of 3",
		" ✦─╮ stoker   kindle-web · plan 2/5",
		"▌╿─┤ kindle-web",
		" ✦─┤ hearth-api",
		" ╿─╯ ember-billing",
		" WORKING · 1",
		" ✦   flint-auth",
	})
}

// TestNoHeadForARestingUnit: a unit with its run off and no agent, as an
// ended one is, heads nothing; its agent's session stays out of the list.
func TestNoHeadForARestingUnit(t *testing.T) {
	m, _ := headModel(t, 160, headState(t, proto.StokerView{}, false))
	if got := strings.Join(listLines(m, 100, 60), "\n"); strings.Contains(got, "stoker") {
		t.Fatalf("a head row for a resting unit:\n%s", got)
	}
}

// TestAHeadThatAsksMovesItsChainToWaits: the chain's group is the
// neediest of its rows and its head.
func TestAHeadThatAsksMovesItsChainToWaits(t *testing.T) {
	st := headState(t, runningPlan(), true, []string{"hearth-api", "ember-billing"})
	asker(&st)
	m, _ := headModel(t, 160, st)
	got := listLines(m, 100, 70)
	waits := slices.Index(got, " WORKING · 1")
	head := slices.IndexFunc(got, func(l string) bool { return strings.Contains(l, "stoker   kindle-web") })
	if head < 0 || waits < 0 || head > waits || !strings.Contains(got[head], "· asks: merge #812 now?") {
		t.Fatalf("the asking head is not in WAITS FOR YOU:\n%s", strings.Join(got, "\n"))
	}
	if !strings.Contains(got[head+1], "hearth-api") || !strings.Contains(got[head+2], "ember-billing") {
		t.Fatalf("the chain does not follow its head:\n%s", strings.Join(got, "\n"))
	}
}

// asker makes the stoker's agent in st ask a question.
func asker(st *proto.State) {
	for i := range st.Sessions {
		if s := &st.Sessions[i]; s.Pane == stokerPane {
			s.Busy, s.Status, s.Asks = false, "idle", true
			s.Ask = &said.Ask{ID: "q1", Questions: []said.Question{{Question: "merge #812 now?"}}}
		}
	}
}

// TestHeadScreens: the head row at 150 and 44 columns, a head that asks,
// and the start sheet of a chain.
func TestHeadScreens(t *testing.T) {
	st := headState(t, runningPlan(), true)
	check(t, st, "chainhead", 150, 30, nil)
	check(t, st, "chainhead", 44, 31, nil)
	asks := headState(t, runningPlan(), true, []string{"hearth-api", "ember-billing"})
	asker(&asks)
	check(t, asks, "chainheadasks", 150, 30, nil)
	check(t, asks, "chainheadasks", 44, 31, nil)
	start := append([]string{"F"}, strings.Split("merge on green", "")...)
	for _, size := range [][2]int{{44, 31}, {104, 38}} {
		check(t, fuseState(t, mockupChain), "chainsheet", size[0], size[1], append(start, "tab", "space", "tab"))
	}
}

func pick(t *testing.T, m Model, pane string) Model {
	t.Helper()
	for _, s := range m.snap.Sessions {
		if s.Pane == pane {
			choose(&m, s.PID)
			return m
		}
	}
	t.Fatalf("no row for %s", pane)
	return m
}

// wentInto presses k on m and says the tmux commands it ran.
func wentInto(t *testing.T, m Model, k string) (Model, [][]string) {
	t.Helper()
	ran := recordRuns(&m)
	next, cmd := key(m, k)
	drain(cmd)
	return next.(Model), *ran
}

// TestAOnHeadOpensStokerPane: a on the head of a run that is on opens the
// stoker's own pane in a popup: it is on matchblox's tmux server (#346).
func TestAOnHeadOpensStokerPane(t *testing.T) {
	t.Skip("replaced by TestEnterOpensTheStokerInAPopup (Task 12 step 1)")
	m, _ := headModel(t, 160, headState(t, runningPlan(), true))
	m = pick(t, m, stokerPane)
	next, _ := wentInto(t, m, "a")
	if next.pending == nil || !strings.HasPrefix(next.pending.label, "open the stoker of chain ") || next.sheet != nil {
		t.Fatalf("pending %+v, sheet %v", next.pending, next.sheet)
	}
}

// TestAOnHeadWithoutRunOpensSheet: with its run off, a on the head opens
// the start sheet of its chain.
func TestAOnHeadWithoutRunOpensSheet(t *testing.T) {
	t.Skip("replaced by TestTalkStartsAStokerWithNoSheet (Task 12 step 1)")
	m, f := headModel(t, 160, headState(t, proto.StokerView{PaneUp: true}, true))
	m = pick(t, m, stokerPane)
	next := sendKey(m, "a")
	s := screen(next)
	for _, want := range []string{"Stoke the chain kindle-web (3 matches)?", "How should I run this chain?", "○ merges  ○ releases  ○ deploys  ○ kills"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q:\n%s", want, s)
		}
	}
	if len(f.acts()) != 0 {
		t.Fatalf("acts %+v", f.acts())
	}
}

// TestAOnHeadAtNightOpensItsPane: a sealed night takes no order, so a on
// the head opens its pane, to answer its prompts.
func TestAOnHeadAtNightOpensItsPane(t *testing.T) {
	t.Skip("replaced by TestEnterOpensTheStokerInAPopup (Task 12 step 1)")
	st := headState(t, proto.StokerView{PaneUp: true}, true)
	host := proto.StokerView{}
	host.On, host.Nightly, host.Until = true, true, demoAt.Add(8*time.Hour)
	st.Stoker = &host
	m, _ := headModel(t, 160, st)
	next, _ := wentInto(t, pick(t, m, stokerPane), "a")
	if next.pending == nil || !strings.HasPrefix(next.pending.label, "open the stoker of chain ") || next.sheet != nil {
		t.Fatalf("pending %+v, sheet %v", next.pending, next.sheet)
	}
}

// TestAOnAPanelessHead: a run that is on whose agent is gone still heads
// its chain; a opens the sheet, whose order starts the agent again. At
// night it says the night is sealed.
func TestAOnAPanelessHead(t *testing.T) {
	t.Skip("replaced by TestTalkToARunningStokerSendsAnOrder (Task 12 step 1)")
	v := runningPlan()
	v.PaneUp = false
	m, f := headModel(t, 160, headState(t, v, false))
	got := listLines(m, 100, 60)
	if !strings.Contains(got[1], "stoker   kindle-web · plan 2/5") {
		t.Fatalf("no head row:\n%s", strings.Join(got, "\n"))
	}
	choose(&m, m.snap.Sessions[0].PID)
	next := typed(sendKey(m, "a"), "go on")
	_, cmd := press(next, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("enter sent nothing")
	}
	drain(cmd)
	if a := f.acts(); len(a) != 1 || a[0].RecID != "stoker:order-chain" || a[0].Text != `{"chain":"c1","words":"go on","grants":[]}` {
		t.Fatalf("acts %+v", a)
	}
	st := headState(t, v, false)
	host := proto.StokerView{}
	host.On, host.Nightly = true, true
	st.Stoker = &host
	m, _ = headModel(t, 160, st)
	choose(&m, m.snap.Sessions[0].PID)
	night := sendKey(m, "a").(Model)
	if night.sheet != nil || night.flash != "the night is sealed: answer the stoker's prompts, or give the order in the morning" {
		t.Fatalf("sheet %v, flash %q", night.sheet, night.flash)
	}
}

// TestAOnAMemberStaysItsChat: a chain with no stoker keeps a on a member
// for that member: into its own pane.
func TestAOnAMemberStaysItsChat(t *testing.T) {
	m := fuseModel(t, 160, fuseState(t, mockupChain), "hearth-api")
	pane := m.snap.Sessions[slices.IndexFunc(m.snap.Sessions, func(s sample.Session) bool { return s.Name == "hearth-api" })].Pane
	next, ran := wentInto(t, m, "a")
	if len(ran) != 1 || strings.Join(ran[0], " ") != "tmux switch-client -t "+pane || next.sheet != nil {
		t.Fatalf("ran %q, sheet %v", ran, next.sheet)
	}
}

// TestFOnABareChainOpensSheet: F on a member of a chain with no head
// opens its start sheet, and the key line offers it; F on a session in
// no chain opens nothing.
func TestFOnABareChainOpensSheet(t *testing.T) {
	t.Skip("replaced by TestFOnACardOfABareChainOpensTheSheet (Task 12 step 1)")
	m := fuseModel(t, 160, fuseState(t, mockupChain), "hearth-api")
	if line := actionLine(m); !strings.Contains(line, "F stoke chain") {
		t.Fatalf("no F on the key line: %q", line)
	}
	if s := screen(sendKey(m, "F")); !strings.Contains(s, "Stoke the chain kindle-web (3 matches)?") {
		t.Fatalf("no sheet:\n%s", s)
	}
	choose(&m, pidOf(m, "flint-auth"))
	if line := actionLine(m); strings.Contains(line, "stoke chain") {
		t.Fatalf("F offered on a session in no chain: %q", line)
	}
	if next := sendKey(m, "F").(Model); next.sheet != nil {
		t.Fatal("F opened a sheet for a session in no chain")
	}
}

// tapFuse taps the fuse of the row named name.
func tapFuse(t *testing.T, m Model, name string) Model {
	t.Helper()
	sc := m.draw(m.width)
	for i, r := range sc.rows {
		if r >= len(m.doors) && r < len(m.doors)+len(m.snap.Sessions) && m.snap.Sessions[r-len(m.doors)].Name == name {
			next, _ := m.Update(tea.MouseClickMsg{X: 2, Y: sc.bodyTop + i, Button: tea.MouseLeft})
			return next.(Model)
		}
	}
	t.Fatalf("no row %s", name)
	return m
}

// TestTapOnBareFuseOpensSheet: a tap on the fuse of a chain with no head
// opens its start sheet. On a chain with a head it is a row tap, and on a
// phone, with no fuse column, it is a row tap too.
func TestTapOnBareFuseOpensSheet(t *testing.T) {
	t.Skip("dropped (Task 12 step 1; deleted at step 7, listed at step 8): the fuse goes; F holds the way in")
	m := fuseModel(t, 160, fuseState(t, mockupChain), "")
	if next := tapFuse(t, m, "hearth-api"); next.sheet == nil || next.sheet.chain != "c1" {
		t.Fatalf("a tap on the fuse opened no sheet: %+v", next.sheet)
	}
	h, _ := headModel(t, 160, headState(t, runningPlan(), true))
	next := tapFuse(t, h, "hearth-api")
	if s, _ := next.selectedSession(); next.sheet != nil || s.Name != "hearth-api" {
		t.Fatalf("a tap on a headed chain's fuse: sheet %v, on %q", next.sheet, s.Name)
	}
	phone := fuseModel(t, 44, fuseState(t, mockupChain), "")
	next = tapFuse(t, phone, "hearth-api")
	if s, _ := next.selectedSession(); next.sheet != nil || s.Name != "hearth-api" {
		t.Fatalf("a phone's tap on the fuse: sheet %v, on %q", next.sheet, s.Name)
	}
}

// TestABareChainStartsNothingAtNight: F, or a tap on the fuse, of a chain
// with no head says the night is sealed and sends nothing.
func TestABareChainStartsNothingAtNight(t *testing.T) {
	t.Skip("replaced by TestFOnACardOfABareChainOpensTheSheet (Task 12 step 1)")
	st := fuseState(t, mockupChain)
	host := proto.StokerView{}
	host.On, host.Nightly = true, true
	st.Stoker = &host
	m, f := headModel(t, 160, st)
	choose(&m, pidOf(m, "hearth-api"))
	for name, next := range map[string]Model{"F": sendKey(m, "F").(Model), "a tap": tapFuse(t, m, "hearth-api")} {
		if next.sheet != nil || next.flash != sealedFamily || len(f.acts()) != 0 {
			t.Errorf("%s at night: sheet %v, flash %q, acts %+v", name, next.sheet, next.flash, f.acts())
		}
	}
}

// TestAHeadIsNoSession: x, r, A and z on a head row send nothing and say
// where its agent quits.
func TestAHeadIsNoSession(t *testing.T) {
	t.Skip("dropped (Task 12 step 1; deleted at step 7, listed at step 8): head rows go with Sessions (re-plan)")
	m, f := headModel(t, 160, headState(t, runningPlan(), true))
	m = pick(t, m, stokerPane)
	for _, k := range []string{"x", "r", "A", "z"} {
		next := sendKey(m, k).(Model)
		if next.flash != "the stoker of chain kindle-web is no session: quit it on the STOKER tab" || next.pending != nil || next.form != nil || len(f.acts()) != 0 {
			t.Errorf("%s on the head: flash %q, pending %v, form %v, acts %+v", k, next.flash, next.pending, next.form != nil, f.acts())
		}
	}
}

// TestTheHostStokerAskingCountsAsToday: with no chain stoker, the host
// stoker's agent at a menu adds nothing to the header's count.
func TestTheHostStokerAskingCountsAsToday(t *testing.T) {
	today := fuseState(t)
	today.StokerServer = "matchblox"
	asking := today
	asking.Sessions = append(slices.Clone(today.Sessions), sample.Session{Host: today.Sessions[0].Host, Name: "stoker", PID: 9002,
		Pane: "%8@matchblox", Target: "stoker:0.0", Status: "idle", Asks: true, Permits: true})
	header := func(st proto.State) string {
		m, _ := headModel(t, 160, st)
		return strings.SplitN(screen(m), "\n", 2)[0]
	}
	if got, want := header(asking), header(today); got != want {
		t.Fatalf("header %q, want %q", got, want)
	}
}
