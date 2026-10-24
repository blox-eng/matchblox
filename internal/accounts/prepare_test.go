package accounts

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func isLink(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func fakeClaude(t *testing.T, home string) string {
	t.Helper()
	d := filepath.Join(home, ".claude")
	write(t, filepath.Join(d, "settings.json"), "{}")
	write(t, filepath.Join(d, "skills", "a", "SKILL.md"), "x")
	write(t, filepath.Join(d, ".claude.json"), "{}")
	write(t, filepath.Join(d, "projects", "p", "s.jsonl"), "x")
	return d
}

func TestLikeDefaultLinksAllButOwn(t *testing.T) {
	home := t.TempDir()
	def := fakeClaude(t, home)
	f := Folder(home, "claude", "work")
	if _, err := Prepare(home, "claude", f, LikeDefault); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"settings.json", "skills"} {
		got, err := os.Readlink(filepath.Join(f, n))
		if err != nil || got != filepath.Join(def, n) {
			t.Errorf("%s: link = %q, %v", n, got, err)
		}
	}
	for _, n := range []string{".claude.json", "projects"} {
		if exists(filepath.Join(f, n)) {
			t.Errorf("%s was shared", n)
		}
	}
}

func TestPrepareLeavesARealFileAlone(t *testing.T) {
	home := t.TempDir()
	fakeClaude(t, home)
	f := Folder(home, "claude", "work")
	write(t, filepath.Join(f, "settings.json"), "mine")
	if _, err := Prepare(home, "claude", f, LikeDefault); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(f, "settings.json"))
	if string(b) != "mine" || isLink(filepath.Join(f, "settings.json")) {
		t.Errorf("settings.json = %q, link %v", b, isLink(filepath.Join(f, "settings.json")))
	}
}

func TestPrepareAddsNewEntries(t *testing.T) {
	home := t.TempDir()
	def := fakeClaude(t, home)
	f := Folder(home, "claude", "work")
	if _, err := Prepare(home, "claude", f, LikeDefault); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(def, "CLAUDE.md"), "hi")
	if _, err := Prepare(home, "claude", f, LikeDefault); err != nil {
		t.Fatal(err)
	}
	if !isLink(filepath.Join(f, "CLAUDE.md")) {
		t.Error("CLAUDE.md was not linked by the second Prepare")
	}
}

func TestCopyIsIndependent(t *testing.T) {
	home := t.TempDir()
	def := fakeClaude(t, home)
	f := Folder(home, "claude", "work")
	if _, err := Prepare(home, "claude", f, Copy); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"settings.json", "skills"} {
		if isLink(filepath.Join(f, n)) {
			t.Errorf("%s is a link", n)
		}
	}
	if !exists(filepath.Join(f, "skills", "a", "SKILL.md")) {
		t.Error("skills was not copied deeply")
	}
	write(t, filepath.Join(f, "settings.json"), "changed")
	b, _ := os.ReadFile(filepath.Join(def, "settings.json"))
	if string(b) != "{}" {
		t.Errorf("default changed: %q", b)
	}
	for _, n := range []string{".claude.json", "projects"} {
		if exists(filepath.Join(f, n)) {
			t.Errorf("%s was copied", n)
		}
	}
}

func TestEmptySharesNothing(t *testing.T) {
	home := t.TempDir()
	fakeClaude(t, home)
	write(t, filepath.Join(home, ".gitconfig"), "x")
	write(t, filepath.Join(home, ".cursor", "hooks.json"), "{}")
	f := Folder(home, "claude", "work")
	if _, err := Prepare(home, "claude", f, Empty); err != nil {
		t.Fatal(err)
	}
	if es, _ := os.ReadDir(f); len(es) != 1 || es[0].Name() != SetupFile {
		t.Errorf("claude folder has %d entries, want only %s", len(es), SetupFile)
	}
	c := Folder(home, "cursor-agent", "work")
	if _, err := Prepare(home, "cursor-agent", c, Empty); err != nil {
		t.Fatal(err)
	}
	if !isLink(filepath.Join(c, "home", ".gitconfig")) {
		t.Error("home was not mirrored in empty mode")
	}
	for _, d := range []string{filepath.Join(c, "home", ".cursor"), filepath.Join(c, "cfg")} {
		if es, err := os.ReadDir(d); err != nil || len(es) != 0 {
			t.Errorf("%s: %v, %d entries", d, err, len(es))
		}
	}
}

func TestCursorHomeMirrorsHome(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".gitconfig"), "x")
	write(t, filepath.Join(home, ".ssh", "config"), "x")
	write(t, filepath.Join(home, "Library", "a"), "x")
	write(t, filepath.Join(home, ".cursor", "hooks.json"), "{}")
	write(t, filepath.Join(home, ".cursor", "auth.json"), "secret")
	c := Folder(home, "cursor-agent", "work")
	if _, err := Prepare(home, "cursor-agent", c, LikeDefault); err != nil {
		t.Fatal(err)
	}
	ah := filepath.Join(c, "home")
	for _, n := range []string{".gitconfig", ".ssh", "Library", filepath.Join(".cursor", "hooks.json")} {
		if !isLink(filepath.Join(ah, n)) {
			t.Errorf("%s is not a link", n)
		}
	}
	if isLink(filepath.Join(ah, ".cursor")) {
		t.Error("home/.cursor is a link to the default")
	}
	if exists(filepath.Join(ah, ".cursor", "auth.json")) {
		t.Error("auth.json was shared")
	}
}

func TestCursorStatusLineSeeded(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".cursor", "cli-config.json"),
		`{"statusLine":{"command":"x"},"authInfo":{"email":"you@example.com"}}`)
	c := Folder(home, "cursor-agent", "work")
	if _, err := Prepare(home, "cursor-agent", c, LikeDefault); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(c, "cfg", "cli-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["statusLine"]; !ok {
		t.Error("statusLine missing")
	}
	if _, ok := m["authInfo"]; ok {
		t.Error("authInfo was seeded")
	}
}

func TestEmptyHomeWorks(t *testing.T) {
	for _, agent := range []string{"claude", "codex", "cursor-agent"} {
		for _, mode := range []Mode{LikeDefault, Copy, Empty} {
			home := t.TempDir()
			if _, err := Prepare(home, agent, Folder(home, agent, "w"), mode); err != nil {
				t.Errorf("%s %s: %v", agent, mode, err)
			}
		}
	}
}

func TestFolderIsPrivate(t *testing.T) {
	home := t.TempDir()
	f := Folder(home, "codex", "work")
	if _, err := Prepare(home, "codex", f, LikeDefault); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(f)
	if err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("mode = %v, %v", fi.Mode().Perm(), err)
	}
}

func TestCodexOwnEntriesStayOut(t *testing.T) {
	home := t.TempDir()
	d := filepath.Join(home, ".codex")
	write(t, filepath.Join(d, "AGENTS.md"), "x")
	for _, n := range []string{"auth.json", "history.jsonl", "state_5.sqlite", "state_5.sqlite-wal", "sessions/a"} {
		write(t, filepath.Join(d, n), "x")
	}
	f := Folder(home, "codex", "work")
	if _, err := Prepare(home, "codex", f, LikeDefault); err != nil {
		t.Fatal(err)
	}
	if !isLink(filepath.Join(f, "AGENTS.md")) {
		t.Error("AGENTS.md was not linked")
	}
	for _, n := range []string{"auth.json", "history.jsonl", "state_5.sqlite", "state_5.sqlite-wal", "sessions"} {
		if exists(filepath.Join(f, n)) {
			t.Errorf("%s was shared", n)
		}
	}
}

func TestClaudeBackupsStayOwn(t *testing.T) {
	for _, mode := range []Mode{LikeDefault, Copy} {
		home := t.TempDir()
		fakeClaude(t, home)
		write(t, filepath.Join(home, ".claude", "backups", ".claude.json.backup.1"), "{}")
		f := Folder(home, "claude", "work")
		if _, err := Prepare(home, "claude", f, mode); err != nil {
			t.Fatal(err)
		}
		if exists(filepath.Join(f, "backups")) {
			t.Errorf("%s: backups was shared", mode)
		}
	}
}

func TestCursorPermissionsSeeded(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".cursor", "cli-config.json"),
		`{"permissions":{"allow":["Shell(ls)"]},"authInfo":{"email":"you@example.com"}}`)
	c := Folder(home, "cursor-agent", "work")
	if _, err := Prepare(home, "cursor-agent", c, LikeDefault); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(c, "cfg", "cli-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	var perm bytes.Buffer
	if err := json.Compact(&perm, m["permissions"]); err != nil || perm.String() != `{"allow":["Shell(ls)"]}` {
		t.Errorf("permissions = %s, %v", m["permissions"], err)
	}
	if _, ok := m["authInfo"]; ok {
		t.Error("authInfo was seeded")
	}
}
