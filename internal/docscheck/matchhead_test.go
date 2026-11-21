package docscheck

import (
	"os"
	"regexp"
	"testing"
)

var matchHeadSVG = regexp.MustCompile(`(?s)<svg class="match-head".*?</svg>`)

// The site and the phone docs show the same match head: one drawing, inline in both, so it
// follows each page's light and dark tokens.
func TestTheMatchHeadIsOneDrawing(t *testing.T) {
	var drawings []string
	for _, path := range []string{"../../www/index.html", "../../docs/phone.md"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		found := matchHeadSVG.FindAllString(string(data), -1)
		if len(found) != 1 {
			t.Fatalf("%s holds %d match-head drawings, want 1", path, len(found))
		}
		drawings = append(drawings, found[0])
	}
	if drawings[0] != drawings[1] {
		t.Fatal("the match head on the site and in docs/phone.md differ: copy one to the other")
	}
}
