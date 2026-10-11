package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/replay"
)

// demoSession is a Claude Code session of the demo machine.
type demoSession struct {
	pid    int
	name   string
	id     string
	status string
	tokens int
}

// demoWorld is a copy of testdata/demo that changes as the demo acts on
// it: a typed answer sets its session to work, a /compact leaves its
// boundary and summary in the transcript, a removed worktree leaves the
// git scan. Nothing runs on the real machine.
type demoWorld struct {
	root string
	clk  *clock
	kill func(pid int)

	mu       sync.Mutex
	sessions map[string]*demoSession // by pane
	typed    map[string]string       // the draft in each pane's field
	removed  map[string]bool         // worktree paths
	ran      [][]string
}

func newWorld(t *testing.T, src string, clk *clock, kill func(int)) *demoWorld {
	t.Helper()
	root := t.TempDir()
	if err := copyTree(src, root); err != nil {
		t.Fatal(err)
	}
	w := &demoWorld{root: root, clk: clk, kill: kill, sessions: map[string]*demoSession{},
		typed: map[string]string{}, removed: map[string]bool{}}
	for pane, pid := range map[string]int{"%1": 1200, "%2": 1310, "%3": 1400, "%5": 1600, "%8": 1900} {
		b, err := os.ReadFile(w.registry(pid))
		if err != nil {
			t.Fatal(err)
		}
		var e struct{ Name, SessionID, Status string }
		if err := json.Unmarshal(b, &e); err != nil {
			t.Fatal(err)
		}
		w.sessions[pane] = &demoSession{pid: pid, name: e.Name, id: e.SessionID, status: e.Status, tokens: lastTokens(t, w.transcript(e.Name, e.SessionID))}
	}
	return w
}

// copyTree copies a fixture tree, its links as links.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		to := filepath.Join(dst, rel)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			l, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(l, to)
		case d.IsDir():
			return os.MkdirAll(to, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(to, b, 0o644)
	})
}

var usageRe = regexp.MustCompile(`"cache_read_input_tokens": ?(\d+)`)

func lastTokens(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m := usageRe.FindAllSubmatch(b, -1)
	if len(m) == 0 {
		t.Fatalf("%s has no usage", path)
	}
	n := 0
	fmt.Sscan(string(m[len(m)-1][1]), &n)
	return n + 10 // the input tokens of each demo turn
}

func (w *demoWorld) registry(pid int) string {
	return filepath.Join(w.root, "home", ".claude", "sessions", fmt.Sprint(pid)+".json")
}

func (w *demoWorld) transcript(name, id string) string {
	return filepath.Join(w.root, "home", ".claude", "projects", "-work-"+name, id+".jsonl")
}

func (w *demoWorld) pane(pane string) string {
	return filepath.Join(w.root, "panes", strings.TrimPrefix(pane, "%")+".txt")
}

// Run is the service's runner: it takes only what the demo does.
func (w *demoWorld) Run(argv []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ran = append(w.ran, argv)
	switch {
	case slices.Equal(argv, []string{"kill", fmt.Sprint(demoPID)}):
		w.kill(demoPID)
		return nil
	case len(argv) == 7 && argv[1] == "send-keys" && argv[4] == "-l":
		w.typed[argv[3]] = argv[6]
		return w.drawField(argv[3])
	case len(argv) == 5 && argv[1] == "send-keys" && argv[4] == "Enter":
		text := w.typed[argv[3]]
		delete(w.typed, argv[3])
		if err := w.drawField(argv[3]); err != nil {
			return err
		}
		return w.submit(argv[3], text)
	case len(argv) == 6 && argv[1] == "send-keys" && argv[4] == "/compact" && argv[5] == "Enter":
		return w.submit(argv[3], "/compact")
	case len(argv) == 6 && argv[0] == "git" && argv[3] == "worktree" && argv[4] == "remove":
		w.removed[argv[5]] = true
		return nil
	}
	return fmt.Errorf("the demo does not run %q", argv)
}

// drawField shows the draft in the pane's input field. Hold w.mu.
func (w *demoWorld) drawField(pane string) error {
	b, err := os.ReadFile(w.pane(pane))
	if errors.Is(err, fs.ErrNotExist) {
		return nil // a pane the demo never reads
	}
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "│ > ") {
			lines[i] = "│ > " + w.typed[pane]
		}
	}
	return os.WriteFile(w.pane(pane), []byte(strings.Join(lines, "\n")), 0o644)
}

// submit is the Enter in a session: an answer sets it to work, a /compact
// leaves the boundary and the summary. Hold w.mu.
func (w *demoWorld) submit(pane, text string) error {
	s, ok := w.sessions[pane]
	if !ok {
		return fmt.Errorf("no session in %s", pane)
	}
	now := w.clk.Now()
	if !strings.HasPrefix(text, "/compact") {
		// The agent takes the answer and works: its newest reply asks no more.
		s.status, s.tokens = "busy", s.tokens+4000
		if err := w.appendTranscript(s, turnLine(now, s.tokens, "I rotate the token on each refresh and keep the old one for a minute.")); err != nil {
			return err
		}
		return w.writeRegistry(s, now)
	}
	resume := "RESUME: " + map[string]string{
		"hearth-api": "add the rate-limit headers, then open the PR",
		"spark-docs": "check every link of the 1.0 docs, then open the PR",
	}[s.name]
	if err := w.appendTranscript(s,
		map[string]any{"type": "system", "subtype": "compact_boundary", "timestamp": now,
			"compactMetadata": map[string]any{"trigger": "manual", "preTokens": s.tokens, "postTokens": 9000}},
		map[string]any{"type": "user", "isCompactSummary": true, "timestamp": now,
			"message": map[string]any{"role": "user", "content": "This session continues from a summary.\n\n" + resume}},
	); err != nil {
		return err
	}
	s.tokens = 9000
	return w.writeRegistry(s, now)
}

// Finish ends the turn of the session in pane with its context at tokens.
func (w *demoWorld) Finish(pane string, tokens int, text string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := w.sessions[pane]
	now := w.clk.Now()
	if err := w.appendTranscript(s, turnLine(now, tokens, text)); err != nil {
		return err
	}
	s.status, s.tokens = "idle", tokens
	return w.writeRegistry(s, now)
}

// turnLine is an assistant turn of a transcript.
func turnLine(at time.Time, tokens int, text string) map[string]any {
	return map[string]any{"type": "assistant", "isSidechain": false, "timestamp": at,
		"message": map[string]any{"model": "sonnet", "content": []map[string]string{{"type": "text", "text": text}},
			"usage": map[string]int{"input_tokens": 10, "cache_read_input_tokens": tokens - 10}}}
}

func (w *demoWorld) appendTranscript(s *demoSession, lines ...map[string]any) error {
	f, err := os.OpenFile(w.transcript(s.name, s.id), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	for _, l := range lines {
		b, err := json.Marshal(l)
		if err != nil {
			f.Close()
			return err
		}
		if _, err := f.Write(append(b, '\n')); err != nil {
			f.Close()
			return err
		}
	}
	return f.Close()
}

func (w *demoWorld) writeRegistry(s *demoSession, now time.Time) error {
	b, err := json.Marshal(map[string]any{"pid": s.pid, "sessionId": s.id, "cwd": "/work/" + s.name, "name": s.name,
		"status": s.status, "startedAt": demoStarted.UnixMilli(), "statusUpdatedAt": now.UnixMilli()})
	if err != nil {
		return err
	}
	return os.WriteFile(w.registry(s.pid), append(b, '\n'), 0o644)
}

// demoStarted is when the demo's Claude Code sessions started.
var demoStarted = time.Date(2026, 10, 1, 19, 30, 0, 0, time.UTC)

// Ran is every argv the service ran.
func (w *demoWorld) Ran() [][]string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([][]string(nil), w.ran...)
}

// Git is the demo's git scan: one repository with a worktree for each
// session and one merged worktree that is safe to remove.
func (w *demoWorld) Git([]string) gitscan.Report {
	w.mu.Lock()
	defer w.mu.Unlock()
	const repo = "/work/hearth"
	pr := func(n int, title string) *gitscan.PR {
		return &gitscan.PR{Number: n, Title: title, URL: fmt.Sprintf("https://example.com/hearth/hearth/pull/%d", n)}
	}
	all := []gitscan.Worktree{
		{Path: repo, Branch: "main", InUse: true},
		{Path: "/work/hearth-api", Branch: "api/rate-limits", Dirty: 3, Sessions: 1, InUse: true},
		{Path: "/work/flint-auth", Branch: "auth/refresh-rotation", Dirty: 1, Sessions: 1, InUse: true},
		{Path: "/work/spark-docs", Branch: "docs/1.0", Dirty: 2, Sessions: 1, InUse: true},
		{Path: "/work/kindle-web", Branch: "web/one-tap-checkout", Sessions: 1, InUse: true, PR: pr(212, "One-tap checkout")},
		{Path: "/work/ember-billing", Branch: "billing/invoice-export", Sessions: 1, InUse: true, PR: pr(214, "Invoice export")},
		{Path: "/work/cinder-ci", Branch: "ci/go-build-cache", Dirty: 1, Sessions: 1, InUse: true},
		{Path: "/work/wick-mobile", Branch: "mobile/settings-layout", Dirty: 4, Sessions: 1, InUse: true},
		{Path: "/work/kindle-cart-v1", Branch: "web/cart-v1", Merged: true, Safe: true,
			Remove: []string{"git", "-C", repo, "worktree", "remove", "/work/kindle-cart-v1"}},
	}
	var wts []gitscan.Worktree
	for _, wt := range all {
		if !w.removed[wt.Path] {
			wts = append(wts, wt)
		}
	}
	return gitscan.Report{At: w.clk.Now(), Repos: []gitscan.Repo{{Path: repo, Main: "main", Worktrees: wts}}, Watching: []string{repo}}
}

func TestPrivateFindsThisMachineInAFrame(t *testing.T) {
	frame := func(text string) []replay.Frame {
		return []replay.Frame{{Lines: [][]replay.Span{{{Text: text}}}}}
	}
	me := []string{"dev-3", "/home/someone"}
	for _, text := range []string{" dev-3 waits", "cwd /home/someone/work", "dev-3"} {
		if err := private(frame(text), me); err == nil {
			t.Errorf("%q passed", text)
		}
	}
	for _, text := range []string{" dev-30 waits", "devdev-3x", " ws-1 · hearth-api"} {
		if err := private(frame(text), me); err != nil {
			t.Errorf("%q: %v", text, err)
		}
	}
}

func TestThisMachineSkipsTheConsolesOwnWords(t *testing.T) {
	for _, n := range thisMachine() {
		if ownWords[n] || n == "" {
			t.Fatalf("thisMachine has %q", n)
		}
	}
}

// ownWords are names this machine may give a session that are also words
// of the console itself.
var ownWords = map[string]bool{"matchblox": true, "main": true}

// thisMachine is what names the machine the test runs on: its host name,
// user and home, and the name of each tmux session and Claude Code session
// on it.
func thisMachine() []string {
	var out []string
	add := func(n string) {
		if n = strings.TrimSpace(n); len(n) >= 3 && !ownWords[n] {
			out = append(out, n)
		}
	}
	h, _ := os.Hostname()
	add(h)
	add(os.Getenv("USER"))
	home, _ := os.UserHomeDir()
	add(home)
	if b, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output(); err == nil {
		for _, n := range strings.Split(string(b), "\n") {
			add(n)
		}
	}
	files, _ := filepath.Glob(filepath.Join(home, ".claude*", "sessions", "*.json"))
	for _, f := range files {
		var e struct{ Name string }
		if b, err := os.ReadFile(f); err == nil && json.Unmarshal(b, &e) == nil {
			add(e.Name)
		}
	}
	return out
}

// private fails when a frame names this machine: a name as a whole word,
// on any line.
func private(frames []replay.Frame, names []string) error {
	var res []*regexp.Regexp
	for _, n := range names {
		res = append(res, regexp.MustCompile(`(^|[^\w-])`+regexp.QuoteMeta(n)+`($|[^\w-])`))
	}
	for _, f := range frames {
		for _, l := range f.Lines {
			var b strings.Builder
			for _, sp := range l {
				b.WriteString(sp.Text)
			}
			for i, re := range res {
				if re.MatchString(b.String()) {
					return fmt.Errorf("the frame at %v names this machine (%q): %q", f.At, names[i], b.String())
				}
			}
		}
	}
	return nil
}
