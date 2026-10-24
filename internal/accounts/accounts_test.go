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
	if got, err := List(home, "claude"); err != nil || got != nil {
		t.Fatalf("List with no root = %v, %v", got, err)
	}
	for _, n := range []string{"zed", "work"} {
		if _, err := Prepare(home, "claude", Folder(home, "claude", n), Empty); err != nil {
			t.Fatal(err)
		}
	}
	got, err := List(home, "claude")
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
