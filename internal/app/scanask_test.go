package app

import (
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

func sendAll(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd != nil {
		cmd()
	}
}

func TestScanBoxShowsOnce(t *testing.T) {
	m, _ := loadedWith(t, 100, askState())
	if !m.asksScan() {
		t.Fatal("no scan question on a host never asked")
	}
	s := screen(m)
	for _, want := range []string{"Find the Git repositories in your home folder?", "they stay on this host and your paired devices", "y find", "n never", "p pick folders", "esc later"} {
		if !strings.Contains(s, want) {
			t.Errorf("the box lacks %q:\n%s", want, s)
		}
	}
}

func TestScanBoxYes(t *testing.T) {
	m, f := loadedWith(t, 100, askState())
	next, cmd := key(m, "y")
	sendAll(t, cmd)
	if a := f.acts(); len(a) != 1 || a[0].RecID != "scan:yes" || a[0].Confirm != "y" || a[0].Text != "" {
		t.Fatalf("sent %+v", a)
	}
	if nm := next.(Model); nm.asksScan() || nm.flash != "finding your Git repositories…" {
		t.Fatalf("open %v, flash %q", nm.asksScan(), nm.flash)
	}
}

func TestScanBoxNo(t *testing.T) {
	m, f := loadedWith(t, 100, askState())
	next, cmd := key(m, "n")
	sendAll(t, cmd)
	if a := f.acts(); len(a) != 1 || a[0].RecID != "scan:no" {
		t.Fatalf("sent %+v", a)
	}
	if next.(Model).asksScan() {
		t.Fatal("the box stayed open")
	}
}

func TestScanBoxLater(t *testing.T) {
	m, f := loadedWith(t, 100, askState())
	next, _ := key(m, "esc")
	if next.(Model).asksScan() {
		t.Fatal("esc kept the box")
	}
	next, _ = next.Update(stateMsg(askState()))
	if next.(Model).asksScan() {
		t.Fatal("the next snapshot asked again")
	}
	m2, _ := loadedWith(t, 100, askState())
	next, _ = tap(m2, 5, 5)
	if next.(Model).asksScan() {
		t.Fatal("a tap kept the box")
	}
	if len(f.acts()) != 0 {
		t.Fatalf("sent %+v", f.acts())
	}
}

func TestScanBoxPickFolders(t *testing.T) {
	m, f := loadedWith(t, 100, askState())
	next, _ := key(m, "p")
	fm := next.(Model).form
	if fm == nil || !fm.pick || fm.roots != "~ ~/dev ~/src ~/code ~/projects ~/work" || !slices.Equal(fm.fields(), []int{fieldFolders}) {
		t.Fatalf("form %+v", fm)
	}
	if s := screen(next); !strings.Contains(s, "folders") {
		t.Fatalf("the form:\n%s", s)
	}
	next, _ = key(next, "esc")
	if !next.(Model).asksScan() {
		t.Fatal("esc in the folders did not go back to the box")
	}
	next, _ = key(next, "p")
	for range len("~/projects ~/work") {
		next, _ = key(next, "backspace")
	}
	next, cmd := key(next, "enter")
	sendAll(t, cmd)
	if a := f.acts(); len(a) != 1 || a[0].RecID != "scan:yes" || a[0].Text != "~\n~/dev\n~/src\n~/code" {
		t.Fatalf("sent %+v", a)
	}
	next, _ = next.Update(resultMsg(proto.Result{ActID: next.(Model).form.act}))
	if nm := next.(Model); nm.form != nil || nm.asksScan() {
		t.Fatalf("form %+v, box %v", nm.form, nm.asksScan())
	}
}

func TestScanBoxClosesWhenAnsweredElsewhere(t *testing.T) {
	m, _ := loadedWith(t, 100, askState())
	st := lifeState()
	st.Found = &proto.Found{Consent: "yes"}
	next, _ := m.Update(stateMsg(st))
	if next.(Model).asksScan() {
		t.Fatal("the box stayed after another console's yes")
	}
}

func TestNoBoxWithoutFound(t *testing.T) {
	m, _ := loadedWith(t, 100, lifeState())
	next, _ := m.Update(stateMsg(lifeState()))
	if next.(Model).asksScan() {
		t.Fatal("asked with no Found: an older service scans nothing")
	}
}

func TestFoundFeedsWhereNotGit(t *testing.T) {
	st := lifeState()
	st.Git = &proto.GitReport{Repos: []gitscan.Repo{{Path: "/w/app", Worktrees: []proto.Worktree{{Path: "/w/app", Branch: "main"}}}}}
	old, recent := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	st.Found = &proto.Found{Consent: "yes", Repos: []proto.Checkout{{Path: "/w/app"}, {Path: "/h/old", Active: old}, {Path: "/h/new", Active: recent}}}
	m, _ := loadedWith(t, 100, st)
	if got := m.gitPaths(); !slices.Equal(got, []string{"/w/app", "/h/new", "/h/old"}) {
		t.Fatalf("gitPaths = %v", got)
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
	for range len(next.(Model).form.where) {
		next, _ = key(next, "backspace")
	}
	next, _ = key(next, "enter")
	if f := next.(Model).form; f == nil || f.err != "where does it start? a path, or ←→ for a repository" {
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
