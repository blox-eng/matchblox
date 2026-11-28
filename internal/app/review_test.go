package app

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// reviewTmux is the console's tmux for the review tests. It keeps the
// window options the review writes and reads, and it records each call.
type reviewTmux struct {
	mu    sync.Mutex
	opts  map[string]string
	calls [][]string
}

func (r *reviewTmux) run(args ...string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, append([]string(nil), args...))
	if r.opts == nil {
		r.opts = map[string]string{}
	}
	switch args[0] {
	case "set-option":
		unset := false
		var rest []string
		for i := 1; i < len(args); i++ {
			switch {
			case args[i] == "-t":
				i++
			case strings.HasPrefix(args[i], "-"):
				unset = unset || strings.Contains(args[i], "u")
			default:
				rest = append(rest, args[i])
			}
		}
		if len(rest) > 0 {
			switch {
			case unset:
				delete(r.opts, rest[0])
			case len(rest) > 1:
				r.opts[rest[0]] = rest[1]
			}
		}
	case "show-option":
		return r.opts[args[len(args)-1]] + "\n", nil
	case "display":
		target := ""
		for i, a := range args {
			if a == "-t" && i+1 < len(args) {
				target = args[i+1]
			}
		}
		switch args[len(args)-1] {
		case "#{pane_id}":
			return target + "\n", nil
		case "#{window_id}":
			if target == "%7" || target == "%8" { // the sessions' panes are in other windows
				return "@2\n", nil
			}
			return "@1\n", nil
		case "#{window_width} #{window_height}":
			return "160 40\n", nil
		case "#{pane_current_command}":
			return "tmux\n", nil
		}
	case "list-panes":
		return "%1\n", nil
	}
	return "", nil
}

func (r *reviewTmux) option(name string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.opts[name]
}

func (r *reviewTmux) lines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, c := range r.calls {
		out = append(out, strings.Join(c, " "))
	}
	return out
}

// respawns are the argv of each respawn-pane, after the `--`.
func (r *reviewTmux) respawns() [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out [][]string
	for _, c := range r.calls {
		if c[0] != "respawn-pane" {
			continue
		}
		if i := slices.Index(c, "--"); i >= 0 {
			out = append(out, c[i+1:])
		}
	}
	return out
}

// typed are the calls that put text or keys in a pane.
func (r *reviewTmux) typed() []string {
	var out []string
	for _, l := range r.lines() {
		if strings.HasPrefix(l, "send-keys") || strings.HasPrefix(l, "paste-buffer") || strings.HasPrefix(l, "load-buffer") {
			out = append(out, l)
		}
	}
	return out
}

func reviewGitIn(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false", "-C", dir}, args...)...) //nolint:gosec // a test's own repository
	cmd.Env = slices.DeleteFunc(os.Environ(), func(kv string) bool { return strings.HasPrefix(kv, "GIT_") })
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %q: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func reviewWrite(t testing.TB, dir, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// reviewRepo is a repository on branch ux-pass, two files ahead of main:
// a.go changes (+5 −1) and b.go is new (+3).
func reviewRepo(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	reviewGitIn(t, dir, "init", "-q", "-b", "main")
	reviewWrite(t, dir, "a.go", "package a\n\nfunc A() int {\n\treturn 1\n}\n")
	reviewWrite(t, dir, "notes.txt", "one\ntwo\nthree\n")
	reviewGitIn(t, dir, "add", ".")
	reviewGitIn(t, dir, "commit", "-q", "-m", "base")
	reviewGitIn(t, dir, "checkout", "-q", "-b", "ux-pass")
	reviewWrite(t, dir, "a.go", "package a\n\nfunc A() int {\n\treturn 2\n}\n\nfunc B() int {\n\treturn 3\n}\n")
	reviewWrite(t, dir, "b.go", "package a\n\nconst X = 1\n")
	reviewGitIn(t, dir, "add", ".")
	reviewGitIn(t, dir, "commit", "-q", "-m", "work")
	return dir
}

// reviewDesk is the list, with ux-pass (%7) and other (%8) in it and the
// agent seat at %9, and the tmux they share.
type reviewDesk struct {
	tm   *reviewTmux
	home *fakeConn // the list's conn to the service
	pane *fakeConn // the review pane's own conn to the service
	m    Model
}

func newReviewDesk(t *testing.T, cwd, start string) *reviewDesk {
	t.Helper()
	tm, home := &reviewTmux{}, newFake()
	pane := newFake()
	var next tea.Model = New(Options{
		NoMotion: true, Conn: home, Now: func() time.Time { return demoAt }, Beside: true, Pane: "%1",
		Tmux: tm.run, Self: "/bin/matchblox", Socket: "/tmp/tmux-1000/default",
	})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	st := hostState(
		sess(7, "ux-pass", "%7", func(s *sample.Session) { s.Cwd, s.SessionID = cwd, "s7" }),
		sess(8, "other", "%8", func(s *sample.Session) { s.Cwd, s.SessionID = t.TempDir(), "s8" }),
	)
	if start != "" {
		st.StartBranch = map[string]string{"ux-pass": start}
	}
	next, _ = next.Update(stateMsg(st))
	m := next.(Model)
	m.splashDone, m.deskFrame, m.beside = true, true, "%9"
	m.toList()
	return &reviewDesk{tm: tm, home: home, pane: pane, m: m}
}

// settle gives m the messages cmd makes, and the ones those make.
func reviewSettle(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for _, msg := range cmdMsgs(cmd) {
		next, c := m.Update(msg)
		m = reviewSettle(t, next.(Model), c)
	}
	return m
}

func (d *reviewDesk) choose(t *testing.T, name string) {
	t.Helper()
	d.m = chooseNamed(t, d.m, name)
}

// press presses k in the list.
func (d *reviewDesk) press(t *testing.T, k string) {
	t.Helper()
	next, cmd := key(d.m, k)
	d.m = reviewSettle(t, next.(Model), cmd)
}

// poll is one look of the list at its tmux, as its quarter-second does.
func (d *reviewDesk) poll(t *testing.T) {
	t.Helper()
	next, cmd := d.m.Update(d.m.watchDeskMode()())
	d.m = reviewSettle(t, next.(Model), cmd)
}

// pastes are the paste: acts the review pane sent to the service. The list
// sends none.
func (d *reviewDesk) pastes() []proto.Act {
	var out []proto.Act
	for _, a := range d.pane.allActs() {
		if strings.HasPrefix(a.RecID, proto.PastePrefix) {
			out = append(out, a)
		}
	}
	for _, a := range d.home.allActs() {
		if strings.HasPrefix(a.RecID, proto.PastePrefix) {
			panic("the list sent a paste: the review pane sends its own")
		}
	}
	return out
}

// answer is the service's answer to the review pane's last paste.
func (d *reviewDesk) answer(t *testing.T, f frameModel, r proto.Result) frameModel {
	t.Helper()
	d.pane.mu.Lock()
	var id string
	for _, e := range d.pane.sent {
		if e.Kind == proto.KindAct {
			var a proto.Act
			_ = json.Unmarshal(e.Body, &a)
			if strings.HasPrefix(a.RecID, proto.PastePrefix) {
				id = e.ID
			}
		}
	}
	d.pane.mu.Unlock()
	r.ActID = id
	next, cmd := f.Update(fromConn{d.pane, resultMsg(r)})
	return frameRun(t, next.(frameModel), cmd)
}

// frame is the review pane the list opened: built from the argv the list
// gave its respawn, as `matchblox frame` would be.
func (d *reviewDesk) frame(t *testing.T) frameModel {
	t.Helper()
	var argv []string
	for _, r := range d.tm.respawns() {
		if slices.Contains(r, "--kind=review") {
			argv = r
		}
	}
	if len(argv) < 3 {
		t.Fatalf("no review pane was started: %q", d.tm.lines())
	}
	pane := New(Options{NoMotion: true, Now: func() time.Time { return demoAt }, Pane: "%9", Tmux: d.tm.run, Conn: d.pane})
	f, err := reviewFrameFromArgv(pane, argv[2:])
	if err != nil {
		t.Fatal(err)
	}
	next, _ := f.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	f = next.(frameModel)
	return frameRun(t, f, f.reviewLoad())
}

// reviewMsgs runs cmd to its end, as the program does: git can take longer
// than cmdMsgs waits. A tick ends the run: the next tick is the test's own.
func reviewMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case nil, frameTick:
		return nil
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, reviewMsgs(c)...)
		}
		return out
	default:
		return []tea.Msg{msg}
	}
}

func frameRun(t *testing.T, f frameModel, cmd tea.Cmd) frameModel {
	t.Helper()
	for _, msg := range reviewMsgs(cmd) {
		next, c := f.Update(msg)
		f = frameRun(t, next.(frameModel), c)
	}
	return f
}

func frameKey(t *testing.T, f frameModel, keys ...string) frameModel {
	t.Helper()
	for _, k := range keys {
		next, cmd := f.Update(keyPress(k))
		f = frameRun(t, next.(frameModel), cmd)
	}
	return f
}

func frameType(t *testing.T, f frameModel, text string) frameModel {
	t.Helper()
	for _, r := range text {
		f = frameKey(t, f, string(r))
	}
	return f
}

func frameText(f frameModel) string { return ansi.Strip(f.View().Content) }

// reviewTwoComments: a comment on a.go line 1 and one on b.go line 3.
func reviewTwoComments(t *testing.T, f frameModel) frameModel {
	t.Helper()
	f = frameKey(t, f, "c")
	f = frameType(t, f, "name this")
	f = frameKey(t, f, "enter", "]", "down", "down", "c")
	f = frameType(t, f, "why 1")
	return frameKey(t, f, "enter")
}

// A hostile test: D on ux-pass, a send, and the list's choice has moved to
// another session by then. The paste goes to ux-pass's pane and to no
// other: the target is what the review opened on, never what the list
// chooses at the send.
func TestReviewPasteHitsOnlyThatPane(t *testing.T) {
	d := newReviewDesk(t, reviewRepo(t), "main")
	d.choose(t, "ux-pass")
	d.press(t, "D")
	f := reviewTwoComments(t, d.frame(t))
	f = frameKey(t, f, "enter")

	d.choose(t, "other") // the choice moves, as a refresh or a click would
	f = d.answer(t, f, proto.Result{})
	_ = f
	d.poll(t)

	pastes := d.pastes()
	if len(pastes) != 1 || pastes[0].RecID != proto.PastePrefix+"%7" {
		t.Fatalf("pastes %+v, want one to paste:%%7", pastes)
	}
	for _, a := range append(d.home.allActs(), d.pane.allActs()...) {
		if !strings.HasPrefix(a.RecID, "watch:") && (strings.Contains(a.RecID, "%8") || strings.Contains(a.RecID, "s8")) { // a watch only reads
			t.Fatalf("an act touched the other session: %+v", a)
		}
	}
	var gaveKeys bool
	for _, a := range d.home.allActs() {
		gaveKeys = gaveKeys || a.RecID == proto.KeysPrefix+"s7"
	}
	if !gaveKeys {
		t.Fatalf("the keys did not go to ux-pass: %+v", d.home.allActs())
	}
	for _, line := range d.tm.typed() {
		t.Fatalf("the console's tmux typed into a pane: %s", line)
	}
}

// A hostile test: a word that names another pane than the one the review
// opened on ends nothing and gives no keys, and the list sends no paste.
func TestReviewHandoffForAnotherPaneIsIgnored(t *testing.T) {
	d := newReviewDesk(t, reviewRepo(t), "main")
	d.choose(t, "ux-pass")
	d.press(t, "D")
	raw, _ := json.Marshal(reviewAct{Act: "sent", Pane: "%8", N: 1})
	_, _ = d.tm.run("set-option", "-w", "-t", "%9", sample.ReviewOption, string(raw))
	d.poll(t)
	if got := d.pastes(); len(got) != 0 {
		t.Fatalf("a review went to a session it did not open on: %+v", got)
	}
	if d.m.reviewing == nil || len(d.tm.respawns()) != 1 {
		t.Fatal("a word for another pane ended the review")
	}
	for _, a := range d.home.allActs() {
		if strings.HasPrefix(a.RecID, proto.KeysPrefix) {
			t.Fatalf("keys given: %+v", a)
		}
	}
}

var reviewSGR = regexp.MustCompile("\x1b\\[[0-9;:]*m")

// A hostile test: a file holds an OSC that sets the terminal title and a
// CSI that clears the screen. The review draws their bytes as visible text,
// never as control, and a terminal that reads the pane's bytes keeps its
// title.
func TestDiffEscapeStaysVisibleText(t *testing.T) {
	repo := reviewRepo(t)
	reviewWrite(t, repo, "esc.txt", "\033]0;pwned\007 hello\n\033[2Jx\n\u009bbad\n")
	reviewGitIn(t, repo, "add", ".")
	reviewGitIn(t, repo, "commit", "-q", "-m", "esc")
	d := newReviewDesk(t, repo, "main")
	d.choose(t, "ux-pass")
	d.press(t, "D")
	f := frameKey(t, d.frame(t), "]", "]") // a.go, b.go, esc.txt

	v := f.View()
	raw := v.Content
	plain := ansi.Strip(raw)
	for _, want := range []string{"^[]0;pwned^G hello", "^[[2Jx"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("the escape is not visible text: want %q in\n%s", want, plain)
		}
	}
	rest := reviewSGR.ReplaceAllString(raw, "")
	if i := strings.IndexFunc(rest, func(r rune) bool { return r < ' ' && r != '\n' || r == 0x7f || r >= 0x80 && r <= 0x9f }); i >= 0 {
		t.Fatalf("the view holds a control byte %q: %q", rest[i], rest)
	}
	if v.WindowTitle != "" {
		t.Fatalf("the view sets the window title %q", v.WindowTitle)
	}
}

func TestDHidesTheAgentView(t *testing.T) {
	d := newReviewDesk(t, reviewRepo(t), "main")
	d.choose(t, "ux-pass")
	d.press(t, "D")
	rs := d.tm.respawns()
	if len(rs) != 1 {
		t.Fatalf("respawns %q, want the review in the agent seat", rs)
	}
	line := strings.Join(rs[0], " ")
	if rs[0][0] != "/bin/matchblox" || !strings.HasPrefix(line, "/bin/matchblox frame --kind=review") || !strings.Contains(line, "--session ux-pass") {
		t.Fatalf("the agent seat runs %q, want matchblox frame --kind=review --session ux-pass", line)
	}
	if strings.Contains(line, "tmux") || strings.Contains(line, "env -u") {
		t.Fatalf("the agent seat still runs the agent view: %q", line)
	}
	if !slices.Contains(d.tm.lines(), "respawn-pane -k -t %9 -- "+line) {
		t.Fatalf("not the beside pane %%9: %q", d.tm.lines())
	}
	if got := d.tm.option(sample.ModeOption); got != "REVIEW" {
		t.Fatalf("mode %q, want REVIEW", got)
	}
	if !slices.Contains(d.tm.lines(), "select-pane -Z -t %9") {
		t.Fatalf("the review does not have the keys: %q", d.tm.lines())
	}
	if !strings.Contains(line, "--base main") {
		t.Fatalf("the start branch is not handed over: %q", line)
	}
}

func TestReviewHeadAndFiles(t *testing.T) {
	d := newReviewDesk(t, reviewRepo(t), "main")
	d.choose(t, "ux-pass")
	d.press(t, "D")
	f := d.frame(t)
	text := frameText(f)
	if !strings.Contains(text, "ux-pass · diff against main · 2 files +8 −1") || !strings.Contains(text, "esc chat") {
		t.Fatalf("head:\n%s", text)
	}
	if !strings.Contains(text, "a.go") || !strings.Contains(text, "return 2") {
		t.Fatalf("no first file in\n%s", text)
	}
	f = frameKey(t, f, "]")
	if text = frameText(f); !strings.Contains(text, "b.go") || !strings.Contains(text, "const X = 1") {
		t.Fatalf("] did not show b.go:\n%s", text)
	}
	f = frameKey(t, f, "[")
	if text = frameText(f); !strings.Contains(text, "a.go") {
		t.Fatalf("[ did not go back:\n%s", text)
	}
}

func TestTwoCommentsSendOnePaste(t *testing.T) {
	d := newReviewDesk(t, reviewRepo(t), "main")
	d.choose(t, "ux-pass")
	d.press(t, "D")
	f := d.frame(t)

	f = frameKey(t, f, "c")
	if got := d.tm.option(sample.ModeOption); got != "COMMENT" {
		t.Fatalf("mode %q, want COMMENT", got)
	}
	if text := frameText(f); !strings.Contains(text, "›  ⏎ add · esc cancel") {
		t.Fatalf("no comment field in\n%s", text)
	}
	f = frameType(t, f, "name this")
	f = frameKey(t, f, "enter")
	if got := d.tm.option(sample.ModeOption); got != "REVIEW" {
		t.Fatalf("mode %q, want REVIEW again", got)
	}
	if text := frameText(f); !strings.Contains(text, "✎") || !strings.Contains(text, "name this") {
		t.Fatalf("the comment is not on its line:\n%s", text)
	}
	f = frameKey(t, f, "]", "down", "down", "c")
	f = frameType(t, f, "why 1")
	f = frameKey(t, f, "enter")
	if len(d.pastes()) != 0 {
		t.Fatal("a comment sent a message")
	}
	f = frameKey(t, f, "enter") // no field open: send
	if d.tm.option(sample.ReviewOption) != "" {
		t.Fatal("the send wrote the tmux option before the service answered")
	}

	pastes := d.pastes()
	if len(pastes) != 1 {
		t.Fatalf("pastes %+v, want one", pastes)
	}
	p := pastes[0]
	if p.RecID != proto.PastePrefix+"%7" || p.Confirm != "y" {
		t.Fatalf("paste act %+v", p)
	}
	for _, want := range []string{"a.go:1", "package a", "name this", "b.go:3", "const X = 1", "why 1"} {
		if !strings.Contains(p.Text, want) {
			t.Fatalf("the message lacks %q:\n%s", want, p.Text)
		}
	}
	if strings.Count(p.Text, "a.go:") != 1 || strings.Count(p.Text, "b.go:") != 1 {
		t.Fatalf("each comment once:\n%s", p.Text)
	}

	f = d.answer(t, f, proto.Result{})
	_ = f
	d.poll(t)
	if got := d.m.flash; got != "sent a review of 2 comments to ux-pass · now in WORKING" {
		t.Fatalf("said %q", got)
	}
	rs := d.tm.respawns()
	if len(rs) != 2 || !strings.Contains(strings.Join(rs[1], " "), "env -u TMUX tmux") || !strings.Contains(strings.Join(rs[1], " "), "%7") {
		t.Fatalf("respawns %q, want ViewArgv of %%7 last", rs)
	}
	if got := d.tm.option(sample.ModeOption); got != "LIST" {
		t.Fatalf("mode %q, want LIST", got)
	}
	if lines := d.tm.lines(); !slices.Contains(lines, "select-pane -Z -t %1") {
		t.Fatalf("the keys did not go to the list: %q", lines)
	}
	var gaveKeys bool
	for _, a := range d.home.allActs() {
		gaveKeys = gaveKeys || a.RecID == proto.KeysPrefix+"s7" && a.Text == ""
	}
	if !gaveKeys {
		t.Fatalf("the send did not count as giving the keys: %+v", d.home.allActs())
	}
	if d.m.reviewing != nil {
		t.Fatal("the review is still open")
	}
	if d.tm.option(sample.ReviewOption) != "" {
		t.Fatal("the word to the list stays in the tmux options")
	}
}

func TestASecondCommentOnTheSameLineReplacesIt(t *testing.T) {
	d := newReviewDesk(t, reviewRepo(t), "main")
	d.choose(t, "ux-pass")
	d.press(t, "D")
	f := d.frame(t)
	f = frameKey(t, f, "c")
	f = frameType(t, f, "first")
	f = frameKey(t, f, "enter", "c")
	f = frameType(t, f, "second")
	f = frameKey(t, f, "enter")
	if text := frameText(f); strings.Contains(text, "first") || !strings.Contains(text, "second") {
		t.Fatalf("the second c did not replace the first:\n%s", text)
	}
	_ = frameKey(t, f, "enter")
	p := d.pastes()
	if len(p) != 1 || strings.Contains(p[0].Text, "first") || !strings.Contains(p[0].Text, "second") || strings.Count(p[0].Text, "a.go:1") != 1 {
		t.Fatalf("pastes %+v", p)
	}
}

func TestEscOnTheFieldCancelsOnlyTheComment(t *testing.T) {
	d := newReviewDesk(t, reviewRepo(t), "main")
	d.choose(t, "ux-pass")
	d.press(t, "D")
	for _, k := range []string{"esc", "ctrl+q"} {
		f := d.frame(t)
		f = frameKey(t, f, "c")
		f = frameType(t, f, "never mind")
		f = frameKey(t, f, k)
		if text := frameText(f); strings.Contains(text, "never mind") || strings.Contains(text, "⏎ add · esc cancel") {
			t.Fatalf("%s left the comment:\n%s", k, text)
		}
		if got := d.tm.option(sample.ModeOption); got != "REVIEW" {
			t.Fatalf("%s: mode %q, want REVIEW", k, got)
		}
		if got := d.tm.option(sample.ReviewOption); got != "" {
			t.Fatalf("%s on the field left the review: %q", k, got)
		}
	}
}

func TestEnterWithNoCommentSendsNothing(t *testing.T) {
	d := newReviewDesk(t, reviewRepo(t), "main")
	d.choose(t, "ux-pass")
	d.press(t, "D")
	f := frameKey(t, d.frame(t), "enter")
	if text := frameText(f); !strings.Contains(text, "no comments yet") {
		t.Fatalf("no word on an empty send:\n%s", text)
	}
	d.poll(t)
	if len(d.pastes()) != 0 {
		t.Fatal("an empty review was sent")
	}
}

func TestEscLeavesReview(t *testing.T) {
	for _, k := range []string{"esc", "ctrl+q"} {
		t.Run(k, func(t *testing.T) {
			d := newReviewDesk(t, reviewRepo(t), "main")
			d.choose(t, "ux-pass")
			d.press(t, "D")
			_ = frameKey(t, d.frame(t), k)
			d.poll(t)
			rs := d.tm.respawns()
			if len(rs) != 2 || !strings.Contains(strings.Join(rs[1], " "), "env -u TMUX tmux") || !strings.Contains(strings.Join(rs[1], " "), "%7") {
				t.Fatalf("respawns %q, want ViewArgv of %%7 last", rs)
			}
			if got := d.tm.option(sample.ModeOption); got != "LIST" {
				t.Fatalf("mode %q, want LIST", got)
			}
			if !slices.Contains(d.tm.lines(), "select-pane -Z -t %1") {
				t.Fatalf("the keys did not go to the list: %q", d.tm.lines())
			}
			if len(d.pastes()) != 0 || d.m.reviewing != nil {
				t.Fatalf("pastes %+v, reviewing %v", d.pastes(), d.m.reviewing)
			}
			if d.tm.option(sample.ReviewOption) != "" {
				t.Fatal("the act stays in the options")
			}
		})
	}
}

func TestAReviewTheServiceRefusesKeepsItsComments(t *testing.T) {
	d := newReviewDesk(t, reviewRepo(t), "main")
	d.choose(t, "ux-pass")
	d.press(t, "D")
	f := reviewTwoComments(t, d.frame(t))
	f = frameKey(t, f, "enter")
	f = d.answer(t, f, proto.Result{Err: "ux-pass is working: wait for its turn"})
	if d.m.reviewing == nil || len(d.tm.respawns()) != 1 {
		t.Fatalf("a refused send ended the review: respawns %q", d.tm.respawns())
	}
	text := frameText(f)
	if !strings.Contains(text, "not sent: ux-pass is working: wait for its turn") || !strings.Contains(text, "why 1") {
		t.Fatalf("the review lost its comments or the reason:\n%s", text)
	}
	if got := d.tm.option(sample.ModeOption); got != "REVIEW" {
		t.Fatalf("mode %q, want REVIEW", got)
	}
	// send again
	f = frameKey(t, f, "enter")
	_ = f
	if p := d.pastes(); len(p) != 2 || !strings.Contains(p[1].Text, "name this") || !strings.Contains(p[1].Text, "why 1") {
		t.Fatalf("pastes %+v, want the second send with both comments", p)
	}
}

func TestAReviewTheListNeverTakesUnfreezes(t *testing.T) {
	d := newReviewDesk(t, reviewRepo(t), "main")
	d.choose(t, "ux-pass")
	d.press(t, "D")
	f := frameKey(t, d.frame(t), "esc")
	for range 20 {
		next, _ := f.Update(frameTick{})
		f = next.(frameModel)
	}
	if text := frameText(f); !strings.Contains(text, "the list did not answer") {
		t.Fatalf("a review nobody answers is a dead end:\n%s", text)
	}
}

// A hostile test: the service never answers the paste. The pane says so, and
// the owner can send again or leave.
func TestAReviewTheServiceNeverAnswersUnfreezes(t *testing.T) {
	d := newReviewDesk(t, reviewRepo(t), "main")
	d.choose(t, "ux-pass")
	d.press(t, "D")
	f := frameKey(t, reviewTwoComments(t, d.frame(t)), "enter")
	for range 30 {
		next, _ := f.Update(frameTick{})
		f = next.(frameModel)
	}
	if text := frameText(f); !strings.Contains(text, "the service did not answer") || !strings.Contains(text, "why 1") {
		t.Fatalf("a paste nobody answers is a dead end:\n%s", text)
	}
	_ = frameKey(t, f, "enter")
	if p := d.pastes(); len(p) != 2 {
		t.Fatalf("pastes %d, want a second send", len(p))
	}
}

// The pane reaches only this machine's service: a session on a peer says so
// and nothing is sent or written.
func TestReviewSendForAPeerSaysSo(t *testing.T) {
	d := newReviewDesk(t, reviewRepo(t), "main")
	pane := New(Options{NoMotion: true, Pane: "%9", Tmux: d.tm.run, Conn: d.pane})
	f, err := reviewFrameFromArgv(pane, []string{"--kind=review", "--session", "ux-pass", "--host", "gpu-1", "--pane", "%7", "--dir", "/wt", "--base", "main"})
	if err != nil {
		t.Fatal(err)
	}
	next, _ := f.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	f = next.(frameModel)
	diff := &reviewDiff{against: "main", files: []reviewFile{{path: "a.go", add: 1, loaded: true, lines: []reviewLine{{kind: '+', text: "x", new: 1}}}}}
	f = frameRun(t, f, func() tea.Msg { return reviewLoaded{diff: diff} })
	f = frameKey(t, f, "c")
	f = frameType(t, f, "hm")
	f = frameKey(t, f, "enter", "enter")
	if text := frameText(f); !strings.Contains(text, "not sent: gpu-1 is another machine") {
		t.Fatalf("no word on a peer:\n%s", text)
	}
	if len(d.pane.allActs()) != 0 || d.tm.option(sample.ReviewOption) != "" {
		t.Fatalf("acts %+v, option %q", d.pane.allActs(), d.tm.option(sample.ReviewOption))
	}
}

func TestReviewNoGitFolder(t *testing.T) {
	d := newReviewDesk(t, t.TempDir(), "main")
	d.choose(t, "ux-pass")
	d.press(t, "D")
	if text := frameText(d.frame(t)); !strings.Contains(text, "No changes to review: ux-pass has no git folder.") {
		t.Fatalf("no git folder:\n%s", text)
	}
}

func TestReviewNoChanges(t *testing.T) {
	repo := t.TempDir()
	reviewGitIn(t, repo, "init", "-q", "-b", "main")
	reviewWrite(t, repo, "a.go", "package a\n")
	reviewGitIn(t, repo, "add", ".")
	reviewGitIn(t, repo, "commit", "-q", "-m", "base")
	d := newReviewDesk(t, repo, "main")
	d.choose(t, "ux-pass")
	d.press(t, "D")
	f := d.frame(t)
	if text := frameText(f); !strings.Contains(text, "ux-pass has no changes against main.") {
		t.Fatalf("no changes:\n%s", text)
	}
	_ = frameKey(t, f, "c", "enter")
	if len(d.pastes()) != 0 {
		t.Fatal("an empty review sent")
	}
}

func TestReviewBaseGone(t *testing.T) {
	repo := reviewRepo(t)
	reviewGitIn(t, repo, "update-ref", "refs/remotes/origin/main", "main")
	reviewGitIn(t, repo, "branch", "-D", "main")
	prev := reviewGit
	reviewGit = func(_, dir string, args ...string) (string, error) { return prev("", dir, args...) }
	t.Cleanup(func() { reviewGit = prev })
	d := newReviewDesk(t, repo, "main")
	d.choose(t, "ux-pass")
	d.press(t, "D")
	text := frameText(d.frame(t))
	if !strings.Contains(text, "The diff shows against origin/main.") || !strings.Contains(text, "main is gone on ") {
		t.Fatalf("base gone:\n%s", text)
	}
}

func TestReviewBaseGoneNamesTheHost(t *testing.T) {
	repo := reviewRepo(t)
	reviewGitIn(t, repo, "update-ref", "refs/remotes/origin/main", "main")
	reviewGitIn(t, repo, "branch", "-D", "main")
	prev := reviewGit
	reviewGit = func(_, dir string, args ...string) (string, error) { return prev("", dir, args...) }
	t.Cleanup(func() { reviewGit = prev })
	pane := New(Options{NoMotion: true, Pane: "%9", Tmux: (&reviewTmux{}).run})
	f, err := reviewFrameFromArgv(pane, []string{"--kind=review", "--session", "ux-pass", "--host", "ws-1", "--pane", "%7", "--dir", repo, "--base", "main"})
	if err != nil {
		t.Fatal(err)
	}
	next, _ := f.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	f = frameRun(t, next.(frameModel), nil)
	f = frameRun(t, f, f.reviewLoad())
	if text := frameText(f); !strings.Contains(text, "The diff shows against origin/main. main is gone on ws-1.") {
		t.Fatalf("base gone:\n%s", text)
	}
}

func TestReviewBaseGoneKeepsTheFallbackOnALongHost(t *testing.T) {
	repo := reviewRepo(t)
	reviewGitIn(t, repo, "update-ref", "refs/remotes/origin/main", "main")
	reviewGitIn(t, repo, "branch", "-D", "main")
	prev := reviewGit
	reviewGit = func(_, dir string, args ...string) (string, error) { return prev("", dir, args...) }
	t.Cleanup(func() { reviewGit = prev })
	host := strings.Repeat("n", 80) + ".local"
	pane := New(Options{NoMotion: true, Pane: "%9", Tmux: (&reviewTmux{}).run})
	f, err := reviewFrameFromArgv(pane, []string{"--kind=review", "--session", "ux-pass", "--host", host, "--pane", "%7", "--dir", repo, "--base", "main"})
	if err != nil {
		t.Fatal(err)
	}
	next, _ := f.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	f = frameRun(t, next.(frameModel), nil)
	f = frameRun(t, f, f.reviewLoad())
	text := frameText(f)
	if !strings.Contains(text, "The diff shows against origin/main.") || !strings.Contains(text, "main is gone on ") {
		t.Fatalf("long host ate the fallback:\n%s", text)
	}
}

func TestReviewBaseFallsBack(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, repo string)
		want  string
	}{
		{"origin/HEAD", func(t *testing.T, repo string) {
			reviewGitIn(t, repo, "update-ref", "refs/remotes/origin/trunk", "main")
			reviewGitIn(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")
		}, "diff against origin/trunk"},
		{"main", func(*testing.T, string) {}, "diff against main"},
		{"master", func(t *testing.T, repo string) { reviewGitIn(t, repo, "branch", "-m", "main", "master") }, "diff against master"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := reviewRepo(t)
			c.setup(t, repo)
			d := newReviewDesk(t, repo, "") // no recorded start branch
			d.choose(t, "ux-pass")
			d.press(t, "D")
			if text := frameText(d.frame(t)); !strings.Contains(text, c.want) || strings.Contains(text, "is gone on") {
				t.Fatalf("want %q in\n%s", c.want, text)
			}
		})
	}
}

func TestReviewTooLargeListsTheFiles(t *testing.T) {
	repo := reviewRepo(t)
	var big strings.Builder
	for i := 1; i <= 4001; i++ {
		big.WriteString("big line number " + strings.Repeat("x", i%7) + "\n")
	}
	reviewWrite(t, repo, "big.txt", big.String())
	reviewGitIn(t, repo, "add", ".")
	reviewGitIn(t, repo, "commit", "-q", "-m", "big")
	d := newReviewDesk(t, repo, "main")
	d.choose(t, "ux-pass")
	d.press(t, "D")
	f := d.frame(t)
	text := frameText(f)
	for _, want := range []string{"a.go", "b.go", "big.txt", "3 files"} {
		if !strings.Contains(text, want) {
			t.Fatalf("no %q in the file list:\n%s", want, text)
		}
	}
	if strings.Contains(text, "return 2") || strings.Contains(text, "big line number") {
		t.Fatalf("a file shows before it is opened:\n%s", text)
	}
	f = frameKey(t, f, "down", "enter")
	if text = frameText(f); !strings.Contains(text, "const X = 1") || strings.Contains(text, "big line number") {
		t.Fatalf("enter did not open b.go on its own:\n%s", text)
	}
	if len(d.pastes()) != 0 || d.tm.option(sample.ReviewOption) != "" {
		t.Fatal("enter in the file list sent")
	}
}

func TestReviewTooLargeOpensTheBigFileOnItsOwn(t *testing.T) {
	repo := reviewRepo(t)
	var big strings.Builder
	for i := 1; i <= 4001; i++ {
		big.WriteString("big line number\n")
	}
	reviewWrite(t, repo, "big.txt", big.String())
	reviewGitIn(t, repo, "add", ".")
	reviewGitIn(t, repo, "commit", "-q", "-m", "big")
	d := newReviewDesk(t, repo, "main")
	d.choose(t, "ux-pass")
	d.press(t, "D")
	f := frameKey(t, d.frame(t), "down", "down", "enter")
	text := frameText(f)
	if !strings.Contains(text, "big line number") || !strings.Contains(text, "big.txt") {
		t.Fatalf("the big file did not open:\n%s", text)
	}
	if !strings.Contains(text, "first 4000 lines") {
		t.Fatalf("a cut file does not say so:\n%s", text)
	}
}

// The review's git runs on the host that owns the session.
func TestReviewGitRunsOnThePeer(t *testing.T) {
	var ran [][]string
	prev := hostRun
	hostRun = func(argv []string) (string, error) {
		ran = append(ran, append([]string(nil), argv...))
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { hostRun = prev })
	pane := New(Options{NoMotion: true, Pane: "%9", Tmux: (&reviewTmux{}).run})
	f, err := reviewFrameFromArgv(pane, []string{"--kind=review", "--session", "ux-pass", "--host", "gpu-1", "--pane", "%7", "--dir", "/wt/ux-pass", "--base", "main"})
	if err != nil {
		t.Fatal(err)
	}
	next, _ := f.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	f = frameRun(t, next.(frameModel), nil)
	f = frameRun(t, f, f.reviewLoad())
	if len(ran) == 0 {
		t.Fatal("git did not run on gpu-1")
	}
	for _, argv := range ran {
		line := strings.Join(argv, " ")
		if argv[0] != "ssh" || !strings.Contains(line, "gpu-1") || !strings.Contains(line, "git") || !strings.Contains(line, "/wt/ux-pass") {
			t.Fatalf("not git on gpu-1 in /wt/ux-pass: %q", argv)
		}
	}
	if text := frameText(f); !strings.Contains(text, "No changes to review: ux-pass has no git folder.") {
		t.Fatalf("no word:\n%s", text)
	}
}

func TestReviewNotReachedHostDoesNotOpen(t *testing.T) {
	home, gpu := newFake(), newFake()
	tm := &reviewTmux{}
	var next tea.Model = New(Options{
		NoMotion: true, Conn: home, Now: func() time.Time { return demoAt },
		Peers: []Peer{{Host: "gpu-1", Conn: gpu}}, Beside: true, Pane: "%1", Tmux: tm.run, Self: "/bin/matchblox", Socket: "/tmp/tmux-1000/default",
	})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	next, _ = next.Update(stateMsg(hostState()))
	ux := hostState(sess(7, "ux-pass", "%7", func(s *sample.Session) { s.Cwd = "/wt/ux-pass" }))
	next, _ = next.Update(peerMsg{host: "gpu-1", conn: gpu, msg: stateMsg(ux)})
	m := next.(Model)
	m.splashDone, m.deskFrame, m.beside = true, true, "%9"
	m.toList()
	m.peers[m.peerAt("gpu-1")].lost = true
	for _, s := range m.all.sessions {
		if s.Name == "ux-pass" {
			m.chooseRow(s)
		}
	}
	m = pressOn(m, "D")
	if m.noticeText != "review did not open: gpu-1 is not reached. esc back to the list" && !strings.Contains(m.flash, "gpu-1 is not reached") {
		t.Fatalf("notice %q flash %q", m.noticeText, m.flash)
	}
	if len(tm.respawns()) != 0 {
		t.Fatalf("the seat was taken: %q", tm.respawns())
	}
}

func TestReviewOnAReachedPeerNamesTheHostAndDoesNotOpen(t *testing.T) {
	home, gpu := newFake(), newFake()
	tm := &reviewTmux{}
	var next tea.Model = New(Options{
		NoMotion: true, Conn: home, Now: func() time.Time { return demoAt },
		Peers: []Peer{{Host: "gpu-1", Conn: gpu}}, Beside: true, Pane: "%1", Tmux: tm.run, Self: "/bin/matchblox", Socket: "/tmp/tmux-1000/default",
	})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	next, _ = next.Update(stateMsg(hostState()))
	ux := hostState(sess(7, "ux-pass", "%7", func(s *sample.Session) { s.Cwd = "/wt/ux-pass" }))
	next, _ = next.Update(peerMsg{host: "gpu-1", conn: gpu, msg: stateMsg(ux)})
	m := next.(Model)
	m.splashDone, m.deskFrame, m.beside = true, true, "%9"
	m.toList()
	for _, s := range m.all.sessions {
		if s.Name == "ux-pass" {
			m.chooseRow(s)
		}
	}
	m = pressOn(m, "D")
	if !strings.Contains(m.noticeText+m.flash, "review did not open: gpu-1 is another machine") {
		t.Fatalf("notice %q flash %q, want one that names gpu-1", m.noticeText, m.flash)
	}
	if m.reviewing != nil || tm.option(sample.ModeOption) == "REVIEW" {
		t.Fatal("a review the owner cannot send was opened")
	}
	if len(tm.respawns()) != 0 {
		t.Fatalf("the seat was taken: %q", tm.respawns())
	}
}

func TestReviewNeedsASession(t *testing.T) {
	m := boardShot(t, hostState(), 160, 40, func(m *Model) { m.deskFrame = true; m.beside = "%9" })
	m = pressOn(m, "D")
	if m.noticeText == "" {
		t.Fatal("D with no session said nothing")
	}
}

func TestTheKeysSheetRunsD(t *testing.T) {
	d := newReviewDesk(t, reviewRepo(t), "main")
	d.choose(t, "ux-pass")
	d.press(t, "?")
	for _, r := range "diff" {
		d.press(t, string(r))
	}
	d.press(t, "enter")
	if rs := d.tm.respawns(); len(rs) != 1 || !slices.Contains(rs[0], "--kind=review") {
		t.Fatalf("respawns %q, want the review", rs)
	}
}

func TestTheListDoesNotPreviewOverAReview(t *testing.T) {
	d := newReviewDesk(t, reviewRepo(t), "main")
	d.choose(t, "ux-pass")
	d.press(t, "D")
	next, cmd := d.m.Update(previewMsg{seq: d.m.previewSeq})
	d.m = reviewSettle(t, next.(Model), cmd)
	if rs := d.tm.respawns(); len(rs) != 1 {
		t.Fatalf("a look took the seat from the review: %q", rs)
	}
}

func TestDOnAChainViewSaysSo(t *testing.T) {
	d := newReviewDesk(t, reviewRepo(t), "main")
	d.m.chain = &chainView{on: []paneKey{{"", "%7"}, {"", "%8"}}, panes: []string{"%9", "%10"}}
	d.choose(t, "ux-pass")
	d.press(t, "D")
	if len(d.tm.respawns()) != 0 || !strings.Contains(d.m.noticeText+d.m.flash, "chain") {
		t.Fatalf("respawns %q notice %q", d.tm.respawns(), d.m.noticeText)
	}
}

// The review's comments, the file text and the paste go to the service and
// to nothing else: a tmux window option is readable by every process of the
// same user, so no call of tmux carries them.
func TestReviewSendKeepsTextOutOfTmuxOptions(t *testing.T) {
	d := newReviewDesk(t, reviewRepo(t), "main")
	d.choose(t, "ux-pass")
	d.press(t, "D")
	f := reviewTwoComments(t, d.frame(t))
	f = frameKey(t, f, "enter")
	f = d.answer(t, f, proto.Result{})
	_ = f
	d.poll(t)
	if len(d.pastes()) != 1 {
		t.Fatal("the review did not reach the service")
	}
	for _, line := range d.tm.lines() {
		for _, secret := range []string{"name this", "why 1", "package a", "const X", "a.go:1", "b.go:3"} {
			if strings.Contains(line, secret) {
				t.Fatalf("tmux was handed the review's text (%q): %s", secret, line)
			}
		}
	}
	if d.tm.option("@matchblox_review_reply") != "" {
		t.Fatal("the reply option is back")
	}
}

// TestReviewShowsUntrackedFile: an agent's new file that it never added to
// git is part of its work. The review lists it, counts it, labels it new,
// and takes a comment on its line (review of 2026-11-27, Important 3).
func TestReviewShowsUntrackedFile(t *testing.T) {
	for _, tc := range []struct {
		name string
		repo func(t testing.TB) string
		head string
	}{
		{"only new files", func(t testing.TB) string {
			repo := t.TempDir()
			reviewGitIn(t, repo, "init", "-q", "-b", "main")
			reviewWrite(t, repo, "a.go", "package a\n")
			reviewGitIn(t, repo, "add", ".")
			reviewGitIn(t, repo, "commit", "-q", "-m", "base")
			return repo
		}, "1 file +3 −0"},
		{"mixed with tracked changes", reviewRepo, "3 files +11 −1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := tc.repo(t)
			reviewWrite(t, repo, "router.go", "package a\n\nconst R = 1\n")
			reviewWrite(t, repo, "ignored.txt", "not the agent's work\n")
			reviewWrite(t, repo, ".git/info/exclude", "ignored.txt\n")
			d := newReviewDesk(t, repo, "main")
			d.choose(t, "ux-pass")
			d.press(t, "D")
			f := d.frame(t)
			text := frameText(f)
			if strings.Contains(text, "has no changes") || !strings.Contains(text, tc.head) {
				t.Fatalf("head %q not in\n%s", tc.head, text)
			}
			if strings.Contains(text, "ignored.txt") {
				t.Fatalf("an ignored file is in the review:\n%s", text)
			}
			f = frameKey(t, f, "]", "]") // the last file is the new one
			if tc.name == "only new files" {
				f = frameKey(t, f, "[")
			}
			text = frameText(f)
			if !strings.Contains(text, "router.go") || !strings.Contains(text, "new") || !strings.Contains(text, "const R = 1") {
				t.Fatalf("router.go is not shown as new:\n%s", text)
			}
			f = frameKey(t, f, "down", "down", "down", "c")
			f = frameType(t, f, "name R")
			_ = frameKey(t, f, "enter", "enter")
			pastes := d.pastes()
			if len(pastes) != 1 || !strings.Contains(pastes[0].Text, "router.go:3") || !strings.Contains(pastes[0].Text, "name R") {
				t.Fatalf("the comment on the new file did not send: %+v", pastes)
			}
		})
	}
}
