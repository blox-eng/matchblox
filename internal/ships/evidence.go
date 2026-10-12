package ships

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Evidence that a session drives a pull request, strongest first.
const (
	Created = "created" // its gh pr create printed the PR's URL
	Pushed  = "pushed"  // it pushed the branch an open PR has as head
	Acted   = "acted"   // it ran gh pr <verb> on the PR, or a command with its URL
	Branch  = "branch"  // it works in a worktree on the PR's head branch
)

func strength(kind string) int {
	switch kind {
	case Created:
		return 4
	case Pushed:
		return 3
	case Acted:
		return 2
	case Branch:
		return 1
	}
	return 0
}

// found is one piece of evidence read from a transcript line, before its
// repository is resolved.
type found struct {
	Kind   string
	Repo   string // owner/name when the command names it
	Dir    string // where the command ran: its git remote names the repository
	Number int    // 0 for a push
	Branch string // a push's branch
	At     time.Time
}

// pending is a tool call whose evidence is in its result: the URL a
// gh pr create printed, or the branch a bare git push pushed.
type pending struct {
	Dir    string `json:"dir"`
	Repo   string `json:"repo,omitempty"`
	Create bool   `json:"create,omitempty"`
}

// maxPending bounds the calls that wait for their result; a result never
// comes for a call the agent was stopped in.
const maxPending = 32

type rawLine struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Cwd       string    `json:"cwd"`
	GitBranch string    `json:"gitBranch"`
	Message   struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`
}

type block struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input struct {
		Command string `json:"command"`
	} `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
}

// toolUseResult is what Claude Code records about a Bash call's git work.
type toolUseResult struct {
	GitOperation struct {
		Push *struct {
			Branch string `json:"branch"`
		} `json:"push"`
		PR *struct {
			URL    string `json:"url"`
			Action string `json:"action"`
		} `json:"pr"`
	} `json:"gitOperation"`
}

// mayHold is the cheap test before a line is parsed: it names a command we
// read (git -C dir push has words between git and push), or it is the
// result of a call we wait for.
func mayHold(line []byte, pend map[string]pending) bool {
	if bytes.Contains(line, []byte("gh pr")) || bytes.Contains(line, []byte("/pull/")) ||
		bytes.Contains(line, []byte("git ")) && bytes.Contains(line, []byte(" push")) {
		return true
	}
	for id := range pend {
		if bytes.Contains(line, []byte(id)) {
			return true
		}
	}
	return false
}

// parseLine reads the evidence of one transcript line. Only the commands
// of Bash calls and the results of the calls in pend count: what the agent
// says in prose never does.
func parseLine(b []byte, pend map[string]pending) []found {
	var l rawLine
	if json.Unmarshal(b, &l) != nil || len(l.Message.Content) == 0 || l.Message.Content[0] != '[' {
		return nil
	}
	var blocks []block
	if json.Unmarshal(l.Message.Content, &blocks) != nil {
		return nil
	}
	var out []found
	for _, bl := range blocks {
		switch {
		case l.Type == "assistant" && bl.Type == "tool_use" && bl.Name == "Bash":
			out = append(out, commandEvidence(bl.Input.Command, l.Cwd, l.GitBranch, bl.ID, l.Timestamp, pend)...)
		case l.Type == "user" && bl.Type == "tool_result":
			p, ok := pend[bl.ToolUseID]
			if !ok {
				continue
			}
			delete(pend, bl.ToolUseID)
			out = append(out, resultEvidence(p, bl.Content, l.ToolUseResult, l.Timestamp)...)
		}
	}
	return out
}

func resultEvidence(p pending, content, raw json.RawMessage, at time.Time) []found {
	var r toolUseResult
	_ = json.Unmarshal(raw, &r) // a string result has no git operation
	var out []found
	if p.Create {
		text := resultText(content)
		if pr := r.GitOperation.PR; pr != nil && pr.Action == "created" {
			text = pr.URL
		}
		if m := pullURL.FindStringSubmatch(text); m != nil {
			n, _ := strconv.Atoi(m[3])
			out = append(out, found{Kind: Created, Repo: m[1] + "/" + m[2], Number: n, At: at})
		}
	}
	if push := r.GitOperation.Push; push != nil && push.Branch != "" {
		out = append(out, found{Kind: Pushed, Repo: p.Repo, Dir: p.Dir, Branch: push.Branch, At: at})
	}
	return out
}

// resultText is a tool result's content: a string, or text blocks.
func resultText(content json.RawMessage) string {
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}
	var parts []struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(content, &parts)
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

var pullURL = regexp.MustCompile(`https?://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/pull/([0-9]+)`)

// prVerbs act on a pull request that already exists.
var prVerbs = map[string]bool{"merge": true, "checks": true, "view": true, "edit": true, "ready": true, "comment": true}

// ghValues are the gh pr flags that take a value, so the selector is the
// first word that is neither a flag nor a flag's value.
var ghValues = set("-R", "--repo", "-b", "--body", "-F", "--body-file", "-t", "--title", "--subject",
	"-B", "--base", "-H", "--head", "--json", "-q", "--jq", "-T", "--template", "-i", "--interval",
	"-A", "--author-email", "--match-head-commit", "--milestone", "--add-label", "--remove-label",
	"--add-reviewer", "--remove-reviewer", "--add-assignee", "--remove-assignee", "--add-project", "--remove-project")

// prose are the flags whose value is text a person reads: a URL there is a
// mention, not a command acting on it.
var prose = set("-m", "--message", "-b", "--body", "-t", "--title", "--subject")

var pushValues = set("-o", "--push-option", "--repo", "--receive-pack", "--exec")

func set(s ...string) map[string]bool {
	m := map[string]bool{}
	for _, v := range s {
		m[v] = true
	}
	return m
}

// commandEvidence reads one Bash call: each simple command in it, with the
// directory a cd before it moved to.
func commandEvidence(script, cwd, branch, id string, at time.Time, pend map[string]pending) []found {
	var out []found
	dir := cwd
	for _, w := range commands(script) {
		w = unwrap(w)
		if len(w) == 0 {
			continue
		}
		for i, word := range w {
			if i > 0 && prose[w[i-1]] {
				continue
			}
			for _, m := range pullURL.FindAllStringSubmatch(word, -1) {
				n, _ := strconv.Atoi(m[3])
				out = append(out, found{Kind: Acted, Repo: m[1] + "/" + m[2], Number: n, At: at})
			}
		}
		switch w[0] {
		case "cd":
			if len(w) > 1 {
				dir = join(dir, w[1])
			}
		case "git":
			gitDir, args := gitArgs(dir, w[1:])
			if len(args) == 0 || args[0] != "push" {
				continue
			}
			branches, current := pushBranches(args[1:])
			for _, b := range branches {
				out = append(out, found{Kind: Pushed, Dir: gitDir, Branch: b, At: at})
			}
			switch {
			case !current:
			case gitDir == cwd && branch != "" && branch != "HEAD":
				// the session's branch, as the transcript line records it
				out = append(out, found{Kind: Pushed, Dir: gitDir, Branch: branch, At: at})
			default:
				addPending(pend, id, pending{Dir: gitDir})
			}
		case "gh":
			if len(w) < 3 || w[1] != "pr" {
				continue
			}
			repo, sel := ghArgs(w[3:])
			switch {
			case w[2] == "create":
				addPending(pend, id, pending{Dir: dir, Repo: repo, Create: true})
			case prVerbs[w[2]] && !pullURL.MatchString(sel): // a URL was read above, with its repository
				if n := prNumber(sel); n > 0 {
					out = append(out, found{Kind: Acted, Repo: repo, Dir: dir, Number: n, At: at})
				}
			}
		}
	}
	return out
}

func addPending(pend map[string]pending, id string, p pending) {
	if id == "" {
		return
	}
	if len(pend) >= maxPending {
		for k := range pend { // the oldest is unknown; any one makes room
			delete(pend, k)
			break
		}
	}
	pend[id] = p
}

// unwrap drops leading variable assignments and the wrappers that run the
// command after them.
func unwrap(w []string) []string {
	for len(w) > 0 {
		switch {
		case strings.Contains(w[0], "=") && !strings.HasPrefix(w[0], "-"):
			w = w[1:]
		case w[0] == "env" || w[0] == "command" || w[0] == "exec" || w[0] == "time" || w[0] == "nohup":
			w = w[1:]
		case w[0] == "ctx-wire" && len(w) > 1 && w[1] == "run":
			w = w[2:]
		default:
			return w
		}
	}
	return w
}

// join is the directory cd p moves to from dir. Empty when the shell
// alone knows it (cd -, cd $X, cd ~user): its commands name no repository.
func join(dir, p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		p = filepath.Join(home, p[1:])
	}
	switch {
	case p == "-" || strings.ContainsAny(p, "$`~*?"):
		return ""
	case filepath.IsAbs(p):
		return filepath.Clean(p)
	case dir == "":
		return ""
	}
	return filepath.Join(dir, p)
}

// gitArgs reads git's own options: -C moves the directory.
func gitArgs(dir string, args []string) (string, []string) {
	for len(args) > 0 {
		switch a := args[0]; {
		case (a == "-C" || a == "-c") && len(args) > 1:
			if a == "-C" {
				dir = join(dir, args[1])
			}
			args = args[2:]
		case strings.HasPrefix(a, "-"):
			args = args[1:]
		default:
			return dir, args
		}
	}
	return dir, nil
}

// pushBranches reads the branches a git push names. current: it pushes
// the checked-out branch without naming it (no refspec, or HEAD).
func pushBranches(args []string) (branches []string, current bool) {
	var pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			pos = append(pos, args[i+1:]...)
			i = len(args)
		case a == "-d" || a == "--delete" || a == "--tags" || a == "--all" || a == "--mirror":
			return nil, false
		case pushValues[a]:
			i++
		case strings.HasPrefix(a, "-"):
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) < 2 {
		return nil, true
	}
	for _, spec := range pos[1:] {
		spec = strings.TrimPrefix(spec, "+")
		if i := strings.IndexByte(spec, ':'); i >= 0 {
			if i == 0 {
				continue // :branch deletes it
			}
			spec = spec[i+1:]
		}
		if strings.HasPrefix(spec, "refs/") && !strings.HasPrefix(spec, "refs/heads/") {
			continue
		}
		spec = strings.TrimPrefix(spec, "refs/heads/")
		if spec == "HEAD" || spec == "@" {
			current = true
			continue
		}
		branches = append(branches, spec)
	}
	return branches, current
}

// ghArgs reads the repository (-R) and the selector of a gh pr command.
func ghArgs(args []string) (repo, sel string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-R" || a == "--repo":
			if i+1 < len(args) {
				repo = repoName(args[i+1])
			}
			i++
		case strings.HasPrefix(a, "--repo="):
			repo = repoName(strings.TrimPrefix(a, "--repo="))
		case ghValues[a]:
			i++
		case strings.HasPrefix(a, "-"):
		case sel == "":
			sel = a
		}
	}
	return repo, sel
}

// prNumber is the number a selector names: 123 or #123. A branch name is
// not read: gh resolves it, we would guess.
func prNumber(sel string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(sel, "#"))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

var ownerName = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// repoName reads owner/name from OWNER/REPO, HOST/OWNER/REPO, a URL or a
// git remote (git@github.com:owner/name.git). Empty when it names none.
func repoName(s string) string {
	s = strings.TrimSpace(s)
	for _, p := range []string{"https://", "http://", "ssh://", "git://"} {
		s = strings.TrimPrefix(s, p)
	}
	s = strings.TrimPrefix(s, "git@")
	s = strings.Replace(s, "github.com:", "github.com/", 1)
	s = strings.TrimPrefix(s, "github.com/")
	s = strings.TrimSuffix(strings.TrimSuffix(s, "/"), ".git")
	parts := strings.Split(s, "/")
	if len(parts) != 2 || !ownerName.MatchString(parts[0]) || !ownerName.MatchString(parts[1]) {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

// commands splits a shell script into the words of each simple command,
// the way sh would: quotes keep a word whole, a heredoc's body is not a
// command, and ; & | && || ( ) and newlines end one.
func commands(script string) [][]string {
	var (
		out      [][]string
		words    []string
		cur      strings.Builder
		inWord   bool
		heredocs []string
	)
	endWord := func() {
		if inWord {
			words = append(words, cur.String())
			cur.Reset()
			inWord = false
		}
	}
	endCmd := func() {
		endWord()
		if len(words) > 0 {
			out = append(out, words)
			words = nil
		}
	}
	s := script
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s):
			i++
			if s[i] != '\n' {
				cur.WriteByte(s[i])
				inWord = true
			}
		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				j = len(s) - i - 1
			}
			cur.WriteString(s[i+1 : i+1+j])
			inWord = true
			i += j + 1
		case c == '"':
			i++
			for ; i < len(s) && s[i] != '"'; i++ {
				if s[i] == '\\' && i+1 < len(s) {
					i++
				}
				cur.WriteByte(s[i])
			}
			inWord = true
		case c == '$' && i+1 < len(s) && s[i+1] == '(':
			j := closeParen(s, i+1)
			cur.WriteString(s[i:j])
			inWord = true
			i = j - 1
		case c == '#' && !inWord:
			for i < len(s) && s[i] != '\n' {
				i++
			}
			i--
		case c == '<' && strings.HasPrefix(s[i:], "<<") && !strings.HasPrefix(s[i:], "<<<"):
			endWord()
			j := i + 2
			if j < len(s) && s[j] == '-' {
				j++
			}
			for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
				j++
			}
			k := j
			for k < len(s) && !strings.ContainsRune(" \t\n;&|()<>", rune(s[k])) {
				k++
			}
			heredocs = append(heredocs, strings.Trim(s[j:k], `'"`))
			i = k - 1
		case c == '\n':
			endCmd()
			for _, delim := range heredocs {
				for i+1 < len(s) {
					end := strings.IndexByte(s[i+1:], '\n')
					if end < 0 {
						end = len(s) - i - 1
					}
					line := strings.TrimLeft(s[i+1:i+1+end], "\t")
					i += end + 1
					if line == delim {
						break
					}
				}
			}
			heredocs = nil
		case strings.IndexByte(";&|()", c) >= 0:
			endCmd()
		case c == ' ' || c == '\t' || c == '\r':
			endWord()
		default:
			cur.WriteByte(c)
			inWord = true
		}
	}
	endCmd()
	return out
}

// closeParen returns the index just past the parenthesis that closes the
// one at open.
func closeParen(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(s)
}
