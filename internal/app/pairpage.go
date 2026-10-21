package app

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The pair page is a layer over the console, as the peek is: the box with
// the one-time code as its label (design/0009-the-box-opens.md §4.2).

const (
	pairWindow    = 5 * time.Minute
	pairPollEvery = 100 * time.Millisecond
)

const (
	pairScanLine    = "Scan with the matchblox app, or paste the link."
	pairKeyLine     = "c copies the link · esc stops"
	pairStoppedLine = "Stopped. The code no longer works."
	pairSmallLine   = "Make this terminal 61 × 33 or larger to show the code, or press c to copy the link."
)

// PairDevice is a device that claimed a pair code, as setup.Device.
type PairDevice struct{ Kind, Name, Fingerprint, ID string }

// PairHost is what the pair page reads and writes on this machine. The
// console does not touch the machine itself: main fills these from setup.
type PairHost struct {
	// Offer writes a one-time key that works until until, and makes its link.
	Offer func(until time.Time) (id, link string, err error)
	// Seen is whether a device reached gate pair, and the name it gave.
	Seen func(id string) (seen bool, name string)
	// Present is whether the key line is still there.
	Present func(id string) (bool, error)
	// Remove takes out the key line and the seen note; removed is false
	// when the line was gone already.
	Remove func(id string) (removed bool, err error)
	// Claim is the device that used the key. It gives a device once.
	Claim func(id string) (PairDevice, bool)
}

type pairPage struct {
	id, link   string
	until      time.Time
	phase      boxPhase
	phaseAt    time.Time
	seenName   string
	device     PairDevice // after the claim
	standalone bool       // matchblox pair: stops after the opening
	beat       int        // 1 shut, 2 opened, 3 in, 4 first act (Task 9)
	line       string     // the line under the box
}

type pairPollMsg struct{}

// pairFrameMsg draws the next frame of the knock or the opening.
type pairFrameMsg struct{}

func pairPoll() tea.Cmd {
	return tea.Tick(pairPollEvery, func(time.Time) tea.Msg { return pairPollMsg{} })
}

func pairFrame() tea.Cmd {
	return tea.Tick(splashTick, func(time.Time) tea.Msg { return pairFrameMsg{} })
}

// osc52 asks the terminal to put text on the clipboard. It works over SSH,
// and it does not move the cursor.
func osc52(text string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\x07"
}

func (m Model) clip(text string) {
	if m.opt.Clip != nil {
		m.opt.Clip(text)
		return
	}
	_, _ = os.Stdout.WriteString(osc52(text))
}

// openPairPage makes the one-time key, writes it and starts the poll.
func (m Model) openPairPage() (Model, tea.Cmd) {
	now := m.now()
	p := &pairPage{phase: boxShut, phaseAt: now, beat: 1}
	m.pair, m.flash = p, ""
	if m.opt.Pair == nil {
		p.line = "Pair from the host's own terminal."
		return m, nil
	}
	p.until = now.Add(pairWindow)
	id, link, err := m.opt.Pair.Offer(p.until)
	if err != nil {
		p.line = err.Error()
		return m, nil
	}
	p.id, p.link, p.line = id, link, pairScanLine
	if m.pairPolling {
		return m, nil // the poll on its way goes on
	}
	m.pairPolling = true
	return m, pairPoll()
}

// closePairPage removes the key line and the .seen note; why goes to the
// flash. A device that claimed the code since the last poll is named.
func (m Model) closePairPage(why string) Model {
	p := m.pair
	m.pair = nil
	if p == nil {
		return m
	}
	m.flash = why
	if p.id == "" || m.opt.Pair == nil {
		return m
	}
	removed, err := m.opt.Pair.Remove(p.id)
	if err == nil && !removed && p.live() {
		if d, ok := m.opt.Pair.Claim(p.id); ok {
			m.flash = pairedLine(d)
		}
	}
	return m
}

// ClosePair removes the one-time key of an open pair page. main calls it
// when the program ends, however it ends.
func (m Model) ClosePair() { _ = m.closePairPage("") }

// quit ends the program; an open pair page takes its code with it.
func (m Model) quit() (tea.Model, tea.Cmd) {
	m = m.closePairPage("")
	m.quitting = true
	return m, tea.Quit
}

// live is a code that still waits for a device.
func (p *pairPage) live() bool {
	return p.link != "" && (p.phase == boxShut || p.phase == boxKnock)
}

func (p *pairPage) to(ph boxPhase, at time.Time) {
	p.phase, p.phaseAt = ph, at
}

func pairedLine(d PairDevice) string {
	return fmt.Sprintf("Paired %s (%s). Not yours? x on it in DEVICES.", d.Name, d.Fingerprint)
}

func (p *pairPage) expiredLine() string {
	if p.standalone {
		return "The code expired. Run matchblox pair again."
	}
	return "The code expired. Enter pairs again."
}

// pairMoving tells if the knock or the opening still plays.
func (m Model) pairMoving() bool {
	if m.pair == nil || m.opt.NoMotion {
		return false
	}
	t := m.now().Sub(m.pair.phaseAt)
	switch m.pair.phase {
	case boxKnock:
		return t < knockFor
	case boxOpen:
		return t < openFor+boxOutFor+boxStrikeFor+standFor
	}
	return false
}

// pairPolled reads the notes: the expiry first, then a device at the door,
// then the claim.
func (m Model) pairPolled() (Model, tea.Cmd) {
	if m.pair == nil {
		m.pairPolling = false
		return m, nil
	}
	if !m.pair.live() {
		return m, pairPoll()
	}
	p := *m.pair
	h, now, was := m.opt.Pair, m.now(), p.phase
	claim := func() bool {
		d, ok := h.Claim(p.id)
		if ok {
			p.device, p.beat, p.line = d, 2, pairedLine(d)
			p.to(boxOpen, now)
		}
		return ok
	}
	if !now.Before(p.until) {
		removed, err := h.Remove(p.id)
		if err != nil || removed || !claim() { // else the device took it at the last moment
			p.line = p.expiredLine()
			p.to(boxExpired, now)
		}
	} else {
		if seen, name := h.Seen(p.id); seen {
			if p.phase == boxShut {
				p.to(boxKnock, now)
			}
			p.seenName, p.line = name, "A device is here…"
			if name != "" {
				p.line = name + " is here…"
			}
		}
		if present, err := h.Present(p.id); err == nil && !present {
			if !claim() { // the line went, but no device used it
				p.line = p.expiredLine()
				p.to(boxExpired, now)
			}
			_, _ = h.Remove(p.id) // the seen note
		}
	}
	m.pair = &p
	if p.phase != was && m.pairMoving() {
		return m, tea.Batch(pairPoll(), pairFrame())
	}
	return m, pairPoll()
}

// pairKey is a key while the page shows: c copies, esc closes, Enter
// makes a new code after the old one expired, ctrl+c quits. Every other key
// does nothing.
func (m Model) pairKey(k string) (tea.Model, tea.Cmd) {
	p := m.pair
	m.flash = ""
	switch k {
	case "c":
		if p.live() {
			m.clip(p.link)
			m.flash = "link copied"
		}
	case "esc":
		why := ""
		if p.live() {
			why = pairStoppedLine
		}
		return m.closePairPage(why), nil
	case "enter":
		if p.phase == boxExpired {
			return m.closePairPage("").openPairPage()
		}
	case "ctrl+c":
		return m.quit()
	}
	return m, nil
}

// pairView takes the whole screen: the box, or the code alone, and the two
// lines under it, in the middle.
func (m Model) pairView(w int) string {
	p := m.pair
	now := m.now()
	centre := func(s string) string {
		s = ansi.Truncate(s, w, "…")
		return strings.Repeat(" ", max((w-lipgloss.Width(s))/2, 0)) + s
	}
	line := m.st.muted.Render(p.line)
	if p.live() {
		left := max(p.until.Sub(now), 0).Round(time.Second)
		line += "   " + m.st.muted.Render(fmt.Sprintf("%d:%02d", int(left.Minutes()), int(left.Seconds())%60))
	}
	var keys []string
	if p.live() {
		keys = append(keys, m.st.muted.Render(pairKeyLine))
	}
	if m.flash != "" {
		keys = append(keys, m.st.text.Render(m.flash))
	}
	text := []string{"", centre(line), centre(strings.Join(keys, "   "))}

	var lines []string
	if p.link != "" {
		s := boxState{phase: p.phase, since: now.Sub(p.phaseAt), link: p.link, name: p.device.Name}
		box, fits := pairBox(s, m.dark, m.opt.NoMotion, w, m.height)
		if fits == fitNone {
			for _, l := range strings.Split(ansi.Wordwrap(pairSmallLine, max(w-2, 1), ""), "\n") {
				box = append(box, centre(m.st.muted.Render(l)))
			}
		}
		lines = box
	}
	lines = append(lines, text...)
	top := max((m.height-len(lines))/2, 0)
	return strings.Repeat("\n", top) + strings.Join(lines, "\n")
}
