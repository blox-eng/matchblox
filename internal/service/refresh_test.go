package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

const workSwitching = "work switches agents now: send again after"

func refreshes(s *Service) []proto.Refresh {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refreshList()
}

func switchingTest(t *testing.T) (*Service, func()) {
	t.Helper()
	s, _, now := peerTest(t)
	x, _ := s.sessionAt("%1")
	_, done := s.startRefresh(x, now.Add(3*time.Minute))
	return s, done
}

func TestASwitchingPaneRefusesMessages(t *testing.T) {
	s, _ := switchingTest(t)
	x, _ := s.sessionAt("%1")
	if err := s.messageable("%1", x.PID); err == nil || err.Error() != workSwitching {
		t.Fatalf("messageable: %v", err)
	}
	if res := s.act(t.Context(), peerAct("message:%1", "hello")); res.Err != workSwitching {
		t.Fatalf("peer send: %q", res.Err)
	}
}

func TestASwitchingPaneSkipsTheStoker(t *testing.T) {
	s, _ := switchingTest(t)
	if err := s.stokable("%1", ""); err == nil || err.Error() != workSwitching {
		t.Fatalf("stokable: %v", err)
	}
	if _, waiting := s.seen(); !waiting["%1"] {
		t.Fatal("a rest trigger on the switching pane is due")
	}
}

func TestASwitchingPaneRefusesSwitchAndKill(t *testing.T) {
	s, _ := switchingTest(t)
	sw := proto.Act{Console: true, RecID: "switch:%1", Confirm: "y", Text: `{"agent":"codex"}`}
	if res := s.act(t.Context(), sw); !strings.Contains(res.Err, "switching already") {
		t.Fatalf("switch: %+v", res)
	}
	if res := s.act(t.Context(), proto.Act{Console: true, RecID: "kill:%1", Confirm: "y"}); !strings.Contains(res.Err, "switching already") {
		t.Fatalf("kill: %+v", res)
	}
}

func TestTheRefreshEndsItsMark(t *testing.T) {
	s, done := switchingTest(t)
	if got := refreshes(s); len(got) != 1 || got[0].Pane != "%1" || got[0].Name != "work" || got[0].Agent != "claude" {
		t.Fatalf("refreshes %+v", got)
	}
	done()
	x, _ := s.sessionAt("%1")
	if s.switching("%1") || s.messageable("%1", x.PID) != nil || s.stokable("%1", "") != nil {
		t.Fatal("a guard still refuses")
	}
	if _, waiting := s.seen(); waiting["%1"] {
		t.Fatal("seen still lists the pane")
	}
	if got := refreshes(s); len(got) != 0 {
		t.Fatalf("refreshes %+v", got)
	}
	if res := s.act(t.Context(), proto.Act{Console: true, RecID: "kill:%1", Confirm: "y"}); strings.Contains(res.Err, "switching already") {
		t.Fatalf("kill: %+v", res)
	}
}

// refreshFake is switchTest with work in %1 a match on Claude Code: a
// verified transcript, a task record matchblox manages, and a pane that
// shows a line typed into it. enter runs as the request's Enter goes.
type refreshFake struct {
	s      *Service
	ran    *[][]string
	record string
	snap   sample.Snapshot
	mu     sync.Mutex
	screen string
	enter  func()
}

func refreshTest(t *testing.T, change func(*sample.Session)) *refreshFake {
	t.Helper()
	limit, poll := refreshLimit, refreshPoll
	refreshLimit, refreshPoll = 5*time.Second, 5*time.Millisecond
	t.Cleanup(func() { refreshLimit, refreshPoll = limit, poll })
	s, ran := switchTest(t)
	f := &refreshFake{s: s, ran: ran, screen: idleScreen, snap: lifeSnap(false)}
	bundle := filepath.Join(s.matchContextRoot(), "outgoing")
	if err := os.MkdirAll(bundle, 0o700); err != nil {
		t.Fatal(err)
	}
	f.record = filepath.Join(bundle, "handover.md")
	if err := os.WriteFile(f.record, []byte(blankRecord), 0o600); err != nil {
		t.Fatal(err)
	}
	hourAgo := time.Now().Add(-time.Hour)
	if err := os.Chtimes(f.record, hourAgo, hourAgo); err != nil {
		t.Fatal(err)
	}
	x := &f.snap.Sessions[0]
	x.Transcript = filepath.Join(t.TempDir(), "s1.jsonl")
	x.MatchContext = filepath.Join(bundle, "awareness.md")
	if change != nil {
		change(x)
	}
	s.publish(f.snap, func(state.Doc) {})
	s.TmuxOut = func(argv []string) (string, error) {
		if slices.Contains(argv, "#{pane_in_mode}") {
			return "0\n", nil
		}
		return "%42\n", nil
	}
	s.Capture = func([]string) map[string]string {
		f.mu.Lock()
		defer f.mu.Unlock()
		return map[string]string{"%1": f.screen, "%3": idleScreen}
	}
	s.Actions.Run = func(argv []string) error {
		f.mu.Lock()
		*ran = append(*ran, argv)
		if slices.Contains(argv, "-l") {
			f.screen = strings.Replace(idleScreen, "> \n", "> "+argv[len(argv)-1]+"\n", 1)
		}
		last := argv[len(argv)-1]
		if last == "Enter" || last == "C-u" {
			f.screen = idleScreen
		}
		enter := f.enter
		f.mu.Unlock()
		if last == "Enter" && enter != nil {
			enter()
		}
		return nil
	}
	return f
}

// endTurn is the sample after work's turn ends now: at rest, or busy again.
func (f *refreshFake) endTurn(busy bool) {
	snap := f.snap
	snap.Sessions = slices.Clone(f.snap.Sessions)
	snap.Sessions[0].TurnEnded = time.Now()
	if busy {
		snap.Sessions[0].Busy, snap.Sessions[0].Status = true, "busy"
	}
	f.s.publish(snap, func(state.Doc) {})
}

func (f *refreshFake) write(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(f.record, []byte("# Task record\n\nGoal and done criteria: ship the CSV export.\n"), 0o600); err != nil {
		t.Error(err)
	}
}

func (f *refreshFake) switchTo(t *testing.T, ctx context.Context, n proto.SwitchAgent) proto.Result {
	t.Helper()
	if n.Agent == "" {
		n.Agent = "codex"
	}
	return f.s.act(ctx, switchAct(t, "%1", n))
}

// typed is each line typed into %1 that starts like a refresh request.
func (f *refreshFake) typed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for i, step := range *f.ran {
		if slices.Contains(step, "-l") && strings.HasPrefix(step[len(step)-1], refreshLead) {
			line := step[len(step)-1]
			if i+1 < len(*f.ran) && slices.Contains((*f.ran)[i+1], "Enter") {
				line += "⏎"
			}
			out = append(out, line)
		}
	}
	return out
}

func (f *refreshFake) respawned() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.ContainsFunc(*f.ran, func(step []string) bool { return slices.Contains(step, "respawn-pane") })
}

// capture is the capture.md of the one bundle the switch made.
func (f *refreshFake) capture(t *testing.T) string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(f.s.matchContextRoot(), "match-*", "capture.md"))
	if len(files) != 1 {
		t.Fatalf("captures %q", files)
	}
	b, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func refreshLineOf(capture string) string {
	for line := range strings.Lines(capture) {
		if strings.HasPrefix(line, "Refresh: ") {
			return strings.TrimSuffix(line, "\n")
		}
	}
	return ""
}

// switched checks a switch that went ahead with note, in its result and
// as the capture's refresh line.
func (f *refreshFake) switched(t *testing.T, res proto.Result, note string) string {
	t.Helper()
	if res.Err != "" || !f.respawned() {
		t.Fatalf("err %q ran %q", res.Err, *f.ran)
	}
	if !slices.Contains(res.Skipped, note) {
		t.Errorf("notes %q, want %q", res.Skipped, note)
	}
	capture := f.capture(t)
	if got := refreshLineOf(capture); got != "Refresh: "+note+"." {
		t.Errorf("refresh line %q, want %q", got, "Refresh: "+note+".")
	}
	return capture
}

func TestAnIdleAgentWritesItsRecordBeforeTheSwitch(t *testing.T) {
	f := refreshTest(t, nil)
	f.enter = func() { f.write(t); f.endTurn(false) }
	res := f.switchTo(t, t.Context(), proto.SwitchAgent{Handoff: proto.DefaultHandoff})
	capture := f.switched(t, res, "record written for this switch")
	if got := f.typed(); !slices.Equal(got, []string{refreshPrompt(f.record) + "⏎"}) {
		t.Errorf("typed %q", got)
	}
	if !strings.Contains(capture, "## Outgoing match's task record (claims to verify)\n\nRefresh: record written for this switch.\n") {
		t.Errorf("the refresh line does not start the record section:\n%s", capture)
	}
	if f.s.switching("%1") || len(refreshes(f.s)) != 0 {
		t.Error("the switch left its mark")
	}
}

func TestARefreshTurnWithNoWriteSwitchesAtOnce(t *testing.T) {
	f := refreshTest(t, nil)
	f.enter = func() { f.endTurn(false) }
	start := time.Now()
	res := f.switchTo(t, t.Context(), proto.SwitchAgent{Handoff: proto.DefaultHandoff})
	f.switched(t, res, "not refreshed: it ended its turn without writing the record")
	if took := time.Since(start); took >= refreshLimit/2 {
		t.Errorf("the switch waited %v", took)
	}
}

func TestARefreshTimesOut(t *testing.T) {
	if got := waitText(3 * time.Minute); got != "3:00" {
		t.Errorf("the limit reads %q", got)
	}
	f := refreshTest(t, nil)
	refreshLimit = 200 * time.Millisecond
	f.enter = func() { // the request's turn runs past the limit
		snap := f.snap
		snap.Sessions = slices.Clone(f.snap.Sessions)
		snap.Sessions[0].Busy, snap.Sessions[0].Status = true, "busy"
		f.s.publish(snap, func(state.Doc) {})
	}
	res := f.switchTo(t, t.Context(), proto.SwitchAgent{Handoff: proto.DefaultHandoff})
	f.switched(t, res, "not refreshed: the refresh timed out after "+waitText(refreshLimit))
	if len(f.typed()) != 1 {
		t.Errorf("typed %q", f.typed())
	}
}

func TestNoRefreshWhen(t *testing.T) {
	for name, c := range map[string]struct {
		change func(*sample.Session)
		n      proto.SwitchAgent
		screen string
		note   string
	}{
		"it works": {
			change: func(x *sample.Session) { x.Busy, x.Status = true, "busy" },
			n:      proto.SwitchAgent{Anyway: true}, note: "not refreshed: work works",
		},
		"it waits": {
			change: func(x *sample.Session) { x.Permits = true },
			n:      proto.SwitchAgent{Anyway: true}, note: "not refreshed: work waits for you",
		},
		"out of usage": {
			change: func(x *sample.Session) { x.Out = time.Date(2026, 11, 9, 15, 4, 0, 0, time.Local) },
			note:   "not refreshed: out of usage until 15:04",
		},
		"opencode": {
			change: func(x *sample.Session) { x.Agent = "opencode" },
			note:   "not refreshed: opencode gives no turn end",
		},
		"cursor": {
			change: func(x *sample.Session) { x.Agent = sample.CursorAgent },
			note:   "not refreshed: cursor-agent gives no turn end",
		},
		"no transcript": {
			change: func(x *sample.Session) { x.Transcript = "" },
			note:   "not refreshed: claude gives no turn end",
		},
		"no record": {
			change: func(x *sample.Session) { x.MatchContext = "" },
			note:   "not refreshed: no managed task record",
		},
		"a record outside matchblox": {
			change: func(x *sample.Session) { x.MatchContext = "/elsewhere/awareness.md" },
			note:   "not refreshed: no managed task record",
		},
		"--no-refresh": {
			n:    proto.SwitchAgent{NoRefresh: true},
			note: "not refreshed: skipped with --no-refresh",
		},
		"a draft": {
			screen: strings.Replace(idleScreen, "> \n", "> half a thought\n", 1),
			note:   "not refreshed: the request did not reach work: not delivered: work has a draft in its prompt",
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := refreshTest(t, c.change)
			if c.screen != "" {
				f.screen = c.screen
			}
			c.n.Handoff = proto.DefaultHandoff
			res := f.switchTo(t, t.Context(), c.n)
			f.switched(t, res, c.note)
			if got := f.typed(); len(got) != 0 {
				t.Errorf("typed %q", got)
			}
		})
	}
	f := refreshTest(t, nil)
	res := f.switchTo(t, t.Context(), proto.SwitchAgent{})
	if res.Err != "" || len(f.typed()) != 0 || slices.ContainsFunc(res.Skipped, func(n string) bool { return strings.Contains(n, "refresh") }) {
		t.Errorf("a switch with no handoff: err %q notes %q typed %q", res.Err, res.Skipped, f.typed())
	}
}

func TestTheBuildersPromptDuringTheRefreshRefusesTheSwitch(t *testing.T) {
	f := refreshTest(t, nil)
	f.enter = func() { f.write(t); f.endTurn(false) }
	f.s.MatchGit = func(context.Context, string, ...string) (string, error) {
		f.endTurn(true)
		return "", nil
	}
	res := f.switchTo(t, t.Context(), proto.SwitchAgent{Handoff: proto.DefaultHandoff})
	if res.Err != "work took your prompt while it wrote its record: switch again" || f.respawned() {
		t.Fatalf("err %q ran %q", res.Err, *f.ran)
	}
	if left, _ := filepath.Glob(filepath.Join(f.s.matchContextRoot(), "match-*")); len(left) != 0 {
		t.Errorf("a refused switch leaves %q", left)
	}
}

func TestACancelledRefreshLeavesNoMark(t *testing.T) {
	f := refreshTest(t, nil)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	x, _ := f.s.sessionAt("%1")
	var during []string
	f.enter = func() {
		if err := f.s.messageable("%1", x.PID); err == nil || err.Error() != workSwitching {
			during = append(during, fmt.Sprintf("messageable: %v", err))
		}
		if err := f.s.stokable("%1", ""); err == nil || err.Error() != workSwitching {
			during = append(during, fmt.Sprintf("stokable: %v", err))
		}
		if got := refreshes(f.s); len(got) != 1 || got[0].Pane != "%1" {
			during = append(during, fmt.Sprintf("refreshes %+v", got))
		}
		cancel()
	}
	res := f.switchTo(t, ctx, proto.SwitchAgent{Handoff: proto.DefaultHandoff})
	if len(during) != 0 {
		t.Errorf("during the wait: %q", during)
	}
	if res.Err != "context canceled" || f.respawned() {
		t.Errorf("err %q ran %q", res.Err, *f.ran)
	}
	if f.s.switching("%1") || len(refreshes(f.s)) != 0 {
		t.Error("the cancelled refresh left its mark")
	}
}

func TestASecondRefreshOfAPaneIsRefused(t *testing.T) {
	s, done := switchingTest(t)
	defer done()
	x, _ := s.sessionAt("%1")
	if run, release := s.startRefresh(x, time.Now().Add(time.Minute)); run != nil || release != nil {
		t.Fatal("a second refresh started")
	}
	if got := refreshes(s); len(got) != 1 {
		t.Fatalf("refreshes %+v", got)
	}
}

// Only the refresh's own request passes the mark: a peer line sent during
// the wait is refused, and the mark stays through the capture until the
// respawn is done.
func TestTheMarkHoldsFromTheRequestToTheRespawn(t *testing.T) {
	f := refreshTest(t, nil)
	chainUp(f.s, "%3", "%1")
	peer := make(chan proto.Result, 1)
	f.enter = func() {
		go func() { // the request's typing holds the pane: this waits for it
			peer <- f.s.act(t.Context(), peerAct("message:%1", "hello"))
			f.write(t)
			f.endTurn(false)
		}()
	}
	var capture []string
	f.s.MatchGit = func(context.Context, string, ...string) (string, error) {
		if !f.s.switching("%1") {
			capture = append(capture, "no mark")
		}
		if res := f.s.act(t.Context(), proto.Act{Console: true, RecID: "kill:%1", Confirm: "y"}); res.Err != "switching already" {
			capture = append(capture, "kill: "+res.Err)
		}
		return "", nil
	}
	res := f.switchTo(t, t.Context(), proto.SwitchAgent{Handoff: proto.DefaultHandoff})
	f.switched(t, res, "record written for this switch")
	if got := <-peer; got.Err != "work switches agents now: send again after" {
		t.Errorf("peer send during the wait: %+v", got)
	}
	if len(capture) != 0 {
		t.Errorf("during the capture: %q", capture)
	}
	if f.s.switching("%1") || len(refreshes(f.s)) != 0 {
		t.Error("the switch left its mark")
	}
	if got := f.typed(); len(got) != 1 {
		t.Errorf("typed %q", got)
	}
}

// A real prompt wraps the request: the draft read back is its first row
// only, and that still lets the Enter go.
func TestAWrappedRequestStillGoes(t *testing.T) {
	f := refreshTest(t, nil)
	f.enter = func() { f.write(t); f.endTurn(false) }
	capture := f.s.Capture
	f.s.Capture = func(ps []string) map[string]string {
		out := capture(ps)
		if sc, ok := out["%1"]; ok {
			var rows []string
			for line := range strings.Lines(sc) {
				for len(line) > 80 {
					rows, line = append(rows, line[:80]+"\n"), line[80:]
				}
				rows = append(rows, line)
			}
			out["%1"] = strings.Join(rows, "")
		}
		return out
	}
	res := f.switchTo(t, t.Context(), proto.SwitchAgent{Handoff: proto.DefaultHandoff})
	f.switched(t, res, "record written for this switch")
}
