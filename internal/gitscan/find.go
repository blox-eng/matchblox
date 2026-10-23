package gitscan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Found is what a scan of the host found, and the consent it ran under.
type Found struct {
	Consent     string        `json:"consent"`         // "" never asked, "yes", "no"
	Roots       []string      `json:"roots,omitempty"` // picked; empty: the defaults
	At          time.Time     `json:"at,omitzero"`     // the last scan's end; zero: none yet
	Took        time.Duration `json:"took_ns,omitempty"`
	Repos       []Checkout    `json:"repos,omitempty"`
	Errors      []string      `json:"errors,omitempty"`
	Skipped     []Skip        `json:"skipped,omitempty"`
	SkippedMore int           `json:"skipped_more,omitempty"` // skipped past the list's cap
	Stopped     bool          `json:"stopped,omitempty"`      // a bound cut the walk: repos may be missing
	Unreadable  int           `json:"unreadable,omitempty"`   // folders that could not be read, all of them
	Unsaved     string        `json:"unsaved,omitempty"`      // why repos.json could not be written
}

// Same reports whether f and g tell a console the same thing: it ignores
// At and Took, which change on every scan. Repos come sorted by path.
func (f Found) Same(g Found) bool {
	return f.Consent == g.Consent && f.SkippedMore == g.SkippedMore && f.Stopped == g.Stopped &&
		f.Unreadable == g.Unreadable && f.Unsaved == g.Unsaved &&
		slices.Equal(f.Roots, g.Roots) && slices.Equal(f.Errors, g.Errors) &&
		slices.Equal(f.Skipped, g.Skipped) &&
		slices.EqualFunc(f.Repos, g.Repos, func(a, b Checkout) bool {
			return a.Path == b.Path && a.Main == b.Main && a.Remote == b.Remote &&
				a.Branch == b.Branch && a.Active.Equal(b.Active)
		})
}

// Summary is what a yes hears of the scan it asked for: how many it
// found, how long it took, and what it did not look at.
type Summary struct {
	Repos       int           `json:"repos"`
	Took        time.Duration `json:"took_ns,omitempty"`
	Errors      []string      `json:"errors,omitempty"`
	Skipped     []Skip        `json:"skipped,omitempty"`
	SkippedMore int           `json:"skipped_more,omitempty"`
	// Stopped: a bound cut the walk (MaxDirs, Budget, a folder that hung
	// mid-walk); its reason is Errors[0]. Other Errors are folders that
	// could not be read in a walk that ran to the end.
	Stopped bool `json:"stopped,omitempty"`
	// Unreadable is how many folders could not be read or looked up: the
	// "cannot read" and "cannot find root" lines, past the cap on Errors.
	Unreadable int `json:"unreadable,omitempty"`
	// Unsaved is why the list could not be written to disk; empty: it was.
	Unsaved string `json:"unsaved,omitempty"`
}

func (f Found) Summary() Summary {
	return Summary{Repos: len(f.Repos), Took: f.Took, Errors: f.Errors, Skipped: f.Skipped, SkippedMore: f.SkippedMore,
		Stopped: f.Stopped, Unreadable: f.Unreadable, Unsaved: f.Unsaved}
}

// Checkout is one repository or linked worktree on the host.
type Checkout struct {
	Path   string    `json:"path"`
	Main   string    `json:"main,omitempty"`   // a linked worktree's main checkout
	Remote string    `json:"remote,omitempty"` // the configured remote's URL, else the first; no credentials
	Branch string    `json:"branch,omitempty"` // empty: detached
	Active time.Time `json:"active"`           // the last checkout or commit
}

// Skip is a folder the scan did not walk, and why.
type Skip struct {
	Path string `json:"path"`
	Why  string `json:"why"`
}

// The reasons a folder is skipped.
const (
	whyNoAnswer  = "it did not answer"
	whyHung      = "it did not answer before"
	whyProtected = "macOS asks first; p picks it"
)

// Unanswered reports whether the folder was skipped because it did not
// answer, this walk or a recent one: what was found there before still
// stands.
func (k Skip) Unanswered() bool { return k.Why == whyNoAnswer || k.Why == whyHung }

func whyNetwork(fstype string) string { return "a network folder (" + fstype + ")" }

// Result is what Find found, and what it did not look at.
type Result struct {
	Repos       []Checkout
	Errors      []string // at most maxErrors lines, the last "and N more"
	Skipped     []Skip   // at most maxSkipped
	SkippedMore int
	Hung        []string // lookups that never answered, as written, and the folder read when Limit fired
	Stopped     bool     // a bound cut the walk: Errors[0] says which
	Missing     []string // Named roots, as written, that are no folder
	Unreadable  int      // folders that could not be read or looked up
	// Resolved is each root that answered its lookup, as written, to the
	// path it resolved to: a later walk where it hangs knows where it led.
	Resolved map[string]string
}

const (
	maxErrors  = 20
	maxSkipped = 500
)

// FindDepth is how far below a root Find looks.
const FindDepth = 4

type FindOpts struct {
	Depth   int           // 0: FindDepth
	Remote  string        // "": origin
	MaxDirs int           // 0: no bound
	Budget  time.Duration // 0: no bound at all
	Home    string        // the one root searched below its own .git: dotfiles kept in git
	Grace   time.Duration // how long past Budget a folder that does not answer is waited on; 0: 5 s
	// Skip is never touched: resolved paths, and roots as written whose
	// lookup hung.
	Skip map[string]bool
	// Protected folders are skipped below a root, never at one.
	Protected []string
	Mounts    func() mountTable // nil: readMounts
	// Named roots, as written, are the builder's: one that is no folder
	// is said in Errors and listed in Missing. Any other root that is not
	// there is a default most homes lack, and says nothing.
	Named map[string]bool
}

func (o FindOpts) grace() time.Duration {
	if o.Grace <= 0 {
		return 5 * time.Second
	}
	return o.Grace
}

// Limit is the longest Find runs when Budget is set.
func (o FindOpts) Limit() time.Duration { return o.Budget + o.grace() }

// DefaultProtected are the folders macOS asks about before an app reads
// them. Elsewhere none.
func DefaultProtected(home string) []string {
	if runtime.GOOS != "darwin" {
		return nil
	}
	var out []string
	for _, d := range []string{"Documents", "Desktop", "Downloads", "Library", "Pictures", "Movies", "Music"} {
		out = append(out, filepath.Join(home, d))
	}
	return out
}

// skipDirs hold dependencies, caches and builds, never a builder's checkout.
var skipDirs = map[string]bool{"node_modules": true, "Library": true, "vendor": true, "target": true}

// readDir and resolve are swapped in tests.
var (
	readDir = os.ReadDir
	resolve = filepath.EvalSymlinks
)

// Find lists the checkouts under roots without running git: it reads
// directories and the few files of each .git. It follows no symlink below
// a root and does not descend into a checkout. Past MaxDirs or Budget it
// keeps what it found and says so in Errors.
//
// With a Budget, nothing outside Limit is waited on: roots are looked up
// in the walk's goroutine, and a folder that does not answer is left
// behind in Hung for the caller to Skip next time.
func Find(ctx context.Context, roots []string, o FindOpts) Result {
	var hung <-chan time.Time
	if o.Budget > 0 {
		t := time.NewTimer(o.Limit())
		defer t.Stop()
		hung = t.C
	}
	if o.Depth <= 0 {
		o.Depth = FindDepth
	}
	if o.Remote == "" {
		o.Remote = "origin"
	}
	if o.Mounts == nil {
		o.Mounts = readMounts
	}
	w := &walker{
		o: o, readDir: readDir, resolve: resolve,
		left: map[string]int{}, found: map[string]Checkout{}, pending: map[string]bool{},
		noted: map[string]bool{}, skipSeen: map[string]bool{},
	}
	if o.Budget > 0 {
		w.deadline = time.Now().Add(o.Budget)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.run(ctx, roots)
	}()
	// A folder on a dead mount blocks its read for good: past Limit the
	// walk is left to end on its own, and what it found so far stands.
	timedOut := false
	select {
	case <-done:
	case <-ctx.Done():
		w.halt(fmt.Sprintf("stopped after %s: some repositories may be missing", o.Budget))
	case <-hung:
		timedOut = true
		w.halt(fmt.Sprintf("stopped: a folder did not answer in %s: some repositories may be missing", o.Limit()))
	}
	return w.result(ctx.Err() == nil, timedOut)
}

type walker struct {
	o        FindOpts
	readDir  func(string) ([]os.DirEntry, error)
	resolve  func(string) (string, error)
	deadline time.Time
	left     map[string]int // depth left when a directory was walked
	dirs     int
	mounts   mountTable
	guarded  map[string]bool // Protected, resolved

	mu       sync.Mutex // all below: Find may stop waiting on the walk
	asked    []string   // looked up, as written, in order
	roots    []string   // as written
	pending  map[string]bool
	at       string // the directory being read
	found    map[string]Checkout
	stop     string
	errs     []string
	errMore  int
	noted    map[string]bool
	skipped  []Skip
	skipMore int
	skipSeen map[string]bool
	resolved map[string]string // Result.Resolved
	missing  []string          // Result.Missing
	unread   int               // Result.Unreadable
}

// lookup is a path as written and what resolving it gave.
type lookup struct {
	written, path string
	err           error
	skip          bool // root: its resolved path is in Skip
	dir           bool // root: a directory
}

// lookUp resolves p and, for a root not in skip, tells if it is a folder.
func lookUp(resolve func(string) (string, error), p string, root bool, skip map[string]bool) lookup {
	r, err := resolve(p)
	if err == nil {
		r = trueCase(r)
	}
	l := lookup{written: p, path: r, err: err}
	if err == nil && root {
		if l.skip = skip[r]; !l.skip {
			var fi os.FileInfo
			if fi, l.err = os.Stat(r); l.err == nil {
				l.dir = fi.IsDir()
			}
		}
	}
	return l
}

// noFolder: the root's lookup answered, and there is no folder there.
func (l lookup) noFolder() bool {
	if l.err != nil {
		return errors.Is(l.err, fs.ErrNotExist) || errors.Is(l.err, syscall.ENOTDIR)
	}
	return !l.skip && !l.dir
}

// run looks up the roots, the home and the protected folders, each in its
// own goroutine, then walks the roots that answered within the grace.
func (w *walker) run(ctx context.Context, roots []string) {
	isRoot := map[string]bool{}
	for _, r := range roots {
		isRoot[r] = true
	}
	var asks []string
	seen := map[string]bool{}
	for _, list := range [][]string{roots, {w.o.Home}, w.o.Protected} {
		for _, p := range list {
			if p != "" && !seen[p] {
				seen[p] = true
				asks = append(asks, p)
			}
		}
	}
	got := map[string]lookup{}
	ch := make(chan lookup, len(asks))
	w.mu.Lock()
	w.asked, w.roots = asks, roots
	for _, p := range asks {
		if w.o.Skip[p] {
			got[p] = lookup{written: p, path: filepath.Clean(p), skip: true}
			continue
		}
		w.pending[p] = true
		go func(p string, root bool) { ch <- lookUp(w.resolve, p, root, w.o.Skip) }(p, isRoot[p])
	}
	n := len(w.pending)
	w.mu.Unlock()
	w.mounts = w.o.Mounts()
	var cut <-chan time.Time
	if w.o.Budget > 0 {
		t := time.NewTimer(w.o.grace())
		defer t.Stop()
		cut = t.C
	}
wait:
	for ; n > 0; n-- {
		select {
		case l := <-ch:
			got[l.written] = l
			w.mu.Lock()
			delete(w.pending, l.written)
			w.mu.Unlock()
		case <-cut:
			break wait // what has not answered stays pending: Hung
		case <-ctx.Done():
			break wait
		}
	}

	resolved := func(p string) string {
		if l, ok := got[p]; ok && l.err == nil {
			return l.path
		}
		return filepath.Clean(p)
	}
	w.guarded = map[string]bool{}
	for _, p := range w.o.Protected {
		w.guarded[resolved(p)] = true
	}
	home := ""
	if l, ok := got[w.o.Home]; ok && l.err == nil {
		home = l.path
	}
	var dirs []string
	answered := map[string]string{}
	for _, r := range roots {
		l, ok := got[r]
		if ok && l.err == nil && !w.o.Skip[r] {
			answered[r] = l.path
		}
		switch {
		case !ok:
		case l.skip: // as written when its lookup hung, else resolved
			w.skip(l.path, whyHung)
		case l.noFolder():
			if w.o.Named[r] {
				w.note("no folder " + r)
				w.mu.Lock()
				w.missing = append(w.missing, r)
				w.mu.Unlock()
			}
		case l.err != nil:
			w.unreadable(fmt.Sprintf("cannot find root %s: %v", r, cause(l.err)))
		default:
			dirs = append(dirs, l.path)
		}
	}
	w.mu.Lock()
	w.resolved = answered
	w.mu.Unlock()
	// The deepest first: ~ then reaches ~/dev already walked, and skips it.
	sort.SliceStable(dirs, func(i, j int) bool {
		return strings.Count(dirs[i], string(filepath.Separator)) > strings.Count(dirs[j], string(filepath.Separator))
	})
	for _, r := range dirs {
		if w.walk(ctx, r, w.o.Depth, home != "" && r == home) {
			return
		}
	}
}

// trueCase is p, an absolute path with no links, in the letter case its
// folders have on disk: on a case-insensitive volume ~/code resolves, but
// a walk of ~ reaches it as ~/Code. Where names never fold case it is p.
// Each name takes the entry of its parent that equals it, else the one
// that equals it in another case and is the same file; a name that is
// not there, has no letters, or whose parent cannot be read is kept as
// written.
// caseInsensitive: the host's volumes fold case by default, so a name
// may resolve in another case than it has on disk.
const caseInsensitive = runtime.GOOS == "darwin" || runtime.GOOS == "windows"

// FoldsCase reports whether the host's folders answer to a name in
// another letter case.
func FoldsCase() bool { return caseInsensitive }

func trueCase(p string) string { return trueCaseIn(p, caseInsensitive, os.ReadDir) }

// trueCaseIn is trueCase where names fold case if fold, reading folders
// with readDir.
func trueCaseIn(p string, fold bool, readDir func(string) ([]os.DirEntry, error)) string {
	if !fold {
		return p
	}
	sep := string(filepath.Separator)
	vol := filepath.VolumeName(p)
	if !strings.HasPrefix(p[len(vol):], sep) {
		return p
	}
	out := vol + sep
	for _, name := range strings.Split(p[len(vol):], sep) {
		if name != "" {
			out = filepath.Join(out, nameOnDisk(out, name, readDir))
		}
	}
	return out
}

// nameOnDisk: an Lstat of the name as written comes first, so a name
// that is not there reads no folder. The Lstat cannot tell the case
// (a folding volume answers to any), so a name with letters still
// reads its parent.
func nameOnDisk(dir, name string, readDir func(string) ([]os.DirEntry, error)) string {
	fi, err := os.Lstat(filepath.Join(dir, name))
	if err != nil || strings.ToLower(name) == strings.ToUpper(name) {
		return name
	}
	ents, err := readDir(dir)
	if err != nil {
		return name
	}
	other := ""
	for _, e := range ents {
		switch n := e.Name(); {
		case n == name:
			return name
		case other == "" && strings.EqualFold(n, name):
			other = n
		}
	}
	if other == "" {
		return name
	}
	if ofi, err := os.Lstat(filepath.Join(dir, other)); err != nil || !os.SameFile(fi, ofi) {
		return name
	}
	return other
}

// cause drops the path a PathError repeats.
func cause(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

func (w *walker) halt(why string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stop == "" {
		w.stop = why
	}
}

// note keeps an error line once, the first maxErrors of them.
func (w *walker) note(msg string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.noteLocked(msg)
}

// unreadable notes a folder that could not be read, and counts it.
func (w *walker) unreadable(msg string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.noteLocked(msg) {
		w.unread++
	}
}

// noteLocked reports whether msg is new.
func (w *walker) noteLocked(msg string) bool {
	if w.noted[msg] {
		return false
	}
	w.noted[msg] = true
	if len(w.errs) < maxErrors {
		w.errs = append(w.errs, msg)
	} else {
		w.errMore++
	}
	return true
}

func (w *walker) skip(path, why string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.skipLocked(path, why)
}

func (w *walker) skipLocked(path, why string) {
	if w.skipSeen[path] {
		return
	}
	w.skipSeen[path] = true
	if len(w.skipped) < maxSkipped {
		w.skipped = append(w.skipped, Skip{Path: path, Why: why})
	} else {
		w.skipMore++
	}
}

func (w *walker) setAt(dir string) {
	w.mu.Lock()
	w.at = dir
	w.mu.Unlock()
}

// result is what the walk has so far. hung: report what did not answer,
// in Hung for the caller and in Skipped for the builder. stuck: Limit
// fired, so the folder being read is one. A home or protected lookup
// that is no root is no folder the scan was asked to walk: it is never
// in Skipped. It is in Hung all the same, wherever it is: a lookup asked
// again blocks one more goroutine on it.
func (w *walker) result(hung, stuck bool) Result {
	w.mu.Lock()
	defer w.mu.Unlock()
	var res Result
	if hung {
		for _, p := range w.asked {
			switch {
			case !w.pending[p]:
			case slices.Contains(w.roots, p):
				res.Hung = append(res.Hung, p)
				w.skipLocked(filepath.Clean(p), whyNoAnswer)
			default:
				res.Hung = append(res.Hung, p)
			}
		}
		if stuck && w.at != "" {
			res.Hung = append(res.Hung, w.at)
			w.skipLocked(w.at, whyNoAnswer)
		}
	}
	for _, c := range w.found {
		res.Repos = append(res.Repos, c)
	}
	sort.Slice(res.Repos, func(i, j int) bool { return res.Repos[i].Path < res.Repos[j].Path })
	if w.stop != "" {
		res.Stopped = true
		res.Errors = append(res.Errors, w.stop)
	}
	res.Missing, res.Unreadable = slices.Clone(w.missing), w.unread
	res.Errors = append(res.Errors, w.errs...)
	if total := len(res.Errors) + w.errMore; total > maxErrors {
		res.Errors = append(res.Errors[:maxErrors-1], fmt.Sprintf("and %d more", total-(maxErrors-1)))
	}
	res.Skipped = append(res.Skipped, w.skipped...)
	res.SkippedMore = w.skipMore
	if len(w.resolved) > 0 {
		res.Resolved = maps.Clone(w.resolved)
	}
	return res
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
	w.at = dir
	w.mu.Unlock()
	if stopped {
		return true
	}
	ents, err := w.readDir(dir)
	if err != nil {
		w.setAt("")
		if !errors.Is(err, fs.ErrNotExist) {
			w.unreadable(fmt.Sprintf("cannot read %s: %v", dir, cause(err)))
		}
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
				w.setAt("")
				return false
			}
		}
	}
	w.setAt("")
	if left == 0 {
		return false
	}
	for _, e := range ents {
		name := e.Name()
		if !e.IsDir() || skipDirs[name] || strings.HasPrefix(name, ".") {
			continue // a symlink is not IsDir: never followed
		}
		// Lookups only, no syscall: a folder that may not answer is
		// never touched.
		p := filepath.Join(dir, name)
		if w.o.Skip[p] {
			w.skip(p, whyHung)
			continue
		}
		if w.guarded[p] {
			w.skip(p, whyProtected)
			continue
		}
		if fstype, ok := w.mounts.network(p); ok {
			w.skip(p, whyNetwork(fstype))
			continue
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
// user alone, which is not a secret. The userinfo ends at the last @; a /, ?
// or # before it could be a password's or the path's, and a URL that cannot
// be told apart, or whose host is not a plain host, shows its scheme only.
// Text before the first :// that is not a scheme makes the URL one without
// a scheme: see stripNoScheme.
func StripCredentials(raw string) string {
	scheme, rest, ok := strings.Cut(raw, "://")
	if !ok || !isScheme(scheme) {
		return stripNoScheme(raw)
	}
	only := scheme + "://"
	user := ""
	if i := strings.LastIndex(rest, "@"); i >= 0 {
		if strings.ContainsAny(rest[:i], "/?#") {
			return only
		}
		switch strings.ToLower(scheme) {
		case "ssh", "git+ssh", "ssh+git":
			user, _, _ = strings.Cut(rest[:i], ":")
		}
		rest = rest[i+1:]
	}
	u, err := url.Parse(only + rest)
	if err != nil || u.Host == "" && rest != "" && !strings.HasPrefix(rest, "/") ||
		strings.Trim(u.Host, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.-_[]:%") != "" {
		return only
	}
	u.User = nil
	if user != "" {
		u.User = url.User(user)
	}
	u.RawQuery, u.Fragment, u.RawFragment, u.ForceQuery = "", "", "", false
	return u.String()
}

// isScheme is RFC 3986's scheme: a letter, then letters, digits, + . or -.
func isScheme(s string) bool {
	if s == "" || !('a' <= s[0]|0x20 && s[0]|0x20 <= 'z') {
		return false
	}
	return strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+.-") == ""
}

// stripNoScheme is StripCredentials for scp-style and malformed remotes.
// A ':' before the last @ is a password's, so all through that @ goes; a
// user alone (git@host:path) is not a secret and stays. Then the query
// and the fragment go.
func stripNoScheme(raw string) string {
	if i := strings.LastIndex(raw, "@"); i >= 0 && strings.Contains(raw[:i], ":") {
		raw = raw[i+1:]
	}
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	return raw
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

// Missing is the roots, as given, that are no folder: looked up as Find
// looks up a root, each in its own goroutine, waited on for the grace.
// One that does not answer in time is not missing: the walk says so.
func (o FindOpts) Missing(roots []string) []string {
	ch := make(chan lookup, len(roots))
	rs := resolve // read here: a goroutine may outlive a test that swaps it back
	asked := map[string]bool{}
	for _, p := range roots {
		if !asked[p] {
			asked[p] = true
			go func() { ch <- lookUp(rs, p, true, nil) }()
		}
	}
	t := time.NewTimer(o.grace())
	defer t.Stop()
	gone := map[string]bool{}
wait:
	for range asked {
		select {
		case l := <-ch:
			gone[l.written] = l.noFolder()
		case <-t.C:
			break wait
		}
	}
	var out []string
	for _, p := range roots {
		if gone[p] {
			out = append(out, p)
			gone[p] = false
		}
	}
	return out
}
