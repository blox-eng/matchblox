package stoker

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/ships"
)

// Seen is what the stoker agent was last told about: when each session's
// turn ended (by pane, zero while one runs), the stage of each ship, and
// each plan window's use in tens of percent. Who and Why hold the words of
// the lines: the session that drives a ship, and why a session is idle.
type Seen struct {
	Turns  map[string]time.Time
	Stages map[string]int // repo#n -> stage
	Usage  map[string]int // provider/account/window -> pct/10
	Who    map[string]string
	Why    map[string]string
	Names  map[string]string // pane -> session name
	Busy   map[string]bool   // pane -> a turn runs
	Asks   map[string]string // pane -> the first line of what it asks
	Said   map[string]string // pane -> the last line it said
}

var stageNames = [...]string{"open", "ci", "green", "merged", "live"}

// Look is what the sample shows now. A session of the stoker's own tmux
// server, and a ship it drives, are left out: the stoker is not told of
// itself. why is the idle reason of each pane, items the queue.
func Look(sessions []sample.Session, why map[string]string, items []queue.Item, rep *ships.Report, accts []limits.Account, now time.Time, stokerServer string) Seen {
	cur := Seen{Turns: map[string]time.Time{}, Stages: map[string]int{}, Usage: map[string]int{}, Who: map[string]string{}, Why: why,
		Names: map[string]string{}, Busy: map[string]bool{}, Asks: map[string]string{}, Said: map[string]string{}}
	own, byID := map[string]bool{}, map[string]string{}
	for _, x := range sessions {
		if x.SessionID != "" {
			byID[x.SessionID] = x.Name
		}
		if x.Pane == "" {
			continue
		}
		if stokerServer != "" && x.Server() == stokerServer {
			own[x.Pane] = true
			continue
		}
		cur.Turns[x.Pane] = x.TurnEnded
		cur.Names[x.Pane] = x.Name
		cur.Busy[x.Pane] = x.Busy
		p := PeekOf(x, items)
		cur.Asks[x.Pane], cur.Said[x.Pane] = p.Asks, lastLine(p.Said)
	}
	if rep != nil {
		for _, sh := range rep.Ships {
			if own[sh.Session.Pane] {
				continue
			}
			k := fmt.Sprintf("%s#%d", sh.Repo, sh.Number)
			who := sh.Session.Name
			if who == "" {
				who = cur.Names[sh.Session.Pane]
			}
			if who == "" {
				who = byID[sh.Session.ID]
			}
			cur.Stages[k], cur.Who[k] = sh.Stage, who
		}
	}
	for _, a := range accts {
		for _, u := range a.Uses(now) {
			if u.Name == "5h" || u.Name == "wk" {
				cur.Usage[a.Provider+"/"+a.Account+"/"+u.Name] = int(u.Pct) / 10
			}
		}
	}
	return cur
}

// Changes are the lines for what happened between prev and cur: a session
// that asks something (with the first line of it), a turn that ended
// (with why it is idle, or the last line it said), a pull request that
// appeared or moved to a stage, a plan window that passed another ten
// percent. What a session said is quoted: it is a report, not an order
// (Ruling AV). What prev did not know is not a change (a run seeds it first),
// and the stoker's own sessions are skipped.
func Changes(prev, cur Seen, sessions []sample.Session, stokerServer string) []string {
	var out []string
	for _, x := range sessions {
		if x.Pane == "" || stokerServer != "" && x.Server() == stokerServer {
			continue
		}
		now, ok := cur.Turns[x.Pane]
		was, known := prev.Turns[x.Pane]
		// An agent whose transcript says nothing about turns (Codex,
		// OpenCode) ends a turn when it stops being busy.
		edge := now.IsZero() && was.IsZero() && prev.Busy[x.Pane] && !cur.Busy[x.Pane]
		if !ok || !known {
			continue
		}
		ended := edge || now.After(was)
		if q := cur.Asks[x.Pane]; q != "" && (ended || q != prev.Asks[x.Pane]) {
			out = append(out, fmt.Sprintf("%s asks: %s", x.Name, strconv.QuoteToGraphic(q)))
			continue
		}
		if !ended {
			continue
		}
		why := cur.Why[x.Pane]
		switch {
		case why != "" && why != "idle":
		case cur.Said[x.Pane] != "":
			why = "it said " + strconv.QuoteToGraphic(cur.Said[x.Pane])
		default:
			why = "idle"
		}
		out = append(out, fmt.Sprintf("%s's turn ended: %s", x.Name, why))
	}
	for _, k := range sortedKeys(cur.Stages) {
		_, num, _ := strings.Cut(k, "#")
		was, known := prev.Stages[k]
		switch st := cur.Stages[k]; {
		case !known && cur.Who[k] == "":
			out = append(out, "new PR #"+num)
		case !known:
			out = append(out, fmt.Sprintf("new PR #%s by %s", num, cur.Who[k]))
		case st != was && st >= 0 && st < len(stageNames):
			out = append(out, fmt.Sprintf("#%s moved to %s", num, stageNames[st]))
		}
	}
	for _, k := range sortedKeys(cur.Usage) {
		was, known := prev.Usage[k]
		if !known || cur.Usage[k] <= was {
			continue
		}
		parts := strings.Split(k, "/")
		window := "5h"
		if parts[len(parts)-1] == "wk" {
			window = "week"
		}
		out = append(out, fmt.Sprintf("%s %s passed %d%%", parts[0], window, cur.Usage[k]*10))
	}
	for i := range out {
		out[i] = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, out[i])
	}
	return out
}

// Coalesce says each thing of a wake once: a line that repeats, and the
// lines of one session's turns or questions, become one line with a count
// (`api's turn ended ×3: <the newest words>`), where the first of them was.
func Coalesce(lines []string) []string {
	type group struct {
		head, rest string
		n          int
	}
	var groups []*group
	byKey := map[string]*group{}
	for _, l := range lines {
		head, rest := l, ""
		if h, r, ok := strings.Cut(l, ": "); ok && (strings.HasSuffix(h, "'s turn ended") || strings.HasSuffix(h, " asks")) {
			head, rest = h, r
		}
		g := byKey[head]
		if g == nil {
			g = &group{head: head}
			byKey[head] = g
			groups = append(groups, g)
		}
		g.rest = rest
		g.n++
	}
	out := make([]string, 0, len(groups))
	for _, g := range groups {
		l := g.head
		if g.n > 1 {
			l += fmt.Sprintf(" ×%d", g.n)
		}
		if g.rest != "" {
			l += ": " + g.rest
		}
		out = append(out, l)
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
