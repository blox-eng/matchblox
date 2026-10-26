package clone

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/gitremote"
)

var (
	receiving = regexp.MustCompile(`^Receiving objects:\s+(\d+)%`)
	userinfo  = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^\s/@'"]*@`)
	query     = regexp.MustCompile(`\?[^\s'"]*`)
)

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
		return fmt.Sprintf("no access to %s: %s", slug, signIn(r))
	case has("Repository not found") || notFound(stderr):
		return fmt.Sprintf("%s is not on %s, or you have no access: %s", slug, r.Host, signIn(r))
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
	// git may echo the URL it was given: its user and query stay out of the words.
	last = query.ReplaceAllString(userinfo.ReplaceAllString(last, "$1"), "")
	return fmt.Sprintf("git clone %s failed: %s", slug, last)
}

// signIn is the advice for a host that wants a sign-in: gh knows github.com only.
func signIn(r gitremote.Remote) string {
	if r.Host == "github.com" {
		return "run gh auth login"
	}
	return "sign in to " + r.Host + " for git"
}

// notFound tells if git said a repository is not there: "repository
// '<url>' not found", not any line that ends in "not found".
func notFound(stderr string) bool {
	for _, l := range strings.Split(stderr, "\n") {
		if i := strings.Index(l, "repository '"); i >= 0 && strings.Contains(l[i:], "' not found") {
			return true
		}
	}
	return false
}

// Cloning is the folder a clone into target works in until it is done.
func Cloning(target string) string {
	return filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+".cloning")
}

// OwnerFile is the file next to Cloning(target) that holds the pid of the
// process that clones into it. git needs the sibling empty or missing, so the
// proof that matchblox made it cannot be inside.
func OwnerFile(target string) string { return Cloning(target) + ".owner" }

// Owner is the pid in OwnerFile(target); ok when there is one.
func Owner(target string) (pid int, ok bool) {
	b, err := os.ReadFile(OwnerFile(target))
	if err != nil {
		return 0, false
	}
	pid, err = strconv.Atoi(strings.TrimSpace(string(b)))
	return pid, err == nil && pid > 0
}

// claim writes OwnerFile(target) with this process's pid, before git starts.
// A file whose process is gone is taken over; one whose process lives is not.
func claim(target string) error {
	f := OwnerFile(target)
	for range 2 {
		fh, err := os.OpenFile(f, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) //nolint:gosec // beside the clone, in the builder's source folder
		if err == nil {
			_, werr := fh.WriteString(strconv.Itoa(os.Getpid()))
			if cerr := fh.Close(); werr == nil {
				werr = cerr
			}
			if werr != nil {
				_ = os.Remove(f)
				return fmt.Errorf("cannot write %s: %w", f, werr)
			}
			return nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("cannot write %s: %w", f, err)
		}
		if pid, ok := Owner(target); ok && Alive(pid) {
			break
		}
		_ = os.Remove(f) // its process is gone
	}
	return fmt.Errorf("%s is there already", f)
}

// Run clones r.URL into target through the sibling Cloning(target), renamed to
// target on success. It never writes into an existing target, and on failure or
// when ctx ends it deletes the sibling only. The parent of target must exist.
// From before git starts until the clone ends, OwnerFile(target) names this
// process.
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
	if err := claim(target); err != nil {
		return err
	}
	defer func() { _ = os.Remove(OwnerFile(target)) }() // after the sibling is gone or renamed

	cmd := exec.CommandContext(ctx, "git", "-c", "core.askPass=", "clone", "--progress", "--", r.URL, sibling) //nolint:gosec // argv, never a shell string
	// git and its ssh have no terminal to ask on (ownGroup) and no askpass:
	// they fail instead of asking for a password, a passphrase or a host
	// key. GIT_TERMINAL_PROMPT=0 does not stop an askpass, and an ssh
	// before OpenSSH 8.4 ignores SSH_ASKPASS_REQUIRE: GIT_ASKPASS,
	// SSH_ASKPASS and core.askPass are empty too. The builder's own ssh
	// command and credential helpers stand. LC_ALL=C: Explain reads git's
	// English.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "SSH_ASKPASS=", "SSH_ASKPASS_REQUIRE=never", "LC_ALL=C")
	ownGroup(cmd)
	cmd.WaitDelay = waitDelay // a child that keeps stderr open does not hold a stop
	pr, pw := io.Pipe()
	cmd.Stderr = pw
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("git clone %s failed: %w", r.Slug(), err)
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
	if errors.Is(werr, exec.ErrWaitDelay) && cmd.ProcessState != nil && cmd.ProcessState.Success() {
		werr = nil // git is done; a child of it kept stderr open
	}
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
		if exists(target) {
			return fmt.Errorf("%s is there already", target)
		}
		return fmt.Errorf("cannot move the clone into %s: %w", target, err)
	}
	return nil
}

// waitDelay is how long a stopped clone waits for git's output to end.
const waitDelay = time.Second

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
