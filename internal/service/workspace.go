package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/said"
)

// gitRunner runs a read-only Git command in dir and answers with its output.
type gitRunner func(ctx context.Context, dir string, args ...string) (string, error)

// workGit answers with Git's stdout only, so a warning never reads as a path.
// It drops the caller's GIT_* variables: GIT_DIR and its kin override -C.
func workGit(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-pager", "--no-optional-locks", "-c", "core.fsmonitor=false", "-C", dir}, args...)...) //nolint:gosec // fixed read-only git commands, separate argv
	cmd.Env = slices.DeleteFunc(os.Environ(), func(kv string) bool { return strings.HasPrefix(kv, "GIT_") })
	var out, stderr matchOutput
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, said.Clean(stderr.String()))
	}
	return out.String(), nil
}

// workspaceLine finds the first line that starts with "Workspace: " at
// column 0. An indented or quoted line is not one.
func workspaceLine(record string) (string, bool) {
	for l := range strings.Lines(record) {
		if rest, ok := strings.CutPrefix(l, "Workspace: "); ok {
			return strings.TrimSpace(rest), true
		}
	}
	return "", false
}

// workFolder picks where a switch starts the next agent. A record with no
// Workspace line keeps the launch folder. A line counts only when it names
// a worktree of the launch folder's repository; work is then its top level.
// Any other line is refused, and the match stays in the launch folder.
// A nil git runs the real Git.
func workFolder(ctx context.Context, git gitRunner, record, launch string) (work, refused string) {
	path, ok := workspaceLine(record)
	if !ok {
		return launch, ""
	}
	if git == nil {
		git = workGit
	}
	if top := worktreeOf(ctx, git, path, launch); top != "" {
		return top, ""
	}
	return launch, path + " is not a worktree of this repository: it starts in " + launch
}

// worktreeOf answers with the top level of path when path is a worktree of
// launch's repository, else "". Outside Git only launch itself passes.
func worktreeOf(ctx context.Context, git gitRunner, path, launch string) string {
	if !filepath.IsAbs(path) {
		return ""
	}
	dir, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return ""
	}
	home, err := filepath.EvalSymlinks(launch)
	if err != nil {
		return ""
	}
	common, err := gitPath(ctx, git, home, "--git-common-dir")
	if err != nil {
		if dir == home {
			return launch
		}
		return ""
	}
	if common == "" {
		return ""
	}
	if theirs, err := gitPath(ctx, git, dir, "--git-common-dir"); err != nil || theirs != common {
		return ""
	}
	// Inside .git there is no top level, so Git fails and the path is refused.
	top, err := gitPath(ctx, git, dir, "--show-toplevel")
	if err != nil || top == "" {
		return ""
	}
	// A shared common dir only proves the folder points at the repository.
	// The repository's own worktree list says which folders are its worktrees.
	list, err := git(ctx, home, "worktree", "list", "--porcelain")
	if err != nil {
		return ""
	}
	for l := range strings.Lines(list) {
		p, ok := strings.CutPrefix(strings.TrimRight(l, "\n"), "worktree ")
		if !ok {
			continue
		}
		if p, err := filepath.EvalSymlinks(p); err == nil && p == top {
			return top
		}
	}
	return ""
}

// gitPath asks git rev-parse for an absolute path, with its links resolved.
// Empty output answers "".
func gitPath(ctx context.Context, git gitRunner, dir, flag string) (string, error) {
	out, err := git(ctx, dir, "rev-parse", "--path-format=absolute", flag)
	if err != nil {
		return "", err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return "", nil
	}
	return filepath.EvalSymlinks(out)
}
