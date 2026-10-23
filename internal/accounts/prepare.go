package accounts

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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

// Prepare makes folder for agent (mode 0700) and fills it as mode says. It
// writes SetupFile when the folder has none. It can run again: it adds what
// the default gained since, and never replaces or removes an entry that is
// already there.
func Prepare(home, agent, folder string, mode Mode) error {
	agent = canon(agent)
	if _, ok := own[agent]; !ok {
		return fmt.Errorf("%s has no accounts", agent)
	}
	switch mode {
	case LikeDefault, Copy, Empty:
	default:
		return fmt.Errorf("unknown setup %q", mode)
	}
	if err := mkdir(folder); err != nil {
		return err
	}
	if err := markSetup(folder, mode); err != nil {
		return err
	}
	def := DefaultDir(home, agent)
	if agent != "cursor-agent" {
		return share(def, folder, mode, func(n string) bool { return isOwn(agent, n) })
	}

	ah := filepath.Join(folder, "home")
	if err := mkdir(ah); err != nil {
		return err
	}
	// The builder's dotfiles, in every mode: the account runs on its own HOME.
	// Not the accounts root: an account never reaches the others.
	if err := share(home, ah, LikeDefault, func(n string) bool { return n == ".cursor" || n == ".matchblox" }); err != nil {
		return err
	}
	cfg := filepath.Join(folder, "cfg")
	if err := mkdir(cfg); err != nil {
		return err
	}
	if err := mkdir(filepath.Join(ah, ".cursor")); err != nil {
		return err
	}
	if mode == Empty {
		return nil
	}
	if err := share(def, filepath.Join(ah, ".cursor"), mode, func(n string) bool { return isOwn(agent, n) }); err != nil {
		return err
	}
	return seedConfig(filepath.Join(def, "cli-config.json"), filepath.Join(cfg, "cli-config.json"))
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
// as copies. An entry that already exists in dst stays as it is.
func share(src, dst string, mode Mode, skip func(string) bool) error {
	if mode == Empty {
		return nil
	}
	es, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
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
			err = copyTree(from, to)
		} else {
			err = os.Symlink(from, to)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// copyTree copies a file or folder. A link it meets is copied as a link.
func copyTree(from, to string) error {
	fi, err := os.Lstat(from)
	if err != nil {
		return err
	}
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(from)
		if err != nil {
			return err
		}
		return os.Symlink(target, to)
	case fi.IsDir():
		if err := os.Mkdir(to, fi.Mode().Perm()|0o700); err != nil {
			return err
		}
		es, err := os.ReadDir(from)
		if err != nil {
			return err
		}
		for _, e := range es {
			if err := copyTree(filepath.Join(from, e.Name()), filepath.Join(to, e.Name())); err != nil {
				return err
			}
		}
		return nil
	case fi.Mode().IsRegular():
		return copyFile(from, to, fi.Mode().Perm())
	}
	return nil
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
