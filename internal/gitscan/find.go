package gitscan

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
}

// skipDirs hold dependencies, caches and builds, never a builder's checkout.
var skipDirs = map[string]bool{"node_modules": true, "Library": true, "vendor": true, "target": true}

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
	for _, r := range roots {
		r, err := filepath.EvalSymlinks(r)
		if err != nil {
			continue
		}
		if fi, err := os.Stat(r); err != nil || !fi.IsDir() {
			continue
		}
		if w.walk(ctx, r, o.Depth, true) {
			break
		}
	}
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
	found    map[string]Checkout
	dirs     int
	stop     string
}

// walk reports true once a bound stops the scan. A root that is a
// checkout (dotfiles kept in git at ~) is still walked.
func (w *walker) walk(ctx context.Context, dir string, left int, root bool) bool {
	if l, ok := w.left[dir]; ok && l >= left {
		return false
	}
	w.left[dir] = left
	w.dirs++
	switch {
	case w.o.MaxDirs > 0 && w.dirs > w.o.MaxDirs:
		w.stop = fmt.Sprintf("stopped after %d folders: some repositories may be missing", w.o.MaxDirs)
	case ctx.Err() != nil, !w.deadline.IsZero() && time.Now().After(w.deadline):
		w.stop = fmt.Sprintf("stopped after %s: some repositories may be missing", w.o.Budget)
	}
	if w.stop != "" {
		return true
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range ents {
		if e.Name() == ".git" {
			if c, ok := readCheckout(dir, w.o.Remote); ok {
				w.found[dir] = c
			}
			if !root {
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
		if w.walk(ctx, filepath.Join(dir, name), left-1, false) {
			return true
		}
	}
	return false
}

// readCheckout reads dir's .git: a directory for a main checkout, a file
// naming the gitdir for a linked worktree.
func readCheckout(dir, remote string) (Checkout, bool) {
	gitdir := filepath.Join(dir, ".git")
	if b, err := os.ReadFile(gitdir); err == nil {
		p, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: ")
		if !ok {
			return Checkout{}, false
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		gitdir = p
	}
	head, err := os.ReadFile(filepath.Join(gitdir, "HEAD"))
	if err != nil {
		return Checkout{}, false
	}
	c := Checkout{Path: dir}
	if b, ok := strings.CutPrefix(strings.TrimSpace(string(head)), "ref: refs/heads/"); ok {
		c.Branch = b
	}
	common := gitdir
	if b, err := os.ReadFile(filepath.Join(gitdir, "commondir")); err == nil {
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
	if b, err := os.ReadFile(filepath.Join(common, "config")); err == nil {
		c.Remote = StripCredentials(remoteURL(b, remote))
	}
	return c, true
}

// remoteURL is the url of [remote "<name>"], else of the first remote.
func remoteURL(config []byte, name string) string {
	var section, first string
	sc := bufio.NewScanner(bytes.NewReader(config))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			section = line
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(k) != "url" || !strings.HasPrefix(section, `[remote "`) {
			continue
		}
		v = strings.TrimSpace(v)
		if section == `[remote "`+name+`"]` {
			return v
		}
		if first == "" {
			first = v
		}
	}
	return first
}

// StripCredentials drops the user and password of an http(s) URL, and the
// password of any other: a token kept there never leaves the service. An
// ssh user alone is not a secret.
func StripCredentials(url string) string {
	scheme, rest, ok := strings.Cut(url, "://")
	if !ok {
		return url
	}
	host, path, slash := strings.Cut(rest, "/")
	i := strings.LastIndex(host, "@")
	if i < 0 {
		return url
	}
	user := host[:i]
	switch s := strings.ToLower(scheme); {
	case s == "http" || s == "https":
		user = ""
	default:
		user, _, _ = strings.Cut(user, ":")
	}
	host = host[i+1:]
	if user != "" {
		host = user + "@" + host
	}
	if !slash {
		return scheme + "://" + host
	}
	return scheme + "://" + host + "/" + path
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
