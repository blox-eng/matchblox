package push

import (
	"math"
	"slices"
	"time"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/holds"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// MaxAccounts is how many accounts a content state carries, which keeps it
// under 2 KB.
const MaxAccounts = 4

// Build is the content state of one state.Doc: the counts, the session the
// island leads with, the usage of each account, and the stoker. It holds
// numbers, states, times and percents only.
// chainState is a chain's word for its neediest session's: a wait asks, a
// finish waits for the owner to look.
var chainState = map[string]string{"waits": "asks", "done": "waits", "out": "quiet", "burns": "burns", "idle": "quiet", "parked": "quiet"}

// chainOf is the chain a session is in; its first when it is in several.
func chainOf(v *state.ChainsView, sessionID string) (chains.Chain, bool) {
	if v == nil || sessionID == "" {
		return chains.Chain{}, false
	}
	for _, c := range v.Chains {
		for _, m := range c.Members {
			if m.SessionID == sessionID {
				return c, true
			}
		}
	}
	return chains.Chain{}, false
}

func Build(doc state.Doc, hostFP string, n *Numbers, now time.Time) ContentState {
	cs := ContentState{Host: Host8(hostFP), Accounts: []AccountUse{}}
	if doc.Stoker != nil {
		cs.Stoker = doc.Stoker.On
	}

	// What each session does, by id. A queue item is the truth for a wait or
	// a finish; a session in neither is out (a limit stops it), burns, or is idle.
	type cand struct {
		s     sample.Session
		state string
		since time.Time
	}
	// A session you parked needs nothing from you: it counts in no wait and
	// comes after every other session (owner, 2026-11-27).
	parked := map[string]bool{}
	for _, h := range doc.Holds {
		if h.By == holds.ByYou {
			parked[h.SessionID] = true
		}
	}
	items := map[string]queue.Item{}
	for _, it := range doc.Queue {
		if it.State != queue.StatePermission && it.State != queue.StateQuestion && it.State != queue.StateFinished || parked[it.SessionID] {
			continue
		}
		if old, ok := items[it.SessionID]; !ok || waits(it) && !waits(old) || waits(it) == waits(old) && it.Since.Before(old.Since) {
			items[it.SessionID] = it
		}
		if waits(it) {
			cs.Wait++
			if it.Since.After(cs.WaitAt) {
				cs.WaitAt = it.Since.UTC()
			}
		}
	}
	var cands []cand
	for _, s := range doc.Sessions {
		if s.SessionID == "" {
			continue
		}
		c := cand{s: s}
		switch it, ok := items[s.SessionID]; {
		case ok && waits(it):
			c.state, c.since = "waits", it.Since
		case ok:
			c.state, c.since = "done", it.Since
		case !s.Out.IsZero():
			c.state = "out"
		case s.Busy:
			c.state = "burns"
		default:
			c.state = "idle" // alive: the match stays lit (owner, 2026-11-27)
		}
		if parked[s.SessionID] {
			c.state = "parked"
		}
		if c.state == "burns" {
			cs.Burn++
		}
		cands = append(cands, c)
	}
	// A wait before a done before an out before a burn before an idle before a
	// parked one; of the waits and dones the one that changed last (owner,
	// 2026-11-27), of the others the one prompted last.
	rank := map[string]int{"waits": 0, "done": 1, "out": 2, "burns": 3, "idle": 4, "parked": 5}
	slices.SortStableFunc(cands, func(a, b cand) int {
		if a.state != b.state {
			return rank[a.state] - rank[b.state]
		}
		if a.state == "waits" || a.state == "done" {
			return b.since.Compare(a.since)
		}
		return b.s.Started.Compare(a.s.Started)
	})
	needKind := map[string]string{queue.StatePermission: "command", queue.StateQuestion: "question", queue.StateFinished: "done"}
	for _, c := range cands {
		if c.state != "waits" && c.state != "done" {
			break // the sort puts them first
		}
		cs.Need++
		if len(cs.Needs) < MaxNeeds {
			cs.Needs = append(cs.Needs, Need{N: n.Session(c.s.SessionID), Kind: needKind[items[c.s.SessionID].State]})
		}
	}
	if len(cands) > 0 {
		c := cands[0]
		st := c.state
		if st == "parked" {
			st = "idle" // alive, and nothing for you to do
		}
		top := &Top{N: n.Session(c.s.SessionID), State: st, Out: c.s.Out.UTC()}
		if c.s.Progress != nil {
			top.Pct = step5(c.s.Progress.Pct)
		}
		cs.Top = top
	}
	// The neediest session's chain; none when it is in no chain, and the
	// island shows that session alone.
	if len(cands) > 0 {
		if ch, ok := chainOf(doc.Chains, cands[0].s.SessionID); ok {
			tc := &TopChain{N: n.Chain(ch.ID), State: chainState[cands[0].state]}
			for _, u := range doc.Stokers {
				if u.View.On {
					tc.Stokers++
				}
				if u.Chain == ch.ID {
					tc.Done, tc.Steps = u.View.PlanProgress()
				}
			}
			cs.Chain = tc
		}
	}

	for _, a := range doc.Limits {
		works := slices.ContainsFunc(doc.Sessions, func(s sample.Session) bool {
			return s.Busy && s.Provider == a.Provider && s.Account == a.Account && s.AccountDir == a.Dir
		})
		au := AccountUse{N: n.Account(a.Provider, a.Account), Lasts: true, Idle: !works}
		for _, u := range a.Uses(now) {
			if u.Name != "5h" && u.Name != "wk" && u.Name != "mo" {
				continue // a window the island has no name for
			}
			pct := min(100, max(0, int(math.Round(u.Pct))))
			au.Windows = append(au.Windows, WindowUse{Name: u.Name, Pct: pct, Resets: u.Resets.UTC(), Spent: pct >= 100 && works})
		}
		if len(au.Windows) == 0 {
			continue
		}
		if f := a.Forecast; f != nil && !f.Lasts {
			au.Lasts, au.RunsOut = false, f.Out.UTC()
		}
		cs.Accounts = append(cs.Accounts, au)
	}
	slices.SortFunc(cs.Accounts, func(a, b AccountUse) int { return a.N - b.N })
	if len(cs.Accounts) > MaxAccounts {
		cs.Accounts = cs.Accounts[:MaxAccounts]
	}
	return cs
}

func waits(it queue.Item) bool {
	return it.State == queue.StatePermission || it.State == queue.StateQuestion
}

// step5 rounds a percent down to a multiple of 5, within 0..100.
func step5(p int) int { return min(100, max(0, p)) / 5 * 5 }
