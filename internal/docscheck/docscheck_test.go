package docscheck

import (
	"os"
	"path/filepath"
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
