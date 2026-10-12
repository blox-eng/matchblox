package ships

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/sample"
)

// world is a machine with transcripts, a state directory, a clock and a
// GitHub that answers every query with the recorded response.
type world struct {
	t      *testing.T
	root   string
	clock  time.Time
	answer []byte
	ghErr  error
	calls  []string
}

func newWorld(t *testing.T) *world {
	b, err := os.ReadFile("testdata/graphql-open-merged.json")
	if err != nil {
		t.Fatal(err)
	}
	return &world{t: t, root: t.TempDir(), clock: now, answer: b}
}

func (w *world) tracker() *Tracker {
	return &Tracker{
		Dir: filepath.Join(w.root, "state"),
		GH: func(_ context.Context, args ...string) ([]byte, error) {
			w.calls = append(w.calls, strings.Join(args, " "))
			if w.ghErr != nil {
				return nil, w.ghErr
			}
			if args[1] == "rate_limit" {
				return []byte(`{"resources":{"graphql":{"reset":1791809103}}}`), nil
			}
			return w.answer, errors.New("exit status 1") // gh's exit for a response with errors
		},
		Git: func(context.Context, string, ...string) ([]byte, error) {
			return []byte("git@github.com:hearth/hearth.git\n"), nil
		},
		Now: func() time.Time { return w.clock },
	}
}

// session writes a transcript where Claude Code keeps it and returns the
// session the sampler would report.
func (w *world) session(id, pane string, lines ...string) sample.Session {
	path := filepath.Join(w.root, "projects", "-w", id+".jsonl")
	os.MkdirAll(filepath.Dir(path), 0o700)
	appendFile(w.t, path, strings.Join(lines, "\n")+"\n")
	return sample.Session{SessionID: id, Pane: pane, Name: "agent " + id, Cwd: "/w", Transcript: path}
}

func (w *world) scan(tr *Tracker, ss ...sample.Session) Report {
	return tr.Scan(context.Background(), Input{Sessions: ss})
}

func find(rep Report, n int) *Ship {
	for i := range rep.Ships {
		if rep.Ships[i].Number == n {
			return &rep.Ships[i]
		}
	}
	return nil
}

func shifted(line string, d time.Duration) string {
	return strings.Replace(line, at.Format(time.RFC3339Nano), at.Add(d).Format(time.RFC3339Nano), 1)
}

// TestOwnership: one PR belongs to the session with the strongest, then
// the newest evidence; the link through a worktree's branch is the weakest.
func TestOwnership(t *testing.T) {
	w := newWorld(t)
	a := w.session("a", "%1",
		toolUse("c1", "/w", "", "gh pr create --fill"),
		toolResult("c1", "/w", "https://github.com/hearth/hearth/pull/4557", nil),
		toolUse("v1", "/w", "", "gh pr view 4551"),
	)
	b := w.session("b", "%2",
		shifted(toolUse("v2", "/w", "", "gh pr merge 4557 --squash && gh pr checks 4551"), time.Hour),
	)
	c := sample.Session{SessionID: "c", Pane: "%3", Cwd: "/wt/conflicted/sub", Started: at}
	git := &gitscan.Report{Repos: []gitscan.Repo{{Path: "/w", Worktrees: []gitscan.Worktree{
		{Path: "/wt/conflicted", Branch: "feat/4545", PR: &gitscan.PR{Number: 4545, URL: "https://github.com/hearth/hearth/pull/4545"}},
	}}}}
	rep := w.tracker().Scan(context.Background(), Input{Sessions: []sample.Session{a, b, c}, Git: git})

	for n, want := range map[int]struct{ pane, evidence string }{
		4557: {"%1", Created}, // created beats a newer merge
		4551: {"%2", Acted},   // the newer of two
		4545: {"%3", Branch},
	} {
		s := find(rep, n)
		if s == nil {
			t.Fatalf("#%d is not a ship: %+v", n, rep.Ships)
		}
		if s.Session.Pane != want.pane || s.Evidence != want.evidence {
			t.Errorf("#%d belongs to %s by %s, want %s by %s", n, s.Session.Pane, s.Evidence, want.pane, want.evidence)
		}
	}
	if s := find(rep, 4557); s.Kind != Stuck || s.Note != "CI failed · ci-ok" || s.Title == "" || s.Session.Name != "agent a" {
		t.Errorf("#4557: %+v", s)
	}
	if len(w.calls) != 1 || !strings.Contains(w.calls[0], "pr4557:pullRequest(number:4557)") {
		t.Fatalf("one query for the repository, got %q", w.calls)
	}
}

// TestPollCadence: a new PR is read at once, then every PollEvery while
// any is in flight, and never when none is.
func TestPollCadence(t *testing.T) {
	w := newWorld(t)
	tr := w.tracker()
	s := w.session("a", "%1", toolUse("v", "/w", "", "gh pr view 4557"))
	w.scan(tr, s)
	w.clock = w.clock.Add(15 * time.Second)
	w.scan(tr, s)
	if len(w.calls) != 1 {
		t.Fatalf("calls %d, want 1", len(w.calls))
	}
	w.clock = w.clock.Add(PollEvery)
	w.scan(tr, s)
	if len(w.calls) != 2 {
		t.Fatalf("calls %d, want 2 after a minute", len(w.calls))
	}

	// #4369 went live days ago and #999999 is no PR: one read, then nothing.
	w2 := newWorld(t)
	tr2 := w2.tracker()
	s2 := w2.session("b", "%2", toolUse("v", "/w", "", "gh pr view 4369; gh pr view 999999"))
	if rep := w2.scan(tr2, s2); len(rep.Ships) != 0 || rep.InFlight != 0 {
		t.Fatalf("finished PRs listed: %+v", rep.Ships)
	}
	for i := 0; i < 3; i++ {
		w2.clock = w2.clock.Add(2 * PollEvery)
		w2.scan(tr2, s2)
	}
	if len(w2.calls) != 1 {
		t.Fatalf("polled with nothing in flight: %q", w2.calls)
	}
}

// TestPersistence: offsets and ships survive a restart; the transcript is
// not read again, only what was written since.
func TestPersistence(t *testing.T) {
	w := newWorld(t)
	s := w.session("a", "%1",
		toolUse("c1", "/w", "", "gh pr create --fill"),
		toolResult("c1", "/w", "https://github.com/hearth/hearth/pull/4557", nil),
		toolUse("p1", "/w", "", "gh pr view 4551"), // its result comes after the restart
	)
	w.scan(w.tracker(), s)
	if _, err := os.Stat(filepath.Join(w.root, "state", "ships.json")); err != nil {
		t.Fatal(err)
	}

	tr := w.tracker() // the service restarted
	tr.load()
	saved := tr.st.Transcripts[s.Transcript]
	if saved == nil || saved.Offset != size(t, s.Transcript) || saved.Session.Pane != "%1" {
		t.Fatalf("offset not kept: %+v", saved)
	}
	if p := tr.st.PRs[key("hearth/hearth", 4557)]; p == nil || p.Claim.Kind != Created || p.Facts == nil {
		t.Fatalf("ship not kept: %+v", p)
	}
	appendFile(t, s.Transcript, shifted(toolUse("v2", "/w", "", "gh pr ready 4548"), time.Minute)+"\n")
	rep := w.scan(tr, s)
	if find(rep, 4557) == nil || find(rep, 4551) == nil || find(rep, 4548) == nil {
		t.Fatalf("after a restart: %+v", rep.Ships)
	}
	if got := tr.st.Transcripts[s.Transcript].Offset; got != size(t, s.Transcript) {
		t.Fatalf("offset %d, file %d", got, size(t, s.Transcript))
	}
}

// TestPushFindsItsPR: a pushed branch becomes a ship when an open PR has
// it as head.
func TestPushFindsItsPR(t *testing.T) {
	w := newWorld(t)
	tr := w.tracker()
	rep := w.scan(tr, w.session("a", "%1", toolUse("p", "/w", "main", "git push -u origin feat/4550")))
	s := find(rep, 4550)
	if s == nil || s.Evidence != Pushed || s.Session.Pane != "%1" {
		t.Fatalf("the push found no ship: %+v", rep.Ships)
	}
	if !strings.Contains(w.calls[0], "open:pullRequests(states:OPEN") {
		t.Fatalf("the query did not ask for open PRs: %s", w.calls[0])
	}
	if len(tr.st.Pushes) != 0 {
		t.Fatalf("the push still waits: %+v", tr.st.Pushes)
	}
}

// TestRateLimitPause: under 10% of the budget left, nothing is read until
// the reset and the notes say so.
func TestRateLimitPause(t *testing.T) {
	w := newWorld(t)
	w.answer = bytes.Replace(w.answer, []byte(`"remaining": 4951`), []byte(`"remaining": 499`), 1)
	tr := w.tracker()
	s := w.session("a", "%1", toolUse("v", "/w", "", "gh pr view 4557"))
	rep := w.scan(tr, s)
	reset := time.Date(2026, 10, 12, 12, 47, 14, 0, time.UTC)
	if !rep.Paused.Equal(reset) {
		t.Fatalf("paused until %v", rep.Paused)
	}
	if sh := find(rep, 4557); sh.Note != "paused · GitHub rate limit until 12:47" || sh.Stage != StageCI {
		t.Fatalf("note %q stage %d", sh.Note, sh.Stage)
	}
	w.clock = w.clock.Add(10 * time.Minute)
	w.scan(tr, s)
	if len(w.calls) != 1 {
		t.Fatalf("read while paused: %d calls", len(w.calls))
	}
	w.clock = reset.Add(time.Second)
	w.scan(tr, s)
	if len(w.calls) != 2 {
		t.Fatalf("not read after the reset: %d calls", len(w.calls))
	}
}

// TestNoGH: without gh, or signed out, ships still list from the
// transcripts, with the reason.
func TestNoGH(t *testing.T) {
	for err, note := range map[error]string{
		errors.New("exit status 4: To get started with GitHub CLI, please run:  gh auth login"): "state unknown · gh not signed in",
		errNoGH: "state unknown · gh not installed",
	} {
		w := newWorld(t)
		w.ghErr = err
		rep := w.scan(w.tracker(), w.session("a", "%1", toolUse("v", "/w", "", "gh pr view 4557")))
		s := find(rep, 4557)
		if s == nil || s.Kind != Unknown || s.Note != note || s.URL != "https://github.com/hearth/hearth/pull/4557" {
			t.Fatalf("%v: %+v", err, s)
		}
	}
}

// TestDropsAtTheEndOfTheDay: a ship that went live stays until the end of
// that day (design 0006 §11.4).
func TestDropsAtTheEndOfTheDay(t *testing.T) {
	w := newWorld(t)
	w.clock = time.Date(2026, 10, 6, 15, 0, 0, 0, time.UTC) // #4369 merged at 11:14 and went live that day
	tr := w.tracker()
	s := w.session("a", "%1", toolUse("v", "/w", "", "gh pr view 4369"))
	rep := w.scan(tr, s)
	sh := find(rep, 4369)
	if sh == nil || sh.Kind != Done || sh.Stage != StageLive || rep.InFlight != 0 {
		t.Fatalf("live today: %+v", rep)
	}
	w.clock = time.Date(2026, 10, 6, 23, 59, 0, 0, time.UTC)
	if find(w.scan(tr, s), 4369) == nil {
		t.Fatal("dropped before the day ended")
	}
	w.clock = time.Date(2026, 10, 7, 0, 0, 1, 0, time.UTC)
	if rep := w.scan(tr, s); len(rep.Ships) != 0 {
		t.Fatalf("still listed the next day: %+v", rep.Ships)
	}
	if len(w.calls) != 1 {
		t.Fatalf("a live ship was read again: %d calls", len(w.calls))
	}
}

// TestMergeCommand: a ship that waits for your merge carries the command
// with the repository's merge method; no other does.
func TestMergeCommand(t *testing.T) {
	tr := &Tracker{}
	tr.load()
	f := open(func(f *facts) { f.Method = "rebase" })
	p := &pr{Repo: "o/r", Number: 12, Facts: &f}
	s := tr.ship(p, false, now)
	want := []string{"gh", "pr", "merge", "12", "--rebase", "-R", "o/r"}
	if strings.Join(s.Merge, " ") != strings.Join(want, " ") {
		t.Fatalf("merge %q", s.Merge)
	}
	if s.Age != 10*time.Minute || !s.Since.Equal(f.Checks.Done) {
		t.Fatalf("since %v age %v", s.Since, s.Age)
	}
	f.AutoMerge = true
	if s := tr.ship(p, false, now); s.Merge != nil {
		t.Fatalf("auto-merge on, merge %q", s.Merge)
	}
}

// TestRepoFailure: a repository whose poll fails keeps the state its ships
// had, says why, and is not asked again before the minute is up.
func TestRepoFailure(t *testing.T) {
	w := newWorld(t)
	tr := w.tracker()
	s := w.session("a", "%1", toolUse("v", "/w", "", "gh pr view 4557; gh pr view 4548"))
	w.scan(tr, s)
	w.ghErr = errors.New("exit status 1: gh: HTTP 504")
	w.clock = w.clock.Add(PollEvery)
	rep := w.scan(tr, s)
	if sh := find(rep, 4557); sh.Kind != Stuck || sh.Note != "CI failed · ci-ok" {
		t.Fatalf("a failed poll lost the state: %+v", sh)
	}
	if len(rep.Errors) != 1 || !strings.HasPrefix(rep.Errors[0], "hearth/hearth: ") || rep.Unknown != "" {
		t.Fatalf("errors %q unknown %q", rep.Errors, rep.Unknown)
	}

	// A PR first seen while the repository fails is asked for once, then
	// waits for the minute like the rest.
	appendFile(t, s.Transcript, toolUse("v2", "/w", "", "gh pr view 4551")+"\n")
	w.clock = w.clock.Add(time.Second)
	if sh := find(w.scan(tr, s), 4551); sh == nil || sh.Kind != Unknown || !strings.HasPrefix(sh.Note, "state unknown · ") {
		t.Fatalf("#4551: %+v", sh)
	}
	calls := len(w.calls)
	w.clock = w.clock.Add(15 * time.Second)
	w.scan(tr, s)
	if len(w.calls) != calls {
		t.Fatalf("a failing repository was asked again within the minute")
	}
}
