package app

import (
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
type roomKeys struct{ Up, Down, Open, Give, Next, Prev, Back, Erase bkey.Binding }

func (k roomKeys) bindings() []bkey.Binding {
	return []bkey.Binding{k.Up, k.Down, k.Open, k.Give, k.Next, k.Prev, k.Back, k.Erase}
}

// goWait is a step of a stoker's run that waits for the owner's go: the
// entry's id, what it says, the go it needs and the command it runs.
type goWait struct {
	id, text, needs, want string
	at                    int64
}

// goWaits are the steps room k's stoker waits on a go for: the last entry
// of each step, when it waits and names its go.
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
	for _, e := range v.Run {
		if e.Step == 0 {
			continue
		}
		if _, seen := last[e.Step]; !seen {
			steps = append(steps, e.Step)
		}
		last[e.Step] = e
	}
	var out []goWait
	for _, n := range steps {
		if e := last[n]; e.State == "waits" && e.Needs != "" {
			out = append(out, goWait{id: e.ID, text: e.Text, needs: e.Needs, want: e.Want, at: e.At.UnixNano()})
		}
	}
	return out
}

// unitText is what stoker:grant names a step of room k by: the unit's id
// and the entry's, or the entry's alone for the host's stoker.
func (m Model) unitText(k chainKey, id string) string {
	if u, ok := m.unitOf(k); ok && !k.isOne() {
		return u.ID + " " + id
	}
	return id
}

// actsOutside tells if a go runs a command that acts outside the session:
// a merge, a kill, a force-push or a removal. It goes through the consent
// box with its exact argv.
func actsOutside(g goWait) bool {
	switch {
	case strings.HasPrefix(g.needs, "merge"), g.needs == "kill":
		return true
	}
	for _, w := range []string{"--force", "push -f", "remove", "rm "} {
		if strings.Contains(g.want, w) {
			return true
		}
	}
	return false
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
	case bkey.Matches(k, rk.Open):
		switch row.kind {
		case roomNeed, roomMatch:
			if s, ok := m.roomSession(row.id); ok {
				m.ghostPick, m.chose, m.selPID, m.selHost = 0, true, s.PID, s.Host
				return m.goInto(s.Host, s.Pane)
			}
		case roomMore:
			r.unfolded = true
		case roomTalk:
			r.typing = true
		}
	}
	return m, nil
}

// give is y on a row: the go its step waits for. A go that acts outside
// the session waits in the consent box for a typed y.
func (m Model) give(row roomRow) (tea.Model, tea.Cmd) {
	k := m.room.k
	i := slices.IndexFunc(m.goWaits(k), func(g goWait) bool { return g.id == row.id })
	if i < 0 || row.kind != roomNeed && row.kind != roomStep {
		m.flash = m.keys.room.Give.Help().Key + " gives the go a step waits for"
		return m, nil
	}
	g := m.goWaits(k)[i]
	text := m.unitText(k, g.id)
	if actsOutside(g) {
		argv := strings.Fields(g.want)
		if len(argv) == 0 {
			argv = []string{g.needs}
		}
		m.pending = &action{label: "give the go: " + g.text, steps: [][]string{argv}, destructive: true,
			rec: "stoker:grant", which: "primary", text: text, host: k.host}
		return m, nil
	}
	m.flash = "go given: " + g.text
	return m, m.sendTo(k.host, proto.Act{RecID: "stoker:grant", Which: "primary", Confirm: "y", Text: text})
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
	r.sent, r.draft = words, ""
	if k.isOne() {
		if v := m.stoker; v == nil || !v.Ready {
			r.queued = true
			return m, m.sendTo(k.host, proto.Act{RecID: "stoker:start", Which: "primary"})
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
	r.act = "a" + strconv.Itoa(m.acts)
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

// roomResult takes the result of the talk line's order: the toast when it
// went, and the words back in the line when it was refused.
func (m Model) roomResult(res proto.Result) (Model, tea.Cmd, bool) {
	if m.room == nil || m.room.act == "" || m.room.act != res.ActID {
		return m, nil, false
	}
	r := *m.room
	m.room = &r
	r.act = ""
	if res.Err != "" {
		m.flash = "not sent: " + res.Err
		if r.draft == "" {
			r.draft = r.sent
		}
		return m, nil, true
	}
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
			if slices.ContainsFunc(m.goWaits(r.k), func(g goWait) bool { return g.id == row.id }) {
				hs = []hint{hintFor(rk.Give, "give")}
			}
		case roomMatch:
			hs = append(hs, hintFor(rk.Open, "go in"))
		case roomStep:
			if slices.ContainsFunc(m.goWaits(r.k), func(g goWait) bool { return g.id == row.id }) {
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
