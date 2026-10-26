package app

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
)

// whereState is 52 repos found on the host (/src/r00 to /src/r48, and
// ~/dev/matchblox, /src/matchblox-site, /src/tools/mbxctl), two of them
// used by matchblox: matchblox last.
func whereState(t *testing.T) proto.State {
	t.Helper()
	st := lifeState()
	t0 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	var repos []proto.Checkout
	for i := range 49 {
		repos = append(repos, proto.Checkout{Path: fmt.Sprintf("/src/r%02d", i), Branch: "main", Active: t0.Add(time.Duration(i) * time.Hour)})
	}
	repos = append(repos,
		proto.Checkout{Path: "/dev/matchblox", Branch: "main", Active: t0},
		proto.Checkout{Path: "/src/matchblox-site", Branch: "main", Active: t0.Add(-time.Hour)},
		proto.Checkout{Path: "/src/tools/mbxctl", Branch: "dev", Active: t0.Add(-2 * time.Hour)})
	st.Found = &proto.Found{Consent: "yes", Repos: repos}
	st.Used = &proto.Used{Repos: []proto.Use{{Path: "/dev/matchblox", At: t0.Add(30 * 24 * time.Hour)}, {Path: "/src/r10", At: t0.Add(20 * 24 * time.Hour)}}}
	return st
}

func selectable(rows []whereRow) []whereRow {
	var out []whereRow
	for _, r := range rows {
		if r.kind != rowLabel {
			out = append(out, r)
		}
	}
	return out
}

func TestFuzzy(t *testing.T) {
	if s, _ := fuzzy("mbx", "~/dev/matchblox"); s < 0 {
		t.Fatal("mbx misses matchblox")
	}
	if s, _ := fuzzy("xyz", "~/dev/matchblox"); s >= 0 {
		t.Fatal("xyz matches")
	}
	if s, hits := fuzzy("MBX", "matchblox"); s < 0 || fmt.Sprint(hits) != "[0 5 8]" {
		t.Fatalf("MBX on matchblox: %d %v", s, hits)
	}
	a, _ := fuzzy("api", "shop-api")
	b, _ := fuzzy("api", "a-pretty-index")
	if a <= b {
		t.Fatalf("a word start scores %d, scattered letters %d", a, b)
	}
}

func TestWhereRowsSuggestedFirst(t *testing.T) {
	st := whereState(t)
	st.Sessions[0].Cwd = "/src/r30/web"
	m, _ := loadedWith(t, 100, st)
	rows := m.whereRows("")
	if rows[0].kind != rowLabel || rows[0].text != "SUGGESTED" {
		t.Fatalf("first row %+v", rows[0])
	}
	want := []string{"/src/r30", "/dev/matchblox", "/src/r10", "/src/r48", "/src/r47"}
	for i, p := range want {
		if r := rows[1+i]; r.kind != rowRepo || r.repo.path != p {
			t.Fatalf("suggested %d is %+v, want %s", i, r, p)
		}
	}
	if rows[1].repo.live != 1 {
		t.Fatalf("r30 has %d live", rows[1].repo.live)
	}
	if r := rows[6]; r.kind != rowLabel || r.text != "ALL · 47 · by name" {
		t.Fatalf("row 6 %+v", r)
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if r.kind == rowRepo {
			if seen[r.repo.path] {
				t.Fatalf("%s twice", r.repo.path)
			}
			seen[r.repo.path] = true
		}
	}
	if len(seen) != 52 || rows[7].repo.name != "matchblox-site" {
		t.Fatalf("%d repos, the first of ALL %+v", len(seen), rows[7])
	}
}

func TestWhereRowsWorktreesUnderTheirRepo(t *testing.T) {
	st := lifeState()
	st.Found = &proto.Found{Consent: "yes", Repos: []proto.Checkout{
		{Path: "/dev/.wt/matchblox-185", Main: "/dev/matchblox", Branch: "fix/185"},
		{Path: "/dev/matchblox", Branch: "main"},
		{Path: "/dev/.wt/matchblox-174", Main: "/dev/matchblox", Branch: "feat/174"},
	}}
	m, _ := loadedWith(t, 100, st)
	rows := selectable(m.whereRows(""))
	if len(rows) != 3 {
		t.Fatalf("rows %+v", rows)
	}
	if rows[0].kind != rowRepo || rows[0].repo.path != "/dev/matchblox" ||
		rows[1].kind != rowTree || rows[1].tree != "├─ " || rows[1].repo.branch != "feat/174" ||
		rows[2].kind != rowTree || rows[2].tree != "╰─ " || rows[2].repo.branch != "fix/185" {
		t.Fatalf("rows %+v", rows)
	}
	// The Git tab's worktrees go under their repo too, each path once.
	st.Git = &proto.GitReport{Repos: []gitscan.Repo{{Path: "/dev/matchblox", Worktrees: []proto.Worktree{
		{Path: "/dev/matchblox", Branch: "main"}, {Path: "/dev/.wt/matchblox-174", Branch: "feat/174", PR: &proto.PR{Number: 201}}}}}}
	m, _ = loadedWith(t, 100, st)
	rows = selectable(m.whereRows(""))
	if len(rows) != 3 || rows[1].repo.pr != 201 {
		t.Fatalf("rows %+v", rows)
	}
}

func TestWhereRowsFilter(t *testing.T) {
	m, _ := loadedWith(t, 100, whereState(t))
	rows := m.whereRows("mbx")
	if len(rows) != 3 || rows[0].repo.name != "matchblox" {
		t.Fatalf("rows %+v", rows)
	}
	for _, r := range rows {
		if r.kind != rowRepo {
			t.Fatalf("a %d row: %+v", r.kind, r)
		}
	}
	next, _ := key(m, "N")
	next = typeText(next, "mbx")
	if s := screen(next); !strings.Contains(s, "3 of 52") || strings.Contains(s, "SUGGESTED") {
		t.Fatalf("the list:\n%s", s)
	}
}

func TestWhereRowsNoMatch(t *testing.T) {
	m, _ := loadedWith(t, 100, whereState(t))
	rows := m.whereRows("zzz")
	if len(rows) != 1 || rows[0].kind != rowNone || rows[0].text != `no repo matches "zzz" · ⏎ uses it as a path` {
		t.Fatalf("rows %+v", rows)
	}
}

func TestNOpensTheListOnTheSessionsRepo(t *testing.T) {
	st := whereState(t)
	st.Found.Repos = append(st.Found.Repos, proto.Checkout{Path: "/src/api", Branch: "main"})
	m, _ := loadedWith(t, 100, st)
	next, _ := key(m, "N")
	f := next.(Model).form
	if f.where != "" || !f.listOpen || next.(Model).whereSelected().path() != "/src/api" {
		t.Fatalf("form %+v, selected %+v", f, next.(Model).whereSelected())
	}
	if s := screen(next); !strings.Contains(s, "SUGGESTED") || !strings.Contains(s, "then  branch · name · agent claude · first") || strings.Contains(s, "blank: no new worktree") {
		t.Fatalf("the list:\n%s", s)
	}
}

func TestWhereEnterPicksAndMovesOn(t *testing.T) {
	m, _ := loadedWith(t, 100, whereState(t))
	next, _ := key(m, "N")
	rows := selectable(next.(Model).whereRows(""))
	next, _ = key(next, "down")
	next, _ = key(next, "enter")
	f := next.(Model).form
	if f.where != rows[1].path() || f.listOpen || f.at != fieldBranch {
		t.Fatalf("form %+v, want %s", f, rows[1].path())
	}
	if s := screen(next); !strings.Contains(s, "blank: no new worktree") || strings.Contains(s, "SUGGESTED") {
		t.Fatalf("the form:\n%s", s)
	}
	// Back on where, the list opens again.
	next, _ = next.Update(keyPress("shift+tab"))
	if f := next.(Model).form; f.at != fieldWhere || !f.listOpen {
		t.Fatalf("form %+v", f)
	}
}

func TestWhereArrowsStillStepRepos(t *testing.T) {
	st := lifeState()
	st.Sessions[0].Cwd = ""
	st.Found = &proto.Found{Consent: "yes", Repos: []proto.Checkout{
		{Path: "/src/api", Branch: "main"},
		{Path: "/src/.wt/api-1", Main: "/src/api", Branch: "feat/1"},
		{Path: "/src/notes", Branch: "main"},
		{Path: "/src/shop-api", Branch: "main"},
	}}
	m, _ := loadedWith(t, 100, st)
	next, _ := key(m, "N")
	next, _ = next.Update(keyPress("right"))
	next, _ = next.Update(keyPress("right"))
	f := next.(Model).form
	if f.where != "/src/shop-api" || !f.picked {
		t.Fatalf("form %+v", f)
	}
	if s := screen(next); !strings.Contains(s, "SUGGESTED") || !strings.Contains(s, "notes") {
		t.Fatalf("the list is filtered by the pick:\n%s", s)
	}
	// Typing drops the pick and filters by what is typed.
	next = typeText(next, "no")
	if f := next.(Model).form; f.where != "no" || f.picked {
		t.Fatalf("form %+v", f)
	}
}

func TestWhereEscKeepsTheText(t *testing.T) {
	m, f := loadedWith(t, 100, whereState(t))
	next, _ := key(m, "N")
	next = typeText(next, "/tmp/x")
	next, _ = key(next, "esc")
	if fm := next.(Model).form; fm == nil || fm.listOpen || fm.where != "/tmp/x" {
		t.Fatalf("form %+v", fm)
	}
	next, _ = key(next, "esc")
	if next.(Model).form != nil || len(f.acts()) != 0 {
		t.Fatal("the second esc kept the form or sent")
	}
}

func TestWhereListFitsAPhone(t *testing.T) {
	st := whereState(t)
	st.Found.Repos = append(st.Found.Repos,
		proto.Checkout{Path: "/src/a-very-long-folder-name/deeper/still/shop-api", Branch: "feat/a-long-branch-name"},
		proto.Checkout{Path: "/src/.wt/shop-api-2", Main: "/src/a-very-long-folder-name/deeper/still/shop-api", Branch: "fix/2"})
	st.Used.Repos = append(st.Used.Repos, proto.Use{Path: "/src/a-very-long-folder-name/deeper/still/shop-api", At: st.Used.Repos[0].At})
	for _, w := range []int{44, 100} {
		m, _ := loadedWith(t, w, st)
		next, _ := key(m, "N")
		out := next.(Model).render()
		for i, l := range strings.Split(out, "\n") {
			if lw := lipgloss.Width(l); lw > w {
				t.Fatalf("%d columns: line %d is %d wide: %q", w, i, lw, l)
			}
		}
		s := screen(next)
		for _, name := range []string{"matchblox", "shop-api", "fix/2", "r10"} {
			if !strings.Contains(s, name) {
				t.Fatalf("%d columns: %s is cut:\n%s", w, name, s)
			}
		}
	}
}

func TestPasteFillsTheFocusedField(t *testing.T) {
	m, _ := loadedWith(t, 100, whereState(t))
	next, _ := key(m, "N")
	next, _ = next.Update(tea.PasteMsg{Content: " /src/x\n"})
	f := next.(Model).form
	if f.where != "/src/x" || f.picked || !f.listOpen {
		t.Fatalf("form %+v", f)
	}
	if s := screen(next); !strings.Contains(s, `no repo matches "/src/x"`) {
		t.Fatalf("the list is not filtered:\n%s", s)
	}
	for range 4 { // branch, name, agent, first
		next, _ = key(next, "tab")
	}
	next = typeText(next, "fix ")
	next, _ = next.Update(tea.PasteMsg{Content: "the\r\nbug "})
	if f := next.(Model).form; f.at != fieldFirst || f.first != "fix thebug" {
		t.Fatalf("form %+v", f)
	}
}

func TestWhereListWithNoRepos(t *testing.T) {
	st := lifeState()
	st.Sessions[0].Cwd = ""
	st.Found = &proto.Found{}
	m, _ := loadedWith(t, 100, st)
	m.scanAsk = nil
	next, _ := key(m, "N")
	s := screen(next)
	if !strings.Contains(s, "no repos known yet · type a path, or paste a git URL") || !strings.Contains(s, "matchblox scan finds your repos") {
		t.Fatalf("the list:\n%s", s)
	}
	st.Found = &proto.Found{Consent: "no"}
	m, _ = loadedWith(t, 100, st)
	next, _ = key(m, "N")
	if s := screen(next); !strings.Contains(s, "no repos known yet") || strings.Contains(s, "finds your repos") {
		t.Fatalf("the list:\n%s", s)
	}
}

func TestNewSessionSendsThePickedPath(t *testing.T) {
	m, f := loadedWith(t, 100, whereState(t))
	next, _ := key(m, "N")
	next = typeText(next, "mbx")
	next, _ = key(next, "enter") // picks matchblox
	next, cmd := key(next, "enter")
	cmd()
	var n proto.NewSession
	if acts := f.acts(); len(acts) != 1 || json.Unmarshal([]byte(acts[0].Text), &n) != nil || n.Where != "/dev/matchblox" {
		t.Fatalf("sent %+v", f.acts())
	}
}

// TestDrawWhereList prints the open list, for a look: go test -run
// TestDrawWhereList -v.
func TestDrawWhereList(t *testing.T) {
	st := whereState(t)
	st.Found.Repos = append(st.Found.Repos,
		proto.Checkout{Path: "/dev/.wt/matchblox-174", Main: "/dev/matchblox", Branch: "feat/174"},
		proto.Checkout{Path: "/dev/.wt/matchblox-185", Main: "/dev/matchblox", Branch: "fix/185"})
	st.Sessions[0].Cwd = "/dev/.wt/matchblox-174"
	for _, w := range []int{100, 44} {
		m, _ := loadedWith(t, w, st)
		next, _ := key(m, "N")
		t.Logf("%d columns:\n%s", w, screen(next))
		next = typeText(next, "mbx")
		t.Logf("%d columns, mbx:\n%s", w, screen(next))
	}
}

// A typed path is that path: a sibling that starts with the same letters
// is not it.
func TestWhereTypedPathIsNotASibling(t *testing.T) {
	st := lifeState()
	st.Sessions[0].Cwd = ""
	st.Found = &proto.Found{Consent: "yes", Repos: []proto.Checkout{
		{Path: "/src/api-gateway", Branch: "main"}, {Path: "/src/api/web", Branch: "main"}}}
	m, f := loadedWith(t, 100, st)
	next, _ := key(m, "N")
	next, _ = next.Update(tea.PasteMsg{Content: "/src/api"})
	rows := next.(Model).form.rows
	if sel := next.(Model).whereSelected(); sel.kind != rowPath || sel.path() != "/src/api" {
		t.Fatalf("selected %+v of %+v", sel, rows)
	}
	for _, r := range rows {
		if r.repo.path == "/src/api-gateway" {
			t.Fatalf("the sibling matches: %+v", rows)
		}
	}
	if !slices.ContainsFunc(rows, func(r whereRow) bool { return r.repo.path == "/src/api/web" }) {
		t.Fatalf("a repo under the path is not listed: %+v", rows)
	}
	next, _ = key(next, "enter")
	next, cmd := key(next, "enter")
	cmd()
	var n proto.NewSession
	if acts := f.acts(); len(acts) != 1 || json.Unmarshal([]byte(acts[0].Text), &n) != nil || n.Where != "/src/api" {
		t.Fatalf("sent %+v", f.acts())
	}
	// The exact path of a known repo selects that repo, with no extra row.
	next, _ = key(m, "N")
	next, _ = next.Update(tea.PasteMsg{Content: "/src/api-gateway"})
	if rows := next.(Model).form.rows; len(rows) != 1 || rows[0].kind != rowRepo || rows[0].repo.path != "/src/api-gateway" {
		t.Fatalf("rows %+v", rows)
	}
	// A known repo at the typed path goes first, ahead of a newer one under it.
	st = lifeState()
	st.Sessions[0].Cwd = ""
	now := time.Now()
	st.Found = &proto.Found{Consent: "yes", Repos: []proto.Checkout{
		{Path: "/src/api", Branch: "main", Active: now.Add(-time.Hour)}, {Path: "/src/api/web", Branch: "main", Active: now}}}
	m, f = loadedWith(t, 100, st)
	next, _ = key(m, "N")
	next, _ = next.Update(tea.PasteMsg{Content: "/src/api"})
	if sel := next.(Model).whereSelected(); sel.kind != rowRepo || sel.path() != "/src/api" {
		t.Fatalf("selected %+v of %+v", sel, next.(Model).form.rows)
	}
	next, _ = key(next, "enter")
	next, cmd = key(next, "enter")
	cmd()
	if acts := f.acts(); len(acts) != 1 || json.Unmarshal([]byte(acts[0].Text), &n) != nil || n.Where != "/src/api" {
		t.Fatalf("sent %+v", f.acts())
	}
	// A worktree's exact path selects the worktree, not its repo above it.
	st.Found.Repos = append(st.Found.Repos, proto.Checkout{Path: "/src/.wt/api-x", Main: "/src/api", Branch: "x"})
	m, _ = loadedWith(t, 100, st)
	next, _ = key(m, "N")
	next, _ = next.Update(tea.PasteMsg{Content: "/src/.wt/api-x"})
	if sel := next.(Model).whereSelected(); sel.kind != rowTree || sel.path() != "/src/.wt/api-x" {
		t.Fatalf("selected %+v of %+v", sel, next.(Model).form.rows)
	}
}

// A repo's row counts its worktrees' sessions too; a worktree's row its own.
func TestWhereRepoLiveCountsItsWorktrees(t *testing.T) {
	st := lifeState()
	st.Found = &proto.Found{Consent: "yes", Repos: []proto.Checkout{
		{Path: "/dev/matchblox", Branch: "main"},
		{Path: "/dev/.wt/matchblox-174", Main: "/dev/matchblox", Branch: "feat/174"}}}
	st.Sessions[0].Cwd = "/dev/matchblox"
	st.Sessions[1].Cwd = "/dev/.wt/matchblox-174/internal"
	m, _ := loadedWith(t, 100, st)
	rows := selectable(m.whereRows(""))
	_, _, _, _, repoLive := m.whereCells(rows[0])
	_, _, _, _, treeLive := m.whereCells(rows[1])
	if repoLive != "2 live" || treeLive != "1 live" {
		t.Fatalf("repo %q, worktree %q", repoLive, treeLive)
	}
}

// A used folder is its own entry: one above other used folders does not
// swallow them, nor count the sessions under them. A used folder in a real
// checkout is that checkout, and so is a session in it.
func TestWhereUsedFolderSwallowsNothing(t *testing.T) {
	st := lifeState()
	t0 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	st.Git = nil
	st.Found = &proto.Found{Consent: "yes", Repos: []proto.Checkout{{Path: "/w/src/r", Branch: "main", Active: t0}}}
	st.Used = &proto.Used{Repos: []proto.Use{
		{Path: "/w/dev/a", At: t0.Add(time.Hour)}, {Path: "/w", At: t0.Add(2 * time.Hour)},
		{Path: "/w/dev/b", At: t0.Add(3 * time.Hour)}, {Path: "/w/src/r/sub", At: t0.Add(4 * time.Hour)}}}
	for i := range st.Sessions {
		st.Sessions[i].Cwd = ""
	}
	st.Sessions[0].Cwd = "/w/dev/c" // under /w only: no entry counts it
	st.Sessions[1].Cwd = "/w/dev/b" // the used folder itself
	in := st.Sessions[1]
	in.Pane, in.Cwd = "%77", "/w/src/r/sub/x" // in the real checkout
	st.Sessions = append(st.Sessions, in)
	m, _ := loadedWith(t, 100, st)
	got := map[string]int{}
	for _, r := range m.wrepos {
		got[r.path] = r.live
	}
	want := map[string]int{"/w": 0, "/w/dev/a": 0, "/w/dev/b": 1, "/w/src/r": 1}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("repos and their sessions %v, want %v", got, want)
	}
}

// A snapshot does not scroll the list: the rows shown stay where they are.
func TestWhereSnapshotKeepsTheScroll(t *testing.T) {
	m, _ := loadedWith(t, 100, whereState(t))
	next, _ := key(m, "N")
	h := next.(Model).listHeight()
	for range h + 4 {
		next, _ = key(next, "down")
	}
	for range h / 2 {
		next, _ = key(next, "up")
	}
	shown := func(m Model) (top int, sel string) {
		f := m.form
		return scrollTop(f.top, f.sel, m.listHeight(), len(f.rows)), m.whereSelected().path()
	}
	top, sel := shown(next.(Model))
	if top == 0 || top == next.(Model).form.sel {
		t.Fatalf("not scrolled with the selection mid-view: top %d, selection %d", top, next.(Model).form.sel)
	}
	next, _ = next.Update(stateMsg(whereState(t)))
	if top2, sel2 := shown(next.(Model)); top2 != top || sel2 != sel {
		t.Fatalf("after a snapshot: top %d → %d, selection %s → %s", top, top2, sel, sel2)
	}
}

// A path of wide runes cut from its start fits its column, at any width.
func TestWhereWidePathFitsItsColumn(t *testing.T) {
	m, _ := loadedWith(t, 100, whereState(t))
	r := whereRow{kind: rowRepo, repo: whereRepo{path: "/中文中文中文中文/x", name: "x", branch: "main"}}
	for w := 1; w <= 20; w++ {
		line := ansi.Strip(m.whereLine(r, false, whereCols{name: 1, path: w}, 80))
		if got := lipgloss.Width(strings.TrimRight(line, " ")); got > 1+1+2+w {
			t.Fatalf("path in %d columns: %q is %d wide", w, line, got)
		}
	}
	st := whereState(t)
	st.Found.Repos = append(st.Found.Repos, proto.Checkout{Path: "/中文中文中文中文/x", Branch: "main"})
	for _, w := range []int{36, 40, 44, 52, 60} {
		m, _ := loadedWith(t, w, st)
		next, _ := key(m, "N")
		next = typeText(next, "/中")
		for i, l := range strings.Split(next.(Model).render(), "\n") {
			if lw := lipgloss.Width(l); lw > w {
				t.Fatalf("%d columns: line %d is %d wide", w, i, lw)
			}
		}
	}
}

// On a disk that ignores case, /src/myrepo is /src/MyRepo: typed, it
// selects that repo. On one that tells them apart, /src/myrepo is another
// folder: its own row comes below, one ↓ away.
func TestWherePathInAnotherCaseSelectsTheRepo(t *testing.T) {
	st := lifeState()
	st.Sessions[0].Cwd = ""
	st.Found = &proto.Found{Consent: "yes", Repos: []proto.Checkout{{Path: "/src/MyRepo", Branch: "main", Active: time.Now()}}}
	m, _ := loadedWith(t, 100, st)
	next, _ := key(m, "N")
	next, _ = next.Update(tea.PasteMsg{Content: "/src/myrepo"})
	fm := next.(Model).form
	if sel := next.(Model).whereSelected(); sel.path() != "/src/MyRepo" {
		t.Fatalf("selected %+v of %+v", sel, fm.rows)
	}
	if i := slices.IndexFunc(fm.rows, func(r whereRow) bool { return r.kind == rowPath && r.path() == "/src/myrepo" }); i <= fm.sel {
		t.Fatalf("no /src/myrepo row below the selected one in %+v", fm.rows)
	}
}

// The path in another case goes below the repo's worktrees, not between
// the repo and them.
func TestWherePathInAnotherCaseGoesBelowTheWorktrees(t *testing.T) {
	st := lifeState()
	st.Sessions[0].Cwd = ""
	now := time.Now()
	st.Found = &proto.Found{Consent: "yes", Repos: []proto.Checkout{
		{Path: "/src/MyRepo", Branch: "main", Active: now},
		{Path: "/src/MyRepo/.wt/one", Main: "/src/MyRepo", Branch: "feat/1"},
		{Path: "/src/MyRepo/.wt/two", Main: "/src/MyRepo", Branch: "feat/2"},
	}}
	m, _ := loadedWith(t, 100, st)
	next, _ := key(m, "N")
	next, _ = next.Update(tea.PasteMsg{Content: "/src/myrepo"})
	fm := next.(Model).form
	rows := selectable(fm.rows)
	if len(rows) != 4 || rows[0].kind != rowRepo || rows[0].path() != "/src/MyRepo" ||
		rows[1].kind != rowTree || rows[2].kind != rowTree ||
		rows[3].kind != rowPath || rows[3].path() != "/src/myrepo" {
		t.Fatalf("rows %+v", rows)
	}
	if sel := next.(Model).whereSelected(); sel.path() != "/src/MyRepo" {
		t.Fatalf("selected %+v of %+v", sel, fm.rows)
	}
}

func TestWhereExactPathKeepsItsCase(t *testing.T) {
	st := lifeState()
	st.Sessions[0].Cwd = ""
	now := time.Now()
	st.Found = &proto.Found{Consent: "yes", Repos: []proto.Checkout{
		{Path: "/src/project", Branch: "main", Active: now.Add(-time.Hour)}, {Path: "/src/Project", Branch: "main", Active: now}}}
	m, _ := loadedWith(t, 100, st)
	for _, p := range []string{"/src/project", "/src/Project"} {
		next, _ := key(m, "N")
		next, _ = next.Update(tea.PasteMsg{Content: p})
		if sel := next.(Model).whereSelected(); sel.path() != p {
			t.Fatalf("%s: selected %+v of %+v", p, sel, next.(Model).form.rows)
		}
	}
}
