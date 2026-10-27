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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/procfs"
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
	// uptime is the demo host's, for the start time of a session the demo
	// starts.
	uptime func() float64
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
	case len(argv) == 5 && argv[1] == "send-keys" && len(argv[4]) == 1 && argv[4][0] >= '1' && argv[4][0] <= '9':
		return w.pick(argv[3], argv[4])
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
	case len(argv) == 4 && argv[0] == "tmux" && argv[1] == "kill-pane":
		return w.closePane(argv[3])
	case len(argv) == 3 && argv[0] == "kill" && argv[1] == "-TERM":
		return nil // the agent went with its pane
	case argv[0] == "env" && slices.Contains(argv, "new-session"):
		return w.start(argv[slices.Index(argv, "tmux"):])
	}
	return fmt.Errorf("the demo does not run %q", argv)
}

// closePane is tmux kill-pane: the pane leaves tmux's list, and its
// process ends. Hold w.mu.
func (w *demoWorld) closePane(pane string) error {
	path := filepath.Join(w.root, "tmux-panes.txt")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var keep []string
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		f := strings.Split(l, "\t")
		if f[0] != pane {
			keep = append(keep, l)
			continue
		}
		pid, err := strconv.Atoi(f[3])
		if err != nil {
			return err
		}
		w.kill(pid)
	}
	delete(w.sessions, pane)
	return writeAtomic(path, []byte(strings.Join(keep, "\n")+"\n"))
}

// newPID and newPane are the first session the demo starts.
const (
	newPID  = 2000
	newPane = "%9"
)

// start is tmux new-session -d -s <name> -n <name> -c <dir> <agent>…: a
// Claude Code process in a new pane, at work on its first message, with
// its session file. Hold w.mu.
func (w *demoWorld) start(argv []string) error {
	name, dir := argv[slices.Index(argv, "-s")+1], argv[slices.Index(argv, "-c")+1]
	agent := argv[slices.Index(argv, "-c")+2:]
	// The real process inherits the launch context through env. Keep that
	// wrapper out of cmdline, as /proc does after env execs the agent.
	contextEnv := ""
	if len(agent) > 2 && agent[0] == "env" && strings.HasPrefix(agent[1], "MATCHBLOX_CONTEXT=") {
		contextEnv, agent = agent[1]+"\x00", agent[2:]
	}
	proc := filepath.Join(w.root, "proc", strconv.Itoa(newPID))
	if err := os.MkdirAll(proc, 0o755); err != nil {
		return err
	}
	ticks := int(w.uptime() * procfs.ClockTicks)
	files := map[string]string{
		"cmdline": strings.Join(agent, "\x00") + "\x00",
		"environ": "HOME=/home/user\x00TMUX_PANE=" + newPane + "\x00TERM=tmux-256color\x00" + contextEnv,
		"stat":    fmt.Sprintf("%d (%s) S 1 %d %d 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 %d 100000 2500", newPID, agent[0], newPID, newPID, ticks),
	}
	for f, body := range files {
		if err := os.WriteFile(filepath.Join(proc, f), []byte(body), 0o644); err != nil {
			return err
		}
	}
	if err := os.Symlink(dir, filepath.Join(proc, "cwd")); err != nil {
		return err
	}
	s := &demoSession{pid: newPID, name: name, id: "5ea1c0de-0000-4000-8000-000000000009", status: "busy"}
	w.sessions[newPane] = s
	now := w.clk.Now().UnixMilli()
	reg, err := json.Marshal(map[string]any{"pid": s.pid, "sessionId": s.id, "cwd": dir, "name": name,
		"status": s.status, "startedAt": now, "statusUpdatedAt": now})
	if err != nil {
		return err
	}
	if err := writeAtomic(w.registry(s.pid), append(reg, '\n')); err != nil {
		return err
	}
	path := filepath.Join(w.root, "tmux-panes.txt")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	line := strings.Join([]string{newPane, name + ":1.1", name, strconv.Itoa(newPID), agent[0], dir}, "\t")
	return writeAtomic(path, append(b, []byte(line+"\n")...))
}

// pick is a digit in the question tool's menu: the call gets its result,
// the agent works on the answer, and the menu leaves the pane. Hold w.mu.
func (w *demoWorld) pick(pane, digit string) error {
	s, ok := w.sessions[pane]
	if !ok {
		return fmt.Errorf("no session in %s", pane)
	}
	now := w.clk.Now()
	s.status, s.tokens = "busy", s.tokens+4000
	if err := w.appendTranscript(s,
		map[string]any{"type": "user", "isSidechain": false, "timestamp": now,
			"message": map[string]any{"content": []map[string]string{{"type": "tool_result", "tool_use_id": "toolu_01DEMO", "content": "The user picked option " + digit}}}},
		turnLine(now, s.tokens, "I rotate the token on each refresh and keep the old one for a minute."),
	); err != nil {
		return err
	}
	if err := writeAtomic(w.pane(pane), []byte("● I rotate the token on each refresh and keep the old one for a minute.\n\n╭──────────────────────────────────────────────╮\n│ > \n╰──────────────────────────────────────────────╯\n")); err != nil {
		return err
	}
	return w.writeRegistry(s, now)
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
	return writeAtomic(w.pane(pane), []byte(strings.Join(lines, "\n")))
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
	return writeAtomic(w.registry(s.pid), append(b, '\n'))
}

// writeAtomic replaces a file in one step: the sampler reads these files
// every 50 ms and must never see one half written.
func writeAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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

// TestASessionNamedLikeAWordOfTheRepoIsNoLeak: a tmux session called
// "work" or "docs" names nothing of the machine, since the fixture and the
// console use those words themselves.
func TestASessionNamedLikeAWordOfTheRepoIsNoLeak(t *testing.T) {
	words, err := repoWords("../../internal", "../../testdata/demo")
	if err != nil {
		t.Fatal(err)
	}
	got := identifying([]string{"work", "web", "docs", "claude", "matchblox", "main", "dev-3", "Ledge Payments"}, words)
	if want := []string{"dev-3", "Ledge Payments"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// thisMachine is what names the machine the test runs on: its host name,
// user, home and git identity, and the name of each tmux session and
// Claude Code session on it.
func thisMachine() []string {
	var out []string
	add := func(n string) {
		if n = strings.TrimSpace(n); len(n) >= 3 {
			out = append(out, n)
		}
	}
	for _, k := range []string{"user.name", "user.email"} {
		if b, err := exec.Command("git", "config", k).Output(); err == nil {
			add(string(b))
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

var word = regexp.MustCompile(`[\w-]+`)

// repoWords are the words of every file under dirs.
func repoWords(dirs ...string) (map[string]bool, error) {
	words := map[string]bool{}
	for _, d := range dirs {
		err := filepath.WalkDir(d, func(p string, e fs.DirEntry, err error) error {
			if err != nil || !e.Type().IsRegular() {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for _, w := range word.FindAll(b, -1) {
				words[string(w)] = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return words, nil
}

// identifying drops the names the repository uses itself: a tmux session
// called "work" or "docs" names nothing of the machine.
func identifying(names []string, words map[string]bool) []string {
	var out []string
	for _, n := range names {
		if !words[n] {
			out = append(out, n)
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
