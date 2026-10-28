package service

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/blox-eng/matchblox/internal/ships"

	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/stoker"
)

// drivenState keeps driven.json, the panes the stoker drove in the run on,
// for the merge guard. Memory holds the truth and the file follows it, so
// a lost or broken file is written again.
type drivenState struct {
	mu sync.Mutex
	d  *stoker.Driven // nil: not read since the service started
}

// runningOrder is the order that runs. Nil: none.
func runningOrder(m stoker.Mode) *stoker.Order {
	if o := m.Order; o != nil && o.State == stoker.OrderRunning {
		return o
	}
	return nil
}

// drivenRun is the order driven.json is for, and the goes it gives.
func drivenRun(m stoker.Mode) (string, []string) {
	if o := runningOrder(m); o != nil {
		return o.ID, o.Grants
	}
	return "since-" + m.Since.UTC().Format(time.RFC3339Nano), nil
}

// updateDriven changes the driven panes; fn tells if it changed them. While
// the stoker is off there is no file and fn does not run. When another
// order runs, the panes the stoker drove stay driven with that order's
// goes; the goes given to steps are gone with their plan.
func (s *Service) updateDriven(fn func(d *stoker.Driven, grants []string) bool) error {
	if s.Stoker == nil {
		return nil
	}
	owns := s.owns()
	ds := &s.driven
	ds.mu.Lock()
	defer ds.mu.Unlock()
	dir := s.Stoker.Dir()
	m := s.Stoker.Mode()
	if !m.On {
		ds.d = &stoker.Driven{}
		return stoker.RemoveDriven(dir)
	}
	if ds.d == nil {
		d, err := stoker.ReadDriven(dir)
		if err != nil {
			d = s.drivenOfLog(m)
		}
		ds.d = &d
	}
	d := ds.d
	run, grants := drivenRun(m)
	changed := false
	if d.Run != run {
		for pane := range d.Panes {
			d.Panes[pane] = slices.Clone(grants)
		}
		d.Run, d.Steps, changed = run, nil, true
	}
	if d.Panes == nil {
		d.Panes = map[string][]string{}
	}
	if d.Steps == nil {
		d.Steps = map[string][]string{}
	}
	if sock := s.stokerSocket(); d.StokerSocket != sock {
		d.StokerSocket, changed = sock, true
	}
	// Every driven pane carries the order's goes: a new order by day
	// changes them at once.
	for pane, has := range d.Panes {
		if !slices.Equal(has, grants) {
			d.Panes[pane], changed = slices.Clone(grants), true
		}
	}
	if fn(d, grants) {
		changed = true
	}
	if next := ownOf(d, owns); !reflect.DeepEqual(next, d.Own) {
		d.Own, changed = next, true
	}
	if !changed {
		if _, err := stoker.ReadDriven(dir); err == nil {
			return nil
		}
	}
	return stoker.WriteDriven(dir, *d)
}

// ownOf is Own for the driven panes whose goes name a session.
func ownOf(d *stoker.Driven, owns map[string]stoker.Own) map[string]stoker.Own {
	names := func(grants []string) bool {
		return slices.ContainsFunc(grants, func(g string) bool { return strings.HasPrefix(g, "merge:@") })
	}
	var out map[string]stoker.Own
	for pane := range owns {
		if names(d.Panes[pane]) || names(d.Steps[pane]) {
			if out == nil {
				out = map[string]stoker.Own{}
			}
			out[pane] = owns[pane]
		}
	}
	return out
}

// owns is, for each pane of the last sample, the names of the session it
// runs and the pull requests the ships poll says that session drives: one
// it created, pushed or works on the branch of. Running gh pr on a PR is
// no claim to it: a merge the guard denied would make it the session's.
func (s *Service) owns() map[string]stoker.Own {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]stoker.Own{}
	for _, se := range s.cur.Sessions {
		o := stoker.Own{Names: []string{se.Pane}}
		for _, n := range []string{se.Name, se.Target} {
			if n != "" && !slices.Contains(o.Names, n) {
				o.Names = append(o.Names, n)
			}
		}
		if s.cur.Ships != nil {
			for _, sh := range s.cur.Ships.Ships {
				// Stricter than ships.Drives: by id whenever the ship has one, even
				// while the session's id is not read yet, since a pane can change
				// hands between two ships reads and a merge must not follow it.
				mine := sh.Session.ID != "" && sh.Session.ID == se.SessionID || sh.Session.ID == "" && sh.Session.Pane != "" && sh.Session.Pane == se.Pane
				if mine && sh.Number > 0 && sh.Evidence != ships.Acted && !slices.Contains(o.PRs, sh.Number) {
					o.PRs = append(o.PRs, sh.Number)
				}
			}
		}
		slices.Sort(o.PRs)
		out[se.Pane] = o
	}
	return out
}

// stokerSocket is the socket of the stoker's tmux server, as the last
// sample listed it, else where tmux puts it: a hook on that server is the
// stoker's in any pane. Empty: no stoker agent.
func (s *Service) stokerSocket() string {
	if s.StokerServer == "" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.cur.Panes {
		if server, _ := sample.SplitPane(p.ID); server == s.StokerServer && p.Socket != "" {
			return p.Socket
		}
	}
	return sample.SocketPath(s.StokerServer)
}

// drivenOfLog are the panes the run's log says the stoker sent a message
// to: what a service that lost driven.json and its memory of it knows. A
// pane the builder freed since is driven again, which fails closed.
func (s *Service) drivenOfLog(m stoker.Mode) stoker.Driven {
	d := stoker.Driven{Panes: map[string][]string{}}
	for _, e := range s.Stoker.Entries() {
		if e.Ordered() && !e.At.Before(m.Since) && e.Pane != "" {
			d.Panes[e.Pane] = nil
		}
	}
	return d
}

// agentsPane tells if the act comes from the stoker's tmux session or from
// a pane the stoker drove in the run on. It reads the service's memory,
// not driven.json, so a broken file frees no pane.
func (s *Service) agentsPane(a proto.Act) bool {
	if a.From == "" || s.Stoker == nil {
		return false
	}
	if s.fromStoker(a) {
		return true
	}
	driven := false
	_ = s.updateDriven(func(d *stoker.Driven, _ []string) bool {
		_, driven = d.Panes[a.From]
		return false
	})
	return driven
}

// syncDriven writes driven.json for the run on, or removes it when the run
// ended.
func (s *Service) syncDriven() {
	_ = s.updateDriven(func(*stoker.Driven, []string) bool { return false })
}

// drive marks a pane the stoker sent a message to, with the order's goes.
func (s *Service) drive(pane string) {
	_ = s.updateDriven(func(d *stoker.Driven, grants []string) bool {
		if _, in := d.Panes[pane]; in {
			return false
		}
		d.Panes[pane] = slices.Clone(grants)
		return true
	})
}

// undrive lets go of a pane the builder typed into: its last input is no
// longer the stoker's.
func (s *Service) undrive(pane string) {
	_ = s.updateDriven(func(d *stoker.Driven, _ []string) bool {
		_, in := d.Panes[pane]
		_, step := d.Steps[pane]
		delete(d.Panes, pane)
		delete(d.Steps, pane)
		return in || step
	})
}

// grantAct gives the go a waiting step needs (stoker:grant, Text: the
// id of the step's entry): its merge, release or deploy in that step's
// pane, once. The stoker hears of it at its next wake.
func (s *Service) grantAct(a proto.Act) proto.Result {
	if s.Stoker == nil {
		return proto.Result{Err: "this service keeps no stoker"}
	}
	if s.fromStoker(a) {
		return proto.Result{Err: "the stoker does not give itself the go"}
	}
	if err := s.grant(strings.TrimSpace(a.Text)); err != nil {
		return proto.Result{Err: err.Error()}
	}
	s.refreshStoker()
	return proto.Result{}
}

func (s *Service) grant(id string) error {
	s.grantMu.Lock()
	defer s.grantMu.Unlock()
	m := s.Stoker.Mode()
	if !m.On {
		return errors.New("the stoker is off: no run")
	}
	run := s.Stoker.View().Run
	i := slices.IndexFunc(run, func(e stoker.Entry) bool { return e.ID == id })
	if i < 0 {
		return errors.New("the run has no such step")
	}
	last := run[i]
	if last.Rule == stoker.RuleGo {
		last = stoker.LastGo(run, last)
	} else if last.Step > 0 {
		// A step of the plan: its last state in the run decides.
		for _, e := range run[i:] {
			if e.Logged() && e.Step == last.Step && e.Rule == "" {
				last = e
			}
		}
	}
	switch {
	case last.State != stoker.StepWaits || last.Needs == "":
		return errors.New("the step waits for no go")
	case last.Pane == "":
		return errors.New("the step names no pane")
	}
	err := s.updateDriven(func(d *stoker.Driven, _ []string) bool {
		// A plan step's y given twice is one go. A go step is done once
		// given, so each of its y is a go of its own: a line with two
		// guarded commands asks twice.
		if last.Rule != stoker.RuleGo && slices.Contains(d.Steps[last.Pane], last.Needs) {
			return false
		}
		d.Steps[last.Pane] = append(d.Steps[last.Pane], last.Needs)
		return true
	})
	if err != nil {
		return err
	}
	what := last.Needs
	if last.Rule == stoker.RuleGo {
		what = last.Want
		now := s.now()
		given := last
		given.ID, given.At, given.State = "go@"+now.UTC().Format(time.RFC3339Nano), now, stoker.StepDone
		given.Text = "go given: " + last.Want
		if err := s.Stoker.Append(given); err != nil {
			return err
		}
	}
	s.wakes.add(fmt.Sprintf("go given for %s: %s", last.Name, what))
	return nil
}

// askGo puts a guarded command the guard denied in a pane the stoker
// drives on the card, as a step that waits for the builder's y: the go
// for one command of that kind, once, in that pane. A step for the same go
// that still waits is not added twice.
func (s *Service) askGo(ev hooks.Event) {
	if s.Stoker == nil || ev.Name != "PreToolUse" || !ev.Denied || ev.Pane == "" || !s.Stoker.Mode().On {
		return
	}
	var needs []string
	if server, _ := sample.SplitPane(ev.Pane); s.StokerServer != "" && server == s.StokerServer {
		return // the stoker's own panes never get a go
	}
	_ = s.updateDriven(func(d *stoker.Driven, _ []string) bool {
		_, driven := d.Panes[ev.Pane]
		if !driven || slices.Contains(ev.Guards, stoker.Guard{What: stoker.Unread}) {
			return false
		}
		_, short := d.Spend(ev.Pane, ev.Guards)
		for _, g := range short {
			needs = append(needs, stoker.NeedOf(g, ev.Pane))
		}
		return false
	})
	if len(needs) == 0 {
		return
	}
	x, _ := s.sessionAt(ev.Pane)
	name := x.Name
	if name == "" {
		name = ev.Pane
	}
	want := firstOf(ev.Command)
	run := s.Stoker.View().Run
	now := s.now()
	added := false
	for _, n := range needs {
		e := stoker.Entry{
			ID: "go@" + now.UTC().Format(time.RFC3339Nano) + "/" + n, At: now, Session: x.SessionID, Name: name, Pane: ev.Pane,
			Rule: stoker.RuleGo, State: stoker.StepWaits, Needs: n, Want: want, Argv: [][]string{{want}},
			Text: name + " wants to run " + want,
		}
		if stoker.LastGo(run, e).State == stoker.StepWaits {
			continue
		}
		if s.Stoker.Append(e) == nil {
			run, added = append(run, e), true
		}
	}
	if added {
		s.refreshStoker()
	}
}

// firstOf is the first line of a shell line, trimmed.
func firstOf(cmd string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(cmd), "\n")
	return strings.TrimSpace(first)
}

// useGrants takes out the steps' goes a call the guard let through used:
// a go given with y is good for one guarded command. The order's goes
// last the run.
// The guards are the ones the hook read in the raw line: the event's
// command is scrubbed and may no longer read as what ran.
func (s *Service) useGrants(ev hooks.Event) {
	gs := ev.Guards
	if ev.Name != "PreToolUse" || len(gs) == 0 || ev.Denied {
		return
	}
	_ = s.updateDriven(func(d *stoker.Driven, _ []string) bool {
		left, _ := d.Spend(ev.Pane, gs)
		if len(left) == len(d.Steps[ev.Pane]) {
			return false
		}
		d.Steps[ev.Pane] = left
		return true
	})
}
