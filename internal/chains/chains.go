// Package chains holds the teams of matches that hear each other, and an
// inbox of news for each match. A match may be in several teams; each team
// hears only itself. It is pure data; the service decides when to join,
// prune and post.
package chains

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"
)

// InboxMax is how many items an inbox holds; the oldest go first.
const InboxMax = 20

// The refusals of Form and Join.
var (
	ErrOff  = errors.New("a match that is off takes no chains")
	ErrSelf = errors.New("a match cannot chain with itself")
	ErrIn   = errors.New("already in the chain")
)

// Member is one match. A session id names it when both sides have one; else
// its pane, pid and start do.
type Member struct {
	SessionID string `json:"session_id,omitempty"`
	Pane      string `json:"pane,omitempty"`
	PID       int    `json:"pid,omitempty"`
	Start     uint64 `json:"start,omitempty"`
}

// Same tells if o is the same match.
func (m Member) Same(o Member) bool {
	if m.SessionID != "" && o.SessionID != "" {
		return m.SessionID == o.SessionID
	}
	return m.Pane == o.Pane && m.PID == o.PID && m.Start == o.Start
}

func (m Member) key() string {
	if m.SessionID != "" {
		return m.SessionID
	}
	return fmt.Sprintf("%s/%d/%d", m.Pane, m.PID, m.Start)
}

// Chain is a team: matches on one host that all hear each other, in any
// repo. Name is the name of the match that started it, kept when that match
// leaves; "" in a store an older build saved, until the service names it.
// Repo is the repo of the match that formed it, "" for none, and does not
// follow later moves. Spark is when the chain last got news worth showing.
type Chain struct {
	ID      string    `json:"id"`
	Name    string    `json:"name,omitempty"`
	Repo    string    `json:"repo"`
	Members []Member  `json:"members"`
	Spark   time.Time `json:"spark,omitzero"`
}

// Item is one piece of news in an inbox, or a line a match sent: From is
// its sender, the zero Member for news. seq is the order it was posted in.
type Item struct {
	At   time.Time `json:"at"`
	Text string    `json:"text"`
	From Member    `json:"from,omitzero"`
	seq  uint64
}

// Why a sent line was lost before its match heard it.
const (
	// LostFull: the inbox was full, and the line was its oldest.
	LostFull = "full"
	// LostUnchained: its match no longer shares a team with its sender.
	LostUnchained = "unchained"
)

// Lost is a sent line its match will not hear: To is the match.
type Lost struct {
	To   Member
	Item Item
	Why  string
}

// Left is a member that dropped out of a chain.
type Left struct {
	Who   Member
	Chain string
}

// Move is a switch that starts another agent in From's pane: the new agent
// takes From's place when it shows there.
type Move struct {
	From Member    `json:"from"`
	At   time.Time `json:"at"`
}

// Store is every chain of a host. The exported fields are saved; the inboxes
// live in memory only.
type Store struct {
	Chains []Chain  `json:"chains,omitempty"`
	Off    []Member `json:"off,omitempty"`
	Moves  []Move   `json:"moves,omitempty"`
	// Unseen is when each member whose pane lives but shows no agent was
	// first missed, by its key.
	Unseen map[string]time.Time `json:"unseen,omitempty"`

	inbox   map[string][]Item
	dropped map[string]int
	lost    []Lost
	posted  uint64
	// renamed is the key each refreshed member had before, to its key now:
	// news given back under an old name reaches the match.
	renamed map[string]string
}

func (s *Store) isOff(m Member) bool {
	return slices.ContainsFunc(s.Off, m.Same)
}

func (s *Store) teamIndex(name string) int {
	return slices.IndexFunc(s.Chains, func(c Chain) bool { return c.Name == name })
}

// Form makes a new team of a and b, named name, or name-2, name-3 when this
// host has a team of that name already.
func (s *Store) Form(a, b Member, name, repo string) (Chain, error) {
	switch {
	case a.Same(b):
		return Chain{}, ErrSelf
	case s.isOff(a) || s.isOff(b):
		return Chain{}, ErrOff
	}
	c := Chain{ID: s.newID(a), Name: s.FreeName(name), Repo: repo, Members: []Member{a, b}}
	s.Chains = append(s.Chains, c)
	return c, nil
}

// FreeName is name, or name-2, name-3 when a team has it.
func (s *Store) FreeName(name string) string {
	free := name
	for n := 2; s.teamIndex(free) >= 0; n++ {
		free = name + "-" + strconv.Itoa(n)
	}
	return free
}

// Join adds m to the team named team. ErrIn comes with the team.
func (s *Store) Join(m Member, team string) (Chain, error) {
	i := s.teamIndex(team)
	if i < 0 {
		return Chain{}, fmt.Errorf("no chain is named %s", team)
	}
	return s.joinAt(m, i)
}

func (s *Store) joinAt(m Member, i int) (Chain, error) {
	switch {
	case slices.ContainsFunc(s.Chains[i].Members, m.Same):
		return s.Chains[i], ErrIn
	case s.isOff(m):
		return Chain{}, ErrOff
	}
	c := &s.Chains[i]
	c.Members = append(slices.Clone(c.Members), m)
	return *c, nil
}

// LeaveTeam takes m out of the team named team; a team of one ends. It
// tells if m was in it.
func (s *Store) LeaveTeam(m Member, team string) bool {
	i := s.teamIndex(team)
	if i < 0 || !slices.ContainsFunc(s.Chains[i].Members, m.Same) {
		return false
	}
	s.leaveAt(m, i)
	return true
}

func (s *Store) leaveAt(m Member, i int) {
	c := &s.Chains[i]
	c.Members = slices.DeleteFunc(slices.Clone(c.Members), m.Same)
	if len(c.Members) < 2 {
		s.Chains = slices.Delete(s.Chains, i, i+1)
	}
	k := m.key()
	if len(s.inbox[k]) == 0 {
		return
	}
	s.inbox[k] = slices.DeleteFunc(s.inbox[k], func(it Item) bool {
		if it.From == (Member{}) {
			return false
		}
		if _, shared := s.Shared(m, it.From); shared {
			return false
		}
		s.lost = append(s.lost, Lost{To: m, Item: it, Why: LostUnchained})
		return true
	})
	if len(s.inbox[k]) == 0 {
		delete(s.inbox, k)
	}
}

func (s *Store) newID(first Member) string {
	for n := 1; ; n++ {
		id := first.Pane + "#" + strconv.Itoa(n)
		if !slices.ContainsFunc(s.Chains, func(c Chain) bool { return c.ID == id }) {
			return id
		}
	}
}

// Leave takes m out of every team and drops its news.
func (s *Store) Leave(m Member) {
	s.leave(m)
}

// leave takes m out of every team, and drops its news: the ids of the
// teams it was in.
func (s *Store) leave(m Member) (ids []string) {
	for _, it := range s.inbox[m.key()] {
		if it.From != (Member{}) {
			s.lost = append(s.lost, Lost{To: m, Item: it, Why: LostUnchained})
		}
	}
	delete(s.inbox, m.key())
	delete(s.dropped, m.key())
	for i := len(s.Chains) - 1; i >= 0; i-- {
		if slices.ContainsFunc(s.Chains[i].Members, m.Same) {
			ids = append(ids, s.Chains[i].ID)
			s.leaveAt(m, i)
		}
	}
	slices.Reverse(ids)
	return ids
}

// JoinID adds m to the team chainID with no ask: a session a chain's stoker
// started. A member of the team joins it again as a no-op. It fails when m
// is off or the team is gone.
func (s *Store) JoinID(chainID string, m Member) error {
	i := slices.IndexFunc(s.Chains, func(c Chain) bool { return c.ID == chainID })
	if i < 0 {
		return fmt.Errorf("chain %s is gone", chainID)
	}
	if _, err := s.joinAt(m, i); err != nil && !errors.Is(err, ErrIn) {
		return err
	}
	return nil
}

// SetOff makes m refuse chains (off leaves its chain too), or accept them again.
func (s *Store) SetOff(m Member, off bool) {
	if !off {
		s.Off = slices.DeleteFunc(s.Off, m.Same)
		return
	}
	s.Leave(m)
	if !s.isOff(m) {
		s.Off = append(s.Off, m)
	}
}

// Move records that a switch starts another agent in m's pane at now. A
// match no chain, off mark or inbox names has no place to keep.
func (s *Store) Move(m Member, now time.Time) {
	s.Stay(m)
	if s.holds(m.key()) {
		s.Moves = append(s.Moves, Move{From: m, At: now})
	}
}

// Stay drops the move of m: its switch did not happen.
func (s *Store) Stay(m Member) {
	s.Moves = slices.DeleteFunc(s.Moves, func(mv Move) bool { return mv.From.Same(m) })
}

// Moving is the move of m that waits for its new agent.
func (s *Store) Moving(m Member) (Move, bool) {
	i := slices.IndexFunc(s.Moves, func(mv Move) bool { return mv.From.Same(m) })
	if i < 0 {
		return Move{}, false
	}
	return s.Moves[i], true
}

// Refresh rewrites each member that cur names otherwise now, as a match
// whose session id changed, in the chains, the off list and the inbox. It tells if any changed.
func (s *Store) Refresh(cur func(Member) (Member, bool)) (changed bool) {
	renew := func(m *Member) {
		n, ok := cur(*m)
		if !ok || n == *m {
			return
		}
		if old, now := m.key(), n.key(); old != now {
			items, dropped := s.inbox[old], s.dropped[old]
			delete(s.inbox, old)
			delete(s.dropped, old)
			s.put(n, now, items, dropped, true)
			if at, ok := s.Unseen[old]; ok {
				delete(s.Unseen, old)
				s.Unseen[now] = at
			}
			if s.renamed == nil {
				s.renamed = map[string]string{}
			}
			s.renamed[old] = now
		}
		*m, changed = n, true
	}
	for i := range s.Chains {
		for j := range s.Chains[i].Members {
			renew(&s.Chains[i].Members[j])
		}
	}
	for i := range s.Off {
		renew(&s.Off[i])
	}
	return changed
}

// Prune drops members that are gone, from every team they are in, the off
// marks of gone matches, teams of one, and the moves of a match nothing
// names any more. live tells if a match is still here. left has one entry
// for each team a gone member was in.
func (s *Store) Prune(live func(Member) bool, now time.Time) (left []Left) {
	var gone []Member
	for _, c := range s.Chains {
		for _, m := range c.Members {
			if !live(m) && !slices.ContainsFunc(gone, m.Same) {
				gone = append(gone, m)
			}
		}
	}
	for _, m := range gone {
		for _, id := range s.leave(m) {
			left = append(left, Left{Who: m, Chain: id})
		}
	}
	s.Off = slices.DeleteFunc(s.Off, func(m Member) bool { return !live(m) })
	s.Moves = slices.DeleteFunc(s.Moves, func(mv Move) bool { return !s.holds(mv.From.key()) })
	maps.DeleteFunc(s.renamed, func(_, now string) bool { return !s.holds(now) })
	maps.DeleteFunc(s.Unseen, func(k string, _ time.Time) bool { return !s.holds(k) })
	return left
}

// holds tells if any chain, off mark or inbox still names the key k.
func (s *Store) holds(k string) bool {
	named := func(m Member) bool { return m.key() == k }
	return slices.ContainsFunc(s.Chains, func(c Chain) bool { return slices.ContainsFunc(c.Members, named) }) ||
		slices.ContainsFunc(s.Off, named) || len(s.inbox[k]) > 0 || s.dropped[k] > 0
}

// MarkUnseen notes that m's pane lives but shows no agent at now; the
// first miss stays.
func (s *Store) MarkUnseen(m Member, now time.Time) {
	if _, ok := s.Unseen[m.key()]; ok {
		return
	}
	if s.Unseen == nil {
		s.Unseen = map[string]time.Time{}
	}
	s.Unseen[m.key()] = now
}

// MarkSeen notes that m shows again.
func (s *Store) MarkSeen(m Member) {
	delete(s.Unseen, m.key())
}

// UnseenSince is when m was first missed, while its pane lives.
func (s *Store) UnseenSince(m Member) (time.Time, bool) {
	at, ok := s.Unseen[m.key()]
	return at, ok
}

// TeamsOf are the teams m is in, in the order they formed.
func (s *Store) TeamsOf(m Member) []Chain {
	var out []Chain
	for _, c := range s.Chains {
		if slices.ContainsFunc(c.Members, m.Same) {
			out = append(out, c)
		}
	}
	return out
}

// Team is the team named name.
func (s *Store) Team(name string) (Chain, bool) {
	if i := s.teamIndex(name); i >= 0 {
		return s.Chains[i], true
	}
	return Chain{}, false
}

// Shared is the first team that holds both a and b.
func (s *Store) Shared(a, b Member) (Chain, bool) {
	for _, c := range s.TeamsOf(a) {
		if slices.ContainsFunc(c.Members, b.Same) {
			return c, true
		}
	}
	return Chain{}, false
}

// Mates are the other members of every team m is in, each once.
func (s *Store) Mates(m Member) []Member {
	var out []Member
	for _, c := range s.TeamsOf(m) {
		for _, o := range c.Members {
			if !o.Same(m) && !slices.ContainsFunc(out, o.Same) {
				out = append(out, o)
			}
		}
	}
	return out
}

// Spark marks the chain as having news worth showing at now.
func (s *Store) Spark(chainID string, now time.Time) {
	for i := range s.Chains {
		if s.Chains[i].ID == chainID {
			s.Chains[i].Spark = now
		}
	}
}

// Post puts news in to's inbox. Past InboxMax the oldest is dropped and counted.
func (s *Store) Post(to Member, text string, now time.Time) {
	s.PostLine(to, Member{}, text, now)
}

// PostLine puts a line from in to's inbox, as Post puts news.
func (s *Store) PostLine(to, from Member, text string, now time.Time) {
	s.posted++
	s.put(to, to.key(), []Item{{At: now, Text: text, From: from, seq: s.posted}}, 0, false)
}

// Lose marks a line to that its match will not hear, for why.
func (s *Store) Lose(to Member, it Item, why string) {
	s.lost = append(s.lost, Lost{To: to, Item: it, Why: why})
}

// HasLine tells if m's inbox holds a line a match sent.
func (s *Store) HasLine(m Member) bool {
	return slices.ContainsFunc(s.inbox[m.key()], func(it Item) bool { return it.From != (Member{}) })
}

// TakeLine takes the oldest line a match sent from m's inbox, and leaves
// its news.
func (s *Store) TakeLine(m Member) (Item, bool) {
	k := m.key()
	i := slices.IndexFunc(s.inbox[k], func(it Item) bool { return it.From != (Member{}) })
	if i < 0 {
		return Item{}, false
	}
	it := s.inbox[k][i]
	s.inbox[k] = slices.Delete(slices.Clone(s.inbox[k]), i, i+1)
	if len(s.inbox[k]) == 0 {
		delete(s.inbox, k)
	}
	return it, true
}

// TakeLost empties the sent lines lost since it last ran.
func (s *Store) TakeLost() []Lost {
	l := s.lost
	s.lost = nil
	return l
}

// InboxCount is how many items m's inbox holds, and if any were dropped
// since m last took it.
func (s *Store) InboxCount(m Member) (n int, full bool) {
	k := m.key()
	return len(s.inbox[k]), s.dropped[k] > 0
}

// put adds items to the inbox of k, to's key, in the order they were
// posted when back (given back after what came since), with dropped more
// dropped. Past InboxMax the oldest go and are counted, and each sent line
// among them is lost.
func (s *Store) put(to Member, k string, items []Item, dropped int, back bool) {
	if s.inbox == nil {
		s.inbox, s.dropped = map[string][]Item{}, map[string]int{}
	}
	items = append(slices.Clone(s.inbox[k]), items...)
	if back {
		slices.SortStableFunc(items, func(a, b Item) int { return cmp.Compare(a.seq, b.seq) })
	}
	if over := len(items) - InboxMax; over > 0 {
		for _, it := range items[:over] {
			if it.From != (Member{}) {
				s.lost = append(s.lost, Lost{To: to, Item: it, Why: LostFull})
			}
		}
		items = items[over:]
		dropped += over
	}
	if len(items) > 0 {
		s.inbox[k] = items
	}
	if dropped > 0 {
		s.dropped[k] += dropped
	}
}

// Taken is what Take emptied from an inbox: its items, and how many were
// dropped before them.
type Taken struct {
	Items   []Item
	Dropped int
}

// Texts are the texts of t's items, with a last one that counts the dropped.
func (t Taken) Texts() []string {
	var out []string
	for _, it := range t.Items {
		out = append(out, it.Text)
	}
	if t.Dropped > 0 && len(out) > 0 {
		out = append(out, fmt.Sprintf("and %d more", t.Dropped))
	}
	return out
}

// Take empties m's inbox and returns what it held.
func (s *Store) Take(m Member) Taken {
	k := m.key()
	t := Taken{Items: s.inbox[k], Dropped: s.dropped[k]}
	delete(s.inbox, k)
	delete(s.dropped, k)
	return t
}

// GiveBack puts what Take emptied back in m's inbox, in the order it was
// posted: before what came since.
func (s *Store) GiveBack(m Member, t Taken) {
	if len(t.Items) == 0 {
		return
	}
	k := m.key()
	for seen := 0; s.renamed[k] != "" && seen < len(s.renamed); seen++ {
		k = s.renamed[k]
	}
	s.put(m, k, t.Items, t.Dropped, true)
}

// Load reads path. A missing file is an empty store.
func Load(path string) (*Store, error) {
	s := &Store{}
	b, err := os.ReadFile(path) //nolint:gosec // our own state dir
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	return s, json.Unmarshal(b, s)
}

// Save replaces path atomically.
func (s *Store) Save(path string) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	_, werr := f.Write(b)
	if err := errors.Join(werr, f.Chmod(0o600), f.Close()); err != nil {
		_ = os.Remove(f.Name())
		return err
	}
	return os.Rename(f.Name(), path)
}
