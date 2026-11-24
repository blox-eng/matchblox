package app

import (
	"encoding/json"
	"os"
	"path/filepath"

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
	b, err := os.ReadFile(reviewsPath(dir))
	if err != nil {
		return p
	}
	_ = json.Unmarshal(b, &p)
	return p
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
