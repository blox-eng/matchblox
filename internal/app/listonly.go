package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
)

// viewPrefs is how the person views the console, kept in the state
// directory: one value for each shape class, so a phone and a laptop on
// the same console do not fight (R9).
type viewPrefs struct {
	// List only went with the Sessions list; an old file's flags still
	// load, and a save keeps them.
	ListOnlyPhone bool `json:"list_only_phone"`
	ListOnlyDesk  bool `json:"list_only_desk"`

	// Told is the notices already shown once, for both shapes.
	Told []string `json:"told"`

	// A paste proved that a copy reaches this shape of screen.
	CopyCheckedPhone bool `json:"copy_checked_phone"`
	CopyCheckedDesk  bool `json:"copy_checked_desk"`
}

const viewPrefsFile = "view.json"

// loadViewPrefs reads the views.
func loadViewPrefs(dir string) viewPrefs {
	var p viewPrefs
	if dir == "" {
		return p
	}
	if b, err := os.ReadFile(filepath.Join(dir, viewPrefsFile)); err == nil {
		_ = json.Unmarshal(b, &p)
	}
	return p
}

func (m *Model) keepViewPrefs() {
	m.flash = ""
	if err := keepState(m.opt.StateDir, viewPrefsFile, m.viewPrefs); err != nil {
		m.sayError("not kept: " + err.Error())
	}
}

// copyChecked: a paste proved that a copy reaches the shape the screen has
// now.
func (m Model) copyChecked() bool {
	if m.upright() {
		return m.viewPrefs.CopyCheckedPhone
	}
	return m.viewPrefs.CopyCheckedDesk
}

// told: this notice was shown before.
func (m Model) told(key string) bool { return slices.Contains(m.viewPrefs.Told, key) }

// tell marks the notice shown, once, and keeps it.
func (m *Model) tell(key string) {
	if m.told(key) {
		return
	}
	m.viewPrefs.Told = append(slices.Clone(m.viewPrefs.Told), key)
	m.keepViewPrefs()
}
