// Package chains holds the matches that agreed to hear from each other: the
// chains, the requests that wait for the other side, and an inbox of news for
// each match. It is pure data; the service decides when to ask, prune and post.
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

const (
	// RequestTTL is how long a request waits for the other side.
	RequestTTL = time.Hour
	// InboxMax is how many items an inbox holds; the oldest go first.
	InboxMax = 20
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

// Chain is a set of matches in one repo that hear from each other. Spark is
// when the chain last got news worth showing.
type Chain struct {
	ID      string    `json:"id"`
	Repo    string    `json:"repo"`
	Members []Member  `json:"members"`
	Spark   time.Time `json:"spark,omitzero"`
}

// Request is one side's ask to chain; it waits for the other side to ask back.
type Request struct {
	From Member    `json:"from"`
	To   Member    `json:"to"`
	At   time.Time `json:"at"`
}

// Item is one piece of news in an inbox. seq is the order it was posted in.
type Item struct {
	At   time.Time `json:"at"`
	Text string    `json:"text"`
	seq  uint64
}

// Left is a member that dropped out of a chain.
type Left struct {
	Who   Member
	Repo  string
	Chain string
}

// Store is every chain of a host. The exported fields are saved; the inboxes
// live in memory only.
type Store struct {
	Chains   []Chain   `json:"chains,omitempty"`
	Requests []Request `json:"requests,omitempty"`
	Off      []Member  `json:"off,omitempty"`

	inbox   map[string][]Item
	dropped map[string]int
	posted  uint64
	// renamed is the key each refreshed member had before, to its key now:
	// news given back under an old name reaches the match.
	renamed map[string]string
}

func (s *Store) isOff(m Member) bool {
	return slices.ContainsFunc(s.Off, m.Same)
}

func (s *Store) chainIndex(m Member) int {
	return slices.IndexFunc(s.Chains, func(c Chain) bool {
		return slices.ContainsFunc(c.Members, m.Same)
	})
}

// side is m with the members of its chain.
func (s *Store) side(m Member) []Member {
	if i := s.chainIndex(m); i >= 0 {
		return s.Chains[i].Members
	}
	return []Member{m}
}

// Ask records that from wants to chain with to. When to (or its chain)
// already asked from (or its chain), the chains of both sides become one and
// formed is true.
func (s *Store) Ask(from, to Member, repo string, now time.Time) (formed bool) {
	if s.isOff(from) || s.isOff(to) || from.Same(to) {
		return false
	}
	fromSide, toSide := s.side(from), s.side(to)
	if slices.ContainsFunc(fromSide, to.Same) {
		return false
	}
	answers := func(r Request) bool {
		return slices.ContainsFunc(toSide, r.From.Same) && slices.ContainsFunc(fromSide, r.To.Same)
	}
	if !slices.ContainsFunc(s.Requests, answers) {
		s.Requests = slices.DeleteFunc(s.Requests, func(r Request) bool { return r.From.Same(from) && r.To.Same(to) })
		s.Requests = append(s.Requests, Request{From: from, To: to, At: now})
		return false
	}
	s.Requests = slices.DeleteFunc(s.Requests, answers)
	s.merge(from, to, repo)
	return true
}

func (s *Store) merge(a, b Member, repo string) {
	members := slices.Clone(s.side(a))
	for _, m := range s.side(b) {
		if !slices.ContainsFunc(members, m.Same) {
			members = append(members, m)
		}
	}
	ia, ib := s.chainIndex(a), s.chainIndex(b)
	var c Chain
	switch {
	case ia >= 0:
		c = s.Chains[ia]
	case ib >= 0:
		c = s.Chains[ib]
	default:
		c = Chain{ID: s.newID(a), Repo: repo}
	}
	c.Members = members
	var kept []Chain
	for i, o := range s.Chains {
		if i != ia && i != ib {
			kept = append(kept, o)
		}
	}
	s.Chains = append(kept, c)
}

func (s *Store) newID(first Member) string {
	for n := 1; ; n++ {
		id := first.Pane + "#" + strconv.Itoa(n)
		if !slices.ContainsFunc(s.Chains, func(c Chain) bool { return c.ID == id }) {
			return id
		}
	}
}

// Leave takes m out of its chain and drops its requests and news. A chain of
// one ends.
func (s *Store) Leave(m Member) {
	s.leave(m)
}

func (s *Store) leave(m Member) (chainID string, ok bool) {
	s.Requests = slices.DeleteFunc(s.Requests, func(r Request) bool { return r.From.Same(m) || r.To.Same(m) })
	delete(s.inbox, m.key())
	delete(s.dropped, m.key())
	i := s.chainIndex(m)
	if i < 0 {
		return "", false
	}
	c := &s.Chains[i]
	chainID = c.ID
	c.Members = slices.DeleteFunc(slices.Clone(c.Members), m.Same)
	if len(c.Members) < 2 {
		s.Chains = slices.Delete(s.Chains, i, i+1)
	}
	return chainID, true
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

// Refresh rewrites each member that cur names otherwise now, as a match
// whose session id changed, in the chains, the requests, the off list and
// the inbox. It tells if any changed.
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
			s.put(now, items, dropped, true)
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
	for i := range s.Requests {
		renew(&s.Requests[i].From)
		renew(&s.Requests[i].To)
	}
	for i := range s.Off {
		renew(&s.Off[i])
	}
	return changed
}

// Prune drops members that are gone or in another repo than their chain,
// requests past RequestTTL or across repos, the off marks of gone matches,
// and chains of one. live tells where a match is now: present, and its
// repo ("" for none).
func (s *Store) Prune(live func(Member) (repo string, present bool), now time.Time) (left []Left) {
	for _, c := range slices.Clone(s.Chains) {
		for _, m := range c.Members {
			repo, ok := live(m)
			if ok && repo != "" && repo == c.Repo {
				continue
			}
			if id, in := s.leave(m); in {
				left = append(left, Left{Who: m, Repo: repo, Chain: id})
			}
		}
	}
	s.Requests = slices.DeleteFunc(s.Requests, func(r Request) bool {
		from, okFrom := live(r.From)
		to, okTo := live(r.To)
		return now.Sub(r.At) > RequestTTL || !okFrom || !okTo || from == "" || from != to
	})
	s.Off = slices.DeleteFunc(s.Off, func(m Member) bool {
		_, present := live(m)
		return !present
	})
	maps.DeleteFunc(s.renamed, func(_, now string) bool { return !s.holds(now) })
	return left
}

// holds tells if any chain, request, off mark or inbox still names the key k.
func (s *Store) holds(k string) bool {
	named := func(m Member) bool { return m.key() == k }
	return slices.ContainsFunc(s.Chains, func(c Chain) bool { return slices.ContainsFunc(c.Members, named) }) ||
		slices.ContainsFunc(s.Requests, func(r Request) bool { return named(r.From) || named(r.To) }) ||
		slices.ContainsFunc(s.Off, named) || len(s.inbox[k]) > 0 || s.dropped[k] > 0
}

// ChainOf is the chain m is in.
func (s *Store) ChainOf(m Member) (Chain, bool) {
	i := s.chainIndex(m)
	if i < 0 {
		return Chain{}, false
	}
	return s.Chains[i], true
}

// Mates are the other members of m's chain.
func (s *Store) Mates(m Member) []Member {
	var out []Member
	for _, o := range s.side(m) {
		if !o.Same(m) {
			out = append(out, o)
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
	s.posted++
	s.put(to.key(), []Item{{At: now, Text: text, seq: s.posted}}, 0, false)
}

// put adds items to the inbox of k, in the order they were posted when
// back (given back after what came since), with dropped more dropped. Past
// InboxMax the oldest go and are counted.
func (s *Store) put(k string, items []Item, dropped int, back bool) {
	if s.inbox == nil {
		s.inbox, s.dropped = map[string][]Item{}, map[string]int{}
	}
	items = append(slices.Clone(s.inbox[k]), items...)
	if back {
		slices.SortStableFunc(items, func(a, b Item) int { return cmp.Compare(a.seq, b.seq) })
	}
	if over := len(items) - InboxMax; over > 0 {
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
	s.put(k, t.Items, t.Dropped, true)
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
