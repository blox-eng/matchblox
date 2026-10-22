package app

import (
	"fmt"
	"strings"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
)

// The scan question (#173): asked once per host, in the consent box. Only
// y and n are answers; esc, any other key or a tap closes it until the
// next console, and records nothing.

const (
	scanTitle    = "Find the Git repositories in your home folder?"
	scanStays    = "they stay on this host and your paired devices"
	defaultRoots = "~ ~/dev ~/src ~/code ~/projects ~/work"
)

// asksScan: the service scans, nobody answered, and nothing else is open.
func (m Model) asksScan() bool {
	return m.found != nil && m.found.Consent == "" && !m.scanLater &&
		m.form == nil && m.pending == nil && m.held == nil && m.peek == nil && m.sheet == nil && !m.pair.owns()
}

func (m Model) answerScan(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	sk := m.keys.scan
	switch {
	case bkey.Matches(k, sk.Find):
		m.scanLater, m.flash = true, "finding your Git repositories…"
		cmd := m.send(proto.Act{RecID: "scan:yes", Confirm: "y"})
		m.scanAct = fmt.Sprintf("a%d", m.acts)
		return m, cmd
	case bkey.Matches(k, sk.Never):
		m.scanLater, m.flash = true, "no scan: matchblox scan finds them later"
		return m, m.send(proto.Act{RecID: "scan:no"})
	case bkey.Matches(k, sk.Pick):
		m.form = &form{pick: true, at: fieldFolders, roots: defaultRoots}
		return m, nil
	}
	m.scanLater = true
	return m, nil
}

// scanResult says how the box's yes went.
func (m Model) scanResult(r proto.Result) (Model, bool) {
	if m.scanAct == "" || r.ActID != m.scanAct {
		return m, false
	}
	m.scanAct = ""
	m.flash = "found your Git repositories: N offers them"
	if r.Err != "" {
		m.flash = "not found: " + r.Err
	}
	return m, true
}

func (m Model) scanBox(bw int) []string {
	st := m.st
	var lines []string
	for _, l := range wrapPlain(scanStays, bw-4-2*consentPad) {
		lines = append(lines, st.text.Render(l))
	}
	roots := defaultRoots
	if len(m.found.Roots) > 0 {
		roots = strings.Join(m.found.Roots, " ")
	}
	for _, l := range wrapPlain(strings.ReplaceAll(roots, " ", "  "), bw-4-2*consentPad) {
		lines = append(lines, st.muted.Render(l))
	}
	sk := m.keys.scan
	box, _ := boxFrame(st.hair, st.accent.Bold(true).Render(scanTitle), padded([][]string{lines}, bw),
		[]footPart{st.footKey(sk.Find, sk.Find.Help().Desc, "", st.accent), st.footKey(sk.Never, sk.Never.Help().Desc, " ─ ", st.accent),
			st.footKey(sk.Pick, sk.Pick.Help().Desc, " ─ ", st.accent), st.footKey(sk.Later, sk.Later.Help().Desc, " ─ ", st.accent)}, bw)
	return box
}

// scanWidth fits the title and the foot on one line each.
func (m Model) scanWidth() int {
	foot := 0
	for i, b := range m.keys.scan.bindings() {
		if i > 0 {
			foot += len([]rune(" ─ "))
		}
		foot += len([]rune(b.Help().Key + " " + b.Help().Desc))
	}
	return max(len([]rune(scanTitle))+8, foot+8)
}
