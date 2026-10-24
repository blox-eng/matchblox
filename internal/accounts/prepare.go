package accounts

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
	// cut: the copy stopped at its limit.
	cut bool
}

// Skip is an entry of the default that was not copied, and why.
type Skip struct {
	Path, Why string
}

func (r *Report) add(o Report) {
	r.Shared = append(r.Shared, o.Shared...)
	r.Skipped = append(r.Skipped, o.Skipped...)
	r.cut = r.cut || o.cut
}

// Prepare makes folder for agent (mode 0700) and fills it as mode says. It
// writes SetupFile when the folder has none. It can run again: it adds what
// the default gained since, and never replaces or removes an entry that is
// already there. A copy that did not end (Incomplete) goes on where it
// stopped. The report names what a copy left shared or out.
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
	resume := mode == Copy && SetupOf(folder) == Copy && Incomplete(folder)
	def := DefaultDir(home, agent)
	isOwnEntry := func(n string) bool { return isOwn(agent, n) }
	if agent != "cursor-agent" {
		rep, err := share(def, folder, mode, isOwnEntry, resume)
		return rep, errors.Join(err, ended(folder, resume, rep, err))
	}

	ah := filepath.Join(folder, "home")
	if err := mkdir(ah); err != nil {
		return Report{}, err
	}
	// The builder's dotfiles, in every mode: the account runs on its own HOME.
	// Not the accounts root: an account never reaches the others. A mirror
	// that fails does not stop the setup of .cursor.
	rep, mirror := share(home, ah, LikeDefault, func(n string) bool { return n == ".cursor" || n == ".matchblox" }, false)
	cfg := filepath.Join(folder, "cfg")
	if err := mkdir(cfg); err != nil {
		return rep, errors.Join(mirror, err)
	}
	if err := mkdir(filepath.Join(ah, ".cursor")); err != nil {
		return rep, errors.Join(mirror, err)
	}
	if mode == Empty {
		return rep, mirror
	}
	got, err := share(def, filepath.Join(ah, ".cursor"), mode, isOwnEntry, resume)
	rep.add(got)
	return rep, errors.Join(mirror, err, ended(folder, resume, got, err),
		seedConfig(filepath.Join(def, "cli-config.json"), filepath.Join(cfg, "cli-config.json")))
}

// incomplete is the second line of SetupFile while a copy has not ended.
const incomplete = "incomplete"

// markSetup writes SetupFile with mode, when folder has none. A copy is
// marked incomplete until it ends.
func markSetup(folder string, mode Mode) error {
	f, err := os.OpenFile(filepath.Join(folder, SetupFile), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	body := string(mode) + "\n"
	if mode == Copy {
		body += incomplete + "\n"
	}
	if _, err := f.WriteString(body); err != nil {
		// A half-written marker would read as an unknown setup: remove it.
		return errors.Join(err, f.Close(), os.Remove(f.Name()))
	}
	if err := f.Close(); err != nil {
		return errors.Join(err, os.Remove(f.Name()))
	}
	return nil
}

// Incomplete tells whether the copy that sets up folder has not ended: it
// stopped at its limit, failed, or was cut short. The next Prepare in copy
// mode goes on with it.
func Incomplete(folder string) bool {
	b, err := os.ReadFile(filepath.Join(folder, SetupFile))
	if err != nil {
		return false
	}
	lines := strings.Fields(string(b))
	return len(lines) > 1 && lines[1] == incomplete
}

// ended clears the incomplete mark of a copy that copied all it could: no
// failure, and not cut by its limit.
func ended(folder string, copying bool, rep Report, err error) error {
	if !copying || err != nil || rep.cut {
		return nil
	}
	tmp := filepath.Join(folder, SetupFile+".new")
	if err := os.WriteFile(tmp, []byte(string(Copy)+"\n"), 0o600); err != nil {
		return errors.Join(err, os.Remove(tmp))
	}
	if err := os.Rename(tmp, filepath.Join(folder, SetupFile)); err != nil {
		return errors.Join(err, os.Remove(tmp))
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
// as copies. An entry that already exists in dst stays as it is; a copy that
// resumes walks into the folders that exist. It goes on after an entry
// fails, and returns every failure.
func share(src, dst string, mode Mode, skip func(string) bool, resume bool) (Report, error) {
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
	c := copier{def: src, acct: dst, own: skip, limit: copyLimit, resume: resume}
	c.deadline = now().Add(c.limit.Time)
	var errs []error
	for _, e := range es {
		name := e.Name()
		if skip(name) {
			continue
		}
		to := filepath.Join(dst, name)
		if _, err := os.Lstat(to); err == nil && !resume {
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
	c.nameDangling()
	c.report.cut = c.stop != ""
	return c.report, errors.Join(errs...)
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
