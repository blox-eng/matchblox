package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/app"
	"github.com/blox-eng/matchblox/internal/replay"
)

// The stoker's screens are the site's (www/stoker.js): the phone's 44
// columns, 34 rows.
const stokerCols, stokerRows = 44, 34

// TestStokerSVG draws the README's animation of the stoker taking orders
// from www/stoker.js: Node prints the six steps on the phone, and the
// replay's SVG writer draws them in the console's tokens. With -update it
// writes www/demo/stoker.svg and the page's still frame (the first step, for
// no script); without, it checks both.
func TestStokerSVG(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed: the stoker's screens come from www/stoker.js")
	}
	script, err := filepath.Abs("../../www/stoker.js")
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, "-e", "process.stdout.write(JSON.stringify(require(process.argv[1]).tour()))", script).Output()
	if err != nil {
		t.Fatalf("node %s: %v", script, err)
	}
	frames, err := stokerFrames(out)
	if err != nil {
		t.Fatal(err)
	}
	tokens := map[bool]map[string]color.Color{true: stokerTokens(true), false: stokerTokens(false)}
	for _, f := range frames {
		if len(f.Lines) != stokerRows {
			t.Fatalf("a stoker screen at %v has %d lines, want %d", f.At, len(f.Lines), stokerRows)
		}
		for y, l := range f.Lines {
			cols := 0
			for _, s := range l {
				cols += len([]rune(s.Text))
				for _, name := range []string{s.FG, s.BG} {
					if _, ok := tokens[true][name]; name != "" && !ok {
						t.Fatalf("a stoker screen at %v uses %q, which is no colour token", f.At, name)
					}
				}
			}
			if cols != stokerCols {
				t.Fatalf("line %d of the stoker screen at %v is %d columns, want %d", y, f.At, cols, stokerCols)
			}
		}
	}

	svg := replay.SVG(stokerCols, stokerRows, frames, tokens)
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write(svg)
	_ = zw.Close()
	if gz.Len() > 150<<10 {
		t.Fatalf("stoker.svg is %d KB gzipped, the budget is 150 KB", gz.Len()>>10)
	}

	svgPath, index := "../../www/demo/stoker.svg", "../../www/index.html"
	page, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	const open, closing = "<!-- stoker:still -->", "<!-- /stoker:still -->"
	i, j := bytes.Index(page, []byte(open)), bytes.Index(page, []byte(closing))
	if i < 0 || j < i {
		t.Fatalf("%s needs %s and %s around the stoker's still frame", index, open, closing)
	}
	still := "\n" + replay.Still(frames[0]) + "\n"
	if !*update {
		was, err := os.ReadFile(svgPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(was, svg) {
			t.Fatalf("%s is out of date with www/stoker.js: run go test ./cmd/matchblox -run TestStokerSVG -update", svgPath)
		}
		if string(page[i+len(open):j]) != still {
			t.Fatalf("the stoker's still frame in %s is out of date: run go test ./cmd/matchblox -run TestStokerSVG -update", index)
		}
		return
	}
	if err := os.WriteFile(svgPath, svg, 0o644); err != nil {
		t.Fatal(err)
	}
	page = append(append(append([]byte{}, page[:i+len(open)]...), still...), page[j:]...)
	if err := os.WriteFile(index, page, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestTheTourStartsTheStokerAgent: the tour shows matchblox starting the
// stoker agent once, on the launch sheet, and the STOKER tab's agent row
// that opens it.
func TestTheTourStartsTheStokerAgent(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed: the stoker's screens come from www/stoker.js")
	}
	out, err := exec.Command(node, "-e", "process.stdout.write(JSON.stringify(require(process.argv[1]).tour()))", "../../www/stoker.js").Output()
	if err != nil {
		t.Fatal(err)
	}
	frames, err := stokerFrames(out)
	if err != nil {
		t.Fatal(err)
	}
	starting, opens := 0, 0
	for _, f := range frames {
		var screen strings.Builder
		for _, l := range f.Lines {
			for _, s := range l {
				screen.WriteString(s.Text)
			}
			screen.WriteByte('\n')
		}
		s := screen.String()
		if strings.Contains(s, "How should I run tonight?") && strings.Contains(s, "starting the stoker…") {
			starting++
		} else if strings.Contains(s, "starting the stoker…") {
			t.Errorf("the stoker starts off the launch sheet at %v:\n%s", f.At, s)
		}
		if strings.Contains(s, "STOKER AGENT") && strings.Contains(s, "⏎ open the stoker") {
			opens++
		}
	}
	if starting != 1 || opens == 0 {
		t.Fatalf("starting the stoker on %d frames (want 1), the agent row that opens it on %d", starting, opens)
	}
}

// stokerFrames reads tour(): each frame's time in ms and its lines of
// [text, fg, bg, flags] spans, flags 1 bold and 2 underline.
func stokerFrames(b []byte) ([]replay.Frame, error) {
	var tour []struct {
		At    int64               `json:"at"`
		Lines [][]json.RawMessage `json:"lines"`
	}
	if err := json.Unmarshal(b, &tour); err != nil {
		return nil, err
	}
	frames := make([]replay.Frame, len(tour))
	for k, f := range tour {
		frames[k].At = time.Duration(f.At) * time.Millisecond
		for _, l := range f.Lines {
			line := make([]replay.Span, 0, len(l))
			for _, raw := range l {
				var text, fg, bg string
				var flags int
				if err := json.Unmarshal(raw, &[4]any{&text, &fg, &bg, &flags}); err != nil {
					return nil, err
				}
				line = append(line, replay.Span{Text: text, FG: fg, BG: bg, Bold: flags&1 != 0, Under: flags&2 != 0})
			}
			frames[k].Lines = append(frames[k].Lines, line)
		}
	}
	return frames, nil
}

// stokerTokens is the console's tokens and the sheet's dimmed screen: the
// text at 28% over the ground, as .fg-dim mixes it on the page.
func stokerTokens(dark bool) map[string]color.Color {
	t := app.Tokens(dark)
	fr, fg, fb, _ := t["fg"].RGBA()
	br, bgG, bb, _ := t["bg"].RGBA()
	mix := func(a, b uint32) uint8 { return uint8((float64(a)*0.28 + float64(b)*0.72) / 257) }
	t["dim"] = color.RGBA{R: mix(fr, br), G: mix(fg, bgG), B: mix(fb, bb), A: 0xFF}
	return t
}
