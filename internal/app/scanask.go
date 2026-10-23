package app

import (
	"fmt"
	"strings"
	"time"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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

// scanAsk is the question up at this console.
type scanAsk struct{}

// askQuiet is how long the box takes no answer after it shows: a key
// meant for what was on screen before must not answer it. Esc and
// ctrl+c work at once.
const askQuiet = 400 * time.Millisecond

// scanShown tells if the box is on top: asked, and nothing over it, no
// other question either, the stoker's take-over ask included.
func (m Model) scanShown() bool {
	return m.scanAsk != nil && !m.splashing() && !m.takeOverDue() && !m.pair.owns() && m.sheet == nil && m.form == nil &&
		m.peek == nil && !m.searching && m.held == nil && m.pending == nil
}

// armScan starts the box's quiet: it shows now.
func (m Model) armScan() Model {
	m.scanAsk = &scanAsk{}
	m.scanAskUntil = time.Now().Add(askQuiet)
	return m
}

// closeScan closes the box at this console; the next console asks again
// if nobody answered.
func (m Model) closeScan() Model {
	m.scanLater, m.scanAsk = true, nil
	return m
}

func (m Model) answerScan(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	sk := m.keys.scan
	if time.Now().Before(m.scanAskUntil) && !bkey.Matches(k, sk.Later) {
		return m, nil
	}
	switch {
	case bkey.Matches(k, sk.Find):
		m = m.closeScan()
		m.flash = "finding your Git repositories…"
		cmd := m.send(proto.Act{RecID: "scan:yes", Confirm: "y"})
		m.scanAct = fmt.Sprintf("a%d", m.acts)
		return m, cmd
	case bkey.Matches(k, sk.Never):
		m = m.closeScan()
		m.flash = "no scan: matchblox scan finds them later"
		return m, m.send(proto.Act{RecID: "scan:no"})
	case bkey.Matches(k, sk.Pick):
		m.form = &form{pick: true, at: fieldFolders, roots: defaultRoots}
		return m, nil
	}
	return m.closeScan(), nil
}

// scanResult says how the box's yes went.
func (m Model) scanResult(r proto.Result) (Model, bool) {
	if m.scanAct == "" || r.ActID != m.scanAct {
		return m, false
	}
	m.scanAct = ""
	roots := defaultRoots
	if len(m.found.Roots) > 0 {
		roots = showFolders(m.found.Roots)
	}
	m.flash = m.scanFlash(r, roots)
	return m, true
}

// scanFlash is how this console's scan:yes of roots, as shown, went, from
// its own reply: the Found broadcast may be another console's scan, or
// none at all.
func (m Model) scanFlash(r proto.Result, roots string) string {
	if r.Err != "" {
		return "not found: " + r.Err
	}
	ok := "found your Git repositories: " + m.keys.main.New.Help().Key + " offers them"
	sum := r.Scan
	if sum == nil {
		return ok
	}
	room := max(m.width, minWidth) - 2
	found := "found " + gitRepos(sum.Repos)
	switch {
	case sum.Stopped:
		line := found + ", but the scan stopped early"
		if len(sum.Errors) > 0 {
			line += ": " + sum.Errors[0] // the bound that cut it
		}
		return line
	case len(sum.Errors) > 0: // the walk ran to the end
		return found + "; " + plural(unread(sum.Errors), "folder") + " could not be read: matchblox scan lists them"
	case sum.Repos == 0 && len(sum.Skipped) == 0:
		return ansi.Truncate("found no Git repositories in "+roots, room, "…")
	case len(sum.Skipped) == 0:
		return ok
	}
	// The folders skipped, in order, as many as the line holds;
	// matchblox scan lists them all.
	line := ok + "; skipped"
	total := len(sum.Skipped) + sum.SkippedMore
	for i, sk := range sum.Skipped {
		next := line + " " + tilde(sk.Path)
		rest := total - i - 1
		more := ""
		if rest > 0 {
			more = fmt.Sprintf(" and %d more", rest)
		}
		if len([]rune(next+more)) > room && i > 0 {
			return line + fmt.Sprintf(" and %d more", total-i)
		}
		line = next
	}
	if sum.SkippedMore > 0 {
		line += fmt.Sprintf(" and %d more", sum.SkippedMore)
	}
	return line
}

// gitRepos is "1 Git repository", "2 Git repositories".
func gitRepos(n int) string {
	if n == 1 {
		return "1 Git repository"
	}
	return fmt.Sprintf("%d Git repositories", n)
}

// unread is how many folders errs says could not be read: a capped list
// ends "and N more".
func unread(errs []string) int {
	n := len(errs)
	var more int
	if _, err := fmt.Sscanf(errs[n-1], "and %d more", &more); err == nil {
		return n - 1 + more
	}
	return n
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
