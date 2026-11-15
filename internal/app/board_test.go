package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// boardFixture is the board's machine as the sketch draws it: fifteen
// matches of the demo fleet, four that need you, three at work, three that
// ship and five quiet; flint-auth, flint-ui and flint-api in the chain
// login; ember-billing, cinder-ci, hearth-notes and tinder-scratch on a
// second host, gpu-1. A match of screensState keeps its pane, agent and
// folder.
func boardFixture(t testing.TB) proto.State {
	st := screensState(t)
	demo := map[string]sample.Session{}
	for _, s := range st.Sessions {
		demo[s.Name] = s
	}
	match := func(name, host, agent string, ctx float64, idle time.Duration, edit func(*sample.Session)) sample.Session {
		s := boardMatch(name, 3*time.Hour)
		if d, ok := demo[name]; ok {
			d.Busy, d.Status, d.Out, d.Started = false, "idle", time.Time{}, s.Started
			d.Shells, d.Agents, d.Asks, d.Permits, d.Ask, d.Progress, d.Do, d.Said = nil, 0, false, false, nil, nil, "", ""
			s = d
		}
		s.Host, s.Agent, s.Context, s.ContextPct, s.Idle = host, agent, "known", ctx, idle
		if edit != nil {
			edit(&s)
		}
		return s
	}
	busy := func(line string) func(*sample.Session) {
		return func(s *sample.Session) { s.Busy, s.Status, s.LastLine = true, "busy", line }
	}
	says := func(line string) func(*sample.Session) {
		return func(s *sample.Session) { s.LastLine = line }
	}
	auth := match("flint-auth", "", "claude", 62, 2*time.Minute, func(s *sample.Session) {
		s.LastLine = "I can fix it in two ways. The token check is a small patch."
		s.Ask = &said.Ask{ID: "toolu_board", Questions: []said.Question{{Question: "Which way do we fix the login?",
			Options: []said.Option{{Label: "Patch the token check"}, {Label: "Move to the new session store"}}}}}
	})
	kindle := match("kindle-web", "", "claude", 48, 9*time.Minute, says("Every check passed on #4556. I did not merge: that is yours."))
	hearth := match("hearth-api", "", "claude", 31, 4*time.Minute, func(s *sample.Session) {
		s.Permits, s.LastLine = true, "The plan changes 2 records and removes none."
	})
	search := match("spark-search", "", "claude", 77, 31*time.Minute, says("Lint fails on a file I did not change. I tried twice and stopped."))
	ui := match("flint-ui", "", "claude", 40, 0, busy("Writing the test for a token that expires mid-request."))
	billing := match("ember-billing", "gpu-1", "claude", 71, 0, func(s *sample.Session) {
		s.Busy, s.Status, s.Agents, s.LastLine = true, sample.StatusAgents, 3, "Three subagents read the invoice paths."
	})
	docs := match("spark-docs", "", "claude", 22, 0, busy("Running the suite."))
	limits := match("hearth-limits", "", "claude", 55, 4*time.Minute, says("Pushed. CI runs."))
	api := match("flint-api", "", "claude", 44, 40*time.Minute, says("Opened #4560 and asked for a review."))
	cinder := match("cinder-ci", "gpu-1", "codex", 37, 2*time.Hour, says("Merged. The deploy runs."))
	notes := match("hearth-notes", "gpu-1", "claude", 18, time.Hour, says("Done."))
	reports := match("ember-reports", "", "cursor", 26, 3*time.Hour, says("Nothing left on my list."))
	wick := match("wick-mobile", "", "opencode", 33, 50*time.Minute, func(s *sample.Session) {
		s.Out, s.LastLine = demoAt.Add(time.Hour), "The plan limit stopped me."
	})
	legacy := match("kindle-legacy", "", "claude", 52, 5*time.Hour, says("Parked."))
	scratch := match("tinder-scratch", "gpu-1", "aider", 9, 2*24*time.Hour, says("Idle since Sunday."))
	ui.Started, billing.Started, docs.Started = demoAt.Add(-3*time.Hour), demoAt.Add(-2*time.Hour), demoAt.Add(-time.Hour)

	st.Sessions = []sample.Session{auth, kindle, hearth, search, ui, billing, docs, limits, api, cinder, notes, reports, wick, legacy, scratch}
	st.Children, st.IdlePanes, st.Exited = nil, nil, nil
	st.Queue = []queue.Item{waitFor(auth, queue.StateQuestion, 2*time.Minute), waitFor(hearth, queue.StatePermission, 4*time.Minute)}
	ship := func(s sample.Session, n int, kind, note string, stage int, ago time.Duration) proto.Ship {
		sh := shipFor(s, n, kind, note, stage, ago)
		sh.Host = s.Host
		return sh
	}
	waits := ship(kindle, 4556, shipWaits, "green · waits for your merge", 2, 9*time.Minute)
	waits.Merge = []string{"gh", "pr", "merge", "4556", "--squash", "-R", waits.Repo}
	st.Ships = &proto.Ships{At: demoAt, Ships: []proto.Ship{
		waits,
		ship(search, 4548, shipStuck, "CI failed · lint, twice", 1, 31*time.Minute),
		ship(limits, 4551, shipRunning, "CI · 6 of 9 jobs", 1, 4*time.Minute),
		ship(api, 4560, shipRunning, "waits for review", 1, 40*time.Minute),
		ship(cinder, 4549, shipRunning, "deploying", 3, 2*time.Hour),
	}}
	st.Holds = []proto.Hold{{SessionID: legacy.SessionID, Pane: legacy.Pane, Name: legacy.Name, By: holdByYou}}
	st.Chains = &state.ChainsView{Chains: []chains.Chain{{ID: "c1", Name: "login", Repo: "/home/user/src/hearth",
		Members: []chains.Member{memberOf(auth), memberOf(ui), memberOf(api)}}}}
	return st
}

// boardShot is the console at w by h with the board on, on the clock of
// demoAt: st's matches of this machine, and gpu-1's when st has some. edit
// changes the model before the frame is drawn.
func boardShot(t testing.TB, st proto.State, w, h int, edit func(*Model)) Model {
	t.Helper()
	home, gpu := st, st
	home.Sessions, home.Queue, gpu.Sessions, gpu.Queue = nil, nil, nil, nil
	home.Ships, gpu.Ships = &proto.Ships{At: demoAt}, &proto.Ships{At: demoAt}
	gpu.Holds, gpu.Chains, gpu.Recommendations = nil, nil, nil
	for _, s := range st.Sessions {
		if s.Host == "gpu-1" {
			s.Host = ""
			gpu.Sessions = append(gpu.Sessions, s)
		} else {
			home.Sessions = append(home.Sessions, s)
		}
	}
	for _, it := range st.Queue {
		if it.Host == "gpu-1" {
			it.Host = ""
			gpu.Queue = append(gpu.Queue, it)
		} else {
			home.Queue = append(home.Queue, it)
		}
	}
	if st.Ships != nil {
		for _, sh := range st.Ships.Ships {
			if sh.Host == "gpu-1" {
				sh.Host = ""
				gpu.Ships.Ships = append(gpu.Ships.Ships, sh)
			} else {
				home.Ships.Ships = append(home.Ships.Ships, sh)
			}
		}
	}
	opt := Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }}
	peer := newFake()
	if len(gpu.Sessions) > 0 {
		opt.Peers = []Peer{{Host: "gpu-1", Conn: peer}}
	}
	var next tea.Model = (New(opt))
	next, _ = next.Update(tea.WindowSizeMsg{Width: w, Height: h})
	next, _ = next.Update(stateMsg(home))
	if len(gpu.Sessions) > 0 {
		next, _ = next.Update(peerMsg{host: "gpu-1", conn: peer, msg: stateMsg(gpu)})
	}
	m := next.(Model)
	m.splashDone = true
	// The screens are checked: the copy check's door has its own test.
	m.viewPrefs.CopyCheckedPhone, m.viewPrefs.CopyCheckedDesk = true, true
	m.toList()
	if edit != nil {
		edit(&m)
	}
	return m
}

// chooseCard chooses the session named name in the list, the keys in the
// list.
func chooseCard(name string) func(*Model) {
	return func(m *Model) {
		for _, s := range m.all.sessions {
			if s.Name == name {
				m.chooseRow(s)
				m.toList()
				return
			}
		}
		panic(name + " is in no row")
	}
}

func frameLines(m Model) []string { return strings.Split(m.render(), "\n") }

func plainFrame(m Model) string { return ansi.Strip(m.render()) }
