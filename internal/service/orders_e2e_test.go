package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/ships"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/stoker"
)

// The acceptance run of the stoker's orders: a stack of three pull
// requests, #1 ← #2 ← #3, each driven by its own session, and a stoker
// agent on its own tmux server (%9@mbtest). The agent and the sessions are
// fakes that read what the service types into their panes and answer as
// the real ones would: the stoker with the acts its CLI sends, a session
// by rebasing, watching CI and merging through the hook's merge guard.

const (
	mergeOnGreen = "set the ready ones up in merge order and merge them on green"
	ciTakes      = 2 * time.Minute
)

// stackSession is a session that drives one pull request of the stack.
type stackSession struct {
	name, pane string
	pr         int
	busy       bool // told, and working on it
	ended      time.Time
	read       int // lines of its pane read
}

// stackShip is the pull request a session drives.
type stackShip struct {
	stage  int
	kind   string
	note   string
	failed string
	ciFrom time.Time
	since  time.Time
}

// stackRun is the world of the acceptance run.
type stackRun struct {
	*Service
	t        *testing.T
	ran      *[][]string
	clock    time.Time
	sessions []*stackSession
	ships    map[int]*stackShip
	redCI    map[int]string // the check that fails on a pull request's CI
	merged   []int
	denied   []int // merges the hook check stopped
	agent    *fakeStoker
}

func newStackRun(t *testing.T) *stackRun {
	t.Helper()
	s, ran := handsTest(t)
	if err := s.Stoker.Set(stoker.Mode{}); err != nil {
		t.Fatal(err)
	}
	w := &stackRun{Service: s, t: t, ran: ran, clock: night0, ships: map[int]*stackShip{}, redCI: map[int]string{}}
	for i, name := range []string{"kindle-web", "flint-auth", "hearth-api"} {
		w.sessions = append(w.sessions, &stackSession{name: name, pane: fmt.Sprintf("%%%d", i+1), pr: i + 1})
		// Stacked: each waits for a rebase onto the one below it.
		w.ships[i+1] = &stackShip{stage: ships.StageCI, kind: ships.Running, note: "needs a rebase", since: night0}
	}
	w.agent = &fakeStoker{w: w, steps: map[int]string{}}
	s.Now = func() time.Time { return w.clock }
	s.Capture = func(panes []string) map[string]string {
		out := map[string]string{}
		for _, p := range panes {
			out[p] = idleScreen
		}
		return out
	}
	s.Ships = func(context.Context, []sample.Session, *gitscan.Report) ships.Report { return w.report() }
	w.sample()
	return w
}

func (w *stackRun) report() ships.Report {
	r := ships.Report{At: w.clock}
	for _, x := range w.sessions {
		sh := w.ships[x.pr]
		r.Ships = append(r.Ships, ships.Ship{
			Repo: "hearth/stack", Number: x.pr, Title: fmt.Sprintf("layer %d", x.pr), Evidence: "created",
			Session: ships.SessionRef{Pane: x.pane, ID: "w-" + x.name, Name: x.name},
			Stage:   sh.stage, Kind: sh.kind, Note: sh.note, Failed: sh.failed, Since: sh.since, Age: w.clock.Sub(sh.since),
		})
		if sh.stage < ships.StageMerged {
			r.InFlight++
		}
	}
	return r
}

// sample is one tick of the service: the ships as shipsLoop reads them,
// the sessions as the sampler sees them, then the stoker's turn.
func (w *stackRun) sample() {
	r := w.Ships(w.t.Context(), nil, nil)
	w.mu.Lock()
	w.cur.Ships = &r
	w.mu.Unlock()
	ss := []sample.Session{{Agent: "claude", Pane: "%9@mbtest", Target: "stoker:1.1", SessionID: "s9", Name: "stoker", Status: "idle"}}
	for _, x := range w.sessions {
		ss = append(ss, sample.Session{
			Agent: "claude", Pane: x.pane, Target: "app:1." + x.pane[1:], SessionID: "w-" + x.name, Name: x.name,
			Status: "idle", Idle: time.Minute, Busy: x.busy, TurnEnded: x.ended,
		})
	}
	w.publish(sample.Snapshot{At: w.clock, Sessions: ss}, func(state.Doc) {})
	w.stoke(w.t.Context(), w.clock)
}

// tick moves the clock a minute: the sessions read what they were told
// and work, the service samples and wakes the stoker, and the stoker
// answers.
func (w *stackRun) tick() {
	w.clock = w.clock.Add(time.Minute)
	for _, x := range w.sessions {
		w.work(x)
	}
	w.sample()
	w.agent.answer()
}

// run ticks until done holds, at most an hour.
func (w *stackRun) run(done func() bool) {
	w.t.Helper()
	for range 60 {
		if done() {
			return
		}
		w.tick()
	}
	w.t.Fatalf("the run did not get there in an hour: merged %v, steps %v, the stoker heard %q", w.merged, w.agent.steps, w.agent.heard)
}

// work is one minute of a session: a message to merge sets it rebasing
// onto its base (or, green already, merging); CI ends after ciTakes; a
// green pull request is merged through the hook's merge guard.
func (w *stackRun) work(x *stackSession) {
	lines := typed(*w.ran, x.pane)
	told := lines[x.read:]
	x.read = len(lines)
	sh := w.ships[x.pr]
	if slices.ContainsFunc(told, func(l string) bool { return strings.Contains(l, "merge") }) && sh.stage < ships.StageMerged {
		x.busy = true
		if sh.stage < ships.StageGreen {
			sh.stage, sh.kind, sh.note, sh.failed, sh.ciFrom, sh.since = ships.StageCI, ships.Running, "CI · rebased", "", w.clock, w.clock
		}
		return // the turn begins with this minute
	}
	if !x.busy {
		return
	}
	if sh.stage == ships.StageCI && w.clock.Sub(sh.ciFrom) >= ciTakes {
		if check := w.redCI[x.pr]; check != "" {
			sh.kind, sh.note, sh.failed = ships.Stuck, "CI red", check
			w.endTurn(x)
			return
		}
		sh.stage, sh.kind, sh.note, sh.since = ships.StageGreen, ships.WaitsForYou, ships.NoteWaitsForMerge, w.clock
	}
	if sh.stage != ships.StageGreen {
		return
	}
	// The merge, as Claude Code runs it: the PreToolUse hook asks the
	// guard, and the service hears of the call.
	cmd := fmt.Sprintf("gh pr merge %d --squash", x.pr)
	denied := stoker.Denies(w.Stoker.Dir(), "/tmp/tmux-test/default", x.pane, stoker.Guards(cmd))
	w.Hook(hooks.Event{Name: "PreToolUse", SessionID: "w-" + x.name, Pane: x.pane, At: w.clock, Command: cmd, Guards: stoker.Guards(cmd), Denied: denied})
	if denied {
		w.denied = append(w.denied, x.pr)
	} else {
		sh.stage, sh.kind, sh.note, sh.since = ships.StageMerged, ships.Done, "merged", w.clock
		w.merged = append(w.merged, x.pr)
	}
	w.endTurn(x)
}

func (w *stackRun) endTurn(x *stackSession) { x.busy, x.ended = false, w.clock }

// fakeStoker is the stoker agent: it reads the lines typed into %9 and
// answers with the acts `matchblox stoker plan|step` and `matchblox send`
// send. It carries the plan out in merge order, one step at a time, and
// moves on when a step is done or waits for a go; a failed step stops it.
type fakeStoker struct {
	w       *stackRun
	read    int
	heard   []string
	cur     int // the step under way, 0 before go
	stopped bool
	steps   map[int]string // the last state it logged, by step
	errs    []string
}

var (
	turnEnded = regexp.MustCompile(`^(.+)'s turn ended: (.+)$`)
	goGiven   = regexp.MustCompile(`^go given for (\S+): `)
	prMerged  = regexp.MustCompile(`^#(\d+) merged · done$`)
	prWaits   = regexp.MustCompile(`^#(\d+) waits your merge$`)
)

func (f *fakeStoker) answer() {
	lines := typed(*f.w.ran, "%9@mbtest")
	news := lines[f.read:]
	f.read = len(lines)
	for _, l := range news {
		f.heard = append(f.heard, l)
		switch {
		case strings.HasPrefix(l, "Read ") && strings.HasSuffix(l, " and follow it."):
			f.plan(strings.TrimSuffix(strings.TrimPrefix(l, "Read "), " and follow it."))
			f.next()
		default:
			_, body, ok := strings.Cut(l, ": ")
			if !ok {
				continue
			}
			for _, part := range strings.Split(body, "; ") {
				f.hear(part)
			}
		}
	}
}

// plan reads the brief and writes the plan: the three layers in merge
// order.
func (f *fakeStoker) plan(path string) {
	if _, err := os.ReadFile(path); err != nil {
		f.errs = append(f.errs, err.Error())
		return
	}
	p := stoker.PlanText{}
	for _, x := range f.w.sessions {
		p.Steps = append(p.Steps, stoker.PlanStep{N: x.pr, Session: x.name, Text: fmt.Sprintf("watch #%d, rebase and merge on green", x.pr)})
	}
	f.act("stoker:plan", p)
}

func (f *fakeStoker) hear(line string) {
	if m := goGiven.FindStringSubmatch(line); m != nil {
		n := slices.IndexFunc(f.w.sessions, func(x *stackSession) bool { return x.name == m[1] }) + 1
		if n > 0 {
			f.send(n, fmt.Sprintf("the go is given: merge #%d now", n))
		}
		return
	}
	m := turnEnded.FindStringSubmatch(line)
	if m == nil {
		return
	}
	n := slices.IndexFunc(f.w.sessions, func(x *stackSession) bool { return x.name == m[1] }) + 1
	if n == 0 {
		return
	}
	switch why := m[2]; {
	case prMerged.MatchString(why):
		f.log(n, stoker.StepDone, "", why)
	case prWaits.MatchString(why):
		f.log(n, stoker.StepWaits, "", "green, waits for the merge")
	case strings.HasPrefix(why, "CI red · "):
		f.log(n, stoker.StepFailed, "", why)
		f.stopped = true
	default:
		return
	}
	if n == f.cur {
		f.next()
	}
}

// next starts the next step of the plan, unless a step failed.
func (f *fakeStoker) next() {
	if f.stopped || f.cur >= len(f.w.sessions) {
		return
	}
	f.cur++
	f.send(f.cur, fmt.Sprintf("watch #%d, rebase and merge on green", f.cur))
	f.log(f.cur, stoker.StepDoing, "", fmt.Sprintf("told %s to rebase #%d", f.w.sessions[f.cur-1].name, f.cur))
}

func (f *fakeStoker) send(n int, text string) {
	f.act("message:"+f.w.sessions[n-1].pane, text)
}

func (f *fakeStoker) log(n int, state, needs, text string) {
	f.steps[n] = state
	f.act("stoker:step", stoker.Entry{Step: n, State: state, Needs: needs, Text: text})
}

// act sends one act from the stoker's pane; a refusal is kept for the
// test to show.
func (f *fakeStoker) act(id string, body any) {
	text, ok := body.(string)
	if !ok {
		b, err := json.Marshal(body)
		if err != nil {
			f.w.t.Fatal(err)
		}
		text = string(b)
	}
	if res := f.w.act(f.w.t.Context(), proto.Act{RecID: id, Confirm: "y", Text: text, From: "%9@mbtest"}); res.Err != "" {
		f.errs = append(f.errs, id+": "+res.Err)
	}
}

// launch gives the order with the chips grants: the run starts at once,
// and the stoker plans it and begins.
func (w *stackRun) launch(words string, grants ...string) {
	w.t.Helper()
	mustAct(w.t, w.Service, orderAct(w.t, words, false, grants...))
	w.agent.answer()
	if o := w.Stoker.Mode().Order; o == nil || o.State != stoker.OrderRunning || len(o.Plan) != 3 {
		w.t.Fatalf("no plan: order %+v, stoker errors %q", o, w.agent.errs)
	}
}

// lastSteps are the last logged state of each step of the run, and its
// entry.
func (w *stackRun) lastSteps() map[int]stoker.Entry {
	out := map[int]stoker.Entry{}
	for _, e := range w.Stoker.View().Run {
		if e.Logged() && e.Step > 0 {
			out[e.Step] = e
		}
	}
	return out
}

func (w *stackRun) noStokerErrors() {
	w.t.Helper()
	if len(w.agent.errs) > 0 {
		w.t.Fatalf("the service refused the stoker: %q", w.agent.errs)
	}
}

func TestMergeOrderLandsTheStack(t *testing.T) {
	w := newStackRun(t)
	w.launch(mergeOnGreen, "merge")
	if o := w.Stoker.Mode().Order; !slices.Equal(o.Grants, []string{"merge"}) {
		t.Fatalf("the order's grants %q", o.Grants)
	}
	w.run(func() bool { return len(w.merged) == 3 })
	w.tick() // the stoker hears of the last merge
	w.noStokerErrors()
	if !slices.Equal(w.merged, []int{1, 2, 3}) || len(w.denied) != 0 {
		t.Fatalf("merged %v, denied %v", w.merged, w.denied)
	}
	steps := w.lastSteps()
	for n := 1; n <= 3; n++ {
		if e := steps[n]; e.State != stoker.StepDone || e.Pane != w.sessions[n-1].pane {
			t.Errorf("step %d ends %+v", n, e)
		}
	}
	// Each session was told once, in merge order.
	for _, x := range w.sessions {
		if got := typed(*w.ran, x.pane); len(got) != 1 || got[0] != fmt.Sprintf("watch #%d, rebase and merge on green", x.pr) {
			t.Errorf("%s was told %q", x.name, got)
		}
	}
}

func TestMergeOrderStopsOnRed(t *testing.T) {
	w := newStackRun(t)
	w.redCI[2] = "test"
	w.launch(mergeOnGreen, "merge")
	w.run(func() bool { return w.agent.steps[2] == stoker.StepFailed })
	for range 10 { // the stoker would go on here, if it went on
		w.tick()
	}
	w.noStokerErrors()
	if !slices.Equal(w.merged, []int{1}) {
		t.Fatalf("merged %v", w.merged)
	}
	steps := w.lastSteps()
	if e := steps[1]; e.State != stoker.StepDone {
		t.Errorf("step 1 ends %+v", e)
	}
	if e := steps[2]; e.State != stoker.StepFailed || e.Text != "CI red · test" {
		t.Errorf("step 2 ends %+v", e)
	}
	if _, logged := steps[3]; logged {
		t.Errorf("step 3 was logged: %+v", steps[3])
	}
	if got := typed(*w.ran, "%3"); len(got) != 0 {
		t.Fatalf("#3 was told %q", got)
	}
}

func TestWithoutGrantEachMergeWaits(t *testing.T) {
	w := newStackRun(t)
	w.launch(mergeOnGreen) // the words say merge; the chips do not
	if o := w.Stoker.Mode().Order; len(o.Grants) != 0 {
		t.Fatalf("the order gave %q", o.Grants)
	}
	allWait := func() bool {
		for n := 1; n <= 3; n++ {
			if w.agent.steps[n] != stoker.StepWaits {
				return false
			}
		}
		return true
	}
	w.run(allWait)
	w.noStokerErrors()
	if len(w.merged) != 0 || !slices.Equal(w.denied, []int{1, 2, 3}) {
		t.Fatalf("merged %v, denied %v", w.merged, w.denied)
	}
	steps := w.lastSteps()
	gos := goSteps(w.Service)
	if len(gos) != 3 {
		t.Fatalf("go steps %+v", gos)
	}
	for i, x := range w.sessions {
		if e := steps[x.pr]; e.State != stoker.StepWaits {
			t.Errorf("step %d ends %+v", x.pr, e)
		}
		// matchblox itself put each denied merge on the card.
		if g := gos[i]; g.State != stoker.StepWaits || g.Needs != fmt.Sprintf("merge:#%d", x.pr) || g.Pane != x.pane ||
			g.Text != fmt.Sprintf("%s wants to run gh pr merge %d --squash", x.name, x.pr) {
			t.Errorf("go step %d is %+v", x.pr, g)
		}
		if !denies(t, w.Service, x.pane, fmt.Sprintf("gh pr merge %d --squash", x.pr)) {
			t.Errorf("the hook check lets %s merge #%d", x.name, x.pr)
		}
	}

	// The go on #1's step lets exactly #1 through.
	mustAct(t, w.Service, grantAct(gos[0].ID))
	w.run(func() bool { return w.agent.steps[1] == stoker.StepDone })
	w.tick()
	w.noStokerErrors()
	if !slices.Equal(w.merged, []int{1}) {
		t.Fatalf("merged %v", w.merged)
	}
	steps = w.lastSteps()
	for _, x := range w.sessions[1:] {
		if e := steps[x.pr]; e.State != stoker.StepWaits {
			t.Errorf("step %d ends %+v", x.pr, e)
		}
		if !denies(t, w.Service, x.pane, fmt.Sprintf("gh pr merge %d --squash", x.pr)) {
			t.Errorf("the go for step 1 lets %s merge #%d", x.name, x.pr)
		}
	}
	if !denies(t, w.Service, "%1", "gh pr merge 1 --squash") {
		t.Error("the go for step 1 was not used by its merge")
	}
}
