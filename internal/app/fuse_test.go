package app

import (
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
		c := chains.Chain{ID: "c" + string(rune('1'+i)), Repo: "/home/user/src/hearth"}
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
	m := fuseModel(t, 160, fuseState(t, mockupChain), "hearth-api")
	m.sessGroup = groupStatus
	m.refresh()
	got := listLines(m, 100, 20)
	sameLines(t, got, []string{
		" ASKS · 1",
		" ╿─╴ kindle-web",
		" BUSY · 2",
		" ✦   flint-auth",
		"▌✦━╸ hearth-api",
		" IDLE · 2",
		" ╿─╴ ember-billing",
		" │   spark-docs",
	})
	var b body
	m.sessionRows(&b, 100)
	lit := m.st.accent.Render("─╴")
	for _, name := range []string{"kindle-web", "ember-billing"} {
		if l := lineWith(b.lines, name); !strings.Contains(l, lit) {
			t.Errorf("%s's stub is not lit: %q", name, l)
		}
	}
	if l := lineWith(b.lines, "hearth-api"); !strings.Contains(l, m.st.accent.Render("━╸")) {
		t.Errorf("the selected row's own stub: %q", l)
	}
	choose(&m, pidOf(m, "flint-auth"))
	b = body{}
	m.sessionRows(&b, 100)
	if l := lineWith(b.lines, "kindle-web"); strings.Contains(l, lit) || !strings.Contains(l, m.st.faint.Render("─╴")) {
		t.Errorf("a stub of a chain not selected is faint: %q", l)
	}
}

func pidOf(m Model, name string) int {
	for _, s := range m.snap.Sessions {
		if s.Name == name {
			return s.PID
		}
	}
	return 0
}

// TestFuseIsWholeInRepo: a chain is in one repo, so its matches are side by
// side there, whatever the sort puts between them.
func TestFuseIsWholeInRepo(t *testing.T) {
	st := fuseState(t, []string{"kindle-web", "ember-billing"})
	for i := range st.Sessions {
		st.Sessions[i].Cwd = "/home/user/src/hearth"
		if st.Sessions[i].Name == "flint-auth" {
			st.Sessions[i].Cwd = "/home/user/src/flint"
		}
	}
	m := fuseModel(t, 160, st, "")
	m.sessGroup = groupRepo
	m.refresh()
	got := listLines(m, 100, 20)
	sameLines(t, got[:1], []string{" flint · 1"})
	sameLines(t, got[2:], []string{
		" hearth · 4",
		"▌╿─╮ kindle-web",
		" ╿─╯ ember-billing",
		" ✦   hearth-api",
		" │   spark-docs",
	})
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
	st := fuseState(t, mockupChain)
	for i := range st.Sessions {
		st.Sessions[i].Cwd = "/home/user/src/hearth"
		if st.Sessions[i].Name != "hearth-api" {
			st.Sessions[i].Tab = "chainlook"
		}
	}
	for g, want := range map[int][]string{
		groupStatus: {" ASKS · 1 · 1 hidden", "▌╿━╸ kindle-web", " BUSY · 1", " ✦   flint-auth", " IDLE · 2", " ╿─╴ ember-billing"},
		groupRepo:   {" hearth · 4 · 1 hidden", "▌╿─╮ kindle-web", " ╿─╯ ember-billing"},
	} {
		m := fuseModel(t, 160, st, "")
		m.sessGroup = g
		m.filter = "chainlook"
		m.refresh()
		sameLines(t, listLines(m, 100, 20), want)
	}
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
	st.Chains = nil
	m = fuseModel(t, 160, st, "")
	m.sessGroup = groupStatus
	m.refresh()
	b = body{}
	m.sessionRows(&b, 100)
	if l := ansi.Strip(lineWith(b.lines, "kindle-web")); strings.Contains(l, hint) {
		t.Errorf("status has no hint: %q", l)
	}
}

// TestNoChainNoFuse: with no chain in the view the list is today's, byte
// for byte, in every grouping, wide and on a phone.
func TestNoChainNoFuse(t *testing.T) {
	rows := func(width, list int, cv *state.ChainsView) []string {
		var out []string
		for g := groupNeed; g <= groupNone; g++ {
			st := fuseState(t)
			st.Chains = cv
			m := fuseModel(t, width, st, "hearth-api")
			m.sessGroup = g
			m.refresh()
			var b body
			m.sessionRows(&b, list)
			out = append(out, b.lines...)
		}
		return out
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
