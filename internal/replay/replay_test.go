package replay

import (
	"encoding/json"
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

var dark = map[string]color.Color{
	"fg":     lipgloss.Color("#EBE6DC"),
	"accent": lipgloss.Color("#C3A56A"),
	"bg":     lipgloss.Color("#0F0D0A"),
}

var light = map[string]color.Color{
	"fg":     lipgloss.Color("#1C1813"),
	"accent": lipgloss.Color("#9A7B3F"),
	"bg":     lipgloss.Color("#F5F1E7"),
}

func TestParseTruecolorSpans(t *testing.T) {
	lines, err := Parse("\x1b[38;2;195;165;106mx\x1b[0m y", dark)
	if err != nil {
		t.Fatal(err)
	}
	want := []Span{{Text: "x", FG: "accent"}, {Text: " y"}}
	if len(lines) != 1 || len(lines[0]) != 2 || lines[0][0] != want[0] || lines[0][1] != want[1] {
		t.Fatalf("got %+v, want %+v", lines, want)
	}
}

func TestParseBoldUnderlineAndBackground(t *testing.T) {
	lines, err := Parse("\x1b[1;4;38;2;235;230;220;48;2;15;13;10mab\x1b[m\nc", dark)
	if err != nil {
		t.Fatal(err)
	}
	if got := lines[0][0]; got != (Span{Text: "ab", FG: "fg", BG: "bg", Bold: true, Under: true}) {
		t.Fatalf("got %+v", got)
	}
	if len(lines) != 2 || lines[1][0].Text != "c" {
		t.Fatalf("second line: %+v", lines)
	}
}

func TestParseRejectsUnknownColour(t *testing.T) {
	if _, err := Parse("\x1b[38;2;1;2;3mx", dark); err == nil || !strings.Contains(err.Error(), "#010203") {
		t.Fatalf("err %v, want the unknown colour named", err)
	}
}

func TestJSONSkipsUnchangedLines(t *testing.T) {
	a := [][]Span{{{Text: "same"}}, {{Text: "one"}}}
	b := [][]Span{{{Text: "same"}}, {{Text: "two", FG: "accent"}}}
	out, err := JSON(10, 2, []Frame{{At: 0, Lines: a}, {At: 1500e6, Lines: b}})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Cols, Rows int
		Frames     []struct {
			At    int               `json:"at"`
			Lines []json.RawMessage `json:"lines"`
		}
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Cols != 10 || doc.Rows != 2 || len(doc.Frames) != 2 || doc.Frames[1].At != 1500 {
		t.Fatalf("doc %+v", doc)
	}
	if string(doc.Frames[1].Lines[0]) != "null" || string(doc.Frames[1].Lines[1]) != `[["two","accent","",0]]` {
		t.Fatalf("second frame lines %s %s", doc.Frames[1].Lines[0], doc.Frames[1].Lines[1])
	}
}

func TestSVGHasNoScriptAndFollowsTheme(t *testing.T) {
	f := []Frame{{At: 0, Lines: [][]Span{{{Text: "hi", FG: "accent"}, {Text: "▀", FG: "fg", BG: "bg"}}}}}
	out := string(SVG(4, 1, f, map[bool]map[string]color.Color{true: dark, false: light}))
	if strings.Contains(out, "<script") {
		t.Fatal("the README SVG must not carry a script")
	}
	for _, want := range []string{"prefers-color-scheme: light", "#C3A56A", "#9A7B3F", "hi"} {
		if !strings.Contains(out, want) {
			t.Fatalf("svg lacks %q", want)
		}
	}
}

func TestStillEscapesAndUsesTokenClasses(t *testing.T) {
	out := Still(Frame{Lines: [][]Span{{{Text: "<b>", FG: "accent", Bold: true}}}})
	if !strings.Contains(out, `<span class="fg-accent b">&lt;b&gt;</span>`) {
		t.Fatalf("still %s", out)
	}
	if strings.Contains(out, "style=") {
		t.Fatal("the CSP forbids inline style")
	}
}
