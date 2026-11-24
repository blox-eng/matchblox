package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blox-eng/matchblox/internal/sample"
)

// TestReviewsStayPerSessionKey: comments are keyed as seen.json is, written
// 0600, and a restart of the console reads them.
func TestReviewsStayPerSessionKey(t *testing.T) {
	dir := t.TempDir()
	s := sample.Session{SessionID: "sid-ux-pass", Name: "ux-pass", Pane: "%3"}
	key := reviewKey(s)
	if key != "sid:sid-ux-pass" {
		t.Fatalf("reviewKey = %q, want sid:sid-ux-pass", key)
	}
	m := Model{opt: Options{StateDir: dir}, reviews: loadReviews(dir)}
	m.setReviewComments(key, []reviewComment{{File: "frame.go", Line: 41, Text: "fold at width"}})
	info, err := os.Stat(filepath.Join(dir, reviewsFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v, want 0600", info.Mode().Perm())
	}
	again := loadReviews(dir)
	if n := len(again.Comments[key]); n != 1 || again.Comments[key][0].Text != "fold at width" {
		t.Fatalf("restart kept %+v", again.Comments[key])
	}
}
