package accounts

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNameRules(t *testing.T) {
	if err := CheckName("claude", "work", "/home/u", "linux"); err != nil {
		t.Fatalf("work: %v", err)
	}
	for _, name := range []string{"Work", "", "default", "abcdefghijk", "a/b"} {
		if err := CheckName("claude", name, "/home/u", "linux"); err == nil {
			t.Errorf("name %q was accepted", name)
		}
	}
}

func TestCodexSocketLimit(t *testing.T) {
	home := "/" + strings.Repeat("h", 59)
	err := CheckName("codex", "abcdefghij", home, "darwin")
	if err == nil || !strings.Contains(err.Error(), "socket") {
		t.Fatalf("long home: err = %v, want a socket error", err)
	}
	if err := CheckName("codex", "abcdefghij", "/home/u", "darwin"); err != nil {
		t.Fatalf("short home: %v", err)
	}
}

func TestEnvOfEachAgent(t *testing.T) {
	f := "/home/u/.matchblox/accounts/claude/work"
	cases := []struct {
		agent string
		fresh bool
		want  []string
	}{
		{"claude", true, []string{"CLAUDE_CONFIG_DIR=" + f, "BROWSER=/usr/bin/true"}},
		{"claude", false, []string{"CLAUDE_CONFIG_DIR=" + f}},
		{"codex", true, []string{"CODEX_HOME=" + f}},
		{"cursor-agent", true, []string{"AGENT_CLI_CREDENTIAL_STORE=file", "HOME=" + f + "/home", "CURSOR_CONFIG_DIR=" + f + "/cfg"}},
	}
	for _, c := range cases {
		if got := Env(c.agent, f, c.fresh); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Env(%s, fresh=%v) = %v, want %v", c.agent, c.fresh, got, c.want)
		}
	}
}

func TestArgsForcesCodexFileStore(t *testing.T) {
	want := []string{"-c", `cli_auth_credentials_store="file"`}
	if got := Args("codex"); !reflect.DeepEqual(got, want) {
		t.Errorf("Args(codex) = %v, want %v", got, want)
	}
	for _, a := range []string{"claude", "cursor-agent"} {
		if got := Args(a); got != nil {
			t.Errorf("Args(%s) = %v, want nil", a, got)
		}
	}
}

func TestFolderAndListAndDefaultDir(t *testing.T) {
	home := t.TempDir()
	if got, skipped, err := List(home, "claude"); err != nil || got != nil || skipped != nil {
		t.Fatalf("List with no root = %v, %v, %v", got, skipped, err)
	}
	for _, n := range []string{"zed", "work"} {
		if _, err := Prepare(home, "claude", Folder(home, "claude", n), Empty); err != nil {
			t.Fatal(err)
		}
	}
	got, _, err := List(home, "claude")
	if err != nil || !reflect.DeepEqual(got, []string{"work", "zed"}) {
		t.Fatalf("List = %v, %v", got, err)
	}
	if Folder(home, "cursor", "a") != Folder(home, "cursor-agent", "a") {
		t.Error("cursor is not an alias of cursor-agent")
	}
	if got := DefaultDir(home, "cursor-agent"); got != home+"/.cursor" {
		t.Errorf("DefaultDir = %s", got)
	}
}

// TestUnderAndSameDirResolveSymlinks: a symlinked root and its real path name
// one folder, also when the folder is not made yet.
func TestUnderAndSameDirResolveSymlinks(t *testing.T) {
	real, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(real, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if !Under(Root(link), Folder(real, "claude", "work")) {
		t.Error("a real path under a symlinked root is not under it")
	}
	if !SameDir(Folder(link, "claude", "work"), Folder(real, "claude", "work")) {
		t.Error("a symlinked and a real path are not the same folder")
	}
	if Under(Root(link), Root(real)) || Under(Root(real), filepath.Join(real, "other")) {
		t.Error("the root itself or a sibling is under the root")
	}
}

// TestAFolderWithABadNameIsNamed: a folder under the root that no account
// may be named is not listed, and List says which and why.
func TestAFolderWithABadNameIsNamed(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(Root(home), "claude")
	for _, n := range []string{"work", "Team", "my_acct", "abcdefghijk"} {
		if err := os.MkdirAll(filepath.Join(root, n), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	got, skipped, err := List(home, "claude")
	if err != nil || !reflect.DeepEqual(got, []string{"work"}) {
		t.Fatalf("List = %v, %v", got, err)
	}
	why := "not a-z, 0-9 and -, up to 10"
	want := []Skip{
		{Path: filepath.Join(root, "Team"), Why: why},
		{Path: filepath.Join(root, "abcdefghijk"), Why: why},
		{Path: filepath.Join(root, "my_acct"), Why: why},
	}
	if !reflect.DeepEqual(skipped, want) {
		t.Fatalf("skipped = %v, want %v", skipped, want)
	}
}

// TestUnreadableAccountsRootIsNamed: a root that cannot be read lists
// nothing, and the error names the folder and says why.
func TestUnreadableAccountsRootIsNamed(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(Root(home), "claude")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
	if _, err := os.ReadDir(root); err == nil {
		t.Skip("this user reads a folder with no permission")
	}
	got, skipped, err := List(home, "claude")
	if err == nil || got != nil {
		t.Fatalf("List = %v, %v", got, err)
	}
	if len(skipped) != 1 || skipped[0].Path != root || skipped[0].Why != err.Error() {
		t.Fatalf("skipped = %v, err %v", skipped, err)
	}
}
