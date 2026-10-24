// Package accounts owns the account folders: where they live, what a name may
// be, the environment each agent needs to use one, and how a folder is set up
// beside the builder's default.
package accounts

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Mode is how a new account starts its setup.
type Mode string

const (
	// LikeDefault links the default's entries, so every account shares one setup.
	LikeDefault Mode = "like-default"
	// Copy copies the default's entries once, so the account can diverge.
	Copy Mode = "copy"
	// Empty shares nothing: the agent's own first run.
	Empty Mode = "empty"
)

const (
	maxName = 10

	// socketExtra is what codex adds to the folder path for its daemon socket.
	socketExtra = 43
	// socketMaxDarwin and socketMaxOther are the longest socket paths allowed.
	socketMaxDarwin = 103
	socketMaxOther  = 107
)

var nameRE = regexp.MustCompile(`^[a-z0-9-]+$`)

// canon returns the agent's own name: cursor is an alias of cursor-agent.
func canon(agent string) string {
	if agent == "cursor" {
		return "cursor-agent"
	}
	return agent
}

// Root is the folder that holds every account.
func Root(home string) string {
	return filepath.Join(home, ".matchblox", "accounts")
}

// CheckName reports why name cannot be an account of agent on goos, or nil.
// Codex keeps a socket inside the folder, and a socket path has a length cap.
func CheckName(agent, name, home, goos string) error {
	agent = canon(agent)
	switch agent {
	case "claude", "codex", "cursor-agent":
	default:
		return fmt.Errorf("%s has no accounts", agent)
	}
	switch {
	case name == "default":
		return errors.New(`"default" is the sign-in you already have, not an account folder`)
	case name == "" || len(name) > maxName:
		return fmt.Errorf("an account name has 1 to %d characters", maxName)
	case !nameRE.MatchString(name):
		return errors.New("an account name uses a-z, 0-9 and -")
	}
	if agent == "codex" {
		limit := socketMaxOther
		if goos == "darwin" {
			limit = socketMaxDarwin
		}
		if n := len(Folder(home, agent, name)) + socketExtra; n > limit {
			return fmt.Errorf("codex puts a socket in the folder, and %d characters is over the %d a socket path may have: use a shorter name", n, limit)
		}
	}
	return nil
}

// Folder is where the account name of agent lives.
func Folder(home, agent, name string) string {
	return filepath.Join(Root(home), canon(agent), name)
}

// List returns the account names of agent, sorted. No folder means no accounts.
func List(home, agent string) ([]string, error) {
	es, err := os.ReadDir(filepath.Join(Root(home), canon(agent)))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range es {
		if e.IsDir() && nameRE.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// Env is the environment that makes agent use folder, as KEY=value pairs. A
// fresh claude account also stops a browser from opening: the sign-in link
// stays in the pane.
func Env(agent, folder string, fresh bool) []string {
	folder = strings.TrimRight(folder, "/")
	switch canon(agent) {
	case "claude":
		env := []string{"CLAUDE_CONFIG_DIR=" + folder}
		if fresh {
			env = append(env, "BROWSER=/usr/bin/true")
		}
		return env
	case "codex":
		return []string{"CODEX_HOME=" + folder}
	case "cursor-agent":
		return []string{
			"AGENT_CLI_CREDENTIAL_STORE=file",
			"HOME=" + filepath.Join(folder, "home"),
			"CURSOR_CONFIG_DIR=" + filepath.Join(folder, "cfg"),
		}
	}
	return nil
}

// SignIn is the environment that signs agent in without its folder: a token
// or an API key that a tmux server can pass on. A start on an account
// unsets it, so the account's own sign-in is used.
func SignIn(agent string) []string {
	switch canon(agent) {
	case "claude":
		return []string{"CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"}
	case "codex":
		return []string{"CODEX_API_KEY", "CODEX_ACCESS_TOKEN", "OPENAI_API_KEY"}
	case "cursor-agent":
		return []string{"CURSOR_API_KEY", "CURSOR_AUTH_TOKEN"}
	}
	return nil
}

// EnvArgv is the env command that starts agent on folder: it unsets the
// SignIn environment, then sets Env.
func EnvArgv(agent, folder string, fresh bool) []string {
	argv := []string{"env"}
	for _, name := range SignIn(agent) {
		argv = append(argv, "-u", name)
	}
	return append(argv, Env(agent, folder, fresh)...)
}

// Args is what agent takes after its binary. Codex keeps its sign-in in the
// account folder, not in the keyring.
func Args(agent string) []string {
	if canon(agent) == "codex" {
		return []string{"-c", `cli_auth_credentials_store="file"`}
	}
	return nil
}

// DefaultDir is the folder of the builder's own setup for agent.
func DefaultDir(home, agent string) string {
	switch canon(agent) {
	case "claude":
		return filepath.Join(home, ".claude")
	case "codex":
		return filepath.Join(home, ".codex")
	}
	return filepath.Join(home, ".cursor")
}

// SetupFile is the file in an account folder that says how it was set up:
// a line with the Mode, and a second line "incomplete" while a copy has not
// ended. It is an own entry.
const SetupFile = ".matchblox-setup"

// SetupOf is how the account folder was set up: the first line of
// SetupFile. A folder with no SetupFile was made before the file existed,
// and is like-default. A file that names no mode gives "".
func SetupOf(folder string) Mode {
	b, err := os.ReadFile(filepath.Join(folder, SetupFile))
	if errors.Is(err, fs.ErrNotExist) {
		return LikeDefault
	}
	if err != nil {
		return ""
	}
	first, _, _ := strings.Cut(string(b), "\n")
	switch m := Mode(strings.TrimSpace(first)); m {
	case LikeDefault, Copy, Empty:
		return m
	}
	return ""
}

// Name is the name of an account folder: the folder under
// ~/.matchblox/accounts/<agent>/ names a matchblox account, any other folder
// (a hand-made one) is named by its base.
func Name(dir string) string {
	dir = filepath.ToSlash(filepath.Clean(dir))
	if _, rest, ok := strings.Cut(dir, "/.matchblox/accounts/"); ok {
		if parts := strings.Split(rest, "/"); len(parts) >= 2 && parts[1] != "" {
			return parts[1]
		}
	}
	return filepath.Base(dir)
}

// resolve is dir with its symlinks resolved, so a symlinked home names one
// folder. A path that does not exist yet resolves its longest existing
// parent; one that resolves nowhere is only cleaned.
func resolve(dir string) string {
	dir = filepath.Clean(dir)
	var rest []string
	for p := dir; ; {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			for i := len(rest) - 1; i >= 0; i-- {
				r = filepath.Join(r, rest[i])
			}
			return r
		}
		parent := filepath.Dir(p)
		if parent == p {
			return dir
		}
		rest = append(rest, filepath.Base(p))
		p = parent
	}
}

// SameDir tells whether a and b name the same folder, their symlinks
// resolved.
func SameDir(a, b string) bool {
	return resolve(a) == resolve(b)
}

// Under tells whether dir is a folder below root, symlinks resolved on both.
func Under(root, dir string) bool {
	if root == "" || dir == "" {
		return false
	}
	rel, err := filepath.Rel(resolve(root), resolve(dir))
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
