package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
)

// viewPrefs is how the person views the console, kept in the state
// directory: list only, one flag for each shape class, so a phone and a
// laptop on the same console do not fight (R9).
type viewPrefs struct {
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

// oneLineMin is the narrowest list with no right side that keeps its rows one
// line.
const oneLineMin = 107

// inPlace: the selected row opens under itself, as on a phone: on a
// phone, in list only, and wherever Sessions has no right side short of
// oneLineMin.
func (m Model) inPlace() bool {
	w := max(m.width, minWidth)
	return m.layoutAt(m.width) == Narrow || m.listOnly() || w < oneLineMin && m.oneColumn(w)
}

// flipListOnly turns list only on or off for the shape the screen has now.
func (m *Model) flipListOnly() {
	if m.upright() {
		m.viewPrefs.ListOnlyPhone = !m.viewPrefs.ListOnlyPhone
	} else {
		m.viewPrefs.ListOnlyDesk = !m.viewPrefs.ListOnlyDesk
	}
	m.keepViewPrefs()
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
// keeps NEEDS YOU verbose and every other page one line.
func (m Model) oneLineIn(c boardCol) bool {
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

// turnStrip moves the strip to its next face.
func (m *Model) turnStrip() {
	next := stripTurn[(slices.Index(stripTurn, m.stripFace())+1)%len(stripTurn)]
	if m.upright() {
		m.viewPrefs.StripPhone = next
	} else {
		m.viewPrefs.StripDesk = next
	}
	m.keepViewPrefs()
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

// listOnlyHint is the key line's switch: the word, then its key.
func (m Model) listOnlyHint() hint {
	if m.listOnly() {
		return hintFor(m.keys.main.ListOnly, "full view")
	}
	return hintFor(m.keys.main.ListOnly, "")
}
