package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/blox-eng/matchblox/internal/remote"
)

// A file of the diff over reviewMaxLines lines or reviewMaxBytes bytes is
// too large to draw at once: the review lists its files, and each opens on
// its own.
const (
	reviewMaxLines = 4000
	reviewMaxBytes = 400 << 10
	reviewGitMax   = 32 << 20 // the most bytes read from one answer of git
)

var errReviewBig = errors.New("git's answer is too large")

// reviewGit runs git in dir on the host that owns the session and returns
// its stdout: "" is this machine, else ssh to that peer. A test replaces it.
// An error never carries git's stderr: it can quote a line of a file.
var reviewGit = func(host, dir string, args ...string) (string, error) {
	argv := append([]string{"git", "--no-pager", "--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.quotepath=off", "-C", dir}, args...)
	if host != "" {
		out, err := hostRun(peerGitArgv(host, argv))
		if err == nil && len(out) > reviewGitMax {
			return "", errReviewBig
		}
		return out, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // read-only git commands, separate argv
	cmd.Env = append(slices.DeleteFunc(os.Environ(), func(kv string) bool { return strings.HasPrefix(kv, "GIT_") }), "GIT_TERMINAL_PROMPT=0")
	out := &capBuf{max: reviewGitMax}
	cmd.Stdout = out
	if err := cmd.Run(); err != nil {
		if out.over {
			return "", errReviewBig
		}
		return "", fmt.Errorf("git %s did not succeed", args[0])
	}
	return string(out.b), nil
}

// peerGitArgv is ssh that runs the git argv on host, as the builder's own
// login, and prints its stdout.
func peerGitArgv(host string, git []string) []string {
	script := remote.Quote(append([]string{"/bin/sh", "-c", `PATH="` + remote.HostPath + `:$PATH"; export PATH; exec "$@"`, "sh"}, git...))
	argv := append([]string{"ssh", "-T", "-o", "BatchMode=yes", "-o", "ConnectTimeout=8"}, remote.NoForward()...)
	return append(argv, "--", host, script)
}

// capBuf keeps what is written to it, up to max bytes.
type capBuf struct {
	b    []byte
	max  int
	over bool
}

func (c *capBuf) Write(p []byte) (int, error) {
	if len(c.b)+len(p) > c.max {
		c.over = true
		return 0, errReviewBig
	}
	c.b = append(c.b, p...)
	return len(p), nil
}

// reviewLine is a line of a file's diff, as the review draws it. kind is
// '@' for a hunk head, '+', '-' or ' ' for a line of source, and 'm' for a
// note that is no line of source. text is visible text: never a byte a
// terminal reads as control.
type reviewLine struct {
	kind     byte
	text     string
	old, new int
}

// commentable: a line of source, where c opens a comment.
func (l reviewLine) commentable() bool { return l.kind == '+' || l.kind == '-' || l.kind == ' ' }

// reviewFile is one file of the diff. loaded: lines are its diff. A file of
// a diff too large to show at once loads when it opens. cut: only its first
// reviewMaxLines lines are kept.
type reviewFile struct {
	path     string
	add, del int
	binary   bool
	loaded   bool
	cut      bool
	lines    []reviewLine
}

// reviewDiff is the session's diff against a base. mergeBase is the commit
// the files were diffed with. big: the files list, and each opens on its
// own.
type reviewDiff struct {
	against, note string
	mergeBase     string
	files         []reviewFile
	big           bool
}

func (d *reviewDiff) totals() (add, del int) {
	for _, f := range d.files {
		add, del = add+f.add, del+f.del
	}
	return add, del
}

// reviewLoaded is the end of reading the diff: the diff, or the sentence
// that says why there is none.
type reviewLoaded struct {
	diff *reviewDiff
	why  string
}

// reviewFileLoaded is a file of a large diff, read when it opened.
type reviewFileLoaded struct {
	at   int
	file reviewFile
	why  string
}

var branchName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/+@-]*$`)

// validBranch: a name that is safe as part of a ref we ask git about. It
// comes from a service, maybe a peer's.
func validBranch(name string) bool {
	return len(name) <= 200 && branchName.MatchString(name) && !strings.Contains(name, "..") && !strings.Contains(name, "@{") && !strings.HasSuffix(name, ".lock")
}

// reviewHostLabel is how a sentence names the host git ran on.
func reviewHostLabel(host string) string {
	if host != "" {
		return hostName(host)
	}
	if name, err := os.Hostname(); err == nil && name != "" {
		return name
	}
	return "this machine"
}

// reviewBase finds the commit to diff against. recorded is the branch the
// session started from, "" when none is known: then the host's default
// branch (origin/HEAD, else main, else master). A recorded branch that is
// gone locally falls back to origin/<it>, else the default branch; gone says
// so. against names the base for the head.
func reviewBase(git func(args ...string) (string, error), recorded string) (against, sha string, gone bool) {
	commit := func(ref string) string {
		out, err := git("rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
		if err != nil {
			return ""
		}
		return strings.TrimSpace(out)
	}
	if recorded != "" && validBranch(recorded) {
		if sha := commit("refs/heads/" + recorded); sha != "" {
			return recorded, sha, false
		}
		gone = true
		if sha := commit("refs/remotes/origin/" + recorded); sha != "" {
			return "origin/" + recorded, sha, true
		}
	}
	if out, err := git("symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
		ref := strings.TrimSpace(out)
		if rest, ok := strings.CutPrefix(ref, "refs/remotes/"); ok && validBranch(rest) {
			if sha := commit(ref); sha != "" {
				return rest, sha, gone
			}
		}
	}
	for _, name := range []string{"main", "master"} {
		if sha := commit("refs/heads/" + name); sha != "" {
			return name, sha, gone
		}
	}
	return "", "", gone
}

var diffFlags = []string{"--no-color", "--no-renames", "--no-ext-diff", "--no-textconv"}

// loadReview reads the diff of the session: its sentence when there is
// none to review, and the whole diff unless it is too large.
func loadReview(spec reviewSpec) reviewLoaded {
	git := func(args ...string) (string, error) { return reviewGit(spec.host, spec.dir, args...) }
	noGit := reviewLoaded{why: "No changes to review: " + spec.name + " has no git folder."}
	if spec.dir == "" {
		return noGit
	}
	if out, err := git("rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(out) != "true" {
		return noGit
	}
	against, sha, gone := reviewBase(git, spec.base)
	if sha == "" {
		return reviewLoaded{why: "No changes to review: " + spec.name + " has no base branch to compare with (main, master or origin/HEAD)."}
	}
	d := &reviewDiff{against: against, mergeBase: sha}
	if gone {
		d.note = spec.base + " is gone on " + reviewHostLabel(spec.host) + ". The diff shows against " + against + "."
	}
	if out, err := git("merge-base", sha, "HEAD"); err == nil && strings.TrimSpace(out) != "" {
		d.mergeBase = strings.TrimSpace(out)
	}
	out, err := git(append([]string{"diff", "--numstat", "-z"}, append(slices.Clone(diffFlags), "--end-of-options", d.mergeBase)...)...)
	if err != nil {
		return reviewLoaded{why: "review did not open: git did not answer."}
	}
	d.files = parseNumstat(out)
	if len(d.files) == 0 {
		return reviewLoaded{why: spec.name + " has no changes against " + against + "."}
	}
	for _, f := range d.files {
		d.big = d.big || f.add+f.del > reviewMaxLines
	}
	if d.big {
		return reviewLoaded{diff: d}
	}
	full, err := git(append([]string{"diff", "-U3"}, append(slices.Clone(diffFlags), "--end-of-options", d.mergeBase)...)...)
	if err != nil {
		d.big = true // too large, or unreadable as a whole: each file opens on its own
		return reviewLoaded{diff: d}
	}
	sections := splitSections(full)
	if len(sections) != len(d.files) {
		d.big = true
		return reviewLoaded{diff: d}
	}
	for i, s := range sections {
		if len(s) > reviewMaxBytes || strings.Count(s, "\n") > reviewMaxLines {
			d.big = true
		}
		d.files[i].lines, d.files[i].binary = parseSection(s)
		d.files[i].loaded = true
	}
	if d.big {
		for i := range d.files {
			d.files[i].lines, d.files[i].loaded = nil, false
		}
	}
	return reviewLoaded{diff: d}
}

// loadReviewFile reads one file of a large diff.
func loadReviewFile(spec reviewSpec, mergeBase string, at int, f reviewFile) reviewFileLoaded {
	out, err := reviewGit(spec.host, spec.dir, append([]string{"diff", "-U3"}, append(slices.Clone(diffFlags), "--end-of-options", mergeBase, "--", ":(literal)"+f.path)...)...)
	switch {
	case errors.Is(err, errReviewBig):
		f.lines, f.loaded, f.cut = []reviewLine{{kind: 'm', text: "this file's diff is too large to show"}}, true, true
		return reviewFileLoaded{at: at, file: f}
	case err != nil:
		return reviewFileLoaded{at: at, file: f, why: "git did not answer for that file"}
	}
	f.lines, f.binary = parseSection(out)
	f.loaded = true
	if len(f.lines) > reviewMaxLines {
		f.lines, f.cut = f.lines[:reviewMaxLines], true
	}
	return reviewFileLoaded{at: at, file: f}
}

// parseNumstat reads `git diff --numstat -z --no-renames`: add TAB del TAB
// path NUL. A binary file counts "-".
func parseNumstat(out string) []reviewFile {
	var files []reviewFile
	for rec := range strings.SplitSeq(out, "\x00") {
		parts := strings.SplitN(rec, "\t", 3)
		if len(parts) != 3 || parts[2] == "" {
			continue
		}
		f := reviewFile{path: parts[2]}
		if parts[0] == "-" || parts[1] == "-" {
			f.binary = true
		} else {
			f.add, _ = strconv.Atoi(parts[0])
			f.del, _ = strconv.Atoi(parts[1])
		}
		files = append(files, f)
	}
	return files
}

// splitSections cuts a diff at each `diff --git` head.
func splitSections(diff string) []string {
	var out []string
	start := -1
	for i := 0; i < len(diff); {
		if strings.HasPrefix(diff[i:], "diff --git ") && (i == 0 || diff[i-1] == '\n') {
			if start >= 0 {
				out = append(out, diff[start:i])
			}
			start = i
		}
		nl := strings.IndexByte(diff[i:], '\n')
		if nl < 0 {
			break
		}
		i += nl + 1
	}
	if start >= 0 {
		out = append(out, diff[start:])
	}
	return out
}

var hunkHead = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// parseSection reads one file's diff. The lines are visible text.
func parseSection(section string) (lines []reviewLine, binary bool) {
	var old, nw int
	inHunk := false
	for l := range strings.SplitSeq(strings.TrimSuffix(section, "\n"), "\n") {
		if m := hunkHead.FindStringSubmatch(l); m != nil {
			old, _ = strconv.Atoi(m[1])
			nw, _ = strconv.Atoi(m[2])
			inHunk = true
			lines = append(lines, reviewLine{kind: '@', text: reviewVisible(l)})
			continue
		}
		if !inHunk {
			if strings.HasPrefix(l, "Binary files ") || strings.HasPrefix(l, "GIT binary patch") {
				binary = true
				lines = append(lines, reviewLine{kind: 'm', text: "binary file"})
			}
			continue
		}
		switch {
		case strings.HasPrefix(l, "+"):
			lines = append(lines, reviewLine{kind: '+', text: reviewVisible(l[1:]), new: nw})
			nw++
		case strings.HasPrefix(l, "-"):
			lines = append(lines, reviewLine{kind: '-', text: reviewVisible(l[1:]), old: old})
			old++
		case strings.HasPrefix(l, " "):
			lines = append(lines, reviewLine{kind: ' ', text: reviewVisible(l[1:]), old: old, new: nw})
			old++
			nw++
		case l == "":
			lines = append(lines, reviewLine{kind: ' ', old: old, new: nw})
			old++
			nw++
		default:
			lines = append(lines, reviewLine{kind: 'm', text: reviewVisible(l)})
		}
	}
	if len(lines) == 0 {
		lines = []reviewLine{{kind: 'm', text: "no lines changed (a mode change or an empty file)"}}
	}
	return lines, binary
}

// reviewVisible is s with every byte a terminal could read as control
// shown as text: ESC is ^[, another C0 key is ^ and its letter, DEL is ^?,
// a C1 character or an invisible one is <U+XXXX>, a byte that is no UTF-8
// is <XX>. Nothing is dropped: the reader sees what the file holds.
func reviewVisible(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			fmt.Fprintf(&b, "<%02X>", s[i])
		case r == '\t':
			b.WriteString("    ")
		case r < 0x20:
			b.WriteByte('^')
			b.WriteByte(byte(r) + 0x40)
		case r == 0x7f:
			b.WriteString("^?")
		case !unicode.IsPrint(r):
			fmt.Fprintf(&b, "<U+%04X>", r)
		default:
			b.WriteRune(r)
		}
		i += n
	}
	return b.String()
}

// reviewComment is a comment on a line of source. removed: the line is one
// the diff takes away, and line is its number before.
type reviewComment struct {
	path    string
	line    int
	removed bool
	src     string
	text    string
}

func (c reviewComment) where() string {
	if c.removed {
		return c.path + ":" + strconv.Itoa(c.line) + " (a removed line)"
	}
	return c.path + ":" + strconv.Itoa(c.line)
}

// reviewText is the message the review sends: each comment with its
// file:line, the line of source, and the words. All of it is visible text.
func reviewText(name, against string, comments []reviewComment) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "Review of %s against %s: %s.\n", reviewVisible(name), reviewVisible(against), plural(len(comments), "comment"))
	for i, c := range comments {
		fmt.Fprintf(&b, "\n%d. %s\n   %s\n   %s\n", i+1, reviewVisible(c.where()), reviewClip(reviewVisible(c.src), 300), reviewClip(reviewVisible(c.text), 500))
	}
	return b.String()
}

func reviewClip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
