package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
	id    string
	link  stoker.Link // Chain is empty for the host
	store *stoker.Store
	// startedAt is when matchblox started the unit's agent; zero once it
	// is ready, or gave up on it. Service.mu guards it.
	startedAt time.Time
	// driven are the panes the unit drove, for the merge guard.
	driven drivenState
	// brief is the line still to type into a chain unit's new agent once
	// it rests, and briefAt when the unit started. Service.mu guards both.
	brief   string
	briefAt time.Time
	// joins are the tmux sessions a chain unit started, which join its
	// chain once a sample finds them in its repo. Service.mu guards it.
	joins []pendingJoin
}

// pendingJoin is a session a chain unit started, not in its chain yet.
type pendingJoin struct {
	name string
	at   time.Time
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
	host := s.hostUnit()
	switch {
	case host == nil:
		return nil, errors.New("this service keeps no stoker")
	case s.StokerServer == "":
		return nil, errors.New("this service starts no stoker agent")
	}
	if why := s.agentMissing(); why != "" {
		return nil, errors.New(why)
	}
	grants, err := goesOf(grants)
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
		return nil, fmt.Errorf("the family has its %d chain stokers: max_chains. Quit one on the STOKER tab first.", s.maxChains()) //nolint:staticcheck // the line a person reads, as it stands
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

	now := s.now()
	o := stoker.Order{Grants: grants, ID: "run-" + now.UTC().Format("20060102-150405.000000000"), Words: strings.TrimSpace(words), At: now, Went: now, State: stoker.OrderRunning}
	board := s.board(now)
	board.Chain = &stoker.BriefChain{Name: name, Repo: c.Repo}
	for _, x := range members {
		board.Chain.Members = append(board.Chain.Members, stoker.MemberLine(x))
	}
	path := filepath.Join(dir, briefName)
	if err := os.WriteFile(path, []byte(stoker.Brief(o, s.Ways, stoker.Hands, board)), 0o600); err != nil {
		return nil, errors.Join(err, s.endUnit(ctx, u))
	}
	if err := s.updateOrder(u, func(m stoker.Mode) (stoker.Mode, error) {
		m, err := s.start(stoker.Mode{}, false, now, members)
		m.Order = &o
		return m, err
	}); err != nil {
		return nil, errors.Join(err, s.endUnit(ctx, u))
	}
	s.mu.Lock()
	u.brief, u.briefAt = "Read "+path+" and follow it.", now
	s.mu.Unlock()
	_ = host.store.Append(familyLine(now, fmt.Sprintf("started the stoker of chain %s (%s)", name, id)))
	s.refreshStoker()
	return u, nil
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

// deliverBriefs types each chain unit's brief line once its new agent
// rests, after a sample. One that does not rest within stokerStartsIn
// drops its line, and its card and the host's say so.
func (s *Service) deliverBriefs(ctx context.Context) {
	now := s.now()
	for _, u := range s.family() {
		s.mu.Lock()
		line, at := u.brief, u.briefAt
		x, up := stokerIn(s.cur.Sessions, s.StokerServer, stoker.SessionName(u.id))
		if line != "" && (up && ready(x) || now.Sub(at) >= stokerStartsIn) {
			u.brief = "" // claimed: typed once at most
		}
		s.mu.Unlock()
		switch {
		case line == "":
		case up && ready(x):
			if err := s.typeInto(ctx, x.Pane, line); err != nil {
				s.mu.Lock()
				if u.brief == "" { // not quit meanwhile: try again at the next rest
					u.brief = line
				}
				s.mu.Unlock()
				continue
			}
			s.mu.Lock()
			u.startedAt = time.Time{}
			s.mu.Unlock()
		case now.Sub(at) >= stokerStartsIn:
			text := fmt.Sprintf("the stoker of chain %s did not start: its brief was not typed", s.chainNameOf(u.link.Chain))
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
	if _, ok := s.sessionAt(a.From); ok {
		return proto.Result{Err: notTheBuilders}
	}
	if _, stokerPane := s.unitOf(a); stokerPane {
		return proto.Result{Err: "the stoker does not start, stop or rest itself"}
	}
	var ask chainAsk
	if err := json.Unmarshal([]byte(a.Text), &ask); err != nil {
		return proto.Result{Err: "the start does not read: " + err.Error()}
	}
	u, err := s.startChainStoker(ctx, ask.Chain, ask.Words, ask.Grants, "person")
	if err != nil {
		return proto.Result{Err: err.Error()}
	}
	return proto.Result{Note: fmt.Sprintf("started the stoker of chain %s (%s)", s.chainNameOf(u.link.Chain), u.id)}
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
	if u.link.Chain != "" {
		if !in || c.ID != u.link.Chain {
			return fmt.Errorf("%s is not in the chain of this stoker.", who) //nolint:staticcheck // the line a person reads, as it stands
		}
		return nil
	}
	if o := s.ownerOfChain(c.ID, in); o != nil && o != u {
		return fmt.Errorf("%s has the stoker of chain %s: give it an order.", who, s.chainNameOf(o.link.Chain)) //nolint:staticcheck // the line a person reads, as it stands
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
	c, ok := chainByID(s.chainsLocked(), u.link.Chain)
	repo, _, known := gitscan.RepoOf(s.cur.Git, filepath.Clean(dir))
	s.mu.Unlock()
	if !ok {
		return errors.New("the chain of this stoker is gone")
	}
	if !filepath.IsAbs(dir) || !known || repo != c.Repo {
		return fmt.Errorf("a session of chain %s starts in its repo: %s", s.chainNameOf(c.ID), c.Repo)
	}
	return nil
}
