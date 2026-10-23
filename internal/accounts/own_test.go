package accounts

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The top-level entries of each agent's default folder on 2026-10-22, with
// Claude Code 2.1.308, codex-cli 0.163.0 and Cursor 2026.10.19. When an agent
// update adds an entry, add it here and decide if it is shared or own.
var (
	claudeShared = []string{".last-cleanup", "agents", "cache", "CLAUDE.md", "file-history", "hooks",
		"paste-cache", "plugins", "session-env", "settings.json", "shell-snapshots", "skills"}
	claudeOwn = []string{".claude.json", ".credentials.json", "backups", "history.jsonl",
		"policy-limits.json", "policy-limits.json.signature-iat.json", "policy-limits.json.signature.json",
		"policy-limits.json.stamp.json", "projects", "remote-settings.json",
		"remote-settings.json.signature-iat.json", "remote-settings.json.signature.json",
		"remote-settings.json.synced-at", "sessions", "state", "stats-cache.json", SetupFile}

	codexShared = []string{".sandbox_migration", "cache", "config.toml", "mcp-oauth-locks",
		"models_cache.json", "plugins", "rules", "shell_snapshots", "skills", "thread-writer-locks", "tmp",
		"tui-thread-reference-capabilities", "version.json"}
	codexOwn = []string{".sqlite-maintenance.lock", ".tmp", "app-server-control", "app-server-daemon", "auth.json",
		"goals_1.sqlite", "history.jsonl", "installation_id", "logs_2.sqlite", "logs_2.sqlite-shm", "logs_2.sqlite-wal",
		"memories_1.sqlite", "packages", "queue_1.sqlite", "queue_1.sqlite-shm", "queue_1.sqlite-wal",
		"session_index.jsonl", "sessions", "state_5.sqlite", "state_5.sqlite-shm", "state_5.sqlite-wal",
		"thread_history_1.sqlite", "thread_history_1.sqlite-shm", "thread_history_1.sqlite-wal", SetupFile}

	cursorShared = []string{"hooks.json", "mcp.json", "plugins", "skills-cursor"}
	cursorOwn    = []string{"agent-cli-state.json", "ai-tracking", "auth.json", "chats", "cli-config.json",
		"managed", "projects", "statsig-cache.json", SetupFile}
)

func TestOwnListsMatchTheAgentsSeen(t *testing.T) {
	for _, c := range []struct {
		agent       string
		shared, own []string
	}{
		{"claude", claudeShared, claudeOwn},
		{"codex", codexShared, codexOwn},
		{"cursor-agent", cursorShared, cursorOwn},
	} {
		for _, n := range c.shared {
			if isOwn(c.agent, n) {
				t.Errorf("%s: %s is own, want shared", c.agent, n)
			}
		}
		for _, n := range c.own {
			if !isOwn(c.agent, n) {
				t.Errorf("%s: %s is shared, want own", c.agent, n)
			}
		}
	}
}

// fakeDefault writes each name as a file in dir.
func fakeDefault(t *testing.T, dir string, names ...[]string) {
	t.Helper()
	for _, n := range slices.Concat(names...) {
		write(t, filepath.Join(dir, n), "x")
	}
}

func TestOwnEntriesAreNeverLinkedOrCopied(t *testing.T) {
	for _, mode := range []Mode{LikeDefault, Copy} {
		for _, c := range []struct {
			agent, def, in string
			shared, own    []string
		}{
			{"claude", ".claude", "", claudeShared, claudeOwn},
			{"codex", ".codex", "", codexShared, codexOwn},
			{"cursor-agent", ".cursor", filepath.Join("home", ".cursor"), cursorShared, cursorOwn},
		} {
			home := t.TempDir()
			fakeDefault(t, filepath.Join(home, c.def), c.shared, c.own)
			f := Folder(home, c.agent, "work")
			if err := Prepare(home, c.agent, f, mode); err != nil {
				t.Fatal(err)
			}
			for _, n := range c.shared {
				if !exists(filepath.Join(f, c.in, n)) {
					t.Errorf("%s %s: %s was not shared", mode, c.agent, n)
				}
			}
			for _, n := range c.own {
				if n == SetupFile {
					continue // the account writes its own
				}
				if exists(filepath.Join(f, c.in, n)) {
					t.Errorf("%s %s: %s was shared", mode, c.agent, n)
				}
			}
		}
	}
}

func TestCursorHomeSkipsTheAccountsRoot(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".gitconfig"), "x")
	c := Folder(home, "cursor-agent", "work")
	if err := Prepare(home, "cursor-agent", c, LikeDefault); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(c, "home", ".matchblox")) {
		t.Error("home/.matchblox links the accounts root")
	}
	if !isLink(filepath.Join(c, "home", ".gitconfig")) {
		t.Error(".gitconfig was not mirrored")
	}
}

func TestSetupMarkerSaysTheMode(t *testing.T) {
	for _, mode := range []Mode{LikeDefault, Copy, Empty} {
		home := t.TempDir()
		f := Folder(home, "claude", "work")
		if err := Prepare(home, "claude", f, mode); err != nil {
			t.Fatal(err)
		}
		fi, err := os.Stat(filepath.Join(f, SetupFile))
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("%s: marker %v, %v", mode, fi, err)
		}
		if got := SetupOf(f); got != mode {
			t.Errorf("SetupOf = %q, want %q", got, mode)
		}
	}
}

func TestNoMarkerIsLikeDefault(t *testing.T) {
	f := t.TempDir()
	if got := SetupOf(f); got != LikeDefault {
		t.Fatalf("SetupOf = %q", got)
	}
	write(t, filepath.Join(f, SetupFile), "something else\n")
	if got := SetupOf(f); got != "" {
		t.Fatalf("an unknown marker: SetupOf = %q, want none", got)
	}
}

func TestPrepareKeepsTheFirstMarker(t *testing.T) {
	home := t.TempDir()
	f := Folder(home, "codex", "work")
	if err := Prepare(home, "codex", f, Copy); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(home, "codex", f, LikeDefault); err != nil {
		t.Fatal(err)
	}
	if got := SetupOf(f); got != Copy {
		t.Fatalf("SetupOf = %q", got)
	}
}

func TestAccountName(t *testing.T) {
	for dir, want := range map[string]string{
		"/home/u/.matchblox/accounts/claude/work":      "work",
		"/home/u/.matchblox/accounts/cursor-agent/w2/": "w2",
		"/home/u/claude-team":                          "claude-team",
	} {
		if got := Name(dir); got != want {
			t.Errorf("Name(%q) = %q, want %q", dir, got, want)
		}
	}
}
