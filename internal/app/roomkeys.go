package app

import (
	"cmp"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
)

// roomKeys are the keys of a chain's room. While the talk line types, every
// key but ⏎, esc and ⌫ is text.
type roomKeys struct{ Up, Down, Open, Give, Next, Prev, Back, Erase, Flip, Walk, Out bkey.Binding }

func (k roomKeys) bindings() []bkey.Binding {
	return []bkey.Binding{k.Up, k.Down, k.Open, k.Give, k.Next, k.Prev, k.Back, k.Erase, k.Flip, k.Walk, k.Out}
}

// goWait is a go room k's stoker waits on: the entry's id, what it says,
// the go it needs, the command it runs, and its plan step (0 for a go the
// guard holds).
type goWait struct {
	id, text, needs, want string
	step                  int
	at                    int64
}

// goWaits are the gos room k's stoker waits on, oldest first: each plan
// step whose last entry waits and names its go, and each go the guard
// holds (rule "go") that is not given yet.
func (m Model) goWaits(k chainKey) []goWait {
	var v *proto.StokerView
	switch u, ok := m.unitOf(k); {
	case k.isOne():
		v = m.stoker
	case ok:
		v = &u.View
	}
	if v == nil {
		return nil
	}
	last := map[int]proto.StokerEntry{}
	var steps []int
	held := map[string]proto.StokerEntry{} // the last go entry of each pane and go
	var gos []string
	for _, e := range v.Run {
		switch {
		case e.Rule == "go":
			key := e.Pane + " " + e.Needs
			if _, seen := held[key]; !seen {
				gos = append(gos, key)
			}
			held[key] = e
		case e.Step > 0:
			if _, seen := last[e.Step]; !seen {
				steps = append(steps, e.Step)
			}
			last[e.Step] = e
		}
	}
	var out []goWait
	for _, key := range gos {
		if e := held[key]; e.State == "waits" && e.Needs != "" {
			out = append(out, goWait{id: e.ID, text: e.Text, needs: e.Needs, want: e.Want, at: e.At.UnixNano()})
		}
	}
	for _, n := range steps {
		if e := last[n]; e.State == "waits" && e.Needs != "" {
			out = append(out, goWait{id: e.ID, text: e.Text, needs: e.Needs, want: e.Want, step: n, at: e.At.UnixNano()})
		}
	}
	slices.SortStableFunc(out, func(a, b goWait) int { return cmp.Compare(a.at, b.at) })
	return out
}

// goOf is the go row waits on: a need row's by its entry, a step row's by
// its plan step.
func (m Model) goOf(k chainKey, row roomRow) (goWait, bool) {
	for _, g := range m.goWaits(k) {
		switch {
		case row.kind == roomNeed && g.id == row.id,
			row.kind == roomStep && g.step > 0 && strconv.Itoa(g.step) == row.id:
			return g, true
		}
	}
	return goWait{}, false
}

// unitText is what stoker:grant names a go of room k by: the unit's id
// and the entry's, or the entry's alone for the host's stoker.
func (m Model) unitText(k chainKey, id string) string {
	if u, ok := m.unitOf(k); ok && !k.isOne() {
		return u.ID + " " + id
	}
	return id
}

// roomKey is a key in the room.
func (m Model) roomKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	r, rk := *m.room, m.keys.room
	m.room = &r
	if r.typing {
		switch {
		case bkey.Matches(k, rk.Open):
			return m.talk()
		case bkey.Matches(k, rk.Back):
			r.typing = false
		case bkey.Matches(k, rk.Erase):
			if _, n := utf8.DecodeLastRuneInString(r.draft); n > 0 {
				r.draft = r.draft[:len(r.draft)-n]
			}
		case k.Text != "":
			r.draft += k.Text
		}
		return m, nil
	}
	rows := m.roomRows(r.k)
	r.at = min(r.at, len(rows)-1)
	row := rows[r.at]
	switch {
	case bkey.Matches(k, rk.Flip):
		return m.flipHere()
	case bkey.Matches(k, rk.Walk):
		return m.walkHere()
	case bkey.Matches(k, rk.Up):
		r.at = max(r.at-1, 0)
	case bkey.Matches(k, rk.Down):
		r.at = min(r.at+1, len(rows)-1)
	case bkey.Matches(k, rk.Back):
		m.room = nil
	case bkey.Matches(k, rk.Next, rk.Prev):
		keys := m.roomKeys()
		if i := slices.Index(keys, r.k); i >= 0 && len(keys) > 1 {
			step := 1
			if bkey.Matches(k, rk.Prev) {
				step = len(keys) - 1
			}
			return m.openRoom(keys[(i+step)%len(keys)]), nil
		}
	case bkey.Matches(k, rk.Give):
		return m.give(row)
	case bkey.Matches(k, rk.Out):
		s, ok := m.roomSession(row.id)
		switch {
		case !ok || row.kind != roomMatch:
			m.flash = rk.Out.Help().Key + " takes the selected match out of the chain"
		case r.k.isOne():
			m.flash = s.Name + " is in no chain"
		default:
			a := m.outOfTeam(s, m.roomName(r.k))
			m.pending = &a
		}
	case bkey.Matches(k, rk.Open):
		switch row.kind {
		case roomNeed, roomMatch:
			if row.kind == roomMatch && row.id == "" { // the chain has no matches: + start one here
				return m.openNew()
			}
			if s, ok := m.roomSession(row.id); ok {
				m.ghostPick, m.chose, m.selPID, m.selHost = 0, true, s.PID, s.Host
				return m.goInto(s.Host, s.Pane)
			}
		case roomMore:
			r.unfolded = true
		case roomTalk:
			r.typing = true
		case roomCard:
			if row.id == "quiet" { // ⏎ asks the quiet stoker
				r.typing = true
			}
		}
	}
	return m, nil
}

// give is y on a row: the go it waits for. Every go the stoker knows
// (merge, release, deploy, kill) acts outside the session, so each waits
// in the consent box for a typed y, with the command it lets run.
func (m Model) give(row roomRow) (tea.Model, tea.Cmd) {
	k := m.room.k
	g, ok := m.goOf(k, row)
	if !ok {
		m.flash = m.keys.room.Give.Help().Key + " gives the go a step waits for"
		return m, nil
	}
	argv := strings.Fields(g.want)
	if len(argv) == 0 {
		argv = []string{g.needs}
	}
	m.pending = &action{label: "give the go: " + g.text, steps: [][]string{argv}, destructive: true,
		rec: "stoker:grant", which: "primary", text: m.unitText(k, g.id), host: k.host}
	return m, nil
}

// talk sends the talk line to the chain's stoker: its first order starts
// it, with no sheet. One order goes at a time. A chain of one talks to the
// host's stoker, and starts it first when it is off (eng review
// correction 9).
func (m Model) talk() (tea.Model, tea.Cmd) {
	r := m.room
	words := strings.TrimSpace(r.draft)
	if words == "" || r.act != "" || r.queued {
		return m, nil
	}
	k := r.k
	if why := m.cannotSend(k.host); why != "" {
		m.flash = "not sent: " + why
		return m, nil
	}
	r.sent, r.draft = words, ""
	if k.isOne() {
		if v := m.stoker; v == nil || !v.Ready {
			cmd := m.sendTo(k.host, proto.Act{RecID: "stoker:start", Which: "primary"})
			r.queued, r.act, r.failed = true, "a"+strconv.Itoa(m.acts), ""
			return m, cmd
		}
		return m.sendRoomOrder()
	}
	body, _ := json.Marshal(struct {
		Chain  string   `json:"chain"`
		Words  string   `json:"words"`
		Grants []string `json:"grants"`
	}{k.id, words, []string{}})
	rec := "stoker:new"
	if m.hasUnit(k) {
		rec = "stoker:order-chain"
	}
	cmd := m.sendTo(k.host, proto.Act{RecID: rec, Which: "primary", Confirm: "y", Text: string(body)})
	r.act, r.starts, r.failed = "a"+strconv.Itoa(m.acts), rec == "stoker:new", ""
	return m, cmd
}

// sendRoomOrder sends a chain of one's words to the host's stoker.
func (m Model) sendRoomOrder() (Model, tea.Cmd) {
	r := m.room
	r.queued = false
	body, _ := json.Marshal(struct {
		Words string `json:"words"`
		Night bool   `json:"night"`
	}{r.sent, false})
	cmd := m.sendTo(r.k.host, proto.Act{RecID: "stoker:order", Which: "primary", Confirm: "y", Text: string(body)})
	r.act = "a" + strconv.Itoa(m.acts)
	return m, cmd
}

// roomQueued sends a chain of one's words once the host's stoker it
// started rests.
func (m Model) roomQueued(host string) (Model, tea.Cmd) {
	if m.room == nil || !m.room.queued || m.room.k.host != host {
		return m, nil
	}
	if v := m.stoker; v == nil || !v.Ready {
		return m, nil
	}
	r := *m.room
	m.room = &r
	return m.sendRoomOrder()
}

// cannotSend is why an order cannot go to host now; "" when it can.
func (m Model) cannotSend(host string) string {
	if why := m.refusal(host); why != "" {
		return why
	}
	if host == m.opt.Host && (m.conn == nil || m.lost) {
		return "no connection"
	}
	return ""
}

// roomLost frees the talk line when host's conn is lost with an order or
// a start on its way: no answer comes, so the words come back.
func (m Model) roomLost(host string) Model {
	if m.room == nil || m.room.k.host != host || m.room.act == "" && !m.room.queued {
		return m
	}
	r := *m.room
	m.room = &r
	r.act, r.queued, r.starts = "", false, false
	if r.draft == "" {
		r.draft = r.sent
	}
	return m
}

// roomResult takes the result of the talk line's order: the toast when it
// went, and the words back in the line when it was refused. A chain of
// one's start that is refused says why on the card.
func (m Model) roomResult(res proto.Result) (Model, tea.Cmd, bool) {
	if m.room == nil || m.room.act == "" || m.room.act != res.ActID {
		return m, nil, false
	}
	r := *m.room
	m.room = &r
	r.act = ""
	if r.queued { // the start's answer: the words go once the stoker is ready
		if res.Err != "" {
			r.queued, r.failed = false, res.Err
			if r.draft == "" {
				r.draft = r.sent
			}
		}
		return m, nil, true
	}
	starts := r.starts
	r.starts = false
	if res.Err != "" {
		if starts {
			r.failed = res.Err // the card says it, in neg
		} else {
			m.flash = "not sent: " + res.Err
		}
		if r.draft == "" {
			r.draft = r.sent
		}
		return m, nil, true
	}
	r.woke = r.woke || starts
	return m, m.say("sent to " + m.roomStokerWord(r.k)), true
}

// roomStokerWord names room k's stoker: "hearth's stoker", or "the stoker" for
// a chain of one.
func (m Model) roomStokerWord(k chainKey) string {
	if k.isOne() {
		return "the stoker"
	}
	return m.roomName(k) + "'s stoker"
}

// roomKeyLine is the key line in a room: the talk line's while it types,
// else the keys of the selected row.
func (m Model) roomKeyLine(w int) string {
	st, rk, r := m.st, m.keys.room, m.room
	if r.typing {
		return fit(" "+st.text.Render("typing to "+m.roomStokerWord(r.k)+" · ")+
			keyWord(rk.Open, st.accent, st.muted, " sends · ")+keyWord(rk.Back, st.accent, st.muted, " stops"), w)
	}
	rows := m.roomRows(r.k)
	var hs []hint
	if r.at < len(rows) {
		switch row := rows[r.at]; row.kind {
		case roomNeed:
			hs = append(hs, hintFor(rk.Open, "go in"))
			if _, ok := m.goOf(r.k, row); ok {
				hs = []hint{hintFor(rk.Give, "give")}
			}
		case roomMatch:
			hs = append(hs, hintFor(rk.Open, "go in"))
			if !r.k.isOne() && row.id != "" {
				hs = append(hs, hintFor(rk.Out, "out"))
			}
		case roomStep:
			if _, ok := m.goOf(r.k, row); ok {
				hs = append(hs, hintFor(rk.Give, "give"))
			}
		case roomMore:
			hs = append(hs, hintFor(rk.Open, "unfold"))
		case roomTalk:
			hs = append(hs, hintFor(rk.Open, "talk"))
		}
	}
	hs = append(hs, hintFor(rk.Next, "next"), hintFor(rk.Back, "back"))
	return fit(" "+st.muted.Render(plainHints(hs...)), w)
}
