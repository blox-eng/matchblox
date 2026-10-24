package accounts

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// own lists the entries an account keeps for itself: sign-ins, the agent's
// state, Codex's installation_id (accounts are not linked in its telemetry)
// and SetupFile. They are never linked or copied. An entry may be a
// glob. For cursor-agent they sit under home/.cursor. The lists are checked
// against the entries of Claude Code 2.1.308, codex-cli 0.163.0 and Cursor
// 2026.10.19 (own_test.go).
var own = map[string][]string{
	"claude": {".claude.json", ".credentials.json", "projects", "sessions", "history.jsonl", "backups",
		"remote-settings.json*", "policy-limits.json*", "stats-cache.json", "state", SetupFile},
	"codex": {"auth.json", "sessions", "archived_sessions", "history.jsonl", "session_index.jsonl", "*.sqlite*",
		"app-server-control", "app-server-daemon", "packages", ".tmp", "installation_id", SetupFile},
	"cursor-agent": {"auth.json", "cli-config.json", "chats", "projects", "agent-cli-state.json",
		"statsig-cache.json", "ai-tracking", "managed", SetupFile},
}

func isOwn(agent, name string) bool {
	for _, pat := range own[agent] {
		if ok, _ := filepath.Match(pat, name); ok {
			return true
		}
	}
	return false
}

// Report is what a Prepare left shared with the default, and what it did
// not copy.
type Report struct {
	// Shared names each link of a copy that still reads the default's
	// files, and why.
	Shared []string
	// Skipped names each entry of the default a copy left out.
	Skipped []Skip
}

// Skip is an entry of the default that was not copied, and why.
type Skip struct {
	Path, Why string
}

func (r *Report) add(o Report) {
	r.Shared = append(r.Shared, o.Shared...)
	r.Skipped = append(r.Skipped, o.Skipped...)
}

// Limit bounds a copy. Bytes counts only what is copied byte by byte, not
// clones.
type Limit struct {
	Entries int
	Bytes   int64
	Time    time.Duration
}

// CopyLimit bounds the copy of a default: an entry past it is not copied,
// and is named in Report.Skipped.
var CopyLimit = Limit{Entries: 200_000, Bytes: 16 << 30, Time: 90 * time.Second}

// copyLimit, now and clone are CopyLimit, the clock and the file clone,
// set apart for the tests.
var (
	copyLimit = CopyLimit
	now       = time.Now
	clone     = cloneFile
)

// Prepare makes folder for agent (mode 0700) and fills it as mode says. It
// writes SetupFile when the folder has none. It can run again: it adds what
// the default gained since, and never replaces or removes an entry that is
// already there. The report names what a copy left shared or out.
func Prepare(home, agent, folder string, mode Mode) (Report, error) {
	agent = canon(agent)
	if _, ok := own[agent]; !ok {
		return Report{}, fmt.Errorf("%s has no accounts", agent)
	}
	switch mode {
	case LikeDefault, Copy, Empty:
	default:
		return Report{}, fmt.Errorf("unknown setup %q", mode)
	}
	if err := mkdir(folder); err != nil {
		return Report{}, err
	}
	if err := markSetup(folder, mode); err != nil {
		return Report{}, err
	}
	def := DefaultDir(home, agent)
	isOwnEntry := func(n string) bool { return isOwn(agent, n) }
	if agent != "cursor-agent" {
		return share(def, folder, mode, isOwnEntry)
	}

	ah := filepath.Join(folder, "home")
	if err := mkdir(ah); err != nil {
		return Report{}, err
	}
	// The builder's dotfiles, in every mode: the account runs on its own HOME.
	// Not the accounts root: an account never reaches the others.
	rep, err := share(home, ah, LikeDefault, func(n string) bool { return n == ".cursor" || n == ".matchblox" })
	if err != nil {
		return rep, err
	}
	cfg := filepath.Join(folder, "cfg")
	if err := mkdir(cfg); err != nil {
		return rep, err
	}
	if err := mkdir(filepath.Join(ah, ".cursor")); err != nil {
		return rep, err
	}
	if mode == Empty {
		return rep, nil
	}
	got, err := share(def, filepath.Join(ah, ".cursor"), mode, isOwnEntry)
	rep.add(got)
	if err != nil {
		return rep, err
	}
	return rep, seedConfig(filepath.Join(def, "cli-config.json"), filepath.Join(cfg, "cli-config.json"))
}

// markSetup writes SetupFile with mode, when folder has none.
func markSetup(folder string, mode Mode) error {
	f, err := os.OpenFile(filepath.Join(folder, SetupFile), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := f.WriteString(string(mode) + "\n"); err != nil {
		// A half-written marker would read as an unknown setup: remove it.
		return errors.Join(err, f.Close(), os.Remove(f.Name()))
	}
	if err := f.Close(); err != nil {
		return errors.Join(err, os.Remove(f.Name()))
	}
	return nil
}

func mkdir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700) //nolint:gosec // a directory: 0700 is the least that opens
}

// share puts the entries of src that skip does not name into dst, as links or
// as copies. An entry that already exists in dst stays as it is. It goes on
// after an entry fails, and returns every failure.
func share(src, dst string, mode Mode, skip func(string) bool) (Report, error) {
	if mode == Empty {
		return Report{}, nil
	}
	es, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return Report{}, nil
	}
	if err != nil {
		return Report{}, err
	}
	c := copier{def: src, acct: dst, own: skip, limit: copyLimit}
	c.deadline = now().Add(c.limit.Time)
	var errs []error
	for _, e := range es {
		name := e.Name()
		if skip(name) {
			continue
		}
		to := filepath.Join(dst, name)
		if _, err := os.Lstat(to); err == nil {
			continue
		}
		from := filepath.Join(src, name)
		if mode == Copy {
			err = c.copyTree(from, to)
		} else {
			err = os.Symlink(from, to)
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return c.report, errors.Join(errs...)
}

// copier copies the default def into the account's acct, within its limit.
type copier struct {
	def, acct string
	// own tells the default's entries the account keeps for itself.
	own      func(string) bool
	limit    Limit
	deadline time.Time
	entries  int
	bytes    int64
	// stop is why the copy stopped, once it is past its limit.
	stop   string
	report Report
}

// past tells whether the entry about to be copied is past the limit, and
// names it in the report when it is.
func (c *copier) past(from string) bool {
	if c.stop == "" {
		c.entries++
		switch {
		case c.entries > c.limit.Entries:
			c.stop = fmt.Sprintf("the copy stops at %d entries", c.limit.Entries)
		case now().After(c.deadline):
			c.stop = fmt.Sprintf("the copy stops after %s", c.limit.Time)
		}
	}
	if c.stop == "" {
		return false
	}
	c.report.Skipped = append(c.report.Skipped, Skip{Path: from, Why: c.stop})
	return true
}

// copyTree copies a file or folder. Regular files are cloned when the file
// system can, else copied byte by byte. A link it meets stays a link: one
// into the default points to the same place in the account.
func (c *copier) copyTree(from, to string) error {
	if c.past(from) {
		return nil
	}
	fi, err := os.Lstat(from)
	if err != nil {
		return err
	}
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		return c.copyLink(from, to)
	case fi.IsDir():
		if err := os.Mkdir(to, fi.Mode().Perm()|0o700); err != nil {
			return err
		}
		es, err := os.ReadDir(from)
		if err != nil {
			return err
		}
		var errs []error
		for _, e := range es {
			if err := c.copyTree(filepath.Join(from, e.Name()), filepath.Join(to, e.Name())); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	case fi.Mode().IsRegular():
		perm := fi.Mode().Perm() | 0o600
		err := clone(from, to, perm)
		if !refused(err) {
			return err
		}
		if c.bytes+fi.Size() > c.limit.Bytes {
			c.stop = fmt.Sprintf("the copy stops at %d bytes copied byte by byte", c.limit.Bytes)
			c.report.Skipped = append(c.report.Skipped, Skip{Path: from, Why: c.stop})
			return nil
		}
		c.bytes += fi.Size()
		return copyFile(from, to, perm)
	}
	return nil
}

// copyLink copies the link from. A target inside the default becomes a
// relative link to the same place in the account, so the account reads its
// own files and can move. A target outside the default, or on an entry the
// account keeps for itself, stays, and is named as still shared.
func (c *copier) copyLink(from, to string) error {
	target, err := os.Readlink(from)
	if err != nil {
		return err
	}
	abs := target
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(filepath.Dir(from), abs)
	}
	rel, inside := c.inDefault(abs)
	switch {
	case !inside:
		c.report.Shared = append(c.report.Shared, fmt.Sprintf("%s -> %s: outside the default", to, target))
	case rel != "." && c.own(strings.Split(rel, string(filepath.Separator))[0]):
		c.report.Shared = append(c.report.Shared, fmt.Sprintf("%s -> %s: the account keeps its own %s, so the link stays on the default's",
			to, target, strings.Split(rel, string(filepath.Separator))[0]))
	default:
		if target, err = filepath.Rel(filepath.Dir(to), filepath.Join(c.acct, rel)); err != nil {
			return err
		}
	}
	return os.Symlink(target, to)
}

// inDefault is where path lies inside the default, and whether it does.
func (c *copier) inDefault(path string) (string, bool) {
	if !SameDir(c.def, path) && !Under(c.def, path) {
		return "", false
	}
	// The place as written, else through the resolved folders.
	for _, p := range [][2]string{{c.def, path}, {resolve(c.def), path}, {resolve(c.def), resolve(path)}} {
		if rel, err := filepath.Rel(p[0], filepath.Clean(p[1])); err == nil && rel != ".." &&
			!strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return rel, true
		}
	}
	return "", false
}

// refused tells whether err is a file system's refusal to clone.
func refused(err error) bool {
	for _, r := range refusals {
		if errors.Is(err, r) {
			return true
		}
	}
	return false
}

func copyFile(from, to string, perm fs.FileMode) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		// A half-copied file would stay: the next Prepare never replaces it.
		return errors.Join(err, out.Close(), os.Remove(to))
	}
	if err := out.Close(); err != nil {
		return errors.Join(err, os.Remove(to))
	}
	return nil
}

// seeded are the keys of the default's cli-config.json an account starts
// with: the rest holds the sign-in and the account's own choices.
var seeded = []string{"statusLine", "permissions"}

// seedConfig gives the account a cli-config.json with the default's seeded
// keys and nothing else. It writes nothing when the default has none of
// them, and it never replaces a file.
func seedConfig(from, to string) error {
	if _, err := os.Lstat(to); err == nil {
		return nil
	}
	b, err := os.ReadFile(from)
	if err != nil {
		return nil
	}
	var all map[string]json.RawMessage
	if json.Unmarshal(b, &all) != nil {
		return nil
	}
	keep := map[string]json.RawMessage{}
	for _, k := range seeded {
		if v, ok := all[k]; ok {
			keep[k] = v
		}
	}
	if len(keep) == 0 {
		return nil
	}
	out, err := json.MarshalIndent(keep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(to, out, 0o600)
}
