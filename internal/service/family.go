package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/stoker"
)

// stokerUnit is one stoker of the family: the host stoker, or the stoker
// of one chain. Each has its store, its tmux session and its driven panes.
type stokerUnit struct {
	id string
	// link is the unit's link.json. Chain is empty for the host. An order
	// changes Giver and Grants, and a join Chain, under stokerFamily.mu:
	// read Chain with chainIDOf.
	link  stoker.Link
	store *stoker.Store
	// startedAt is when matchblox started the unit's agent; zero once it
	// is ready, or gave up on it. Service.mu guards it.
	startedAt time.Time
	// driven are the panes the unit drove, for the merge guard.
	driven drivenState
	// brief are the lines still to type into a chain unit's new agent once
	// it rests, its brief first, and briefAt when they were queued. dropped
	// are the lines of an agent that did not rest in time: an order queues
	// them again first. Service.mu guards the three.
	brief   []string
	briefAt time.Time
	dropped []string
	// joins are the tmux sessions a chain unit started, which join its
	// chain once a sample finds them in its repo. Service.mu guards it.
	joins []*pendingJoin
	// wakes are what the unit's agent was told and is still to be told.
	wakes wakeState
}

// pendingJoin is a session a chain unit started, not in its chain yet: the
// pane and tmux session the start made, and the agent's session id once a
// sample finds it.
type pendingJoin struct {
	pane, name, sessionID string
	at                    time.Time
}

// stokerFamily holds the units. The host unit wraps Service.Stoker, so
// what sets that field sets the host's store.
type stokerFamily struct {
	// start is held through a chain unit's start, so two starts never
	// take one id or pass the cap together.
	start  sync.Mutex
	mu     sync.Mutex
	host   stokerUnit
	chains []*stokerUnit // by id
}

// hostUnit is the unit of the host stoker. Nil: the service keeps no
// stoker.
func (s *Service) hostUnit() *stokerUnit {
	if s.Stoker == nil {
		return nil
	}
	f := &s.fam
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.host.store != s.Stoker { // written only on a change: readers hold no lock
		f.host.store = s.Stoker
	}
	return &f.host
}

// family is every unit: the host first, then the chain units by id.
func (s *Service) family() []*stokerUnit {
	var out []*stokerUnit
	if h := s.hostUnit(); h != nil {
		out = append(out, h)
	}
	s.fam.mu.Lock()
	defer s.fam.mu.Unlock()
	return append(out, s.fam.chains...)
}

// unitByID is the unit of a stoker id. Nil: none is loaded.
func (s *Service) unitByID(id string) *stokerUnit {
	if id == stoker.HostID {
		return s.hostUnit()
	}
	s.fam.mu.Lock()
	defer s.fam.mu.Unlock()
	for _, u := range s.fam.chains {
		if u.id == id {
			return u
		}
	}
	return nil
}

// unitOfChain is the unit of a chain's stoker. Nil: the chain has none.
func (s *Service) unitOfChain(chainID string) *stokerUnit {
	if chainID == "" {
		return nil
	}
	s.fam.mu.Lock()
	defer s.fam.mu.Unlock()
	for _, u := range s.fam.chains {
		if u.link.Chain == chainID {
			return u
		}
	}
	return nil
}

// unitOf is the unit whose tmux session the act was typed in. stokerPane
// tells if the pane is on the stoker's tmux server at all: the server
// decides, not the agent list, and the builder's pane of the same id never
// is. A stoker pane with no unit is an unknown stoker session, and fails
// closed: a session the sample does not name yet, while chain units run,
// or a session no loaded unit runs.
func (s *Service) unitOf(a proto.Act) (u *stokerUnit, stokerPane bool) {
	if s.StokerServer == "" || a.From == "" {
		return nil, false
	}
	if server, _ := sample.SplitPane(a.From); server != s.StokerServer {
		return nil, false
	}
	name, known := s.tmuxSessionOf(a.From)
	if !known {
		s.fam.mu.Lock()
		chains := len(s.fam.chains)
		s.fam.mu.Unlock()
		if chains > 0 {
			return nil, true
		}
		return s.hostUnit(), true
	}
	id, ok := stoker.IDOfSession(name)
	if !ok {
		return nil, true
	}
	return s.unitByID(id), true
}

// tmuxSessionOf is the tmux session of a pane, as the last sample listed
// it. False: the sample names no session for the pane.
func (s *Service) tmuxSessionOf(pane string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name := ""
	for _, p := range s.cur.Panes {
		if p.ID == pane {
			name, _, _ = strings.Cut(p.Target, ":")
		}
	}
	if x, ok := s.sessionAtLocked(pane); ok && name == "" {
		name = x.TmuxSession()
	}
	return name, name != ""
}

// loadFamily reads the chain units from stokers/ in the host's state dir:
// each store and its link.json. A unit already loaded keeps its memory. A
// unit that does not read is left out, and its error returned.
func (s *Service) loadFamily() error {
	h := s.hostUnit()
	if h == nil {
		return nil
	}
	state := h.store.Dir()
	ids, err := stoker.FamilyIDs(state)
	if err != nil {
		return err
	}
	var errs []error
	units := make([]*stokerUnit, 0, len(ids))
	for _, id := range ids {
		if u := s.unitByID(id); u != nil {
			units = append(units, u)
			continue
		}
		dir := stoker.FamilyDir(state, id)
		st, err := stoker.Open(dir)
		if err != nil {
			errs = append(errs, fmt.Errorf("stoker %s: %w", id, err))
			continue
		}
		l, err := stoker.ReadLink(dir)
		if err != nil {
			errs = append(errs, fmt.Errorf("stoker %s: %w", id, err))
			continue
		}
		units = append(units, &stokerUnit{id: id, link: l, store: st})
	}
	s.fam.mu.Lock()
	s.fam.chains = units
	s.fam.mu.Unlock()
	return errors.Join(errs...)
}

// chainName is the name of the chain's first member that has one, else the
// chain's id.
func chainName(c chains.Chain, sessions []sample.Session) string {
	for _, m := range c.Members {
		for _, x := range sessions {
			if x.Name != "" && memberOf(x).Same(m) {
				return x.Name
			}
		}
	}
	return c.ID
}

// chainNamed is the chain of who: the chain's name or id, or the name,
// pane or target of a member. members are its sessions in the sample, as
// a brief names them.
func (s *Service) chainNamed(who string) (c chains.Chain, name string, members []sample.Session, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.chainsLocked()
	for _, ch := range st.Chains {
		if ch.ID == who || chainName(ch, s.cur.Sessions) == who {
			c, ok = ch, true
			break
		}
	}
	for _, x := range s.cur.Sessions {
		if ok {
			break
		}
		if who != "" && (x.Name == who || x.Pane == who || x.Target == who) {
			c, ok = st.ChainOf(memberOf(x))
		}
	}
	if !ok {
		return c, "", nil, false
	}
	c.Members = slices.Clone(c.Members)
	for _, x := range s.cur.Sessions {
		if slices.ContainsFunc(c.Members, memberOf(x).Same) {
			members = append(members, x)
		}
	}
	return c, chainName(c, s.cur.Sessions), members, true
}

// chainNameOf is the name of the chain with id chainID now: its id when
// it is gone.
func (s *Service) chainNameOf(chainID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.chainsLocked().Chains {
		if c.ID == chainID {
			return chainName(c, s.cur.Sessions)
		}
	}
	return chainID
}

// chainsAtWork counts the chain units the cap counts: an agent that is up
// on the stoker's server, or a run that is on. A unit that quit with its
// run off counts no more; its dir and card stay.
func (s *Service) chainsAtWork() int {
	n := 0
	for _, u := range s.family() {
		if u.id == stoker.HostID {
			continue
		}
		s.mu.Lock()
		_, up := stokerIn(s.cur.Sessions, s.StokerServer, stoker.SessionName(u.id))
		s.mu.Unlock()
		if up || u.store.Mode().On {
			n++
		}
	}
	return n
}

// nextStokerID is the id after every dir in stokers/: an id is never
// taken again while its dir is there, whether it holds a link or not.
func nextStokerID(state string) (string, error) {
	entries, err := os.ReadDir(filepath.Dir(stoker.FamilyDir(state, "s1")))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	var ids []string
	for _, e := range entries {
		ids = append(ids, e.Name())
	}
	return stoker.NextID(ids), nil
}

// familyLine is a line about the family on a card.
func familyLine(now time.Time, text string) stoker.Entry {
	return stoker.Entry{ID: "family@" + now.UTC().Format(time.RFC3339Nano), At: now, Rule: stoker.RuleFamily, State: stoker.StepDone, Text: text}
}

// startChainStoker starts a stoker for the chain of who, with the order's
// words and goes, given by giver ("person" or "host"). Its link.json comes
// first, as the merge guard finds a unit by it. The agent starts in
// stoker-<id>; the brief is typed into it once it rests (deliverBriefs),
// and its run is on at once.
func (s *Service) startChainStoker(ctx context.Context, who, words string, grants []string, giver string) (*stokerUnit, error) {
	host, grants, err := s.givenGoes(grants, giver)
	if err != nil {
		return nil, err
	}
	s.fam.start.Lock()
	defer s.fam.start.Unlock()
	c, name, members, ok := s.chainNamed(who)
	switch {
	case !ok:
		return nil, fmt.Errorf("%s is in no chain.", who) //nolint:staticcheck // the line a person reads, as it stands
	case s.unitOfChain(c.ID) != nil:
		return nil, fmt.Errorf("chain %s has its stoker: give it an order.", name) //nolint:staticcheck // the line a person reads, as it stands
	case s.chainsAtWork() >= s.maxChains():
		return nil, s.atCap()
	}
	state := host.store.Dir()
	id, err := nextStokerID(state)
	if err != nil {
		return nil, err
	}
	dir := stoker.FamilyDir(state, id)
	link := stoker.Link{ID: id, Chain: c.ID, Giver: giver, Grants: grants}
	if err := stoker.WriteLink(dir, link); err != nil {
		return nil, err
	}
	st, err := stoker.Open(dir)
	if err != nil {
		return nil, errors.Join(err, os.RemoveAll(dir))
	}
	u := &stokerUnit{id: id, link: link, store: st}
	s.fam.mu.Lock()
	s.fam.chains = append(s.fam.chains, u)
	s.fam.mu.Unlock()
	if err := s.startStoker(ctx, u); err != nil {
		return nil, errors.Join(err, s.dropUnit(u))
	}
	if err := s.runChain(u, c, name, members, words, grants); err != nil {
		return nil, errors.Join(err, s.endUnit(ctx, u))
	}
	return u, nil
}

// givenGoes are the goes an order asks for, checked against its giver: the
// host gives none it lacks. The person is the root of every go.
func (s *Service) givenGoes(grants []string, giver string) (*stokerUnit, []string, error) {
	host := s.hostUnit()
	switch {
	case host == nil:
		return nil, nil, errors.New("this service keeps no stoker")
	case s.StokerServer == "":
		return nil, nil, errors.New("this service starts no stoker agent")
	}
	if why := s.agentMissing(); why != "" {
		return nil, nil, errors.New(why)
	}
	grants, err := goesOf(grants)
	if err != nil {
		return nil, nil, err
	}
	if giver == "host" {
		if err := checkGiven(grants, s.grantsOf(host)); err != nil {
			return nil, nil, err
		}
	}
	return host, grants, nil
}

func (s *Service) atCap() error {
	return fmt.Errorf("the family has its %d chain stokers: max_chains. Quit one on the STOKER tab first.", s.maxChains()) //nolint:staticcheck // the line a person reads, as it stands
}

// runChain starts a run of chain unit u, whose agent is up or starting:
// its brief, its mode on with the order, and the brief's line queued for
// the agent's first rest.
func (s *Service) runChain(u *stokerUnit, c chains.Chain, name string, members []sample.Session, words string, grants []string) error {
	now := s.now()
	night := false // a run that starts in the night is part of it
	if h := s.hostUnit(); h != nil {
		m := h.store.Mode()
		night = m.On && m.Nightly
	}
	o := stoker.Order{Grants: grants, ID: "run-" + now.UTC().Format("20060102-150405.000000000"), Words: strings.TrimSpace(words), Night: night, At: now, Went: now, State: stoker.OrderRunning}
	board := s.board(now)
	board.Chain = &stoker.BriefChain{Name: name, Repo: c.Repo}
	for _, x := range members {
		board.Chain.Members = append(board.Chain.Members, stoker.MemberLine(x))
	}
	if err := os.WriteFile(filepath.Join(u.store.Dir(), briefName), []byte(stoker.Brief(o, s.Ways, stoker.Hands, board)), 0o600); err != nil {
		return err
	}
	if err := s.updateOrder(u, func(m stoker.Mode) (stoker.Mode, error) {
		if m.On {
			return m, errors.New("the run changed meanwhile: look at it again")
		}
		m, err := s.start(stoker.Mode{Acked: m.Ack(now).Acked}, night, now, members)
		m.Order = &o
		return m, err
	}); err != nil {
		return err
	}
	s.mu.Lock()
	u.brief, u.briefAt, u.dropped = []string{briefLine(u)}, now, nil
	s.mu.Unlock()
	if h := s.hostUnit(); h != nil {
		_ = h.store.Append(familyLine(now, fmt.Sprintf("started the stoker of chain %s (%s)", name, u.id)))
	}
	s.handOver(u)
	s.refreshStoker()
	return nil
}

// handOver passes to chain unit u, whose run starts, the host stoker's
// work on its chain's matches. The host's triggers that act on a member
// move to u. Each kill or go on the host's card that waits for a y for a
// member is dropped, with a line that says so. The host drives the members
// no more. A hold passes with its match: the owner of a match is the one
// stoker that may let it go (scopeOf). Each write is best effort: what
// fails stays with the host, whose acts on the members scopeOf refuses.
func (s *Service) handOver(u *stokerUnit) {
	host := s.hostUnit()
	if host == nil || u == host {
		return
	}
	_, name, members, ok := s.chainNamed(s.chainIDOf(u))
	if !ok {
		return
	}
	panes, ids := map[string]string{}, map[string]string{}
	for _, x := range members {
		panes[x.Pane] = x.Name
		if x.SessionID != "" {
			ids[x.SessionID] = x.Name
		}
	}
	s.moveTriggers(host, u, panes)
	s.dropWaits(host, name, panes, ids)
	_ = s.updateDriven(host, func(d *stoker.Driven, _ []string) bool {
		changed := false
		for pane := range panes {
			_, driven := d.Panes[pane]
			_, steps := d.Steps[pane]
			delete(d.Panes, pane)
			delete(d.Steps, pane)
			changed = changed || driven || steps
			for from, needs := range d.Steps { // a kill's y is kept by the pane that asked
				if i := slices.Index(needs, "kill:"+pane); i >= 0 {
					d.Steps[from], changed = slices.Delete(needs, i, i+1), true
				}
			}
		}
		return changed
	})
}

// moveTriggers moves the triggers of from that act on one of panes to to,
// each with an id of to's run.
func (s *Service) moveTriggers(from, to *stokerUnit, panes map[string]string) {
	var moved []stoker.Trigger
	if err := s.updateOrder(from, func(m stoker.Mode) (stoker.Mode, error) {
		moved = nil
		var kept []stoker.Trigger
		for _, t := range m.Triggers {
			if _, in := panes[t.Pane]; in {
				moved = append(moved, t)
			} else {
				kept = append(kept, t)
			}
		}
		if len(moved) == 0 {
			return m, errNoneMoves
		}
		m.Triggers = kept
		return m, nil
	}); err != nil {
		return
	}
	_ = s.updateOrder(to, func(m stoker.Mode) (stoker.Mode, error) {
		for _, t := range moved {
			m.NextTrigger++
			t.ID = "t" + strconv.Itoa(m.NextTrigger)
			m.Triggers = append(slices.Clip(m.Triggers), t)
		}
		return m, nil
	})
}

// errNoneMoves leaves the triggers as they are.
var errNoneMoves = errors.New("no trigger moves")

// dropWaits drops each kill or go on unit u's card that waits for a y for
// a member of the chain name, by its pane or its session id.
func (s *Service) dropWaits(u *stokerUnit, name string, panes, ids map[string]string) {
	run := u.store.View().Run
	now := s.now()
	done := map[string]bool{}
	for _, e := range run {
		key := e.Pane + " " + e.Needs
		if e.Rule != stoker.RuleGo || done[key] {
			continue
		}
		done[key] = true
		last := stoker.LastGo(run, e)
		if last.State != stoker.StepWaits {
			continue
		}
		target, kill := strings.CutPrefix(last.Needs, "kill:")
		if !kill {
			target = last.Pane
		}
		who, in := panes[target]
		if !in {
			who, in = ids[last.Session]
		}
		if !in {
			continue
		}
		if who == "" {
			who = target
		}
		what := "go"
		if kill {
			what = "kill"
		}
		d := last
		d.ID, d.At, d.State = "dropped@"+now.UTC().Format(time.RFC3339Nano)+"/"+last.Pane+"/"+last.Needs, now, stoker.StepFailed
		d.Text = fmt.Sprintf("dropped the %s for %s: the stoker of chain %s owns it now", what, who, name)
		_ = u.store.Append(d)
	}
}

// briefLine is the line that points chain unit u's agent at its brief.
func briefLine(u *stokerUnit) string {
	return "Read " + filepath.Join(u.store.Dir(), briefName) + " and follow it."
}

// rerunChain gives chain unit u, whose run is off, a new run: its agent
// starts again if it quit, and the cap counts it again.
func (s *Service) rerunChain(ctx context.Context, u *stokerUnit, words string, grants []string, giver string) error {
	s.fam.start.Lock()
	defer s.fam.start.Unlock()
	c, ok := s.chainOfUnit(u)
	if !ok {
		return errors.New("the chain of this stoker is gone")
	}
	_, name, members, _ := s.chainNamed(c.ID)
	if u.store.Mode().On {
		return errors.New("the run changed meanwhile: look at it again")
	}
	if s.chainsAtWork() >= s.maxChains() {
		return s.atCap()
	}
	if err := s.relink(u, grants, giver); err != nil {
		return err
	}
	if err := s.startStoker(ctx, u); err != nil {
		return err
	}
	return s.runChain(u, c, name, members, words, grants)
}

// chainOfUnit is the chain of chain unit u now.
func (s *Service) chainOfUnit(u *stokerUnit) (chains.Chain, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return chainByID(s.chainsLocked(), s.chainIDOf(u))
}

// relink gives chain unit u the giver and goes of a new order, in its
// link.json first: the merge guard reads them there.
func (s *Service) relink(u *stokerUnit, grants []string, giver string) error {
	s.fam.mu.Lock()
	defer s.fam.mu.Unlock()
	l := u.link
	l.Giver, l.Grants = giver, slices.Clone(grants)
	if err := stoker.WriteLink(u.store.Dir(), l); err != nil {
		return err
	}
	u.link.Giver, u.link.Grants = l.Giver, l.Grants
	return nil
}

// relinkChain makes chain unit u the stoker of chain, in its link.json
// first: the merge guard reads it there.
func (s *Service) relinkChain(u *stokerUnit, chain string) error {
	s.fam.mu.Lock()
	defer s.fam.mu.Unlock()
	l := u.link
	l.Chain = chain
	if err := stoker.WriteLink(u.store.Dir(), l); err != nil {
		return err
	}
	u.link.Chain = chain
	return nil
}

// chainIDOf is the id of unit u's chain: "" for the host. A join changes
// it under stokerFamily.mu.
func (s *Service) chainIDOf(u *stokerUnit) string {
	s.fam.mu.Lock()
	defer s.fam.mu.Unlock()
	return u.link.Chain
}

// grantsOf are the goes unit u has now: the host's are its order's. A
// chain unit's are its order's, those its link holds and, when the host
// gave them, those the host has now: a go the host loses, or a night
// that ends, takes them away.
func (s *Service) grantsOf(u *stokerUnit) []string {
	_, grants := drivenRun(u.store.Mode())
	if s.chainIDOf(u) == "" {
		return grants
	}
	s.fam.mu.Lock()
	given, giver := u.link.Grants, u.link.Giver
	s.fam.mu.Unlock()
	grants = stoker.Given(grants, given)
	if giver == "host" {
		var host []string
		if h := s.hostUnit(); h != nil && h.store.Mode().On {
			host = s.grantsOf(h)
		}
		grants = stoker.Given(grants, host)
	}
	return grants
}

// checkGiven refuses the first of grants its giver does not have.
func checkGiven(grants, giver []string) error {
	for _, g := range grants {
		if !slices.Equal(stoker.Given([]string{g}, giver), []string{g}) {
			return fmt.Errorf("the order gives %s, which its giver does not have", g)
		}
	}
	return nil
}

// endUnit ends a chain unit whose start failed after its agent started:
// its tmux session first, then the unit and its dir. A unit whose run is
// on stays: the guard finds its panes by its link.json.
func (s *Service) endUnit(ctx context.Context, u *stokerUnit) error {
	if u.store.Mode().On {
		return nil
	}
	var err error
	if res := s.Actions.Do(ctx, advice.Action{Label: "quit the stoker", Steps: [][]string{s.stokerSessionArgv(u, "kill-session")}, Destructive: true}, "y"); res.Err != "" {
		err = errors.New("its stoker session did not end: " + res.Err)
	}
	return errors.Join(err, s.dropUnit(u))
}

// dropUnit takes a chain unit that never ran out of the family, with its
// dir: its id is free again.
func (s *Service) dropUnit(u *stokerUnit) error {
	s.fam.mu.Lock()
	s.fam.chains = slices.DeleteFunc(s.fam.chains, func(x *stokerUnit) bool { return x == u })
	s.fam.mu.Unlock()
	return os.RemoveAll(u.store.Dir())
}

// maxChains is [stoker] max_chains; none or less than one is the default.
func (s *Service) maxChains() int {
	if s.MaxChains < 1 {
		return config.Default().Stoker.MaxChains
	}
	return s.MaxChains
}

// deliverBriefs types each chain unit's brief line, and the orders that
// wait behind it, once its new agent rests, after a sample. One that does
// not rest within stokerStartsIn drops its lines, and its card and the
// host's say so.
func (s *Service) deliverBriefs(ctx context.Context) {
	now := s.now()
	for _, u := range s.family() {
		s.mu.Lock()
		lines, at := u.brief, u.briefAt
		x, up := stokerIn(s.cur.Sessions, s.StokerServer, stoker.SessionName(u.id))
		late := len(lines) > 0 && (!up || !ready(x)) && now.Sub(at) >= stokerStartsIn
		if len(lines) > 0 && (up && ready(x) || late) {
			u.brief = nil // claimed: typed once at most
		}
		if late {
			u.dropped = lines
		}
		s.mu.Unlock()
		switch {
		case len(lines) == 0:
		case up && ready(x):
			typed := 0
			for _, line := range lines {
				if s.typeInto(ctx, x.Pane, line) != nil {
					break
				}
				typed++
			}
			s.mu.Lock()
			if typed < len(lines) { // try the rest again at the next rest, before what came meanwhile
				u.brief = append(slices.Clone(lines[typed:]), u.brief...)
				if typed > 0 {
					u.briefAt = now // the agent took a line: the rest waits anew
				}
			}
			if typed > 0 {
				u.startedAt = time.Time{}
			}
			s.mu.Unlock()
		case late:
			text := fmt.Sprintf("the stoker of chain %s did not start: its brief was not typed", s.chainNameOf(s.chainIDOf(u)))
			if slices.ContainsFunc(lines, func(l string) bool { return l != briefLine(u) }) {
				text = fmt.Sprintf("the stoker of chain %s did not take its order: its agent did not rest", s.chainNameOf(s.chainIDOf(u)))
			}
			_ = u.store.Append(familyLine(now, text))
			if h := s.hostUnit(); h != nil {
				_ = h.store.Append(familyLine(now, text))
			}
			s.refreshStoker()
		}
	}
}

// chainAsk is what stoker:new sends: the chain (any member, its name or
// its id), the order's words and its goes.
type chainAsk struct {
	Chain  string   `json:"chain"`
	Words  string   `json:"words"`
	Grants []string `json:"grants"`
}

// newChainAct starts a chain stoker (stoker:new): the person's, at a
// console or in a shell of their own, never from an agent's pane.
func (s *Service) newChainAct(ctx context.Context, a proto.Act) proto.Result {
	// A console hello does not make an agent's pane, or a stoker's, a
	// person's.
	if why := s.notThePersons(a); why != "" {
		return proto.Result{Err: why}
	}
	if _, stokerPane := s.unitOf(a); stokerPane {
		return proto.Result{Err: "the stoker does not start, stop or rest itself"}
	}
	if !personPlaced(a) {
		return proto.Result{Err: "not started: matchblox cannot tell who starts this. Start from a terminal, a tmux pane or the console."}
	}
	if h := s.hostUnit(); h != nil {
		if m := h.store.Mode(); m.On && m.Nightly {
			return proto.Result{Err: sealedFamily}
		}
	}
	var ask chainAsk
	if err := json.Unmarshal([]byte(a.Text), &ask); err != nil {
		return proto.Result{Err: "the start does not read: " + err.Error()}
	}
	u, err := s.startChainStoker(ctx, ask.Chain, ask.Words, ask.Grants, "person")
	if err != nil {
		return proto.Result{Err: err.Error()}
	}
	return proto.Result{Note: fmt.Sprintf("started the stoker of chain %s (%s)", s.chainNameOf(s.chainIDOf(u)), u.id)}
}

// ownerOf is the unit that drives the match x: the stoker of x's chain
// while its run is on, else the host.
func (s *Service) ownerOf(x sample.Session) *stokerUnit {
	s.mu.Lock()
	c, in := s.chainsLocked().ChainOf(memberOf(x))
	s.mu.Unlock()
	return s.ownerOfChain(c.ID, in)
}

func (s *Service) ownerOfChain(chainID string, in bool) *stokerUnit {
	if in {
		if u := s.unitOfChain(chainID); u != nil && u.store.Mode().On {
			return u
		}
	}
	return s.hostUnit()
}

// scopeOf refuses a pane act of unit u on a match it does not own: a chain
// unit acts on its chain's members only, and the host on no match a chain
// unit owns. Both read the chains as they are at the act, by the match's
// identity, not its pane.
func (s *Service) scopeOf(u *stokerUnit, pane string) error {
	s.mu.Lock()
	x, ok := s.sessionAtLocked(pane)
	var c chains.Chain
	in := false
	if ok {
		c, in = s.chainsLocked().ChainOf(memberOf(x))
	}
	s.mu.Unlock()
	who := pane
	if ok && x.Name != "" {
		who = x.Name
	}
	if s.chainIDOf(u) != "" {
		if !in || c.ID != s.chainIDOf(u) {
			return fmt.Errorf("%s is not in the chain of this stoker.", who) //nolint:staticcheck // the line a person reads, as it stands
		}
		return nil
	}
	if o := s.ownerOfChain(c.ID, in); o != nil && o != u {
		return fmt.Errorf("%s has the stoker of chain %s: give it an order.", who, s.chainNameOf(s.chainIDOf(o))) //nolint:staticcheck // the line a person reads, as it stands
	}
	return nil
}

// inChainRepo refuses a new session of chain unit u that starts outside
// its chain's repo, by the repos the last scan knows.
func (s *Service) inChainRepo(u *stokerUnit, text string) error {
	n, err := readNew(text)
	if err != nil {
		return err
	}
	dir := n.Where
	if rest, ok := cutHome(dir); ok {
		dir = filepath.Join(s.home(), rest)
	}
	s.mu.Lock()
	c, ok := chainByID(s.chainsLocked(), s.chainIDOf(u))
	repo, wt, known := gitscan.RepoOf(s.cur.Git, filepath.Clean(dir))
	s.mu.Unlock()
	if !ok {
		return errors.New("the chain of this stoker is gone")
	}
	if !filepath.IsAbs(dir) || !known || repo != c.Repo || !inCheckout(dir, wt) {
		return fmt.Errorf("a session of chain %s starts in its repo: %s", s.chainNameOf(c.ID), c.Repo)
	}
	return nil
}

// inCheckout tells if dir, its links resolved, is in the checkout wt, its own
// resolved: a link in the repo that leads out of it is not. A path that
// does not resolve is not.
func inCheckout(dir, wt string) bool {
	d, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	w, err := filepath.EvalSymlinks(wt)
	if err != nil {
		return false
	}
	return d == w || strings.HasPrefix(d, w+string(filepath.Separator))
}
