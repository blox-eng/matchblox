package app

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
)

func askState() proto.State {
	st := lifeState()
	st.Found = &proto.Found{}
	return st
}

// asked is the console with the scan question up and its quiet over.
func asked(t *testing.T) (Model, *fakeConn) {
	t.Helper()
	m, f := loadedWith(t, 100, askState())
	if m.scanAsk == nil || !m.scanShown() {
		t.Fatal("no scan question on a host never asked")
	}
	return calm(m), f
}

// calm ends the box's quiet, as if the builder had read it.
func calm(m tea.Model) Model {
	mm := m.(Model)
	mm.scanAskUntil = time.Time{}
	return mm
}

func sendAll(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd != nil {
		cmd()
	}
}

func TestScanBoxShowsOnce(t *testing.T) {
	m, _ := asked(t)
	s := screen(m)
	for _, want := range []string{"Find the Git repositories in your home folder?", "they stay on this host and your paired devices", "y find", "n never", "p pick folders", "esc later"} {
		if !strings.Contains(s, want) {
			t.Errorf("the box lacks %q:\n%s", want, s)
		}
	}
}

func TestScanBoxYes(t *testing.T) {
	m, f := asked(t)
	next, cmd := key(m, "y")
	sendAll(t, cmd)
	if a := f.acts(); len(a) != 1 || a[0].RecID != "scan:yes" || a[0].Confirm != "y" || a[0].Text != "" {
		t.Fatalf("sent %+v", a)
	}
	if nm := next.(Model); nm.scanShown() || nm.flash != "finding your Git repositories…" {
		t.Fatalf("open %v, flash %q", nm.scanShown(), nm.flash)
	}
}

func TestScanBoxYesResult(t *testing.T) {
	m, _ := asked(t)
	next, _ := key(m, "y")
	act := next.(Model).scanAct
	done, _ := next.Update(resultMsg(proto.Result{ActID: act}))
	if f := done.(Model).flash; f != "found your Git repositories: N offers them" {
		t.Fatalf("flash %q", f)
	}
	refused, _ := next.Update(resultMsg(proto.Result{ActID: act, Err: "boom"}))
	if f := refused.(Model).flash; f != "not found: boom" {
		t.Fatalf("flash %q", f)
	}
}

func TestScanBoxNo(t *testing.T) {
	m, f := asked(t)
	next, cmd := key(m, "n")
	sendAll(t, cmd)
	if a := f.acts(); len(a) != 1 || a[0].RecID != "scan:no" {
		t.Fatalf("sent %+v", a)
	}
	if next.(Model).scanShown() {
		t.Fatal("the box stayed open")
	}
}

func TestScanBoxLater(t *testing.T) {
	m, f := asked(t)
	next, _ := key(m, "esc")
	if next.(Model).scanShown() {
		t.Fatal("esc kept the box")
	}
	next, _ = next.Update(stateMsg(askState()))
	if next.(Model).scanShown() {
		t.Fatal("the next snapshot asked again")
	}
	m2, _ := asked(t)
	next, _ = tap(m2, 5, 5)
	if next.(Model).scanShown() {
		t.Fatal("a tap kept the box")
	}
	if len(f.acts()) != 0 {
		t.Fatalf("sent %+v", f.acts())
	}
}

func TestScanBoxPickFolders(t *testing.T) {
	m, f := asked(t)
	next, _ := key(m, "p")
	fm := next.(Model).form
	if fm == nil || !fm.pick || fm.roots != "~ ~/dev ~/src ~/code ~/projects ~/work" || !slices.Equal(fm.fields(), []int{fieldFolders}) {
		t.Fatalf("form %+v", fm)
	}
	if s := screen(next); !strings.Contains(s, "folders") {
		t.Fatalf("the form:\n%s", s)
	}
	next, _ = key(next, "esc")
	if !next.(Model).scanShown() {
		t.Fatal("esc in the folders did not go back to the box")
	}
	next, _ = key(calm(next), "p")
	for range len("~/projects ~/work") {
		next, _ = key(next, "backspace")
	}
	next, cmd := key(next, "enter")
	sendAll(t, cmd)
	if a := f.acts(); len(a) != 1 || a[0].RecID != "scan:yes" || a[0].Text != "~\n~/dev\n~/src\n~/code" {
		t.Fatalf("sent %+v", a)
	}
	next, _ = next.Update(resultMsg(proto.Result{ActID: next.(Model).form.act}))
	if nm := next.(Model); nm.form != nil || nm.scanShown() || nm.scanAsk != nil {
		t.Fatalf("form %+v, box %v", nm.form, nm.scanShown())
	}
}

func TestScanBoxClosesWhenAnsweredElsewhere(t *testing.T) {
	m, _ := asked(t)
	st := lifeState()
	st.Found = &proto.Found{Consent: "yes"}
	next, _ := m.Update(stateMsg(st))
	if next.(Model).scanShown() {
		t.Fatal("the box stayed after another console's yes")
	}
}

func TestNoBoxWithoutFound(t *testing.T) {
	m, _ := loadedWith(t, 100, lifeState())
	next, _ := m.Update(stateMsg(lifeState()))
	if next.(Model).scanShown() {
		t.Fatal("asked with no Found: an older service scans nothing")
	}
}

func TestFoundFeedsWhereNotGit(t *testing.T) {
	st := lifeState()
	st.Git = &proto.GitReport{Repos: []gitscan.Repo{{Path: "/w/app", Worktrees: []proto.Worktree{{Path: "/w/app", Branch: "main"}}}}}
	old, recent := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	st.Found = &proto.Found{Consent: "yes", Repos: []proto.Checkout{{Path: "/w/app"}, {Path: "/h/old", Active: old}, {Path: "/h/new", Active: recent}}}
	m, _ := loadedWith(t, 100, st)
	if got := repoPaths(m.whereRepos()); !slices.Equal(got, []string{"/w/app", "/h/new", "/h/old"}) {
		t.Fatalf("where's repos = %v", got)
	}
	for _, r := range m.allWorktreeRows() {
		if r.wt.Path != "/w/app" {
			t.Fatalf("the Git tab shows %s", r.wt.Path)
		}
	}
}

func TestEmptyWhereNamesTheRepositories(t *testing.T) {
	st := lifeState()
	st.Found = &proto.Found{Consent: "yes", Repos: []proto.Checkout{{Path: "/h/a"}}}
	m, _ := loadedWith(t, 100, st)
	next, _ := key(m, "N")
	next, _ = key(next, "esc") // the list closes, where stays empty
	next, _ = key(next, "enter")
	if f := next.(Model).form; f == nil || f.err != "where does it start? a path, or ←→ for a repository" || !f.listOpen {
		t.Fatalf("form %+v", f)
	}
}

func TestFoundKeptWhenNil(t *testing.T) {
	st := lifeState()
	st.Found = &proto.Found{Consent: "yes", Repos: []proto.Checkout{{Path: "/h/a"}}}
	m, _ := loadedWith(t, 100, st)
	next, _ := m.Update(stateMsg(lifeState()))
	if f := next.(Model).found; f == nil || len(f.Repos) != 1 {
		t.Fatalf("found = %+v", f)
	}
}

func TestScanBoxIgnoresKeysWhileQuiet(t *testing.T) {
	m, f := loadedWith(t, 100, askState())
	at := time.Now().Add(-100 * time.Millisecond)
	m.scanAskUntil = at.Add(askQuiet)
	next, cmd := key(m, "y")
	sendAll(t, cmd)
	if len(f.acts()) != 0 || !next.(Model).scanShown() {
		t.Fatalf("y at +100ms: sent %+v, box %v", f.acts(), next.(Model).scanShown())
	}
	m = next.(Model)
	at = time.Now().Add(-500 * time.Millisecond)
	m.scanAskUntil = at.Add(askQuiet)
	_, cmd = key(m, "y")
	sendAll(t, cmd)
	if a := f.acts(); len(a) != 1 || a[0].RecID != "scan:yes" {
		t.Fatalf("y at +500ms: sent %+v", a)
	}
}

func TestScanBoxEscAndQuitWhileQuiet(t *testing.T) {
	m, _ := loadedWith(t, 100, askState())
	next, _ := key(m, "esc")
	if nm := next.(Model); nm.scanShown() || nm.scanAsk != nil {
		t.Fatal("esc in the quiet kept the box")
	}
	m, _ = loadedWith(t, 100, askState())
	if next, cmd := m.Update(ctrlC); !quits(cmd) && !next.(Model).quitting {
		t.Fatal("ctrl+c in the quiet did not quit")
	}
}

func TestScanBoxNotFocusedOverSearchOrForm(t *testing.T) {
	m, f := asked(t)
	s := withLayers(m, "search")
	if got := s.focus(keyPress("y")); got != layerSearch {
		t.Fatalf("searching: focus %v", got)
	}
	next, cmd := key(s, "y")
	sendAll(t, cmd)
	if nm := next.(Model); nm.filter != "y" || nm.scanAsk == nil || len(f.acts()) != 0 {
		t.Fatalf("searching: filter %q, ask %v, sent %+v", nm.filter, nm.scanAsk != nil, f.acts())
	}
	if strings.Contains(screen(next), scanTitle) {
		t.Fatalf("the box shows over the search:\n%s", screen(next))
	}
	fm := withLayers(m, "form")
	if got := fm.focus(keyPress("y")); got != layerForm {
		t.Fatalf("form: focus %v", got)
	}
	next, cmd = key(fm, "y")
	sendAll(t, cmd)
	if nm := next.(Model); nm.scanAsk == nil || len(f.acts()) != 0 {
		t.Fatalf("form: ask %v, sent %+v", nm.scanAsk != nil, f.acts())
	}
}

func TestScanBoxReArmsAfterPeekCloses(t *testing.T) {
	m, f := asked(t)
	m = withLayers(m, "peek")
	next, _ := key(m, "esc")
	if nm := next.(Model); nm.peek != nil || !nm.scanShown() {
		t.Fatalf("peek %v, box %v", nm.peek != nil, nm.scanShown())
	}
	next, cmd := key(next, "y")
	sendAll(t, cmd)
	if len(f.acts()) != 0 || !next.(Model).scanShown() {
		t.Fatalf("y in the new quiet: sent %+v", f.acts())
	}
	_, cmd = key(calm(next), "y")
	sendAll(t, cmd)
	if a := f.acts(); len(a) != 1 || a[0].RecID != "scan:yes" {
		t.Fatalf("sent %+v", a)
	}
}

// pickSent is the pick form's yes sent: the model and its act.
func pickSent(t *testing.T) (tea.Model, string) {
	t.Helper()
	m, _ := asked(t)
	next, _ := key(m, "p")
	next, _ = key(next, "enter")
	return next, next.(Model).form.act
}

func TestScanFlashOnError(t *testing.T) {
	m, _ := asked(t)
	next, _ := key(m, "y")
	done, _ := next.Update(resultMsg(proto.Result{ActID: next.(Model).scanAct, Err: "a no came"}))
	if f := done.(Model).flash; f != "not found: a no came" {
		t.Fatalf("box: flash %q", f)
	}
	next, act := pickSent(t)
	roots := next.(Model).form.roots
	done, _ = next.Update(resultMsg(proto.Result{ActID: act, Err: "stopped"}))
	if nm := done.(Model); nm.form == nil || nm.form.err != "stopped" || nm.form.roots != roots || !nm.form.pick {
		t.Fatalf("pick: form %+v, flash %q: the error stays in the form, with the folders typed", nm.form, nm.flash)
	}
}

// TestScanBoxNeverOverAConfirm: a y meant for a destructive step runs
// it, and does not answer the scan.
func TestScanBoxNeverOverAConfirm(t *testing.T) {
	m, f := asked(t)
	m.pending = &action{label: "kill", say: "kill it", rec: "kill:%1", destructive: true}
	if m.scanShown() || m.focus(keyPress("y")) != layerPending {
		t.Fatalf("box %v, focus %v", m.scanShown(), m.focus(keyPress("y")))
	}
	next, cmd := key(m, "y")
	sendAll(t, cmd)
	a := f.acts()
	if len(a) != 1 || a[0].RecID != "kill:%1" || a[0].Confirm != "y" {
		t.Fatalf("sent %+v", a)
	}
	if nm := next.(Model); nm.scanAsk == nil || !nm.scanShown() {
		t.Fatal("the scan question went with the confirm")
	}
}

func TestScanFlashPartial(t *testing.T) {
	m, _ := asked(t)
	next, _ := key(m, "y")
	r := proto.Result{ActID: next.(Model).scanAct, Scan: &proto.ScanSummary{Repos: 7, Stopped: true, Errors: []string{"the scan took too long"},
		Skipped: []gitscan.Skip{{Path: "/x/a", Why: "it did not answer"}}}}
	done, _ := next.Update(resultMsg(r))
	want := "found 7 Git repositories, but the scan stopped early: the scan took too long"
	if f := done.(Model).flash; f != want {
		t.Fatalf("flash %q, want %q", f, want)
	}
	next, act := pickSent(t)
	r.ActID = act
	done, _ = next.Update(resultMsg(r))
	if f := done.(Model).flash; f != want {
		t.Fatalf("pick: flash %q, want %q", f, want)
	}
}

func TestScanFlashFromReply(t *testing.T) {
	const ok = "found your Git repositories: N offers them"
	broken := askState()
	broken.Found = &proto.Found{Consent: "yes", Repos: []proto.Checkout{{Path: "/h/a"}}, Errors: []string{"the broadcast's own"}}
	reply := func(m tea.Model) resultMsg {
		return resultMsg(proto.Result{ActID: m.(Model).scanAct, Scan: &proto.ScanSummary{Repos: 1}})
	}

	m, _ := asked(t)
	next, _ := key(m, "y")
	next, _ = next.Update(reply(next))
	next, _ = next.Update(stateMsg(broken))
	if f := next.(Model).flash; f != ok {
		t.Fatalf("result, then broadcast: flash %q", f)
	}

	m, _ = asked(t)
	next, _ = key(m, "y")
	r := reply(next)
	next, _ = next.Update(stateMsg(broken))
	next, _ = next.Update(r)
	if f := next.(Model).flash; f != ok {
		t.Fatalf("broadcast, then result: flash %q", f)
	}

	m, _ = asked(t)
	next, _ = key(m, "y")
	next, _ = next.Update(reply(next))
	if f := next.(Model).flash; f != ok {
		t.Fatalf("no broadcast: flash %q", f)
	}
}

func TestScanFlashListsSkipped(t *testing.T) {
	m, _ := asked(t)
	next, _ := key(m, "y")
	act := next.(Model).scanAct
	h := home
	if h == "" || h == "/" {
		h = "/home/b"
	}
	three := []gitscan.Skip{{Path: h + "/Documents", Why: "macOS asks first; p picks it"}, {Path: h + "/Downloads"}, {Path: "/mnt/nas", Why: "it did not answer"}}
	done, _ := next.Update(resultMsg(proto.Result{ActID: act, Scan: &proto.ScanSummary{Repos: 4, Skipped: three}}))
	want := "found your Git repositories: N offers them; skipped " + tilde(h+"/Documents") + " " + tilde(h+"/Downloads") + " /mnt/nas"
	if f := done.(Model).flash; f != want {
		t.Fatalf("flash %q, want %q", f, want)
	}

	var forty []gitscan.Skip
	for i := range 40 {
		forty = append(forty, gitscan.Skip{Path: fmt.Sprintf("/mnt/share-%02d", i), Why: "it did not answer"})
	}
	done, _ = next.Update(resultMsg(proto.Result{ActID: act, Scan: &proto.ScanSummary{Repos: 4, Skipped: forty, SkippedMore: 5}}))
	f := done.(Model).flash
	if !regexp.MustCompile(`^found your Git repositories: N offers them; skipped /mnt/share-00 .* and \d+ more$`).MatchString(f) {
		t.Fatalf("flash %q", f)
	}
	if n := len([]rune(f)); n > m.width-2 {
		t.Fatalf("flash is %d wide on %d columns: %q", n, m.width, f)
	}
	var shown int
	fmt.Sscanf(f[strings.LastIndex(f, "and ")+4:], "%d", &shown)
	if listed := strings.Count(f, "/mnt/share-"); listed+shown != 45 {
		t.Fatalf("lists %d and %d more of 45: %q", listed, shown, f)
	}
}

// manyFound is 5000 checkouts found and 10 worktrees on the Git tab, 3
// of them found too.
func manyFound() proto.State {
	st := lifeState()
	var repos []gitscan.Repo
	for i := range 10 {
		p := fmt.Sprintf("/w/%d", i)
		repos = append(repos, gitscan.Repo{Path: p, Worktrees: []proto.Worktree{{Path: p, Branch: "main"}}})
	}
	st.Git = &proto.GitReport{Repos: repos}
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var found []proto.Checkout
	for i := range 4997 {
		found = append(found, proto.Checkout{Path: fmt.Sprintf("/h/%04d", i), Active: t0.Add(time.Duration(i) * time.Minute)})
	}
	for i := range 3 {
		found = append(found, proto.Checkout{Path: fmt.Sprintf("/w/%d/", i), Active: t0.Add(time.Hour * 24 * 365)})
	}
	st.Found = &proto.Found{Consent: "yes", Repos: found}
	return st
}

// repoPaths are the paths of repos, each repo's worktrees after it.
func repoPaths(repos []whereRepo) []string {
	var out []string
	for _, r := range repos {
		out = append(out, r.path)
		for _, t := range r.trees {
			out = append(out, t.path)
		}
	}
	return out
}

func TestWhereReposBuiltOnceAndDeduped(t *testing.T) {
	m, _ := loadedWith(t, 100, manyFound())
	got := repoPaths(m.wrepos)
	if len(got) != 5007 || len(m.wrepos) != 5007 {
		t.Fatalf("%d paths, want 5007", len(got))
	}
	next, _ := key(m, "N")
	if next.(Model).form == nil {
		t.Fatal("no form")
	}
	rows := next.(Model).form.rows
	first := rows[slices.IndexFunc(rows, func(r whereRow) bool { return r.kind == rowRepo })]
	if p := first.repo.path; p != "/w/0" && p != "/w/1" && p != "/w/2" {
		t.Fatalf("the first suggested is %s: want a found Git tab repo, the newest", p)
	}
	before := next.(Model).wrepos
	for range 100 {
		screen(next)
	}
	if after := next.(Model).wrepos; &after[0] != &before[0] || &next.(Model).form.rows[0] != &rows[0] {
		t.Fatal("drawing the form rebuilt the repos")
	}
	next = typeText(next, "h/4996")
	if r := next.(Model).whereSelected(); r.path() != "/h/4996" {
		t.Fatalf("selected %+v", r)
	}
}

func lifeStateWithGit(g *proto.GitReport) proto.State {
	st := lifeState()
	st.Git = g
	return st
}

func TestWhereReposRebuiltOnGitReport(t *testing.T) {
	st := lifeState()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	st.Found = &proto.Found{Consent: "yes", Repos: []proto.Checkout{{Path: "/h/a", Active: t0}, {Path: "/h/b", Active: t0.Add(time.Hour)}}}
	m, _ := loadedWith(t, 100, st)
	next, _ := m.Update(stateMsg(lifeStateWithGit(&proto.GitReport{Repos: []gitscan.Repo{{Path: "/h/a", Worktrees: []proto.Worktree{
		{Path: "/h/a", Branch: "main"}, {Path: "/h/a-2", Branch: "feat/2"}}}}})))
	if got := repoPaths(next.(Model).wrepos); !slices.Equal(got, []string{"/h/a", "/h/a-2", "/h/b"}) {
		t.Fatalf("where's repos = %v", got)
	}
}

// TestScanBoxNotDrawnWhileTakeOverIsDue: with the stoker on, a key that
// acts on a session asks first; the scan box is not on top and takes no y.
func TestScanBoxNotDrawnWhileTakeOverIsDue(t *testing.T) {
	st := nightState()
	st.Found = &proto.Found{}
	m, _ := loadedWith(t, 100, st)
	if m.scanAsk == nil {
		t.Fatal("no scan question on a host never asked")
	}
	if m.scanShown() || m.consenting() || m.focus(keyPress("y")) == layerScan {
		t.Fatalf("box %v, consenting %v, focus %v", m.scanShown(), m.consenting(), m.focus(keyPress("y")))
	}
}

// pickTyped opens the folders form, types text over the usual folders and
// sends it.
func pickTyped(t *testing.T, text string) (tea.Model, *fakeConn) {
	t.Helper()
	m, f := asked(t)
	next, _ := key(m, "p")
	for range len([]rune(defaultRoots)) {
		next, _ = key(next, "backspace")
	}
	next = typeText(next, text)
	next, cmd := key(next, "enter")
	sendAll(t, cmd)
	return next, f
}

func TestPickFolderWithSpace(t *testing.T) {
	_, f := pickTyped(t, `"~/My Projects" ~/dev`)
	if a := f.acts(); len(a) != 1 || a[0].RecID != "scan:yes" || a[0].Text != "~/My Projects\n~/dev" {
		t.Fatalf("sent %+v, want two folders, the space kept", a)
	}
	next, f := pickTyped(t, `~/a "~/My Projects`)
	if nm := next.(Model); len(f.acts()) != 0 || nm.form == nil || nm.form.err == "" {
		t.Fatalf("an open quote: sent %+v, form %+v", f.acts(), nm.form)
	}
}

func TestSplitFolders(t *testing.T) {
	for in, want := range map[string][]string{
		`~ ~/dev`:                    {"~", "~/dev"},
		`  "~/My Projects"   ~/dev `: {"~/My Projects", "~/dev"},
		`~/a"b c"`:                   {"~/ab c"},
		`""`:                         nil,
		``:                           nil,
	} {
		got, err := splitFolders(in)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("splitFolders(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := splitFolders(`"~/a`); err == nil {
		t.Error("an open quote: no error")
	}
}

// A folder that is not there keeps the form open, with why and the
// folders typed: nothing replaced the usual ones.
func TestPickMissingFolderStaysOpen(t *testing.T) {
	next, _ := pickTyped(t, "~/dve")
	done, _ := next.Update(resultMsg(proto.Result{ActID: next.(Model).form.act, Err: "no folder ~/dve"}))
	nm := done.(Model)
	if nm.form == nil || !nm.form.pick || nm.form.err != "no folder ~/dve" || nm.form.roots != "~/dve" || nm.scanAsk == nil {
		t.Fatalf("form %+v, flash %q", nm.form, nm.flash)
	}
	if s := screen(done); !strings.Contains(s, "no folder ~/dve") {
		t.Fatalf("the form:\n%s", s)
	}
}

func TestScanFlashSaysNoneFound(t *testing.T) {
	m, _ := asked(t)
	next, _ := key(m, "y")
	done, _ := next.Update(resultMsg(proto.Result{ActID: next.(Model).scanAct, Scan: &proto.ScanSummary{}}))
	if f, want := done.(Model).flash, "found no Git repositories in "+defaultRoots; f != want {
		t.Fatalf("box: flash %q, want %q", f, want)
	}
	next, _ = pickTyped(t, `~/w "~/My Work"`)
	done, _ = next.Update(resultMsg(proto.Result{ActID: next.(Model).form.act, Scan: &proto.ScanSummary{}}))
	if f, want := done.(Model).flash, `found no Git repositories in ~/w "~/My Work"`; f != want {
		t.Fatalf("pick: flash %q, want %q", f, want)
	}
	long := strings.Repeat("~/folder ", 30)
	next, _ = pickTyped(t, strings.TrimSpace(long))
	done, _ = next.Update(resultMsg(proto.Result{ActID: next.(Model).form.act, Scan: &proto.ScanSummary{}}))
	if f := done.(Model).flash; !strings.HasPrefix(f, "found no Git repositories in ~/folder") || len([]rune(f)) > m.width-2 {
		t.Fatalf("pick, many folders: flash %q on %d columns", f, m.width)
	}
}

func TestScanFlashUnreadableIsNotAStop(t *testing.T) {
	m, _ := asked(t)
	next, _ := key(m, "y")
	act := next.(Model).scanAct
	for _, c := range []struct {
		sum  proto.ScanSummary
		want string
	}{
		{proto.ScanSummary{Repos: 12, Unreadable: 1, Errors: []string{"cannot read /home/x/data/lost+found: permission denied"}},
			"found 12 Git repositories; 1 folder could not be read: matchblox scan lists them"},
		{proto.ScanSummary{Repos: 1, Unreadable: 2, Errors: []string{"cannot read /a: permission denied", "cannot read /b: permission denied"}},
			"found 1 Git repository; 2 folders could not be read: matchblox scan lists them"},
		{proto.ScanSummary{Repos: 3, Unreadable: 25, Errors: append(slices.Repeat([]string{"cannot read /a: permission denied"}, 19), "and 6 more")},
			"found 3 Git repositories; 25 folders could not be read: matchblox scan lists them"},
		{proto.ScanSummary{Repos: 1, Stopped: true, Errors: []string{"stopped after 30s: some repositories may be missing"}},
			"found 1 Git repository, but the scan stopped early: stopped after 30s: some repositories may be missing"},
		// Lines that are no unreadable folder are not counted as one.
		{proto.ScanSummary{Repos: 4, Errors: []string{"no folder /h/gone"}},
			"found your Git repositories: N offers them"},
		{proto.ScanSummary{Repos: 1, Unsaved: "open /x/repos.json: permission denied", Errors: []string{"repos.json: open /x/repos.json: permission denied"}},
			"found 1 Git repository, but they could not be saved: open /x/repos.json: permission denied"},
		{proto.ScanSummary{Repos: 2, Unreadable: 1, Unsaved: "disk full", Errors: []string{"cannot read /a: permission denied", "repos.json: disk full"}},
			"found 2 Git repositories, but they could not be saved: disk full"},
	} {
		done, _ := next.Update(resultMsg(proto.Result{ActID: act, Scan: &c.sum}))
		if f := done.(Model).flash; f != c.want {
			t.Errorf("%+v: flash %q, want %q", c.sum, f, c.want)
		}
	}
}

// None found with folders skipped says none were found, then lists them.
func TestScanFlashNoneFoundButSkipped(t *testing.T) {
	m, _ := asked(t)
	next, _ := key(m, "y")
	act := next.(Model).scanAct
	two := []gitscan.Skip{{Path: "/mnt/a", Why: "it did not answer"}, {Path: "/mnt/b", Why: "it did not answer"}}
	done, _ := next.Update(resultMsg(proto.Result{ActID: act, Scan: &proto.ScanSummary{Skipped: two}}))
	if f, want := done.(Model).flash, "found no Git repositories; skipped /mnt/a /mnt/b"; f != want {
		t.Fatalf("flash %q, want %q", f, want)
	}
	var forty []gitscan.Skip
	for i := range 40 {
		forty = append(forty, gitscan.Skip{Path: fmt.Sprintf("/mnt/share-%02d", i), Why: "it did not answer"})
	}
	done, _ = next.Update(resultMsg(proto.Result{ActID: act, Scan: &proto.ScanSummary{Skipped: forty, SkippedMore: 5}}))
	f := done.(Model).flash
	if !regexp.MustCompile(`^found no Git repositories; skipped /mnt/share-00 .* and \d+ more$`).MatchString(f) || len([]rune(f)) > m.width-2 {
		t.Fatalf("flash %q on %d columns", f, m.width)
	}
}

func TestPickSeveralMissingFoldersStaysOpen(t *testing.T) {
	next, _ := pickTyped(t, `~/dve "~/My Work"`)
	done, _ := next.Update(resultMsg(proto.Result{ActID: next.(Model).form.act, Err: "no folder ~/dve, ~/My Work"}))
	if nm := done.(Model); nm.form == nil || nm.form.err != "no folder ~/dve, ~/My Work" || nm.form.roots != `~/dve "~/My Work"` {
		t.Fatalf("form %+v", nm.form)
	}
}
