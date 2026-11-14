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

	// The board's views, for each shape: the columns hidden (boardCol.short),
	// one line a card, and the strip's face (usage, machine, week or off;
	// empty reads as usage). A phone's density is nil until its first v.
	HiddenPhone  []string `json:"hidden_phone"`
	HiddenDesk   []string `json:"hidden_desk"`
	OneLinePhone *bool    `json:"one_line_phone,omitempty"`
	OneLineDesk  bool     `json:"one_line_desk"`
	StripPhone   string   `json:"strip_phone"`
	StripDesk    string   `json:"strip_desk"`

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
		m.flash = "not kept: " + err.Error()
	}
}

// colHidden: the board leaves this column out, for the shape the screen has
// now. (Not hidden: Model.hidden counts the hosts the builder hid.)
func (m Model) colHidden(c boardCol) bool {
	if m.upright() {
		return slices.Contains(m.viewPrefs.HiddenPhone, c.short())
	}
	return slices.Contains(m.viewPrefs.HiddenDesk, c.short())
}

// flipHidden hides the column, or shows it again.
func (m *Model) flipHidden(c boardCol) {
	list := &m.viewPrefs.HiddenDesk
	if m.upright() {
		list = &m.viewPrefs.HiddenPhone
	}
	if i := slices.Index(*list, c.short()); i >= 0 {
		*list = slices.Delete(slices.Clone(*list), i, i+1)
	} else {
		*list = append(slices.Clone(*list), c.short())
	}
	m.keepViewPrefs()
}

// oneLine: every card takes one line, by the person's choice.
func (m Model) oneLine() bool {
	if m.upright() {
		return m.viewPrefs.OneLinePhone != nil && *m.viewPrefs.OneLinePhone
	}
	return m.viewPrefs.OneLineDesk
}

// oneLineIn: a card of column c takes one line. A phone with no choice yet
// keeps NEEDS YOU verbose and every other page one line; at Medium WORKING
// and SHIPPING are rows of the stack.
func (m Model) oneLineIn(c boardCol) bool {
	if (c == colWork || c == colShip) && m.layoutAt(max(m.width, minWidth)) == Medium {
		return true
	}
	if m.upright() && m.viewPrefs.OneLinePhone == nil {
		return c != colNeed
	}
	return m.oneLine()
}

func (m *Model) flipOneLine() {
	if m.upright() {
		one := !m.oneLine()
		m.viewPrefs.OneLinePhone = &one
	} else {
		m.viewPrefs.OneLineDesk = !m.viewPrefs.OneLineDesk
	}
	m.keepViewPrefs()
}

// stripFace is what the strip shows: usage, machine, week or off.
func (m Model) stripFace() string {
	face := m.viewPrefs.StripDesk
	if m.upright() {
		face = m.viewPrefs.StripPhone
	}
	if face == "" {
		return "usage"
	}
	return face
}

// stripTurn is the order the strip turns through. Week joins it when the
// daily record exists.
var stripTurn = []string{"usage", "machine", "off"}

// stripNext is the face after face.
func stripNext(face string) string {
	return stripTurn[(slices.Index(stripTurn, face)+1)%len(stripTurn)]
}

// turnStrip moves the strip to its next face.
func (m *Model) turnStrip() { m.setStripFace(stripNext(m.stripFace())) }

// setStripFace keeps face as this shape's strip.
func (m *Model) setStripFace(face string) {
	if m.upright() {
		m.viewPrefs.StripPhone = face
	} else {
		m.viewPrefs.StripDesk = face
	}
	m.keepViewPrefs()
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
