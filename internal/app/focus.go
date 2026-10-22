package app

import tea "charm.land/bubbletea/v2"

// layerID is a layer that takes keys. The order is the precedence: the
// first one open takes the key.
type layerID int

const (
	layerPair         layerID = iota // the pair page, before its strip
	layerPairStripEsc                // esc on the pair strip, with nothing else to close
	layerSheet
	layerForm
	layerPeek
	layerSearch
	layerHeld     // a key waits for the take-over answer
	layerTakeOver // a key that acts on a session the stoker runs
	layerPreview  // ↑↓ and friends on a door's diff, under its confirm
	layerBlocked  // ⏎ on a host that is blocked or speaks another version
	layerPending
	layerMain
)

// focus is the layer that takes k.
func (m Model) focus(k tea.KeyPressMsg) layerID {
	s := k.String()
	switch {
	case m.pair.owns():
		return layerPair
	case m.pairStripEsc(s):
		return layerPairStripEsc
	case m.sheet != nil:
		return layerSheet
	case m.form != nil:
		return layerForm
	case m.peek != nil:
		return layerPeek
	case m.searching:
		return layerSearch
	}
	return m.under(s)
}

// under is focus below the overlays: what a tapped key hint reaches too.
func (m Model) under(k string) layerID {
	switch {
	case m.held != nil:
		return layerHeld
	case m.asksTakeOver(k):
		return layerTakeOver
	case m.pending != nil && m.pending.door != "" && m.pending.which != "secondary" && m.scrolls(k):
		return layerPreview
	case m.pending == nil && k == "enter" && (m.mismatch || m.blocked != nil):
		return layerBlocked
	case m.pending != nil:
		return layerPending
	}
	return layerMain
}

// scrolls tells if the open diff takes k; other keys go on to the confirm.
func (m Model) scrolls(k string) bool {
	_, ok := m.scrollPreview(k)
	return ok
}
