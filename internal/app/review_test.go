package app

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

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

// TestAHostileReviewIsStripped: control bytes in a comment and a diff line
// leave the paste with those bytes gone (pasteText's rule). A review over
// MaxPaste is refused; the comments stay.
func TestAHostileReviewIsStripped(t *testing.T) {
	m := reviewDesk(t)
	diff := &proto.Diff{
		Against: "main",
		Files: []proto.DiffFile{{
			Path: "hostile.go", Added: 1,
			Hunks: []proto.DiffHunk{{
				NewStart: 1, NewCount: 1,
				Lines: []proto.DiffLine{{Kind: "+", Text: "\x1b]52;c;cHduZWQ=\x07 ok", New: 1}},
			}},
		}},
	}
	m = openReviewWith(t, m, diff, "")
	m.setReviewComments(m.review.key, []reviewComment{{
		File: "hostile.go", Line: 1, Text: "clear\x1b[2J\x00\x9bit",
	}})
	m = pressOn(m, "enter")
	acts := messageActs(boardFake(m))
	if len(acts) != 1 || !strings.HasPrefix(acts[0].RecID, proto.PastePrefix) {
		t.Fatalf("sent %+v, want one paste:", acts)
	}
	text := acts[0].Text
	for _, bad := range []string{"\x1b", "\x00", "\x9b"} {
		if strings.Contains(text, bad) {
			t.Fatalf("paste still holds %q: %q", bad, text)
		}
	}
	if !strings.Contains(text, "ok") || !strings.Contains(text, "clear") || !strings.Contains(text, "it") {
		t.Fatalf("paste lost the safe text: %q", text)
	}

	m = reviewDesk(t)
	m = openReviewWith(t, m, reviewCommentDiff(), "")
	// Pad so the paste body is just over 17 KB and rounds to 18 KB.
	overhead := len(reviewPasteText(m.reviewMessage([]reviewComment{{File: "frame.go", Line: 41, Text: ""}})))
	long := strings.Repeat("x", 18*1024-overhead)
	m.setReviewComments(m.review.key, []reviewComment{{File: "frame.go", Line: 41, Text: long}})
	before := len(messageActs(boardFake(m)))
	m = pressOn(m, "enter")
	if got := messageActs(boardFake(m)); len(got) != before {
		t.Fatalf("an over-long review sent %+v", got[before:])
	}
	want := "not sent: the review is too long (18 KB, 16 KB at most). Send fewer comments at once."
	if m.noticeText != want {
		t.Fatalf("notice %q, want %q", m.noticeText, want)
	}
	if n := len(m.reviews.Comments[m.review.key]); n != 1 {
		t.Fatalf("comments cleared (%d); want them kept", n)
	}
	if m.over != "review" {
		t.Fatalf("over %q, want the review still open", m.over)
	}
}

// TestSendGivesOneMessage: two comments become one paste:; the notice names
// both, and the unsent store is empty.
func TestSendGivesOneMessage(t *testing.T) {
	m := reviewDesk(t)
	diff := &proto.Diff{
		Against: "main",
		Files: []proto.DiffFile{
			{Path: "internal/app/footer.go", Added: 1, Hunks: []proto.DiffHunk{{
				NewStart: 41, NewCount: 1,
				Lines: []proto.DiffLine{{Kind: "+", Text: "    return foot", New: 41}},
			}}},
			{Path: "internal/app/view.go", Added: 1, Hunks: []proto.DiffHunk{{
				NewStart: 12, NewCount: 1,
				Lines: []proto.DiffLine{{Kind: "+", Text: "package app", New: 12}},
			}}},
		},
	}
	m = openReviewWith(t, m, diff, "")
	m.setReviewComments(m.review.key, []reviewComment{
		{File: "internal/app/footer.go", Line: 41, Text: "fold at width"},
		{File: "internal/app/view.go", Line: 12, Text: "keep the package line"},
	})
	m = pressOn(m, "enter")
	acts := messageActs(boardFake(m))
	if len(acts) != 1 || acts[0].RecID != proto.PastePrefix+"%ux-pass" || acts[0].Confirm != "y" {
		t.Fatalf("sent %+v", acts)
	}
	text := acts[0].Text
	for _, want := range []string{
		"internal/app/footer.go:41",
		"    return foot",
		"fold at width",
		"internal/app/view.go:12",
		"package app",
		"keep the package line",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("paste missing %q:\n%s", want, text)
		}
	}
	wantNotice := "sent a review of 2 comments to ux-pass · now in WORKING"
	if m.noticeText != wantNotice {
		t.Fatalf("notice %q, want %q", m.noticeText, wantNotice)
	}
	if m.over == "review" || m.review != nil {
		t.Fatalf("the review stayed open")
	}
	s, _ := m.sideChosen()
	if n := len(m.reviewsOf(s)); n != 0 {
		t.Fatalf("unsent store has %d comments", n)
	}
}

// TestABusyAgentGetsItAtTheEndOfItsTurn: OpenCode that works holds the
// review; the notice queues it; ✎ stays until the turn ends and the paste goes.
func TestABusyAgentGetsItAtTheEndOfItsTurn(t *testing.T) {
	dir := t.TempDir()
	ux := workingAs("ux-pass", "opencode")
	ux.Cwd = "/dev/matchblox"
	m := withAgent(t, deskWith(t, 160, 40, true, boardState(ux), boardState()), agentRows(36), false)
	m.opt.StateDir = dir
	m.reviews = loadReviews(dir)
	m = chooseNamed(t, m, "ux-pass")
	if m.sideStateOf(m.all.sessions[0]) != sideWork {
		t.Fatalf("ux-pass does not work: %v", m.sideStateOf(m.all.sessions[0]))
	}
	m = openReviewWith(t, m, reviewCommentDiff(), "")
	m.setReviewComments(m.review.key, []reviewComment{{File: "frame.go", Line: 41, Text: "fold at width"}})
	m = pressOn(m, "enter")
	if got := messageActs(boardFake(m)); len(got) != 0 {
		t.Fatalf("sent to a busy agent: %+v", got)
	}
	want := "queued for ux-pass · sends at the end of its turn"
	if m.noticeText != want {
		t.Fatalf("notice %q, want %q", m.noticeText, want)
	}
	if strings.Contains(m.noticeText, "now in WORKING") {
		t.Fatalf("busy notice said now in WORKING: %q", m.noticeText)
	}
	s, _ := m.sideChosen()
	if n := len(m.reviewsOf(s)); n != 1 {
		t.Fatalf("✎ cleared early: %d comments", n)
	}
	if row := reviewRowOf(m, "ux-pass"); !strings.Contains(ansi.Strip(row), "✎") {
		t.Fatalf("the row lost ✎:\n%s", row)
	}

	idle := boardMatch("ux-pass", time.Hour)
	idle.Cwd = "/dev/matchblox"
	next, cmd := m.Update(stateMsg(boardState(idle)))
	drain(cmd)
	m = next.(Model)
	acts := messageActs(boardFake(m))
	if len(acts) != 1 || acts[0].RecID != proto.PastePrefix+"%ux-pass" || !strings.Contains(acts[0].Text, "fold at width") {
		t.Fatalf("after the turn: %+v", acts)
	}
	if n := len(m.reviewsOf(idle)); n != 0 {
		t.Fatalf("comments left after send: %d", n)
	}
}

// TestTheReviewGoesToItsSession: the sheet of ux-pass sends to ux-pass
// even when the list's choice has moved to scout.
func TestTheReviewGoesToItsSession(t *testing.T) {
	dir := t.TempDir()
	ux := doneMatch("ux-pass", 2*time.Minute)
	ux.Cwd = "/dev/matchblox"
	scout := boardMatch("scout", time.Hour)
	scout.Cwd = "/dev/matchblox"
	m := withAgent(t, deskWith(t, 160, 40, true, boardState(ux, scout), boardState()), agentRows(36), false)
	m.opt.StateDir = dir
	m.reviews = loadReviews(dir)
	m = chooseNamed(t, m, "ux-pass")
	m = openReviewWith(t, m, reviewCommentDiff(), "")
	m.setReviewComments(m.review.key, []reviewComment{{File: "frame.go", Line: 41, Text: "fold at width"}})
	m = chooseNamed(t, m, "scout")
	m = pressOn(m, "enter")
	acts := messageActs(boardFake(m))
	if len(acts) != 1 || acts[0].RecID != proto.PastePrefix+"%ux-pass" {
		t.Fatalf("sent %+v, want paste:%%ux-pass only", acts)
	}
	for _, a := range boardFake(m).acts() {
		if strings.HasPrefix(a.RecID, proto.PastePrefix) && strings.Contains(a.RecID, "scout") {
			t.Fatalf("a paste reached scout: %+v", a)
		}
	}
}
