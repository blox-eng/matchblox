package service

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
)

// diffOnlyFromConsole refuses a diff: act that no person at a console sent.
const diffOnlyFromConsole = "only a person at a console reviews a session's diff"

// diffMaxBytes is the raw unified-diff size above which the answer has the
// file list and counts only; each file can be asked for alone.
const diffMaxBytes = 2 << 20

const diffTimeout = 10 * time.Second

// sessionDiff answers diff:<pane> or diff:<pane>:<file>: the session's
// worktree against the merge base with Repo.Main.
func (s *Service) sessionDiff(ctx context.Context, a proto.Act, rest string) proto.Result {
	if !s.aConsole(a.Console, a) {
		return proto.Result{Err: diffOnlyFromConsole}
	}
	pane, file, _ := strings.Cut(rest, ":")
	x, ok := s.sessionAt(pane)
	if !ok {
		return refuseText(fmt.Sprintf("not kept: no session %q", pane))
	}
	if x.Cwd == "" || gitscan.Toplevel(x.Cwd) == "" {
		return proto.Result{Err: fmt.Sprintf("No changes to review: %s has no git folder.", x.Name)}
	}
	main := s.repoMain(x.Cwd)
	against, note, base, err := s.diffAgainst(ctx, x.Cwd, main)
	if err != nil {
		return proto.Result{Err: err.Error()}
	}
	raw, err := s.gitDiffBytes(ctx, x.Cwd, base, file)
	if err != nil {
		return proto.Result{Err: err.Error()}
	}
	if len(raw) == 0 {
		return proto.Result{Err: fmt.Sprintf("%s has no changes against %s.", x.Name, against)}
	}
	d := &proto.Diff{Against: against, Note: note}
	if file == "" && len(raw) > diffMaxBytes {
		files, err := s.gitDiffFilesOnly(ctx, x.Cwd, base)
		if err != nil {
			return proto.Result{Err: err.Error()}
		}
		d.Files = files
		return proto.Result{Diff: d}
	}
	d.Files = parseUnifiedDiff(raw)
	if file != "" {
		d.Files = filterDiffFile(d.Files, file)
	}
	return proto.Result{Diff: d}
}

// repoMain is Repo.Main for the checkout that holds cwd. Empty report: main.
func (s *Service) repoMain(cwd string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, _, ok := gitscan.RepoOf(s.cur.Git, cwd)
	if !ok || s.cur.Git == nil {
		return "main"
	}
	for _, r := range s.cur.Git.Repos {
		if r.Path == path && r.Main != "" {
			return r.Main
		}
	}
	return "main"
}

// diffAgainst picks the base branch name and the merge-base rev to diff.
// When Main is gone and origin/Main is there, Against is origin/Main and
// Note names the host.
func (s *Service) diffAgainst(ctx context.Context, cwd, main string) (against, note, base string, err error) {
	host := s.Host
	if host == "" {
		host, _ = os.Hostname()
	}
	against = main
	if !s.gitRevExists(ctx, cwd, main) {
		remote := "origin/" + main
		if !s.gitRevExists(ctx, cwd, remote) {
			return "", "", "", fmt.Errorf("%s is gone on %s, and %s is not there either", main, host, remote)
		}
		against, note = remote, main+" is gone on "+host
	}
	out, err := s.gitDiffCmd(ctx, cwd, "merge-base", against, "HEAD")
	if err != nil {
		return "", "", "", err
	}
	base = strings.TrimSpace(string(out))
	if base == "" {
		return "", "", "", fmt.Errorf("no merge-base of %s and HEAD in %s", against, cwd)
	}
	return against, note, base, nil
}

func (s *Service) gitRevExists(ctx context.Context, cwd, rev string) bool {
	_, err := s.gitDiffCmd(ctx, cwd, "rev-parse", "--verify", "--quiet", rev)
	return err == nil
}

// gitDiffBytes is the committed work against base plus the unstaged changes,
// optionally limited to one path. Output is raw: control bytes stay.
func (s *Service) gitDiffBytes(ctx context.Context, cwd, base, file string) ([]byte, error) {
	committed, err := s.gitDiffCmd(ctx, cwd, diffArgs(base, "HEAD", file)...)
	if err != nil {
		return nil, err
	}
	unstaged, err := s.gitDiffCmd(ctx, cwd, diffArgs("", "", file)...)
	if err != nil {
		return nil, err
	}
	if len(committed) == 0 {
		return unstaged, nil
	}
	if len(unstaged) == 0 {
		return committed, nil
	}
	out := make([]byte, 0, len(committed)+len(unstaged))
	out = append(out, committed...)
	if !bytes.HasSuffix(committed, []byte("\n")) {
		out = append(out, '\n')
	}
	return append(out, unstaged...), nil
}

func diffArgs(base, head, file string) []string {
	args := []string{"diff", "--no-color", "--no-ext-diff", "--no-textconv", "-M"}
	if base != "" {
		args = append(args, base)
		if head != "" {
			args = append(args, head)
		}
	}
	if file != "" {
		args = append(args, "--", file)
	}
	return args
}

// gitDiffFilesOnly lists paths and counts with no hunks, for a big diff.
func (s *Service) gitDiffFilesOnly(ctx context.Context, cwd, base string) ([]proto.DiffFile, error) {
	committed, err := s.gitDiffCmd(ctx, cwd, "diff", "--numstat", "--no-color", "--no-ext-diff", "--no-textconv", "-M", base, "HEAD")
	if err != nil {
		return nil, err
	}
	unstaged, err := s.gitDiffCmd(ctx, cwd, "diff", "--numstat", "--no-color", "--no-ext-diff", "--no-textconv")
	if err != nil {
		return nil, err
	}
	by := map[string]proto.DiffFile{}
	order := []string{}
	add := func(b []byte) {
		for _, line := range bytes.Split(b, []byte("\n")) {
			if len(line) == 0 {
				continue
			}
			parts := bytes.SplitN(line, []byte("\t"), 3)
			if len(parts) < 3 {
				continue
			}
			path := string(parts[2])
			f := by[path]
			if f.Path == "" {
				order = append(order, path)
				f.Path = path
			}
			if a, err := strconv.Atoi(string(parts[0])); err == nil {
				f.Added += a
			}
			if r, err := strconv.Atoi(string(parts[1])); err == nil {
				f.Removed += r
			}
			by[path] = f
		}
	}
	add(committed)
	add(unstaged)
	out := make([]proto.DiffFile, 0, len(order))
	for _, p := range order {
		out = append(out, by[p])
	}
	return out, nil
}

// gitDiffCmd runs a read-only git command with no pager, no external diff
// and no textconv; timeout diffTimeout. Stdout is returned as raw bytes.
func (s *Service) gitDiffCmd(ctx context.Context, cwd string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, diffTimeout)
	defer cancel()
	argv := append([]string{"-c", "core.pager=cat", "--no-pager", "-C", cwd}, args...)
	cmd := exec.CommandContext(ctx, "git", argv...) //nolint:gosec // argv from fixed flags and a path, never a shell string
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_PAGER=cat", "PAGER=cat")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s", args[0], msg)
	}
	return stdout.Bytes(), nil
}

func filterDiffFile(files []proto.DiffFile, want string) []proto.DiffFile {
	var out []proto.DiffFile
	for _, f := range files {
		if f.Path == want {
			out = append(out, f)
		}
	}
	return out
}

// parseUnifiedDiff turns raw git diff bytes into files, hunks and lines.
// Line text keeps every byte after the leading mark; no Clean.
func parseUnifiedDiff(raw []byte) []proto.DiffFile {
	var files []proto.DiffFile
	var cur *proto.DiffFile
	var hunk *proto.DiffHunk
	oldLine, newLine := 0, 0
	finishHunk := func() {
		if cur != nil && hunk != nil {
			cur.Hunks = append(cur.Hunks, *hunk)
			hunk = nil
		}
	}
	finishFile := func() {
		finishHunk()
		if cur != nil {
			files = append(files, *cur)
			cur = nil
		}
	}
	for _, line := range bytes.Split(raw, []byte("\n")) {
		switch {
		case bytes.HasPrefix(line, []byte("diff --git ")):
			finishFile()
			cur = &proto.DiffFile{Path: diffPath(line)}
		case cur == nil:
			continue
		case bytes.HasPrefix(line, []byte("@@ ")):
			finishHunk()
			h := proto.DiffHunk{}
			h.OldStart, h.OldCount, h.NewStart, h.NewCount = parseHunkHeader(line)
			oldLine, newLine = h.OldStart, h.NewStart
			hunk = &h
		case hunk == nil:
			continue
		case len(line) == 0:
			// A split on \n leaves an empty last piece; skip a bare empty.
			continue
		default:
			mark := line[0]
			body := string(line[1:])
			switch mark {
			case ' ':
				hunk.Lines = append(hunk.Lines, proto.DiffLine{Kind: " ", Text: body, Old: oldLine, New: newLine})
				oldLine++
				newLine++
			case '+':
				hunk.Lines = append(hunk.Lines, proto.DiffLine{Kind: "+", Text: body, New: newLine})
				cur.Added++
				newLine++
			case '-':
				hunk.Lines = append(hunk.Lines, proto.DiffLine{Kind: "-", Text: body, Old: oldLine})
				cur.Removed++
				oldLine++
			case '\\': // "\ No newline at end of file"
				continue
			}
		}
	}
	finishFile()
	return files
}

func diffPath(diffGitLine []byte) string {
	// diff --git a/path b/path
	rest := bytes.TrimPrefix(diffGitLine, []byte("diff --git "))
	parts := bytes.SplitN(rest, []byte(" "), 2)
	if len(parts) < 2 {
		return string(rest)
	}
	b := string(parts[1])
	if strings.HasPrefix(b, "b/") {
		return strings.TrimPrefix(b, "b/")
	}
	return b
}

func parseHunkHeader(line []byte) (oldStart, oldCount, newStart, newCount int) {
	// @@ -l,s +l,s @@
	s := string(line)
	i := strings.Index(s, "-")
	j := strings.Index(s, "+")
	if i < 0 || j < 0 {
		return 0, 0, 0, 0
	}
	oldPart := strings.FieldsFunc(s[i+1:j], func(r rune) bool { return r == ' ' || r == '@' })[0]
	newPart := strings.FieldsFunc(s[j+1:], func(r rune) bool { return r == ' ' || r == '@' })[0]
	oldStart, oldCount = hunkSpan(oldPart)
	newStart, newCount = hunkSpan(newPart)
	return
}

func hunkSpan(s string) (start, count int) {
	startStr, countStr, ok := strings.Cut(s, ",")
	start, _ = strconv.Atoi(startStr)
	if !ok {
		return start, 1
	}
	count, _ = strconv.Atoi(countStr)
	return start, count
}
