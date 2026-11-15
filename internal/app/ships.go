package app

import (
	"cmp"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

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

// mergeAction is the merge of sh, behind a typed y; nil when sh has none.
func (m Model) mergeAction(sh proto.Ship) *action {
	if len(sh.Merge) == 0 {
		return nil
	}
	return &action{label: "merge", steps: [][]string{sh.Merge}, destructive: true,
		rec: "ship:" + sh.Repo + "#" + strconv.Itoa(sh.Number), which: "secondary", host: sh.Host}
}

// shipCards are the ships that no session drives, each a row of its own
// in the list (ruling 31, #80): named by its branch, its pane the ship's
// mark. A merged or live ship has none.
func (m Model) shipCards() []sample.Session {
	var out []sample.Session
	for _, sh := range m.ships() {
		if sh.Kind == shipDone || sh.Session.ID != "" || sh.Session.Pane != "" {
			continue
		}
		out = append(out, sample.Session{Host: sh.Host, Name: m.shipCardName(sh), Pane: shipCardPane(sh), Cwd: sh.Where,
			Status: "idle", Started: sh.Since})
	}
	return out
}

// shipCardPane is the pane of a ship's row: "#4570@hearth/hearth-api". A
// tmux pane's id starts with %, so it is never a pane's.
func shipCardPane(sh proto.Ship) string { return "#" + strconv.Itoa(sh.Number) + "@" + sh.Repo }

// isShipCard: s is the row of a ship no session drives, and has no pane.
func isShipCard(s sample.Session) bool { return strings.HasPrefix(s.Pane, "#") }

// shipCardName is the name of a ship's row: the branch of its checkout,
// else its repository and number.
func (m Model) shipCardName(sh proto.Ship) string {
	if r := m.gitOf(sh.Host); r != nil && sh.Where != "" {
		for _, repo := range r.Repos {
			for _, wt := range repo.Worktrees {
				if wt.Path == sh.Where && wt.Branch != "" {
					return wt.Branch
				}
			}
		}
	}
	return filepath.Base(sh.Repo) + " #" + strconv.Itoa(sh.Number)
}

// shipOfCard is the ship of a ship's row.
func (m Model) shipOfCard(s sample.Session) (proto.Ship, bool) {
	if !isShipCard(s) {
		return proto.Ship{}, false
	}
	i := slices.IndexFunc(m.ships(), func(sh proto.Ship) bool { return sh.Host == s.Host && shipCardPane(sh) == s.Pane })
	if i < 0 {
		return proto.Ship{}, false
	}
	return m.ships()[i], true
}

// chosenShip is the chosen row when it is a ship no session drives.
func (m Model) chosenShip() (sample.Session, proto.Ship, bool) {
	s, ok := m.sideChosen()
	if !ok {
		return s, proto.Ship{}, false
	}
	sh, card := m.shipOfCard(s)
	return s, sh, card
}

// shipWord is the state of a ship in words, as its row says it.
func shipWord(sh proto.Ship) string {
	switch sh.Kind {
	case shipWaits:
		return "waits for your merge"
	case shipStuck:
		return "stuck"
	}
	return stageNames[sh.Stage]
}

// shipPanel is the chat's place on a ship no session drives: its pull
// request, its stage and its checks, and the two acts it takes.
func (m Model) shipPanel(s sample.Session, sh proto.Ship, w, h int) []string {
	st := m.st
	lines := []string{m.chatHead(s, w), strings.Repeat(" ", w)}
	add := func(text string, ink lipgloss.Style) {
		for _, l := range wrapPlain(text, max(w-2, 1)) {
			lines = append(lines, fit(" "+ink.Render(l), w))
		}
	}
	add("#"+strconv.Itoa(sh.Number)+" "+sh.Title, st.text.Bold(true))
	add(sh.Repo+" · "+shipWord(sh), st.muted)
	if sh.Note != "" && sh.Note != shipWord(sh) {
		add(sh.Note, st.muted)
	}
	if sh.URL != "" {
		add(sh.URL, st.faint)
	}
	lines = append(lines, strings.Repeat(" ", w))
	acts := m.keys.side.Enter.Help().Key + " new session on its branch"
	if _, ok := m.mergeable(s); ok {
		acts += " · " + m.keys.board.Merge.Help().Key + " merge, after a typed y"
	}
	add(acts, st.text)
	for len(lines) < h-1 {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return append(lines[:min(len(lines), max(h-1, 0))], m.chatLine(s, w))
}

// shipCardWhy is what a ship's row says of keys that need a session.
func (m Model) shipCardWhy(sh proto.Ship) string {
	return "no session drives #" + strconv.Itoa(sh.Number) + " · " + m.keys.side.Enter.Help().Key + " starts one on its branch"
}

// takeShip opens New session in the checkout of a ship no session
// drives, its first message to take the ship over (#80); with no
// checkout of it here it says so.
func (m Model) takeShip(sh proto.Ship) Model {
	if sh.Where == "" {
		m.flash = "no session drives #" + strconv.Itoa(sh.Number) + ", and no checkout of " + sh.Repo + " is here"
		return m
	}
	return m.openNewIn(sh.Host, sh.Where, "Take over #"+strconv.Itoa(sh.Number)+" ("+sh.URL+"): read where it stands, then carry it to live.")
}

// openNewIn is N on host with where picked and the cursor on agent, so
// ⏎ ⏎ starts it, with first as the first message to edit; with no folder,
// N as it is.
func (m Model) openNewIn(host, where, first string) Model {
	if where == "" {
		next, _ := m.openNew()
		return next.(Model)
	}
	nm := m.openNewAt(host, where, first)
	if nm.form == nil {
		return nm
	}
	f := *nm.form
	f.where, f.picked, f.query, f.extra = where, true, "", ""
	f.listOpen, f.at = false, fieldAgent
	nm.form = &f
	return nm
}
