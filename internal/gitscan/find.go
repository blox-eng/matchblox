package gitscan

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Found is what a scan of the host found, and the consent it ran under.
type Found struct {
	Consent string        `json:"consent"`         // "" never asked, "yes", "no"
	Roots   []string      `json:"roots,omitempty"` // picked; empty: the defaults
	At      time.Time     `json:"at,omitzero"`     // the last scan's end; zero: none yet
	Took    time.Duration `json:"took_ns,omitempty"`
	Repos   []Checkout    `json:"repos,omitempty"`
	Errors  []string      `json:"errors,omitempty"`
}

// Checkout is one repository or linked worktree on the host.
type Checkout struct {
	Path   string    `json:"path"`
	Main   string    `json:"main,omitempty"`   // a linked worktree's main checkout
	Remote string    `json:"remote,omitempty"` // the configured remote's URL, else the first; no credentials
	Branch string    `json:"branch,omitempty"` // empty: detached
	Active time.Time `json:"active"`           // the last checkout or commit
}

// FindDepth is how far below a root Find looks.
const FindDepth = 4

type FindOpts struct {
	Depth   int    // 0: FindDepth
	Remote  string // "": origin
	MaxDirs int    // 0: no bound
	Budget  time.Duration
	Home    string // the one root searched below its own .git: dotfiles kept in git
}

// skipDirs hold dependencies, caches and builds, never a builder's checkout.
var skipDirs = map[string]bool{"node_modules": true, "Library": true, "vendor": true, "target": true}

// readDir and devOf are swapped in tests. hangGrace is how long past its
// Budget Find waits on a folder that does not answer.
var (
	readDir   = os.ReadDir
	devOf     = statDev
	hangGrace = 5 * time.Second
)

// Find lists the checkouts under roots without running git: it reads
// directories and the few files of each .git. It follows no symlink below
// a root and does not descend into a checkout. Past MaxDirs or Budget it
// keeps what it found and says so in errs.
func Find(ctx context.Context, roots []string, o FindOpts) (repos []Checkout, errs []string) {
	if o.Depth <= 0 {
		o.Depth = FindDepth
	}
	if o.Remote == "" {
		o.Remote = "origin"
	}
	w := &walker{o: o, left: map[string]int{}, found: map[string]Checkout{}}
	if o.Budget > 0 {
		w.deadline = time.Now().Add(o.Budget)
	}
	var dirs []string
	for _, r := range roots {
		r, err := filepath.EvalSymlinks(r)
		if err != nil {
			continue
		}
		if fi, err := os.Stat(r); err != nil || !fi.IsDir() {
			continue
		}
		dirs = append(dirs, r)
	}
	// The deepest first: ~ then reaches ~/dev already walked, and skips it.
	sort.SliceStable(dirs, func(i, j int) bool {
		return strings.Count(dirs[i], string(filepath.Separator)) > strings.Count(dirs[j], string(filepath.Separator))
	})
	home, _ := filepath.EvalSymlinks(o.Home)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, r := range dirs {
			w.dev, w.devOK = devOf(r)
			if w.walk(ctx, r, o.Depth, home != "" && r == home) {
				return
			}
		}
	}()
	// A folder on a dead mount blocks its read for good: past the Budget
	// the walk is left to end on its own, and what it found so far stands.
	var hung <-chan time.Time
	if o.Budget > 0 {
		t := time.NewTimer(o.Budget + hangGrace)
		defer t.Stop()
		hung = t.C
	}
	select {
	case <-done:
	case <-ctx.Done():
		w.halt(fmt.Sprintf("stopped after %s: some repositories may be missing", o.Budget))
	case <-hung:
		w.halt(fmt.Sprintf("stopped: a folder did not answer in %s: some repositories may be missing", o.Budget+hangGrace))
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, c := range w.found {
		repos = append(repos, c)
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].Path < repos[j].Path })
	if w.stop != "" {
		errs = append(errs, w.stop)
	}
	return repos, errs
}

type walker struct {
	o        FindOpts
	deadline time.Time
	left     map[string]int // depth left when a directory was walked
	dirs     int
	dev      uint64 // the device of the root being walked, if devOK
	devOK    bool

	mu    sync.Mutex // found and stop: Find may stop waiting on the walk
	found map[string]Checkout
	stop  string
}

func (w *walker) halt(why string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stop == "" {
		w.stop = why
	}
}

// walk reports true once a bound stops the scan. home: dir is searched
// below its own .git.
func (w *walker) walk(ctx context.Context, dir string, left int, home bool) bool {
	if l, ok := w.left[dir]; ok && l >= left {
		return false
	}
	w.left[dir] = left
	w.dirs++
	switch {
	case w.o.MaxDirs > 0 && w.dirs > w.o.MaxDirs:
		w.halt(fmt.Sprintf("stopped after %d folders: some repositories may be missing", w.o.MaxDirs))
	case ctx.Err() != nil, !w.deadline.IsZero() && time.Now().After(w.deadline):
		w.halt(fmt.Sprintf("stopped after %s: some repositories may be missing", w.o.Budget))
	}
	w.mu.Lock()
	stopped := w.stop != ""
	w.mu.Unlock()
	if stopped {
		return true
	}
	ents, err := readDir(dir)
	if err != nil {
		return false
	}
	for _, e := range ents {
		// A .git that is a link, a pipe or a device is no checkout, and
		// reading one could hang.
		if t := e.Type(); e.Name() == ".git" && (t.IsDir() || t.IsRegular()) {
			if c, ok := readCheckout(dir, w.o.Remote); ok {
				w.mu.Lock()
				w.found[dir] = c
				w.mu.Unlock()
			}
			if !home {
				return false
			}
		}
	}
	if left == 0 {
		return false
	}
	for _, e := range ents {
		name := e.Name()
		if !e.IsDir() || skipDirs[name] || strings.HasPrefix(name, ".") {
			continue // a symlink is not IsDir: never followed
		}
		p := filepath.Join(dir, name)
		if d, ok := devOf(p); w.devOK && ok && d != w.dev {
			continue // another mount: a network one may never answer
		}
		if w.walk(ctx, p, left-1, false) {
			return true
		}
	}
	return false
}

// maxGitFile bounds a read of HEAD, commondir, config or a .git file.
const maxGitFile = 1 << 20

// readSmall reads a regular file, its first maxGitFile bytes at most.
func readSmall(path string) ([]byte, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxGitFile))
}

// readCheckout reads dir's .git: a directory for a main checkout, a file
// naming the gitdir for a linked worktree.
func readCheckout(dir, remote string) (Checkout, bool) {
	gitdir := filepath.Join(dir, ".git")
	if b, err := readSmall(gitdir); err == nil {
		p, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: ")
		if !ok {
			return Checkout{}, false
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		gitdir = p
	}
	head, err := readSmall(filepath.Join(gitdir, "HEAD"))
	if err != nil {
		return Checkout{}, false
	}
	c := Checkout{Path: dir}
	if b, ok := strings.CutPrefix(strings.TrimSpace(string(head)), "ref: refs/heads/"); ok {
		c.Branch = b
	}
	common := gitdir
	if b, err := readSmall(filepath.Join(gitdir, "commondir")); err == nil {
		if p := strings.TrimSpace(string(b)); filepath.IsAbs(p) {
			common = p
		} else {
			common = filepath.Clean(filepath.Join(gitdir, p))
		}
		if filepath.Base(common) == ".git" && filepath.Dir(common) != dir {
			c.Main = filepath.Dir(common)
		}
	}
	if fi, err := os.Stat(filepath.Join(gitdir, "logs", "HEAD")); err == nil {
		c.Active = fi.ModTime()
	} else if fi, err := os.Stat(filepath.Join(gitdir, "HEAD")); err == nil {
		c.Active = fi.ModTime()
	}
	if b, err := readSmall(filepath.Join(common, "config")); err == nil {
		c.Remote = StripCredentials(parseGitConfig(b).remoteURL(remote))
	}
	return c, true
}

// StripCredentials drops the user, the password, the query and the fragment
// of a URL: a token kept there never leaves the service. An ssh URL keeps its
// user alone, which is not a secret. The userinfo is cut first, at the last @,
// because a password with a / or ? still parses, with its tail in the path.
func StripCredentials(raw string) string {
	scheme, rest, ok := strings.Cut(raw, "://")
	if !ok {
		return raw
	}
	user := ""
	if i := strings.LastIndex(rest, "@"); i >= 0 {
		switch strings.ToLower(scheme) {
		case "ssh", "git+ssh", "ssh+git":
			user, _, _ = strings.Cut(rest[:i], ":")
		}
		rest = rest[i+1:]
	}
	if u, err := url.Parse(scheme + "://" + rest); err == nil {
		u.User = nil
		if user != "" {
			u.User = url.User(user)
		}
		u.RawQuery, u.Fragment, u.RawFragment, u.ForceQuery = "", "", "", false
		return u.String()
	}
	// Unparsable: keep what is left, without the query and the fragment.
	if i := strings.IndexAny(rest, "?#"); i >= 0 {
		rest = rest[:i]
	}
	if user != "" {
		rest = user + "@" + rest
	}
	return scheme + "://" + rest
}

// ScanRoots are the folders a scan starts from: the picked ones, else the
// home and its usual code folders; then the configured repositories and
// extra folders. A leading ~ is the home, and so is the base of a relative
// folder: the service's own directory means nothing to the builder.
func ScanRoots(home string, picked, repos, extra []string) []string {
	base := picked
	if len(base) == 0 {
		base = []string{"~", "~/dev", "~/src", "~/code", "~/projects", "~/work"}
	}
	var out []string
	seen := map[string]bool{}
	for _, list := range [][]string{base, repos, extra} {
		for _, p := range list {
			switch {
			case p == "~":
				p = home
			case strings.HasPrefix(p, "~/"):
				p = filepath.Join(home, p[2:])
			case !filepath.IsAbs(p):
				p = filepath.Join(home, p)
			}
			if p = filepath.Clean(p); p != "" && !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}
