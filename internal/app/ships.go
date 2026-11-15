package app

import (
	"cmp"
	"slices"
	"strconv"

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
