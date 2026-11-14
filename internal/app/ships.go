package app

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// Ships are the pull requests the sessions drive, each on its track from
// open to live (design/0006 §8). The service reads them; the console
// draws the report it sends.

// Ship kinds and stages, as the tracker names them (proto aliases its
// types; the console does not import it).
const (
	shipRunning = "running"
	shipWaits   = "waits-for-you"
	shipStuck   = "stuck"
	shipDone    = "done"
	stageLive   = 4
)

var stageNames = [...]string{"open", "ci", "green", "merged", "live"}

// ships are every host's ships, merged (see shipsMerged).
func (m Model) ships() []proto.Ship { return m.allShips }

// shipOrder is the order of the service's report: what waits for you, what
// is stuck, what runs, then what is done.
var shipOrder = map[string]int{shipWaits: 0, shipStuck: 1, shipRunning: 2, shipDone: 4}

// shipsMerged are the ships of every host that sent a state, each with its
// host, in the report's order; a tie keeps Hosts order. One local host:
// the home report's ships as they came.
func (m Model) shipsMerged() []proto.Ship {
	home := m.shipReport
	if m.opt.Host == "" && !slices.ContainsFunc(m.peers, func(l link) bool { return l.have && l.st.Ships != nil }) {
		if home == nil {
			return nil
		}
		return home.Ships
	}
	var out []proto.Ship
	add := func(host string, r *proto.Ships) {
		if r == nil {
			return
		}
		for _, sh := range r.Ships {
			sh.Host = host
			out = append(out, sh)
		}
	}
	add(m.opt.Host, home)
	for _, l := range m.peers {
		if l.have {
			add(l.host, l.st.Ships)
		}
	}
	slices.SortStableFunc(out, func(a, b proto.Ship) int {
		if c := cmp.Compare(shipRank(a), shipRank(b)); c != 0 {
			return c
		}
		return b.Since.Compare(a.Since)
	})
	return out
}

func shipRank(sh proto.Ship) int {
	if r, ok := shipOrder[sh.Kind]; ok {
		return r
	}
	return 3
}

// shipReports are the ship reports of the hosts, in Hosts order.
func (m Model) shipReports() []hostShips {
	var out []hostShips
	if m.shipReport != nil {
		out = append(out, hostShips{m.opt.Host, m.shipReport})
	}
	for _, l := range m.peers {
		if l.have && l.st.Ships != nil {
			out = append(out, hostShips{l.host, l.st.Ships})
		}
	}
	return out
}

type hostShips struct {
	host string
	r    *proto.Ships
}

// shipsOf are the ships a session drives, the one that most needs you
// first: stuck, waiting for you, in flight, merged, then live (as the
// service, the CLI and the stoker find them).
func (m Model) shipsOf(s sample.Session) []proto.Ship {
	if sh, ok := m.shipOfCard(s); ok {
		return []proto.Ship{sh}
	}
	// A pane and a session id can repeat on another host.
	return slices.DeleteFunc(proto.ShipsOf(m.ships(), s.Pane, s.SessionID), func(sh proto.Ship) bool { return sh.Host != s.Host })
}

// shipsMore is what a row says after its lead ship when the session drives
// others: " +1 merged" names what the others need (stuck, then waits) or
// else the furthest stage among them; short is " +1".
func (m Model) shipsMore(ss []proto.Ship, short bool) string {
	if len(ss) < 2 {
		return ""
	}
	more := m.st.muted.Render(" +" + strconv.Itoa(len(ss)-1))
	if short {
		return more
	}
	stage, need := 0, proto.Ship{}
	for _, sh := range ss[1:] {
		stage = max(stage, min(sh.Stage, stageLive))
		if (sh.Kind == shipStuck || sh.Kind == shipWaits) && need.Kind != shipStuck {
			need = sh
		}
	}
	switch need.Kind {
	case shipStuck:
		return more + " " + m.shipStyle(need).Render("stuck")
	case shipWaits:
		return more + " " + m.shipStyle(need).Render("waits")
	}
	return more + m.st.muted.Render(" "+stageNames[stage])
}

// shipStyle is the colour of a ship's current stage and note: warn while
// it runs, the accent when it waits for you, neg when it is stuck.
func (m Model) shipStyle(sh proto.Ship) lipgloss.Style {
	switch sh.Kind {
	case shipRunning:
		return m.st.warn
	case shipWaits, shipDone:
		return m.st.accent
	case shipStuck:
		return m.st.neg
	}
	return m.st.faint
}

// trackW is the width of a ship's track: a mark for each stage and a line
// between two.
var trackW = 2*len(stageNames) - 1

// numW is the width of the widest ship number read, "#4551", so the tracks
// after the numbers line up.
func (m Model) numW() int {
	n := 2
	for _, sh := range m.ships() {
		n = max(n, len("#"+strconv.Itoa(sh.Number)))
	}
	return n
}

// track is a ship's five stages, ●━●━◉┄○┄○: done muted, the current one in
// its colour (✕ when stuck), the rest faint.
func (m Model) track(sh proto.Ship) string { return m.trackAt(sh, -1) }

// trackAt is the track with its cell spark lit in flame-core, -1 for none.
func (m Model) trackAt(sh proto.Ship, spark int) string {
	st := m.st
	var b strings.Builder
	x := 0
	put := func(ink lipgloss.Style, cell string) {
		if x == spark {
			ink = m.sparkStyle()
		}
		b.WriteString(ink.Render(cell))
		x++
	}
	for i := range stageNames {
		switch {
		case i < sh.Stage:
			put(st.muted, "●")
		case i == sh.Stage && sh.Kind == shipStuck:
			put(st.neg, "✕")
		case i == sh.Stage && sh.Kind == shipDone:
			put(m.shipStyle(sh), "●")
		case i == sh.Stage:
			put(m.shipStyle(sh), "◉")
		default:
			put(st.faint, "○")
		}
		switch {
		case i == len(stageNames)-1:
		case i < sh.Stage:
			put(st.muted, "━")
		default:
			put(st.faint, "┄")
		}
	}
	return b.String()
}

// shipKey is where a ship's spark is kept among the changes: apart from
// every pane and chain.
func shipKey(sh proto.Ship) paneKey {
	return paneKey{sh.Host, "ship:" + sh.Repo + "#" + strconv.Itoa(sh.Number)}
}

// noteShipSparks plays the spark of each ship that passed a stage since
// the console last saw it.
func (m *Model) noteShipSparks(next []proto.Ship) {
	if m.opt.NoMotion {
		return
	}
	was := map[paneKey]int{}
	for _, sh := range m.allShips {
		was[shipKey(sh)] = sh.Stage
	}
	now := m.now()
	for _, sh := range next {
		if stage, ok := was[shipKey(sh)]; ok && sh.Stage > stage {
			if m.changes == nil {
				m.changes = map[paneKey]change{}
			}
			m.changes[shipKey(sh)] = change{kind: changeSpark, at: now}
		}
	}
}

// shipSpark is the cell of sh's track the spark is on now, -1 for none:
// it runs along the track once, in sparkFor.
func (m Model) shipSpark(sh proto.Ship) int {
	c, ok := m.changes[shipKey(sh)]
	if m.opt.NoMotion || !ok || c.kind != changeSpark {
		return -1
	}
	e := m.now().Sub(c.at)
	if e < 0 || e >= sparkFor {
		return -1
	}
	return int(e * time.Duration(trackW) / sparkFor)
}

// shipCell is "#4551 ●━◉┄○┄○", with the note when note is set.
func (m Model) shipCell(sh proto.Ship, note bool) string {
	cell := padTo(m.st.accent.Render("#"+strconv.Itoa(sh.Number)), m.numW()) + " " + m.track(sh)
	if note {
		cell += "  " + m.shipStyle(sh).Render(sh.Note)
	}
	return cell
}

// shipsBehind are the reasons the ships may be behind, one each, in plain
// words; with two or more hosts each starts with its host.
func (m Model) shipsBehind() []string {
	var says []string
	for _, h := range m.shipReports() {
		on, r := "", h.r
		if m.hostCount() > 1 {
			on = m.nameOf(h.host) + ": "
		}
		if !r.Paused.IsZero() {
			says = append(says, on+"paused · GitHub's rate limit is low, ships read again at "+r.Paused.Local().Format("15:04"))
		}
		if r.Unknown != "" {
			says = append(says, on+"GitHub cannot be read: "+r.Unknown)
		}
		for _, e := range r.Errors {
			says = append(says, on+"not read: "+e)
		}
	}
	return says
}

// takeShip opens New session in the checkout of a ship no session owns,
// its first message to take the ship over (#80); with no checkout of it
// here it says so.
func (m Model) takeShip(sh proto.Ship) Model {
	if sh.Where == "" {
		m.flash = "no session owns #" + strconv.Itoa(sh.Number) + ", and no checkout of " + sh.Repo + " is here"
		return m
	}
	return m.openNewIn(sh.Host, sh.Where, "Take over #"+strconv.Itoa(sh.Number)+" ("+sh.URL+"): read where it stands, then carry it to live.")
}

// mergeAction is the merge of sh, behind a typed y; nil when sh has none.
func (m Model) mergeAction(sh proto.Ship) *action {
	if len(sh.Merge) == 0 {
		return nil
	}
	return &action{label: "merge", steps: [][]string{sh.Merge}, destructive: true,
		rec: "ship:" + sh.Repo + "#" + strconv.Itoa(sh.Number), which: "secondary", host: sh.Host}
}
