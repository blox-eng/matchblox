package sample

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"
)

// Compact is what a /compact left in a Claude Code transcript.
type Compact struct {
	At     time.Time
	Before int    // context tokens before the compact
	After  int    // context tokens the summary left
	Resume string // the RESUME: line of the summary, "" when it wrote none
}

type boundaryLine struct {
	Type      string    `json:"type"`
	Subtype   string    `json:"subtype"`
	Timestamp time.Time `json:"timestamp"`
	Meta      struct {
		Pre  int `json:"preTokens"`
		Post int `json:"postTokens"`
	} `json:"compactMetadata"`
}

// Compacted finds the last compact at or after `after` in the end of a
// transcript: its boundary and, when written, its summary.
func Compacted(path string, after time.Time) (Compact, bool) {
	buf, ok := tail(path)
	if !ok {
		return Compact{}, false
	}
	var c Compact
	found := false
	for _, l := range bytes.Split(buf, []byte{'\n'}) {
		switch {
		case bytes.Contains(l, []byte(`"compact_boundary"`)):
			var b boundaryLine
			if json.Unmarshal(l, &b) != nil || b.Subtype != "compact_boundary" || b.Timestamp.Before(after) {
				continue
			}
			c, found = Compact{At: b.Timestamp, Before: b.Meta.Pre, After: b.Meta.Post}, true
		case found && bytes.Contains(l, []byte(`"isCompactSummary":true`)):
			var s struct {
				Message struct {
					Content json.RawMessage `json:"content"`
				} `json:"message"`
			}
			if json.Unmarshal(l, &s) == nil {
				c.Resume = ResumeLine(replyText(s.Message.Content))
			}
		}
	}
	return c, found
}

// ResumeLine is the text after "RESUME:" on the last line that starts with
// it, without the markdown a model wraps it in.
func ResumeLine(text string) string {
	const mark = "RESUME:"
	out := ""
	for _, line := range strings.Split(text, "\n") {
		l := strings.TrimLeft(strings.TrimSpace(line), "*_-> ")
		if rest, ok := strings.CutPrefix(l, mark); ok {
			out = strings.Trim(strings.TrimSpace(rest), "*_ ")
		}
	}
	return out
}

// tail reads the last tailBytes of a file.
func tail(path string) ([]byte, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, false
	}
	off := max(fi.Size()-tailBytes, 0)
	buf := make([]byte, fi.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && err != io.EOF {
		return nil, false
	}
	return buf, true
}
