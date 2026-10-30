package app

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// viewPrefs is how the person views the console, kept in the state
// directory: list only, one flag for each shape class, so a phone and a
// laptop on the same console do not fight (R9).
type viewPrefs struct {
	ListOnlyPhone bool `json:"list_only_phone"`
	ListOnlyDesk  bool `json:"list_only_desk"`
}

const viewPrefsFile = "view.json"

// loadViewPrefs reads the views; with no file, a phone starts list only.
func loadViewPrefs(dir string) viewPrefs {
	p := viewPrefs{ListOnlyPhone: true}
	if dir == "" {
		return p
	}
	if b, err := os.ReadFile(filepath.Join(dir, viewPrefsFile)); err == nil {
		_ = json.Unmarshal(b, &p)
	}
	return p
}

// listOnly: the list takes the whole screen, with no detail pane and no
// dock, and the selected row opens in place. It is the flag of the shape
// the screen has now.
func (m Model) listOnly() bool {
	if m.upright() {
		return m.viewPrefs.ListOnlyPhone
	}
	return m.viewPrefs.ListOnlyDesk
}

// inPlace: the selected row opens under itself, as on a phone.
func (m Model) inPlace() bool { return m.layoutAt(m.width) == Narrow || m.listOnly() }

// flipListOnly turns list only on or off for the shape the screen has now.
func (m *Model) flipListOnly() {
	if m.upright() {
		m.viewPrefs.ListOnlyPhone = !m.viewPrefs.ListOnlyPhone
	} else {
		m.viewPrefs.ListOnlyDesk = !m.viewPrefs.ListOnlyDesk
	}
	m.flash = ""
	if err := keepState(m.opt.StateDir, viewPrefsFile, m.viewPrefs); err != nil {
		m.flash = "not kept: " + err.Error()
	}
}

// listOnlyHint is the key line's switch: the word, then its key.
func (m Model) listOnlyHint() hint {
	if m.listOnly() {
		return hintFor(m.keys.main.ListOnly, "full view")
	}
	return hintFor(m.keys.main.ListOnly, "")
}
