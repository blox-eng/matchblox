package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/blox-eng/matchblox/internal/sample"
)

// reviews.json keeps unsent review comments per session, keyed as seen.json
// is (conversation id, else the tmux session name). Mode 0600.

const reviewsFile = "reviews.json"

// reviewComment is one unsent note on a file:line of a session's diff.
type reviewComment struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// reviewDraft is the last unfinished comment text for a file:line.
type reviewDraft struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// reviewsState is the console's unsent reviews, kept across a restart.
type reviewsState struct {
	Comments map[string][]reviewComment `json:"comments,omitempty"`
	Drafts   map[string]reviewDraft     `json:"drafts,omitempty"`
}

func reviewsPath(dir string) string { return filepath.Join(dir, reviewsFile) }

func loadReviews(dir string) reviewsState {
	var p reviewsState
	if dir == "" {
		return p
	}
	path := reviewsPath(dir)
	b, err := os.ReadFile(path)
	if err != nil {
		return p
	}
	if err = json.Unmarshal(b, &p); err == nil {
		return p
	}
	p = reviewsState{}
	bad, aerr := setAsideState(path, time.Now())
	if aerr != nil {
		fmt.Fprintf(os.Stderr, "matchblox: reviews.json did not read, so unsent comments start empty: %v; it stays at %s: %v\n", err, path, aerr)
		return p
	}
	fmt.Fprintf(os.Stderr, "matchblox: reviews.json did not read, so unsent comments start empty; it is kept at %s: %v\n", bad, err)
	return p
}

// setAsideState moves path to path.bad, or to path.bad-<time> when an
// older one is there: no bad file is overwritten (as seen.json).
func setAsideState(path string, now time.Time) (string, error) {
	bad := path + ".bad"
	err := os.Link(path, bad)
	if errors.Is(err, fs.ErrExist) {
		bad = path + ".bad-" + now.UTC().Format("20060102T150405")
		err = os.Link(path, bad)
	}
	if err != nil {
		return "", err
	}
	return bad, os.Remove(path)
}

func (p reviewsState) save(dir string) error {
	return keepState(dir, reviewsFile, p)
}

// reviewKey is a session's key in reviews.json: its conversation id, else
// its tmux session's name (a pane id changes after a recover).
func reviewKey(s sample.Session) string {
	if s.SessionID != "" {
		return "sid:" + s.SessionID
	}
	if name := s.TmuxSession(); name != "" {
		return "tmux:" + name
	}
	if s.Name != "" {
		return "name:" + s.Name
	}
	return ""
}

func (m Model) reviewsOf(s sample.Session) []reviewComment {
	key := reviewKey(s)
	if key == "" || m.reviews.Comments == nil {
		return nil
	}
	return m.reviews.Comments[key]
}

func (m *Model) setReviewComments(key string, cs []reviewComment) {
	if m.reviews.Comments == nil {
		m.reviews.Comments = map[string][]reviewComment{}
	}
	if len(cs) == 0 {
		delete(m.reviews.Comments, key)
	} else {
		m.reviews.Comments[key] = cs
	}
	_ = m.reviews.save(m.opt.StateDir)
}

func (m *Model) setReviewDraft(key string, d reviewDraft) {
	if m.reviews.Drafts == nil {
		m.reviews.Drafts = map[string]reviewDraft{}
	}
	if d.Text == "" {
		delete(m.reviews.Drafts, key)
	} else {
		m.reviews.Drafts[key] = d
	}
	_ = m.reviews.save(m.opt.StateDir)
}

func (m Model) reviewDraftOf(key, file string, line int) string {
	d, ok := m.reviews.Drafts[key]
	if !ok || d.File != file || d.Line != line {
		return ""
	}
	return d.Text
}
