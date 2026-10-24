package clone

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/gitremote"
)

var receiving = regexp.MustCompile(`^Receiving objects:\s+(\d+)%`)

// Progress reads one git progress line. Only "Receiving objects" gives a percent.
func Progress(line string) (pct int, ok bool) {
	m := receiving.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// Explain says in plain words why a clone of r failed, from git's stderr.
func Explain(stderr string, r gitremote.Remote) string {
	slug := r.Slug()
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(stderr, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("Permission denied (publickey)"):
		return fmt.Sprintf("no access to %s: add an SSH key for %s, or use its https URL", slug, r.Host)
	case has("could not read Username", "Authentication failed"):
		return fmt.Sprintf("no access to %s: run gh auth login", slug)
	case has("Repository not found", "not found"):
		return fmt.Sprintf("%s is not on %s, or you have no access: run gh auth login", slug, r.Host)
	case has("Could not resolve host", "Network is unreachable", "Connection timed out"):
		return fmt.Sprintf("no network: %s does not answer", r.Host)
	case has("No space left on device"):
		return fmt.Sprintf("the disk is full: %s is not cloned", slug)
	}
	last := ""
	for _, l := range strings.Split(stderr, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			last = l
		}
	}
	return fmt.Sprintf("git clone %s failed: %s", slug, last)
}

// Cloning is the folder a clone into target works in until it is done.
func Cloning(target string) string {
	return filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+".cloning")
}

// Run clones r.URL into target through the sibling Cloning(target), renamed to
// target on success. It never writes into an existing target, and on failure or
// when ctx ends it deletes the sibling only. The parent of target must exist.
func Run(ctx context.Context, r gitremote.Remote, target string, progress func(pct int)) error {
	sibling := Cloning(target)
	if exists(target) {
		return fmt.Errorf("%s is there already", target)
	}
	if exists(sibling) {
		return fmt.Errorf("%s is there already", sibling)
	}
	if fi, err := os.Stat(filepath.Dir(target)); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s is not a folder", filepath.Dir(target))
	}
	if ctx.Err() != nil {
		return errors.New("stopped")
	}

	cmd := exec.CommandContext(ctx, "git", "clone", "--progress", "--", r.URL, sibling) //nolint:gosec // argv, never a shell string
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_SSH_COMMAND="+sshCommand(os.Getenv("GIT_SSH_COMMAND")))
	ownGroup(cmd)
	cmd.WaitDelay = waitDelay // a child that keeps stderr open does not hold a stop
	pr, pw := io.Pipe()
	cmd.Stderr = pw
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("git clone %s failed: %v", r.Slug(), err)
	}
	var tail []string
	read := make(chan struct{})
	go func() {
		defer close(read)
		sc := bufio.NewScanner(pr)
		sc.Split(splitLines)
		for sc.Scan() {
			line := sc.Text()
			if pct, ok := Progress(line); ok && progress != nil {
				progress(pct)
			}
			if strings.TrimSpace(line) != "" {
				tail = append(tail, line)
				if len(tail) > 20 {
					tail = tail[1:]
				}
			}
		}
		if err := sc.Err(); err != nil { // git's last words are lost: say why
			tail = append(tail, "reading git's output: "+err.Error())
			_, _ = io.Copy(io.Discard, pr) // git must not block on a full pipe
		}
	}()
	werr := cmd.Wait()
	_ = pw.Close()
	<-read
	if ctx.Err() != nil {
		_ = os.RemoveAll(sibling)
		return errors.New("stopped")
	}
	if werr != nil {
		_ = os.RemoveAll(sibling)
		return errors.New(Explain(strings.Join(tail, "\n"), r))
	}
	if exists(target) {
		_ = os.RemoveAll(sibling)
		return fmt.Errorf("%s is there already", target)
	}
	if err := os.Rename(sibling, target); err != nil {
		_ = os.RemoveAll(sibling)
		return fmt.Errorf("%s is there already", target)
	}
	return nil
}

// waitDelay is how long a stopped clone waits for git's output to end.
const waitDelay = time.Second

// sshCommand is the ssh git runs: the builder's own, if set, in batch mode,
// so ssh fails instead of asking for a password, a passphrase or a host key.
// It overrides a core.sshCommand, as actions.Exec does.
func sshCommand(own string) string {
	return strings.TrimSpace(cmp.Or(strings.TrimSpace(own), "ssh") + " -o BatchMode=yes")
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

// splitLines splits on \r and \n, since git redraws progress with \r.
func splitLines(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}
