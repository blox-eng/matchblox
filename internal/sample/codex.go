package sample

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/limits"
)

// codexRollout finds the rollout a Codex process writes: Codex opens
// <CODEX_HOME>/sessions/YYYY/MM/DD/rollout-*.jsonl for append at its first
// turn and holds it open. An npm install runs the native binary as a child,
// so the children are searched too. read is false when the open files of
// the process cannot be read; read with no path: the session is fresh.
func (s *Sampler) codexRollout(pid int, children map[int][]int) (path string, read bool) {
	stack := []int{pid}
	for n := 0; len(stack) > 0 && n < 64; n++ {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		files, ok := s.FS.OpenFiles(cur)
		read = read || ok
		for _, f := range files {
			base := filepath.Base(f)
			if !strings.HasPrefix(base, "rollout-") || !strings.HasSuffix(base, ".jsonl") || !strings.Contains(f, "/sessions/") {
				continue
			}
			// More than one open: the one written last is on screen.
			if path == "" || newer(f, path) {
				path = f
			}
		}
		stack = append(stack, children[cur]...)
	}
	return path, read
}

// codexInDir finds the rollout of a Codex that holds none open: Codex
// 0.163 closes it after each write (#179), and macOS lists no open files
// at all. It is the one session's rollout of the process's directory
// begun and written since the process started, among those created since
// then (a month at most). Two files of one session are one: a revert
// (Esc Esc on an earlier prompt) writes on in a new file, and the newer is
// taken. It must be this session's for sure, or z parks the wrong pane:
// only the CLI's rollouts count, not a `codex exec` run's or a thread of
// the Codex app or an editor. Two sessions begun since the start (a /new)
// cannot be told apart, and neither can one begun before it and written
// since (a resume without an id, a session that ended after it started):
// ambiguous is true. The caller asks only when no other Codex session
// runs in the directory. Codex names its day directories in its own zone,
// so the scan takes a day either side; a directory not written since the
// start is not read.
func (r *agentReader) codexInDir(home, cwd string, started, now time.Time) (path string, ambiguous bool) {
	path, sessions, older := r.codexScan(home, cwd, started, now)
	if sessions > 1 || sessions == 0 && older {
		return "", true
	}
	return path, false
}

// codexScan is codexInDir's look at the directory: the newest rollout of
// the sessions begun since the start, how many sessions those are, and
// whether a CLI rollout begun before the start was written since.
func (r *agentReader) codexScan(home, cwd string, started, now time.Time) (path string, sessions int, older bool) {
	if home == "" || cwd == "" {
		return "", 0, false
	}
	started, now = started.Local(), now.Local()
	day := time.Date(started.Year(), started.Month(), started.Day()-1, 0, 0, 0, 0, time.Local)
	if month := now.AddDate(0, -1, 0); day.Before(month) {
		day = time.Date(month.Year(), month.Month(), month.Day(), 0, 0, 0, 0, time.Local)
	}
	id := ""
	for last := now.AddDate(0, 0, 1); !day.After(last); day = day.AddDate(0, 0, 1) {
		dir := filepath.Join(home, "sessions", day.Format("2006/01/02"))
		if st, err := os.Stat(dir); err != nil || st.ModTime().Before(started) {
			continue // no rollout was created there since
		}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			name := e.Name()
			if !strings.HasPrefix(name, "rollout-") || !strings.HasSuffix(name, ".jsonl") {
				continue
			}
			f := filepath.Join(dir, name)
			if k, ok := statKey(f); !ok || k.mtime.Before(started) {
				continue
			}
			m := r.metaOf(f)
			if m.cwd != cwd || !codexCLI[m.originator] {
				continue
			}
			// Process start times have tick precision; a session's own
			// rollout begins a second or more after it.
			if !m.began.IsZero() && m.began.Before(started.Add(-time.Second)) {
				older = true
				continue
			}
			switch {
			case sessions == 0 || m.id == "" || m.id != id:
				path, id, sessions = f, m.id, sessions+1
			case newer(f, path): // the same session's later rollout: a revert
				path = f
			}
		}
	}
	return path, sessions, older
}

// codexCLI are the originators of the Codex CLI's own rollouts: codex-tui,
// codex_cli_rs before it, none before that. A `codex exec` run writes
// codex_exec, and the Codex app and editors write their own names.
var codexCLI = map[string]bool{"": true, "codex-tui": true, "codex_cli_rs": true}

// codexArgv is argv as Codex reads it: an npm install runs
// `node <script> ...`.
func codexArgv(argv []string) []string {
	if len(argv) > 1 && interpreter(filepath.Base(argv[0])) {
		for i, a := range argv[1:] {
			if !strings.HasPrefix(a, "-") {
				return argv[i+1:]
			}
		}
	}
	return argv
}

// codexValued are Codex's options that take the next argument as their
// value, before a subcommand and after `resume`.
var codexValued = map[string]bool{
	"-m": true, "--model": true, "-c": true, "--config": true, "-p": true, "--profile": true,
	"-C": true, "--cd": true, "-s": true, "--sandbox": true, "-a": true, "--ask-for-approval": true,
	"-i": true, "--image": true, "--add-dir": true, "--local-provider": true, "--enable": true, "--disable": true,
}

// codexWords are argv's words that are no option nor an option's value:
// the subcommand, then its arguments.
func codexWords(argv []string) []string {
	var words []string
	for i := 1; i < len(argv); i++ {
		switch a := argv[i]; {
		case codexValued[a]:
			i++
		case !strings.HasPrefix(a, "-"):
			words = append(words, a)
		}
	}
	return words
}

// codexResume: argv resumes a session. "resume" anywhere counts: a wrong
// guess costs only "unmeasured".
func codexResume(argv []string) bool {
	return len(argv) > 1 && slices.Contains(argv[1:], "resume")
}

// codexResumeID is the session id of `codex resume <id>`, in the lower case
// of its rollout's name, or "".
func codexResumeID(argv []string) string {
	w := codexWords(argv)
	if len(w) < 2 || w[0] != "resume" || strings.Trim(w[1], "0123456789abcdefABCDEF-") != "" {
		return "" // --last, the picker, or no id
	}
	return strings.ToLower(w[1])
}

// codexResumed finds the rollout of `codex resume <id>`, on whatever day it
// began: Codex writes on in its newest, which it names after the session
// id. A revert (Esc Esc on an earlier prompt) writes on in
// rollout-<ts>-<id>_<rollout>.jsonl, on the day it is made, so the days
// around now are looked at again on each sample. The newest found is kept;
// a miss is tried again after 30 seconds, as Codex can unpack a compressed
// rollout late.
func (r *agentReader) codexResumed(home, id string, now time.Time) string {
	key := home + "\x00" + id
	path, ok := r.resumed[key]
	if !ok {
		if at, missed := r.resumedMiss[key]; !missed || now.Sub(at) >= 30*time.Second {
			matches, _ := filepath.Glob(filepath.Join(home, "sessions", "*", "*", "*", "rollout-*-"+id+".jsonl"))
			reverts, _ := filepath.Glob(filepath.Join(home, "sessions", "*", "*", "*", "rollout-*-"+id+"_*.jsonl"))
			for _, f := range append(matches, reverts...) {
				if r.metaOf(f).id == id && (path == "" || newer(f, path)) {
					path = f
				}
			}
			if path == "" {
				r.resumedMiss[key] = now
			} else {
				r.resumed[key] = path
				delete(r.resumedMiss, key)
			}
		}
	}
	for d := -1; d <= 1; d++ {
		dir := filepath.Join(home, "sessions", now.Local().AddDate(0, 0, d).Format("2006/01/02"))
		reverted, _ := filepath.Glob(filepath.Join(dir, "rollout-*-"+id+"_*.jsonl"))
		for _, f := range reverted {
			if r.metaOf(f).id == id && (path == "" || newer(f, path)) {
				path = f
			}
		}
	}
	if path != "" {
		r.resumed[key] = path
	}
	return path
}

// rolloutMeta is what a rollout's session_meta, its first line, says: the
// session's id, its directory, what started it (codex-tui, codex_exec)
// and when it began.
type rolloutMeta struct {
	id, cwd, originator string
	began               time.Time
}

func (r *agentReader) metaOf(path string) rolloutMeta {
	if m, ok := r.rolloutMeta[path]; ok {
		return m
	}
	f, err := os.Open(path)
	if err != nil {
		return rolloutMeta{}
	}
	defer f.Close()
	line, err := bufio.NewReader(io.LimitReader(f, 1<<20)).ReadBytes('\n')
	if err != nil { // the first line is still being written
		return rolloutMeta{}
	}
	var meta struct {
		Type    string `json:"type"`
		Payload struct {
			ID         string    `json:"id"`
			Cwd        string    `json:"cwd"`
			Originator string    `json:"originator"`
			Timestamp  time.Time `json:"timestamp"`
		} `json:"payload"`
	}
	var m rolloutMeta
	if json.Unmarshal(line, &meta) == nil && meta.Type == "session_meta" {
		p := meta.Payload
		m = rolloutMeta{p.ID, p.Cwd, p.Originator, p.Timestamp}
	}
	r.rolloutMeta[path] = m
	return m
}

func newer(a, b string) bool {
	ka, okA := statKey(a)
	kb, okB := statKey(b)
	return okA && (!okB || ka.mtime.After(kb.mtime))
}

// codexFound says what the end of a rollout told.
type codexFound int

const (
	codexNone      codexFound = iota // the whole file has no usage yet: the first turn runs
	codexNotInTail                   // the last tailBytes hold no usage: not measured
	codexMeasured
)

type codexLine struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Payload   struct {
		Type       string `json:"type"`
		Model      string `json:"model"`
		RateLimits *struct {
			Primary   *codexWindow `json:"primary"`
			Secondary *codexWindow `json:"secondary"`
		} `json:"rate_limits"`
		Info *struct {
			Last struct {
				Total int `json:"total_tokens"`
			} `json:"last_token_usage"`
			Window int `json:"model_context_window"` // null for a model Codex has no window for
		} `json:"info"`
	} `json:"payload"`
}

type codexWindow struct {
	Used     float64 `json:"used_percent"`
	Minutes  int     `json:"window_minutes"`
	ResetsAt int64   `json:"resets_at"`
}

// codexUsage reads the end of a rollout: the newest token_count with info
// gives the tokens of the last turn (what Codex counts as in the context
// window) and the window; the newest turn_context gives the model; the
// newest rate_limits gives the plan's limits (nil: none in the tail).
func (r *agentReader) codexUsage(path string) (Usage, *limits.Reading, codexFound) {
	key, ok := statKey(path)
	if !ok {
		return Usage{}, nil, codexNone
	}
	if c, ok := r.codex[path]; ok && c.key == key {
		return c.val.u, c.val.lim, c.val.found
	}
	f, err := os.Open(path)
	if err != nil {
		return Usage{}, nil, codexNone
	}
	defer f.Close()
	off := max(key.size-tailBytes, 0)
	buf := make([]byte, key.size-off)
	if _, err := f.ReadAt(buf, off); err != nil && err != io.EOF {
		return Usage{}, nil, codexNone
	}
	var u Usage
	// lim is the newest reading with a window; empty, the newest without
	// one (Codex also writes limits of other ids, all null).
	var lim, empty *limits.Reading
	found := false
	lines := bytes.Split(buf, []byte{'\n'})
	for i := len(lines) - 1; i >= 0 && (!found || u.Model == "" || lim == nil); i-- {
		l := lines[i]
		needUsage := !found && bytes.Contains(l, []byte(`"token_count"`)) && bytes.Contains(l, []byte(`"last_token_usage"`))
		needModel := u.Model == "" && bytes.Contains(l, []byte(`"turn_context"`))
		needLimits := lim == nil && bytes.Contains(l, []byte(`"rate_limits"`))
		if !needUsage && !needModel && !needLimits {
			continue
		}
		var c codexLine
		if json.Unmarshal(l, &c) != nil {
			continue
		}
		if needUsage && c.Payload.Type == "token_count" && c.Payload.Info != nil {
			u.Tokens, u.Window, found = c.Payload.Info.Last.Total, c.Payload.Info.Window, true
		}
		if needModel && c.Type == "turn_context" {
			u.Model = c.Payload.Model
		}
		if needLimits && c.Payload.RateLimits != nil {
			r := &limits.Reading{At: c.Timestamp}
			for _, w := range []*codexWindow{c.Payload.RateLimits.Primary, c.Payload.RateLimits.Secondary} {
				if w != nil && w.Minutes > 0 {
					r.Windows = append(r.Windows, limits.Window{UsedPct: w.Used, Minutes: w.Minutes, ResetsAt: limits.Unix(w.ResetsAt)})
				}
			}
			switch {
			case len(r.Windows) > 0:
				lim = r
			case empty == nil:
				empty = r
			}
		}
	}
	if lim == nil {
		lim = empty
	}
	res := codexMeasured
	switch {
	case !found && off > 0:
		res = codexNotInTail
	case !found:
		res = codexNone
	}
	r.codex[path] = cached[codexRead]{key, codexRead{u, lim, res}}
	return u, lim, res
}

type codexRead struct {
	u     Usage
	lim   *limits.Reading
	found codexFound
}
