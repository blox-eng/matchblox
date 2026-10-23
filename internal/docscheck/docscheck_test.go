package docscheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeysReadsTablesWithTheirSectionAndKeysInProse(t *testing.T) {
	f := filepath.Join(t.TempDir(), "a.md")
	doc := "# Page\n\n`E` lists them, and `esc` goes back. `matchblox` and `✦` are no keys.\n\n" +
		"## Hosts\n\n| Key | What |\n|---|---|\n| `s` | Sorts, after `y`. |\n\n| Limit | Why |\n|---|---|\n| one | `h` goes back |\n\n```\n`z` in a block\n```\n"
	if err := os.WriteFile(f, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	refs, err := Keys([]string{f})
	if err != nil {
		t.Fatal(err)
	}
	want := []Ref{
		{File: f, Line: 3, Text: "E", Section: "Page"},
		{File: f, Line: 3, Text: "esc", Section: "Page"},
		{File: f, Line: 9, Text: "s", Section: "Hosts", Table: true},
		{File: f, Line: 9, Text: "y", Section: "Hosts"},
		{File: f, Line: 13, Text: "h", Section: "Hosts"},
	}
	if len(refs) != len(want) {
		t.Fatalf("got %+v, want %+v", refs, want)
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Errorf("ref %d: got %+v, want %+v", i, refs[i], want[i])
		}
	}
}

func TestCommandsReadsTildeFences(t *testing.T) {
	f := filepath.Join(t.TempDir(), "a.md")
	if err := os.WriteFile(f, []byte("~~~sh\nmatchblox stat_us\n~~~\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	refs, err := Commands([]string{f})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].Text != "matchblox stat_us" {
		t.Fatalf("got %+v", refs)
	}
}

func TestFilesSkipsSuperpowers(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"README.md",
		"CONTRIBUTING.md",
		"docs/phone.md",
		"docs/superpowers/plans/x.md",
		"docs/superpowers/specs/y.md",
	} {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("# x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := Files(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.Contains(f, "superpowers") {
			t.Fatalf("Files included agent plan %q", f)
		}
	}
	want := []string{
		filepath.Join(root, "README.md"),
		filepath.Join(root, "CONTRIBUTING.md"),
		filepath.Join(root, "docs", "phone.md"),
	}
	if len(files) != len(want) {
		t.Fatalf("got %v, want %v", files, want)
	}
	for i := range want {
		if files[i] != want[i] {
			t.Errorf("file %d: got %q, want %q", i, files[i], want[i])
		}
	}
}
