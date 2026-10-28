package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"

	"github.com/blox-eng/matchblox/internal/advice"
)

// recsLabelRow is the RECOMMENDATIONS label: a tap on it folds the section.
const recsLabelRow = -3

// recPrefs is what the person did to the recommendations, kept in the
// state directory: the ones dismissed, each with the evidence it was
// dismissed on, and whether the section is folded.
type recPrefs struct {
	Dismissed map[string]string `json:"dismissed,omitempty"`
	Folded    bool              `json:"folded,omitempty"`
}

func recPrefsPath(dir string) string { return filepath.Join(dir, "recommendations.json") }

func loadRecPrefs(dir string) recPrefs {
	var p recPrefs
	if dir == "" {
		return p
	}
	if b, err := os.ReadFile(recPrefsPath(dir)); err == nil {
		_ = json.Unmarshal(b, &p)
	}
	return p
}

// save writes the prefs; a console with no state directory keeps them
// for its run only.
func (p recPrefs) save(dir string) error {
	if dir == "" {
		return nil
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp := recPrefsPath(dir) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, recPrefsPath(dir))
}

// dismissed: the person dismissed this recommendation, and its evidence
// has not changed since. A new id or new words bring it back.
func (p recPrefs) dismissed(r advice.Rec) bool {
	ev, ok := p.Dismissed[r.ID]
	return ok && ev == r.Evidence
}

// recRows is how many recommendation rows can be selected: none while the
// section is folded.
func (m Model) recRows() int {
	if m.recPrefs.Folded {
		return 0
	}
	return len(m.recs)
}

// dismissRec hides the selected recommendation until its evidence changes.
func (m *Model) dismissRec() {
	r, ok := m.selectedRec()
	if !ok {
		m.flash = "d dismisses the selected recommendation"
		return
	}
	p := m.recPrefs
	p.Dismissed = map[string]string{}
	for k, v := range m.recPrefs.Dismissed {
		p.Dismissed[k] = v
	}
	p.Dismissed[r.ID] = r.Evidence
	m.recPrefs = p
	at := slices.IndexFunc(m.recs, func(x advice.Rec) bool { return x.ID == r.ID })
	m.flash = "dismissed: " + r.Title + " · back when it changes"
	if err := p.save(m.opt.StateDir); err != nil {
		m.flash = "dismissed for now, not kept: " + err.Error()
	}
	m.refresh()
	if n := len(m.recs); n > 0 { // the next one takes the selection
		m.recPick, m.ghostPick = m.recs[min(at, n-1)].ID, 0
	}
}

// foldRecs folds the recommendations to one line, or unfolds them.
func (m *Model) foldRecs() {
	if _, ok := m.selectedRec(); ok && !m.recPrefs.Folded {
		m.chose = true // the selection leaves the folded section for the sessions
		m.recPick, m.ghostPick = "", 0
	}
	m.recPrefs.Folded = !m.recPrefs.Folded
	m.flash = ""
	if err := m.recPrefs.save(m.opt.StateDir); err != nil {
		m.flash = "not kept: " + err.Error()
	}
}
