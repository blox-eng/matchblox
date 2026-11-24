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

// TestACorruptReviewsFileIsSetAside: a reviews.json that does not read
// is kept as reviews.json.bad and the store starts empty.
func TestACorruptReviewsFileIsSetAside(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, reviewsFile)
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := loadReviews(dir)
	if len(got.Comments) != 0 || len(got.Drafts) != 0 {
		t.Fatalf("corrupt file loaded as %+v", got)
	}
	bad := path + ".bad"
	if b, err := os.ReadFile(bad); err != nil || string(b) != "{not-json" {
		t.Fatalf("set aside as %q (%v), want the bad bytes at %s", b, err, bad)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("corrupt file still at %s: %v", path, err)
	}
}
