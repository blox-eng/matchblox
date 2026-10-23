package app

import (
	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// layerID is a layer that takes keys. The order is the precedence: the
// first one open takes the key.
type layerID int

const (
	layerPair         layerID = iota // the pair page, before its strip
	layerPairStripEsc                // esc on the pair strip, with nothing else to close
	layerSheet
	layerForm
	layerPeek
	layerPeekGone // a peek whose session is gone: any key closes it, ctrl+c too
	layerSearch
	layerHeld     // a key waits for the take-over answer
	layerScan     // the scan question
	layerTakeOver // a key that acts on a session the stoker runs
	layerPreview  // ↑↓ and friends on a door's diff, under its confirm
	layerBlocked  // ⏎ on a host that is blocked or speaks another version
	layerPending
	layerMain
)

// underOverlays is a layer under the overlays: its key marks the stoker's
// steps seen when the tab shows them.
func (f layerID) underOverlays() bool { return f >= layerHeld }

// focus is the layer that takes k.
func (m Model) focus(k tea.KeyPressMsg) layerID {
	switch {
	case m.pair.owns():
		return layerPair
	case m.pairStripEsc(k):
		return layerPairStripEsc
	case m.sheet != nil:
		return layerSheet
	case m.form != nil:
		return layerForm
	case m.peek != nil:
		if _, _, ok := m.peekNow(); !ok {
			return layerPeekGone
		}
		return layerPeek
	case m.searching:
		return layerSearch
	}
	return m.under(k)
}

// under is focus below the overlays: what a tapped key hint reaches too.
func (m Model) under(k tea.KeyPressMsg) layerID {
	switch {
	case m.held != nil:
		return layerHeld
	case m.scanAsk != nil && !m.splashing():
		return layerScan
	case m.asksTakeOver(k):
		return layerTakeOver
	case m.scrolls(k):
		return layerPreview
	case m.pending == nil && bkey.Matches(k, m.keys.main.Open) && (m.mismatch || m.blocked != nil):
		return layerBlocked
	case m.pending != nil:
		return layerPending
	}
	return layerMain
}

// scrolls tells if a door's diff is open and takes k; other keys go on to
// the confirm.
func (m Model) scrolls(k tea.KeyPressMsg) bool {
	_, ok := m.scrollPreview(k)
	return ok
}
