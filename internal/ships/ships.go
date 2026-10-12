// Package ships follows the pull requests sessions drive: found in each
// session's own transcript, read on GitHub through CI to live. The service
// runs it; consoles draw the report it puts in the state.
package ships

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/sample"
)

// Report is every ship to show, the ones in flight first.
type Report struct {
	At    time.Time `json:"at"`
	Ships []Ship    `json:"ships"`
	// InFlight counts the ships that are not live yet: the N of the tab.
	InFlight int `json:"in_flight"`
	// Paused: GitHub's rate limit is low, nothing is read until then.
	Paused time.Time `json:"paused,omitzero"`
	// Unknown says why GitHub could not be read ("gh not signed in").
	Unknown string `json:"unknown,omitempty"`
	// Errors are the repositories whose last poll failed, and why. Their
	// ships keep the state read before.
	Errors []string `json:"errors,omitempty"`
}

// Ship is a pull request a session drives.
type Ship struct {
	Repo     string     `json:"repo"` // owner/name
	Number   int        `json:"number"`
	Title    string     `json:"title"`
	URL      string     `json:"url"`
	Session  SessionRef `json:"session"`
	Evidence string     `json:"evidence"` // created | pushed | acted | branch
	Stage    int        `json:"stage"`    // StageOpen … StageLive
	Kind     string     `json:"kind"`     // running | waits-for-you | stuck | done | unknown
	Note     string     `json:"note"`
	Stages   [5]Stage   `json:"stages"`
	// Since is when the current stage began; Age is how long ago that was.
	Since time.Time     `json:"since"`
	Age   time.Duration `json:"age_ns"`
	// Merge is the command that merges a ship that waits for your merge,
	// with the repository's merge method. The console shows it and asks.
	Merge []string `json:"merge,omitempty"`
}

// SessionRef names the session that drives a ship.
type SessionRef struct {
	Pane string `json:"pane"`
	ID   string `json:"session_id"`
	Name string `json:"name,omitempty"`
}

// Input is what the service knows: the live sessions with their
// transcripts, and the git scan's worktrees with their open PRs.
type Input struct {
	Sessions []sample.Session
	Git      *gitscan.Report
}

const (
	// PollEvery is the GitHub cadence while any ship is in flight.
	PollEvery = time.Minute
	// pushPollEvery is how often a repository with only pushes waiting for
	// their PR is asked for its open pull requests.
	pushPollEvery = 10 * time.Minute
	// pushFor is how long a push waits for a PR with its branch as head.
	pushFor = 24 * time.Hour
	// evidenceFor: older evidence starts no ship, so a long transcript
	// read for the first time does not wake every PR it ever touched.
	evidenceFor = 14 * 24 * time.Hour
	// stuckFor: a merge that never went live leaves the list after a day.
	stuckFor = 24 * time.Hour
	// forgetAfter drops what the store keeps about finished PRs and gone
	// transcripts.
	forgetAfter = 30 * 24 * time.Hour
	seenFor     = 7 * 24 * time.Hour
	// maxPerQuery bounds the pull requests of one query; the rest wait for
	// the next poll.
	maxPerQuery = 50
	// A PR never read stops being asked for after unreadFor or maxFails
	// failed polls: its repository is gone, or not ours to read.
	unreadFor = 24 * time.Hour
	maxFails  = 10
	// A repository whose poll fails is asked again after PollEvery, then
	// twice as long each time it fails again, up to backoffMax.
	backoffMax = 30 * time.Minute
	// repoRetry is how soon a directory whose remote could not be read is
	// read again; its evidence waits, at most unresolvedTries reads.
	repoRetry       = time.Minute
	unresolvedTries = 10
	maxUnresolved   = 200
)

// Tracker keeps what it found between scans and in Dir between runs.
// Scan runs on one goroutine at a time.
type Tracker struct {
	Dir    string // the state directory, for ships.json. Empty: nothing persists.
	GH     GHRunner
	Git    gitscan.Runner
	Remote string              // the remote that names a checkout's repository
	Deploy map[string][]string // owner/name -> the workflows that deploy it
	Now    func() time.Time

	st         *store
	repos      map[string]remote // directory -> its repository, as last read
	polled     time.Time
	pushPolled map[string]time.Time
	reason     string             // why gh cannot read GitHub at all
	failed     map[string]string  // repository -> why its last poll failed
	backoff    map[string]backoff // repository -> when a failing one is asked again
	tried      map[string]bool    // PRs asked for at least once since start
	dirty      bool
}

// remote is a directory's repository: "" when it is not on GitHub. A read
// that failed (ok false) is tried again after repoRetry.
type remote struct {
	repo string
	ok   bool
	at   time.Time
}

type backoff struct {
	fails int
	until time.Time
}

// unresolved is evidence whose directory's remote could not be read yet.
type unresolved struct {
	Found   found      `json:"found"`
	Session SessionRef `json:"session"`
	Tries   int        `json:"tries"`
}

type claim struct {
	Kind    string     `json:"kind"`
	At      time.Time  `json:"at"`
	Session SessionRef `json:"session"`
}

// beats: one PR belongs to the session with the strongest, then the
// newest evidence.
func (c claim) beats(o claim) bool {
	a, b := strength(c.Kind), strength(o.Kind)
	return a > b || a == b && c.At.After(o.At)
}

type pr struct {
	Repo   string    `json:"repo"`
	Number int       `json:"number"`
	Claim  claim     `json:"claim"`
	Facts  *facts    `json:"facts,omitempty"`
	LiveAt time.Time `json:"live_at,omitzero"`
	// Done: never read or shown again (closed, not a PR, or dropped).
	Done bool `json:"done,omitempty"`
	// Asked is the first poll that asked for it; Fails counts the polls
	// that failed while it was never read.
	Asked time.Time `json:"asked,omitzero"`
	Fails int       `json:"fails,omitempty"`
}

func (p *pr) inFlight() bool {
	return !p.Done && (p.Facts == nil || p.Facts.State == "OPEN" || p.Facts.State == "MERGED" && p.LiveAt.IsZero())
}

// push is a pushed branch that no known PR has as head yet.
type push struct {
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	Claim  claim  `json:"claim"`
}

type store struct {
	Transcripts map[string]*transcript `json:"transcripts"`
	PRs         map[string]*pr         `json:"prs"`
	Pushes      []push                 `json:"pushes,omitempty"`
	Unresolved  []unresolved           `json:"unresolved,omitempty"`
	Paused      time.Time              `json:"paused,omitzero"`
}

func (t *Tracker) path() string { return filepath.Join(t.Dir, "ships.json") }

func (t *Tracker) load() {
	t.st = &store{}
	if t.Dir != "" {
		if b, err := os.ReadFile(t.path()); err == nil {
			_ = json.Unmarshal(b, t.st) // a broken file starts over: transcripts are read again
		}
	}
	if t.st.Transcripts == nil {
		t.st.Transcripts = map[string]*transcript{}
	}
	// Keys are lower case: GitHub names a repository in any case. A file
	// from before keeps one PR per key, the stronger claim.
	prs := map[string]*pr{}
	for _, p := range t.st.PRs {
		p.Repo = strings.ToLower(p.Repo)
		k := key(p.Repo, p.Number)
		if o, ok := prs[k]; !ok || p.Claim.beats(o.Claim) {
			prs[k] = p
		}
	}
	t.st.PRs = prs
	for i := range t.st.Pushes {
		t.st.Pushes[i].Repo = strings.ToLower(t.st.Pushes[i].Repo)
	}
	t.repos, t.pushPolled = map[string]remote{}, map[string]time.Time{}
	t.failed, t.tried, t.backoff = map[string]string{}, map[string]bool{}, map[string]backoff{}
}

// save replaces ships.json atomically.
func (t *Tracker) save() error {
	if t.Dir == "" || !t.dirty {
		return nil
	}
	b, err := json.Marshal(t.st)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(t.Dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(t.Dir, ".ships-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), t.path()); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	t.dirty = false
	return nil
}

func (t *Tracker) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now()
}

// Scan reads the new bytes of every live session's transcripts, reads
// GitHub when a poll is due, and reports.
func (t *Tracker) Scan(ctx context.Context, in Input) Report {
	if t.st == nil {
		t.load()
	}
	now := t.now()
	t.resolve(ctx, now)
	for _, s := range in.Sessions {
		if s.Transcript == "" {
			continue
		}
		ref := SessionRef{Pane: s.Pane, ID: s.SessionID, Name: s.Name}
		// A subagent's calls are its session's work.
		subs, _ := filepath.Glob(filepath.Join(filepath.Dir(s.Transcript), s.SessionID, "subagents", "*.jsonl"))
		for _, path := range append([]string{s.Transcript}, subs...) {
			t.readTranscript(ctx, path, ref, now)
		}
	}
	t.links(in, now)
	t.prune(now)
	t.poll(ctx, now)
	rep := t.report(now)
	_ = t.save() // a full disk loses the offsets, not the ships on screen
	return rep
}

func (t *Tracker) readTranscript(ctx context.Context, path string, ref SessionRef, now time.Time) {
	tr, ok := t.st.Transcripts[path]
	if !ok {
		tr = &transcript{}
		t.st.Transcripts[path] = tr
	}
	before := tr.Offset
	tr.Session, tr.Seen = ref, now
	finds, err := read(path, tr)
	if err != nil {
		return
	}
	if tr.Offset != before {
		t.dirty = true
	}
	for _, f := range finds {
		t.apply(ctx, f, ref, now)
	}
}

func (t *Tracker) apply(ctx context.Context, f found, ref SessionRef, now time.Time) {
	if now.Sub(f.At) > evidenceFor {
		return
	}
	repo := f.Repo
	if repo == "" {
		r, known := t.repoOf(ctx, f.Dir, now)
		if !known {
			// The remote could not be read now: the evidence waits for it.
			if len(t.st.Unresolved) < maxUnresolved {
				t.st.Unresolved, t.dirty = append(t.st.Unresolved, unresolved{Found: f, Session: ref}), true
			}
			return
		}
		repo = r
	}
	if repo == "" {
		return
	}
	repo = strings.ToLower(repo)
	c := claim{Kind: f.Kind, At: f.At, Session: ref}
	if f.Kind != Pushed {
		t.claim(repo, f.Number, c)
		return
	}
	for _, p := range t.st.PRs {
		if p.Repo == repo && p.Facts != nil && p.Facts.State == "OPEN" && p.Facts.Head == f.Branch {
			t.claim(repo, p.Number, c)
			return
		}
	}
	for i, p := range t.st.Pushes {
		if p.Repo == repo && p.Branch == f.Branch {
			if c.beats(p.Claim) {
				t.st.Pushes[i].Claim, t.dirty = c, true
			}
			return
		}
	}
	t.st.Pushes = append(t.st.Pushes, push{Repo: repo, Branch: f.Branch, Claim: c})
	t.dirty = true
}

// resolve applies the evidence that waits for its directory's remote, now
// that it may be read.
func (t *Tracker) resolve(ctx context.Context, now time.Time) {
	if len(t.st.Unresolved) == 0 {
		return
	}
	waiting := t.st.Unresolved
	t.st.Unresolved, t.dirty = nil, true
	for _, u := range waiting {
		if now.Sub(u.Found.At) > evidenceFor || u.Tries >= unresolvedTries {
			continue
		}
		if r, ok := t.repos[u.Found.Dir]; ok && !r.ok && now.Sub(r.at) < repoRetry {
			t.st.Unresolved = append(t.st.Unresolved, u) // not due yet
			continue
		}
		u.Tries++
		if _, known := t.repoOf(ctx, u.Found.Dir, now); !known {
			t.st.Unresolved = append(t.st.Unresolved, u)
			continue
		}
		t.apply(ctx, u.Found, u.Session, now)
	}
}

// key names a pull request: owner/name in lower case, as GitHub matches it.
func key(repo string, n int) string { return strings.ToLower(repo) + "#" + strconv.Itoa(n) }

func (t *Tracker) claim(repo string, n int, c claim) {
	repo = strings.ToLower(repo)
	k := key(repo, n)
	p, ok := t.st.PRs[k]
	if !ok {
		p = &pr{Repo: repo, Number: n}
		t.st.PRs[k] = p
	}
	if c.beats(p.Claim) {
		p.Claim, t.dirty = c, true
	}
}

// links adds today's link through the worktree's branch: a session that
// works in a worktree whose branch has an open PR.
func (t *Tracker) links(in Input, now time.Time) {
	if in.Git == nil {
		return
	}
	for _, r := range in.Git.Repos {
		for _, wt := range r.Worktrees {
			if wt.PR == nil {
				continue
			}
			m := pullURL.FindStringSubmatch(wt.PR.URL)
			if m == nil {
				continue
			}
			for _, s := range in.Sessions {
				if s.Cwd == wt.Path || strings.HasPrefix(s.Cwd, wt.Path+string(filepath.Separator)) {
					at := s.Started
					if at.IsZero() {
						at = now
					}
					t.claim(m[1]+"/"+m[2], wt.PR.Number, claim{Kind: Branch, At: at, Session: SessionRef{Pane: s.Pane, ID: s.SessionID, Name: s.Name}})
				}
			}
		}
	}
}

// repoOf names the GitHub repository of a directory by its remote: ""
// when it is not on GitHub. known is false when the remote could not be
// read: it is read again after repoRetry. A directory that is gone (a
// removed worktree) is read at the nearest parent that is not.
func (t *Tracker) repoOf(ctx context.Context, dir string, now time.Time) (repo string, known bool) {
	if dir == "" || t.Git == nil {
		return "", true
	}
	if r, ok := t.repos[dir]; ok && (r.ok || now.Sub(r.at) < repoRetry) {
		return r.repo, r.ok
	}
	d := filepath.Clean(dir)
	for d != "/" && d != "." {
		if _, err := os.Stat(d); err == nil {
			break
		}
		d = filepath.Dir(d)
	}
	name := t.Remote
	if name == "" {
		name = "origin"
	}
	out, err := t.Git(ctx, d, "remote", "get-url", name)
	r := remote{ok: err == nil, at: now}
	if err == nil {
		r.repo = repoName(string(out))
	}
	t.repos[dir] = r
	return r.repo, r.ok
}

func (t *Tracker) prune(now time.Time) {
	for path, tr := range t.st.Transcripts {
		if now.Sub(tr.Seen) > seenFor {
			delete(t.st.Transcripts, path)
			t.dirty = true
		}
	}
	for k, p := range t.st.PRs {
		if p.Done && now.Sub(p.Claim.At) > forgetAfter {
			delete(t.st.PRs, k)
			t.dirty = true
		}
	}
	kept := t.st.Pushes[:0]
	for _, p := range t.st.Pushes {
		if now.Sub(p.Claim.At) <= pushFor {
			kept = append(kept, p)
		}
	}
	if len(kept) != len(t.st.Pushes) {
		t.dirty = true
	}
	t.st.Pushes = kept
}

// poll reads GitHub when a ship in flight is due (every PollEvery, or now
// for one never asked for) or a push waits for its PR: one query per
// repository. Nothing in flight, nothing read.
func (t *Tracker) poll(ctx context.Context, now time.Time) {
	if now.Before(t.st.Paused) {
		return
	}
	byRepo := map[string][]*pr{}
	fresh := false
	for k, p := range t.st.PRs {
		if p.inFlight() {
			byRepo[p.Repo] = append(byRepo[p.Repo], p)
			fresh = fresh || p.Facts == nil && !t.tried[k]
		}
	}
	due := fresh || now.Sub(t.polled) >= PollEvery
	repos := map[string]bool{}
	if due {
		for r := range byRepo {
			repos[r] = true
		}
	}
	pushes := map[string]bool{}
	for _, p := range t.st.Pushes {
		if now.Sub(t.pushPolled[p.Repo]) >= pushPollEvery || repos[p.Repo] {
			repos[p.Repo], pushes[p.Repo] = true, true
		}
	}
	for r := range repos {
		if now.Before(t.backoff[r].until) {
			delete(repos, r) // it failed: asked again later, each time later
		}
	}
	if len(repos) == 0 {
		return
	}
	if t.GH == nil {
		t.reason = why(errNoGH)
		return
	}
	names := make([]string, 0, len(repos))
	for r := range repos {
		names = append(names, r)
	}
	sort.Strings(names)
	if due {
		t.polled = now
	}
	t.reason = ""
	for _, repo := range names {
		err := t.pollRepo(ctx, repo, byRepo[repo], pushes[repo], now)
		if err == nil {
			delete(t.failed, repo)
			delete(t.backoff, repo)
			if now.Before(t.st.Paused) {
				return
			}
			continue
		}
		switch w := why(err); w {
		case "GitHub rate limit":
			t.pause(ctx, now)
			return
		case "gh not installed", "gh not signed in":
			t.reason = w
			return
		default:
			t.failed[repo] = w
			t.fail(repo, byRepo[repo], now)
		}
	}
}

// fail backs a repository off, and gives up on its PRs that were never
// read once they failed long or often enough.
func (t *Tracker) fail(repo string, prs []*pr, now time.Time) {
	b := t.backoff[repo]
	b.fails++
	wait := backoffMax
	if b.fails <= 6 {
		wait = min(PollEvery<<(b.fails-1), backoffMax)
	}
	b.until = now.Add(wait)
	t.backoff[repo] = b
	for _, p := range prs {
		if p.Facts != nil || p.Asked.IsZero() {
			continue
		}
		p.Fails++
		if p.Fails >= maxFails || now.Sub(p.Asked) >= unreadFor {
			p.Done = true
		}
		t.dirty = true
	}
}

func (t *Tracker) pollRepo(ctx context.Context, repo string, prs []*pr, pushes bool, now time.Time) error {
	owner, name, _ := strings.Cut(repo, "/")
	sort.Slice(prs, func(i, j int) bool {
		if (prs[i].Facts == nil) != (prs[j].Facts == nil) {
			return prs[i].Facts == nil
		}
		return prs[i].Number > prs[j].Number
	})
	if len(prs) > maxPerQuery {
		prs = prs[:maxPerQuery]
	}
	var numbers []int
	history := false
	for _, p := range prs {
		if p.Asked.IsZero() {
			p.Asked, t.dirty = now, true
		}
		if prNumber(strconv.Itoa(p.Number)) == 0 {
			p.Done, t.dirty = true, true // a number GitHub cannot take
			continue
		}
		numbers = append(numbers, p.Number)
		t.tried[key(repo, p.Number)] = true
		history = history || p.Facts != nil && p.Facts.superseded()
	}
	out, err := t.GH(ctx, "api", "graphql", "-f", "query="+query(owner, name, numbers, history, pushes))
	a, perr := parseAnswer(out, numbers, history, now)
	var bad badPRs
	if errors.As(perr, &bad) {
		// GitHub refused the query for some of its PRs: they are dropped,
		// and the rest asked again without them.
		kept := prs[:0:0]
		for _, p := range prs {
			if slices.Contains(bad.numbers, p.Number) {
				p.Done, t.dirty = true, true
				continue
			}
			kept = append(kept, p)
		}
		if len(kept) == len(prs) {
			return perr
		}
		return t.pollRepo(ctx, repo, kept, pushes, now)
	}
	if perr != nil {
		if err == nil {
			err = perr
		}
		return err
	}
	// gh exits non-zero for a response with errors, which still carries
	// the data of every PR it could read.
	for _, p := range prs {
		f, ok := a.PRs[p.Number]
		switch {
		case !ok:
		case f == nil || f.State == "CLOSED":
			p.Done = true // no such pull request in this repository, or closed
		default:
			p.Facts = f
			if tr := track(*f, t.deploy(repo), now); tr.Stage == StageLive {
				p.LiveAt = tr.LiveAt
			}
		}
	}
	t.dirty = true
	if a.Heads != nil {
		t.pushPolled[repo] = now
		kept := t.st.Pushes[:0]
		for _, p := range t.st.Pushes {
			if n, ok := a.Heads[p.Branch]; ok && p.Repo == repo {
				t.claim(repo, n, p.Claim)
				continue
			}
			kept = append(kept, p)
		}
		t.st.Pushes = kept
	}
	if r := a.Rate; r != nil && r.Limit > 0 && r.Remaining*10 < r.Limit {
		t.st.Paused = r.ResetAt
	}
	return nil
}

// deploy is the deploy workflows the config names for a repository, in
// any case.
func (t *Tracker) deploy(repo string) []string {
	for r, ws := range t.Deploy {
		if strings.EqualFold(r, repo) {
			return ws
		}
	}
	return nil
}

// pause stops reading until the GraphQL budget resets. The rate_limit
// endpoint costs no budget.
func (t *Tracker) pause(ctx context.Context, now time.Time) {
	t.st.Paused, t.dirty = now.Add(15*time.Minute), true
	out, err := t.GH(ctx, "api", "rate_limit")
	var rl struct {
		Resources struct {
			GraphQL struct {
				Reset int64 `json:"reset"`
			} `json:"graphql"`
		} `json:"resources"`
	}
	if err == nil && json.Unmarshal(out, &rl) == nil && rl.Resources.GraphQL.Reset > 0 {
		t.st.Paused = time.Unix(rl.Resources.GraphQL.Reset, 0)
	}
}

// report builds the list. A live ship stays until the end of the day it
// went live; a merge that never went live, a day after it merged.
func (t *Tracker) report(now time.Time) Report {
	rep := Report{At: now, Ships: []Ship{}, Unknown: t.reason}
	for repo, w := range t.failed {
		rep.Errors = append(rep.Errors, repo+": "+w)
	}
	sort.Strings(rep.Errors)
	paused := now.Before(t.st.Paused)
	if paused {
		rep.Paused = t.st.Paused
	}
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	for _, p := range t.st.PRs {
		if p.Done {
			continue
		}
		f := p.Facts
		if f != nil && (!p.LiveAt.IsZero() && p.LiveAt.Before(today) || p.LiveAt.IsZero() && f.State == "MERGED" && now.Sub(f.Merged) > stuckFor) {
			p.Done, t.dirty = true, true
			continue
		}
		rep.Ships = append(rep.Ships, t.ship(p, paused, now))
	}
	order := map[string]int{WaitsForYou: 0, Stuck: 1, Running: 2, Unknown: 3, Done: 4}
	sort.Slice(rep.Ships, func(i, j int) bool {
		a, b := rep.Ships[i], rep.Ships[j]
		if order[a.Kind] != order[b.Kind] {
			return order[a.Kind] < order[b.Kind]
		}
		return a.Since.After(b.Since)
	})
	for _, s := range rep.Ships {
		if s.Kind != Done {
			rep.InFlight++
		}
	}
	return rep
}

func (t *Tracker) ship(p *pr, paused bool, now time.Time) Ship {
	s := Ship{
		Repo: p.Repo, Number: p.Number, Session: p.Claim.Session, Evidence: p.Claim.Kind,
		URL:  fmt.Sprintf("https://github.com/%s/pull/%d", p.Repo, p.Number),
		Kind: Unknown, Since: p.Claim.At,
	}
	if f := p.Facts; f != nil {
		tr := track(*f, t.deploy(p.Repo), now)
		s.Title, s.URL = f.Title, f.URL
		if m := pullURL.FindStringSubmatch(f.URL); m != nil {
			s.Repo = m[1] + "/" + m[2] // the case GitHub names it in
		}
		s.Stage, s.Kind, s.Note, s.Stages = tr.Stage, tr.Kind, tr.Note, tr.Stages
		if at := tr.Stages[tr.Stage].At; !at.IsZero() {
			s.Since = at
		} else if !f.Created.IsZero() {
			s.Since = f.Created
		}
		if tr.Note == "green · waits for your merge" && f.HeadOID != "" {
			// Pinned to the head that was read green: a push since fails it.
			s.Merge = []string{"gh", "pr", "merge", strconv.Itoa(p.Number), "--" + f.Method, "--match-head-commit", f.HeadOID, "-R", s.Repo}
		}
	}
	// A merge is offered only on a fresh read: not while paused, not when
	// the repository's last poll failed, not two polls after the last read.
	stale := p.Facts != nil && now.Sub(p.Facts.Fetched) > 2*PollEvery
	switch {
	case !p.inFlight():
	case paused:
		s.Note = "paused · GitHub rate limit until " + t.st.Paused.In(now.Location()).Format("15:04")
	case t.reason != "":
		s.Kind, s.Note, s.Merge = Unknown, "state unknown · "+t.reason, nil
	case p.Facts == nil && t.failed[p.Repo] != "":
		s.Note = "state unknown · " + t.failed[p.Repo]
	case p.Facts == nil:
		s.Note = "state unknown · not read yet"
	case s.Kind == WaitsForYou && t.failed[p.Repo] != "":
		s.Kind, s.Note = Unknown, "state unknown · "+t.failed[p.Repo]
	case s.Kind == WaitsForYou && stale:
		s.Kind, s.Note = Unknown, "state unknown · last read "+p.Facts.Fetched.In(now.Location()).Format("15:04")
	}
	if paused || t.failed[p.Repo] != "" || stale {
		s.Merge = nil
		if s.Kind == WaitsForYou {
			s.Kind = Unknown
		}
	}
	s.Stages[s.Stage].Note = s.Note
	s.Age = now.Sub(s.Since)
	return s
}
