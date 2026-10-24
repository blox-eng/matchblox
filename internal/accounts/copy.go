package accounts

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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

// copyLimit, now, clone and chunk are CopyLimit, the clock, the file clone
// and the size of a byte copy's step between two looks at the clock, set
// apart for the tests.
var (
	copyLimit       = CopyLimit
	now             = time.Now
	clone           = cloneFile
	chunk     int64 = 8 << 20
)

// partSuffix names a file while it is copied: a copy cut short leaves no
// half file under the real name, and the next one removes the part.
const partSuffix = ".matchblox-part"

// copier copies the default def into the account's acct, within its limit.
type copier struct {
	def, acct string
	// own tells the default's entries the account keeps for itself.
	own      func(string) bool
	limit    Limit
	deadline time.Time
	// resume: the account holds part of the copy already.
	resume  bool
	entries int
	bytes   int64
	// stop is why the copy stopped, once it is past its limit.
	stop string
	// moved are the links that point to a place inside the account.
	moved  [][2]string
	report Report
}

func (c *copier) skip(from, why string) {
	c.report.Skipped = append(c.report.Skipped, Skip{Path: from, Why: why})
}

// late stops the copy when it is past its time, and tells whether it is.
func (c *copier) late() bool {
	if c.stop == "" && now().After(c.deadline) {
		c.stop = fmt.Sprintf("the copy stops after %s", c.limit.Time)
	}
	return c.stop != ""
}

// past tells whether the entry about to be copied is past the limit, and
// names it in the report when it is.
func (c *copier) past(from string) bool {
	if c.stop == "" {
		c.entries++
		if c.entries > c.limit.Entries {
			c.stop = fmt.Sprintf("the copy stops at %d entries", c.limit.Entries)
		}
	}
	if !c.late() {
		return false
	}
	c.skip(from, c.stop)
	return true
}

// copyTree copies a file or folder. Regular files are cloned when the file
// system can, else copied byte by byte. A link it meets stays a link: one
// into the default points to the same place in the account. When the copy
// resumes, an entry that is there already stays, and a folder that is
// there is walked into.
func (c *copier) copyTree(from, to string) error {
	if c.resume {
		if have, err := os.Lstat(to); err == nil {
			if fi, err := os.Lstat(from); err == nil && fi.IsDir() && have.IsDir() {
				return c.copyDir(from, to)
			}
			return nil
		}
	}
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
		return c.copyDir(from, to)
	case fi.Mode().IsRegular():
		return c.copyFile(from, to, fi)
	}
	c.skip(from, "not a file, folder or link")
	return nil
}

// copyDir copies the entries of the folder from into to. It goes on after
// an entry fails, and returns every failure.
func (c *copier) copyDir(from, to string) error {
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
}

// copyFile clones the regular file from, or copies it byte by byte when
// the file system refuses. It writes a part file first and renames it.
func (c *copier) copyFile(from, to string, fi fs.FileInfo) error {
	perm := fi.Mode().Perm() | 0o600
	part := to + partSuffix
	if err := os.Remove(part); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	err := clone(from, part, perm)
	if refused(err) {
		if c.bytes+fi.Size() > c.limit.Bytes {
			c.stop = fmt.Sprintf("the copy stops at %d bytes copied byte by byte", c.limit.Bytes)
			c.skip(from, c.stop)
			return nil
		}
		var done bool
		if done, err = c.copyBytes(from, part, perm); !done {
			return err
		}
	}
	if err != nil {
		return err
	}
	if err := os.Rename(part, to); err != nil {
		return errors.Join(err, os.Remove(part))
	}
	return nil
}

// copyBytes copies from to the new file to, a chunk at a time, and looks
// at the clock after each. Past its time it removes the half file, names
// from as skipped and is not done.
func (c *copier) copyBytes(from, to string, perm fs.FileMode) (bool, error) {
	in, err := os.Open(from)
	if err != nil {
		return false, err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return false, err
	}
	for {
		n, err := io.CopyN(out, in, chunk)
		c.bytes += n
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return false, errors.Join(err, out.Close(), os.Remove(to))
		}
		if c.late() {
			c.skip(from, c.stop)
			return false, errors.Join(out.Close(), os.Remove(to))
		}
	}
	if err := out.Chmod(perm); err != nil {
		return false, errors.Join(err, out.Close(), os.Remove(to))
	}
	if err := out.Close(); err != nil {
		return false, errors.Join(err, os.Remove(to))
	}
	return true, nil
}

// copyLink copies the link from. A target inside the default becomes a
// relative link to the same place in the account, so the account reads its
// own files and can move. A target outside the default, or on an entry the
// account keeps for itself, stays on the default's file, written absolute,
// and is named as still shared.
func (c *copier) copyLink(from, to string) error {
	target, err := os.Readlink(from)
	if err != nil {
		return err
	}
	abs := target
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(filepath.Dir(from), abs)
	}
	abs = filepath.Clean(abs)
	rel, inside := c.inDefault(abs)
	top, _, _ := strings.Cut(rel, string(filepath.Separator))
	switch {
	case !inside:
		target = abs
		c.report.Shared = append(c.report.Shared, fmt.Sprintf("%s -> %s: outside the default", to, target))
	case rel != "." && c.own(top):
		target = abs
		c.report.Shared = append(c.report.Shared, fmt.Sprintf("%s -> %s: the account keeps its own %s, so the link stays on the default's",
			to, target, top))
	default:
		if target, err = filepath.Rel(filepath.Dir(to), filepath.Join(c.acct, rel)); err != nil {
			return err
		}
		c.moved = append(c.moved, [2]string{from, to})
	}
	return os.Symlink(target, to)
}

// nameDangling names each link into the account whose target the copy
// left out when it stopped.
func (c *copier) nameDangling() {
	if c.stop == "" {
		return
	}
	for _, l := range c.moved {
		if _, err := os.Stat(l[1]); errors.Is(err, fs.ErrNotExist) {
			c.skip(l[0], "a link to what the copy left out")
		}
	}
}

// inDefault is where path lies inside the default, and whether it does.
func (c *copier) inDefault(path string) (string, bool) {
	if !SameDir(c.def, path) && !Under(c.def, path) {
		return "", false
	}
	// The place as written, else through the resolved folders.
	for _, p := range [][2]string{{c.def, path}, {resolve(c.def), path}, {resolve(c.def), resolve(path)}} {
		if rel, err := filepath.Rel(p[0], p[1]); err == nil && rel != ".." &&
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
