package app

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	vt "github.com/charmbracelet/x/vt"

	"github.com/blox-eng/matchblox/internal/proto"
)

// hostileDiffText is the file content from Task 13's hostile test: OSC title
// and OSC 52 clipboard, kept as data until the review draws them.
const hostileDiffText = "\x1b]0;pwned\x07\x1b]52;c;cHduZWQ=\x07"

// TestAnEscapeInTheDiffDrawsAsText: the file from Task 13 draws as visible
// control pictures, and the terminal's title does not change.
func TestAnEscapeInTheDiffDrawsAsText(t *testing.T) {
	m := reviewDesk(t)
	m = openReviewWith(t, m, &proto.Diff{
		Against: "main",
		Files: []proto.DiffFile{{
			Path: "hostile.txt", Added: 1,
			Hunks: []proto.DiffHunk{{
				NewStart: 1, NewCount: 1,
				Lines: []proto.DiffLine{{Kind: "+", Text: hostileDiffText, New: 1}},
			}},
		}},
	}, "")
	plain := ansi.Strip(m.render())
	if !strings.Contains(plain, "␛]0;pwned␇") {
		t.Fatalf("the review does not draw the escape as visible text:\n%s", plain)
	}
	var title string
	e := vt.NewEmulator(200, 50)
	e.SetCallbacks(vt.Callbacks{Title: func(s string) { title = s }})
	_, _ = e.WriteString(m.render())
	if title != "" {
		t.Fatalf("the terminal's title became %q; the escape must not run", title)
	}
}

// TestDShowsTheReview: D on ux-pass opens the review sheet with its head
// and file line; ] and [ move the file.
func TestDShowsTheReview(t *testing.T) {
	m := reviewDesk(t)
	m = openReviewWith(t, m, reviewFourFiles(), "")
	plain := ansi.Strip(m.render())
	wantHead := "ux-pass · diff against main · 4 files +212 −38   esc chat"
	if !strings.Contains(plain, wantHead) {
		t.Fatalf("the review head is missing %q:\n%s", wantHead, plain)
	}
	if !strings.Contains(plain, "frame.go") {
		t.Fatalf("the file line is missing:\n%s", plain)
	}
	m = pressOn(m, "]")
	if got := m.review.file; got != 1 {
		t.Fatalf("] moved to file %d, want 1", got)
	}
	if !strings.Contains(ansi.Strip(m.render()), "view.go") {
		t.Fatalf("] did not show view.go:\n%s", ansi.Strip(m.render()))
	}
	m = pressOn(m, "[")
	if got := m.review.file; got != 0 {
		t.Fatalf("[ moved to file %d, want 0", got)
	}
}

// TestCWritesAComment: ↓, c, type, ⏎ keeps a ✎ comment under line 41.
func TestCWritesAComment(t *testing.T) {
	m := reviewDesk(t)
	m = openReviewWith(t, m, reviewCommentDiff(), "")
	m = pressOn(m, "down")
	m = pressOn(m, "c")
	m = typeText(m, "fold at width").(Model)
	m = pressOn(m, "enter")
	plain := ansi.Strip(m.render())
	if !strings.Contains(plain, "✎") || !strings.Contains(plain, "fold at width") {
		t.Fatalf("no ✎ comment under the line:\n%s", plain)
	}
	wantFoot := "1 comment · c comment · ] [ file · ⏎ send review to ux-pass"
	if !strings.Contains(plain, wantFoot) {
		t.Fatalf("the foot is missing %q:\n%s", wantFoot, plain)
	}
}

// TestEscKeepsTheDraft: esc while typing keeps the draft for the next c
// (owner, 2026-11-23, design review D8).
func TestEscKeepsTheDraft(t *testing.T) {
	m := reviewDesk(t)
	m = openReviewWith(t, m, reviewCommentDiff(), "")
	m = pressOn(m, "down") // line 41
	m = pressOn(m, "c")
	m = typeText(m, "fold at").(Model)
	m = pressOn(m, "esc")
	if m.review.typing {
		t.Fatal("esc left typing on")
	}
	if n := len(m.reviews.Comments[m.review.key]); n != 0 {
		t.Fatalf("esc added %d comments; want none", n)
	}
	for _, line := range strings.Split(ansi.Strip(m.render()), "\n") {
		if strings.Contains(line, "✎") && strings.Contains(line, "fold at") {
			t.Fatalf("esc added a ✎ line:\n%s", line)
		}
	}
	m = pressOn(m, "c")
	if !m.review.typing || m.review.draft != "fold at" {
		t.Fatalf("next c opened with typing=%v draft %q, want fold at", m.review.typing, m.review.draft)
	}
}

// TestUnsentCommentsStay: esc closes the sheet; the chat head and the row
// show ✎; a restart keeps them in reviews.json.
func TestUnsentCommentsStay(t *testing.T) {
	dir := t.TempDir()
	m := reviewDeskAt(t, dir)
	m = openReviewWith(t, m, reviewCommentDiff(), "")
	m = pressOn(m, "down", "c")
	m = typeText(m, "fold at width").(Model)
	m = pressOn(m, "enter", "esc")
	if m.over == "review" || m.review != nil {
		t.Fatalf("esc left the review open (%q, review %v)", m.over, m.review != nil)
	}
	s, _ := m.sideChosen()
	head := ansi.Strip(m.chatHead(s, 120))
	if !strings.Contains(head, "✎ 1 unsent · D reviews") {
		t.Fatalf("the chat's head is %q, want ✎ 1 unsent · D reviews", head)
	}
	row := reviewRowOf(m, "ux-pass")
	if !strings.Contains(ansi.Strip(row), "✎") {
		t.Fatalf("the row has no ✎:\n%s", row)
	}
	path := filepath.Join(dir, reviewsFile)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("reviews.json was not written: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("reviews.json mode %v, want 0600", info.Mode().Perm())
	}
	again := reviewDeskAt(t, dir)
	again = chooseNamed(t, again, "ux-pass")
	s, _ = again.sideChosen()
	if n := len(again.reviewsOf(s)); n != 1 {
		t.Fatalf("a restart kept %d comments, want 1", n)
	}
	head = ansi.Strip(again.chatHead(s, 120))
	if !strings.Contains(head, "✎ 1 unsent · D reviews") {
		t.Fatalf("after restart the chat's head is %q", head)
	}
}

// TestTheReviewStates: no git, no changes, the fallback base, a big diff
// and the loading words each show.
func TestTheReviewStates(t *testing.T) {
	m := reviewDesk(t)
	next, cmd := key(chooseNamed(t, m, "ux-pass"), "D")
	m = next.(Model)
	drain(cmd)
	plain := ansi.Strip(m.render())
	if !strings.Contains(plain, "reading the diff of ux-pass") {
		t.Fatalf("while loading, want reading the diff of ux-pass…:\n%s", plain)
	}

	cases := []struct {
		name string
		diff *proto.Diff
		err  string
		want string
	}{
		{"no git", nil, "No changes to review: ux-pass has no git folder.", "No changes to review: ux-pass has no git folder."},
		{"no changes", nil, "ux-pass has no changes against main.", "ux-pass has no changes against main."},
		{"fallback", &proto.Diff{Against: "origin/main", Note: "main is gone on ws-1", Files: []proto.DiffFile{{Path: "a.go", Added: 1, Hunks: []proto.DiffHunk{{NewStart: 1, NewCount: 1, Lines: []proto.DiffLine{{Kind: "+", Text: "x", New: 1}}}}}}}, "", "main is gone on ws-1"},
		{"big", &proto.Diff{Against: "main", Files: []proto.DiffFile{{Path: "big.go", Added: 9}}}, "", "big.go"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := openReviewWith(t, reviewDesk(t), c.diff, c.err)
			plain := ansi.Strip(m.render())
			if !strings.Contains(plain, c.want) {
				t.Fatalf("want %q:\n%s", c.want, plain)
			}
		})
	}
}

// TestTheDiffInksAreTokens: each diff ink is an existing token (D11).
func TestTheDiffInksAreTokens(t *testing.T) {
	m := reviewDesk(t)
	m = openReviewWith(t, m, &proto.Diff{
		Against: "main",
		Files: []proto.DiffFile{{
			Path: "frame.go", Added: 2, Removed: 1,
			Hunks: []proto.DiffHunk{{
				OldStart: 39, OldCount: 1, NewStart: 39, NewCount: 2,
				Lines: []proto.DiffLine{
					{Kind: "+", Text: "added line", New: 39},
					{Kind: "-", Text: "removed line", Old: 50},
					{Kind: " ", Text: "context", Old: 40, New: 40},
				},
			}},
		}},
	}, "")
	m = pressOn(m, "c")
	m = typeText(m, "note").(Model)
	m = pressOn(m, "enter")
	lineOf := func(sub string) string {
		for _, line := range strings.Split(m.render(), "\n") {
			if strings.Contains(ansi.Strip(line), sub) {
				return line
			}
		}
		t.Fatalf("no line holds %q", sub)
		return ""
	}
	checks := []struct{ sub, want string }{
		{"added line", "fg"},
		{"removed line", "faint"},
		{"✎", "accent"},
	}
	for _, c := range checks {
		if got := inkOf(t, lineOf(c.sub), c.sub); !strings.HasPrefix(got, c.want) {
			t.Errorf("%q wears %s, want %s", c.sub, got, c.want)
		}
	}
	addLine := lineOf("added line")
	if got := inkOf(t, addLine, "+"); !strings.HasPrefix(got, "accent") {
		t.Errorf("+ wears %s, want accent", got)
	}
	// an unselected line's number is faint (the chosen one is accent, as the mockup)
	delLine := lineOf("removed line")
	if got := inkOf(t, delLine, "50"); !strings.HasPrefix(got, "faint") {
		t.Errorf("line number wears %s, want faint", got)
	}
	if got := inkOf(t, delLine, "−"); !strings.HasPrefix(got, "neg") {
		t.Errorf("− wears %s, want neg", got)
	}
	// colour off: + and − still tell the lines apart
	m.st = newStyles(false)
	plain := ansi.Strip(m.render())
	if !strings.Contains(plain, "+") || !strings.Contains(plain, "−") {
		t.Fatalf("with colour off the +/− glyphs are gone:\n%s", plain)
	}
}

func reviewDesk(t *testing.T) Model {
	t.Helper()
	return reviewDeskAt(t, t.TempDir())
}

func reviewDeskAt(t *testing.T, dir string) Model {
	t.Helper()
	m := withAgent(t, mockDesk(t, 160, 40, true), agentRows(36), false)
	m.opt.StateDir = dir
	m.reviews = loadReviews(dir)
	return chooseNamed(t, m, "ux-pass")
}

// openReviewWith presses D on the chosen session and feeds the diff result.
func openReviewWith(t *testing.T, m Model, diff *proto.Diff, err string) Model {
	t.Helper()
	next, cmd := key(m, "D")
	m = next.(Model)
	drain(cmd)
	id := "a" + strconv.Itoa(m.acts)
	m = upd(t, m, resultMsg(proto.Result{ActID: id, Diff: diff, Err: err}))
	if m.over != "review" {
		t.Fatalf("D opened %q, want review", m.over)
	}
	return m
}

func reviewFourFiles() *proto.Diff {
	return &proto.Diff{
		Against: "main",
		Files: []proto.DiffFile{
			{Path: "frame.go", Added: 120, Removed: 0, Hunks: []proto.DiffHunk{{NewStart: 1, NewCount: 1, Lines: []proto.DiffLine{{Kind: "+", Text: "package app", New: 1}}}}},
			{Path: "view.go", Added: 61, Removed: 22, Hunks: []proto.DiffHunk{{NewStart: 1, NewCount: 1, Lines: []proto.DiffLine{{Kind: "+", Text: "x", New: 1}}}}},
			{Path: "sidepanel.go", Added: 19, Removed: 16, Hunks: []proto.DiffHunk{{NewStart: 1, NewCount: 1, Lines: []proto.DiffLine{{Kind: "+", Text: "x", New: 1}}}}},
			{Path: "console.md", Added: 12, Removed: 0, Hunks: []proto.DiffHunk{{NewStart: 1, NewCount: 1, Lines: []proto.DiffLine{{Kind: "+", Text: "x", New: 1}}}}},
		},
	}
}

// reviewCommentDiff has line 40 then 41 so one ↓ lands on 41.
func reviewCommentDiff() *proto.Diff {
	return &proto.Diff{
		Against: "main",
		Files: []proto.DiffFile{{
			Path: "frame.go", Added: 2,
			Hunks: []proto.DiffHunk{{
				NewStart: 40, NewCount: 2,
				Lines: []proto.DiffLine{
					{Kind: "+", Text: "    usage := m.usageChips(st)", New: 40},
					{Kind: "+", Text: "    if width(chips)+width(usage) > w {", New: 41},
				},
			}},
		}},
	}
}

func reviewRowOf(m Model, name string) string {
	for _, s := range m.all.sessions {
		if s.Name == name || m.cardName(s) == name {
			return strings.Join(m.sideRow(s, 40, true, sideLink{}), "\n")
		}
	}
	return ""
}
