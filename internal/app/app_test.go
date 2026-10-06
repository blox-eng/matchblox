package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/procfs"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/transport"
)

const fixture = "../../testdata/machine"

func fixtureState() proto.State {
	s := &sample.Sampler{
		FS:    procfs.FS{Root: filepath.Join(fixture, "proc")},
		Home:  filepath.Join(fixture, "home"),
		Tmux:  func() ([]byte, error) { return os.ReadFile(filepath.Join(fixture, "tmux-panes.txt")) },
		Rules: sample.DefaultRules,
	}
	snap := s.Sample()
	q := queue.New()
	q.Merge(snap.Sessions, snap.At)
	return proto.State{Doc: state.Doc{Snapshot: snap, Recommendations: advice.Build(snap, nil), Queue: q.Items()}}
}

// fakeConn is a service that answers with what the test puts in.
type fakeConn struct {
	in     chan proto.Envelope
	mu     sync.Mutex
	sent   []proto.Envelope
	closed bool
}

func newFake() *fakeConn { return &fakeConn{in: make(chan proto.Envelope, 16)} }

func envelope(kind proto.Kind, id string, body any) proto.Envelope {
	raw, _ := json.Marshal(body)
	return proto.Envelope{Kind: kind, ID: id, Body: raw}
}

func (f *fakeConn) Send(kind proto.Kind, id string, body any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, envelope(kind, id, body))
	return nil
}

func (f *fakeConn) Recv() (proto.Envelope, error) {
	env, ok := <-f.in
	if !ok {
		return proto.Envelope{}, io.EOF
	}
	return env, nil
}

func (f *fakeConn) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		f.closed = true
		close(f.in)
	}
	return nil
}

func (f *fakeConn) acts() []proto.Act {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []proto.Act
	for _, e := range f.sent {
		if e.Kind == proto.KindAct {
			var a proto.Act
			_ = json.Unmarshal(e.Body, &a)
			out = append(out, a)
		}
	}
	return out
}

func loadedWith(t *testing.T, width int, st proto.State) (Model, *fakeConn) {
	t.Helper()
	f := newFake()
	m := New(Options{NoMotion: true, Conn: f})
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	next, _ = next.Update(stateMsg(st))
	return next.(Model), f
}

// loaded is the console on the fixture machine, on the sessions tab.
func loaded(t *testing.T, width int) Model {
	t.Helper()
	m, _ := loadedWith(t, width, fixtureState())
	m.tab = tabSessions
	return m
}

func TestViewFitsWidth(t *testing.T) {
	for _, w := range []int{80, 100, 160} {
		out := loaded(t, w).render()
		for i, line := range strings.Split(out, "\n") {
			if lw := lipgloss.Width(line); lw > w {
				t.Fatalf("width %d: line %d is %d wide: %q", w, i, lw, line)
			}
		}
		for _, want := range []string{"app-feature", "app-review", "! compact", "▲ clear", "fresh", "1 without an agent: notes:1.1 zsh"} {
			if !strings.Contains(out, want) {
				t.Fatalf("width %d: view lacks %q:\n%s", w, want, out)
			}
		}
	}
}

func key(m tea.Model, k string) (tea.Model, tea.Cmd) {
	r := []rune(k)
	msg := tea.KeyPressMsg{Text: k, Code: r[0]}
	switch k {
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "down":
		msg = tea.KeyPressMsg{Code: tea.KeyDown}
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEsc}
	}
	return m.Update(msg)
}

func TestJumpPrintsCommandThenRunsOnConfirm(t *testing.T) {
	var ran []string
	m := loaded(t, 100)
	m.opt.Run = func(argv []string) error { ran = argv; return nil }

	next, _ := key(m, "enter")
	out := next.(Model).render()
	if !strings.Contains(out, "tmux switch-client -t %1") || ran != nil {
		t.Fatalf("enter must print the command and wait; ran=%v\n%s", ran, out)
	}
	next, cmd := key(next, "enter")
	if cmd == nil {
		t.Fatal("confirm returned no command")
	}
	next.Update(cmd())
	if strings.Join(ran, " ") != "tmux switch-client -t %1" {
		t.Fatalf("ran %v", ran)
	}
}

func TestEscCancelsAction(t *testing.T) {
	m := loaded(t, 100)
	m.opt.Run = func([]string) error { t.Fatal("must not run"); return nil }
	next, _ := key(m, "enter")
	next, cmd := key(next, "esc")
	if cmd != nil || !strings.Contains(next.(Model).render(), "cancelled") {
		t.Fatal("esc must cancel without running")
	}
}

func TestDestructiveNeedsTypedYes(t *testing.T) {
	m := loaded(t, 100)
	m.opt.Run = func([]string) error { t.Fatal("enter must not confirm a destructive action"); return nil }
	m.pending = &action{steps: [][]string{{"kill", "123"}}, destructive: true}
	if _, cmd := key(m, "enter"); cmd != nil {
		t.Fatal("enter ran a destructive action")
	}
}

func TestSelectionFollowsPID(t *testing.T) {
	m := loaded(t, 100)
	next, _ := key(m, "down")
	sel, _ := next.(Model).selected()
	next, _ = next.Update(stateMsg(fixtureState()))
	again, _ := next.(Model).selected()
	if sel.PID != again.PID {
		t.Fatalf("selection moved from %d to %d on resample", sel.PID, again.PID)
	}
}

// syncBuffer lets the program write while the test reads.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// TestSmoke runs the real program against a service that sends the
// fixture machine, waits for the first rendered state, and quits.
func TestSmoke(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var out syncBuffer
	f := newFake()
	f.in <- envelope(proto.KindHello, "", proto.Hello{Version: proto.Version, Host: "ws-1"})
	f.in <- envelope(proto.KindSnapshot, "", fixtureState())
	p := tea.NewProgram(New(Options{NoMotion: true, Conn: f}), tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(&out), tea.WithWindowSize(100, 30))
	done := make(chan error, 1)
	go func() { _, err := p.Run(); done <- err }()

	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(out.String(), "app-review") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	p.Quit()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "app-review") {
		t.Fatalf("rendered output lacks the fixture session:\n%q", out.String())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sent) == 0 || f.sent[0].Kind != proto.KindHello {
		t.Fatalf("the console must say hello first, sent %+v", f.sent)
	}
}

func TestVersionMismatchShowsFix(t *testing.T) {
	m := New(Options{NoMotion: true, Conn: newFake()})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	next, _ = next.Update(helloMsg(proto.Hello{Version: 2, Binary: "v0.9.0", Host: "ws-1"}))
	out := next.(Model).render()
	for _, want := range []string{
		"the console is version 1, the host is version 2",
		"curl -fsSL https://matchblox.sh | sh",
		"update the console",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("view lacks %q:\n%s", want, out)
		}
	}
	next, _ = m.Update(helloMsg(proto.Hello{Version: 0, Host: "ws-1"}))
	if out := next.(Model).render(); !strings.Contains(out, "the console is version 1, the host is version 0") || !strings.Contains(out, "update ws-1") {
		t.Fatalf("an older host must name the host as the side to update:\n%s", out)
	}
}

func TestBeforeTheFirstStateTheViewSaysWhy(t *testing.T) {
	m := New(Options{NoMotion: true, Conn: newFake()})
	if out := m.render(); !strings.Contains(out, "waiting for the service") {
		t.Fatalf("an empty screen is a dead end:\n%s", out)
	}
}

func TestConfirmedRecActIsSentNotRun(t *testing.T) {
	st := fixtureState()
	st.Recommendations = []advice.Rec{{ID: "r9", Level: "warn", Title: "Kill it", Second: &advice.Action{
		Label: "kill", Steps: [][]string{{"kill", "42"}}, Destructive: true}}}
	m, f := loadedWith(t, 100, st)
	m.opt.Run = func([]string) error { t.Fatal("a service step ran in the console"); return nil }
	m.tab = tabRecs
	next, _ := key(m, "x")
	next, cmd := key(next, "y")
	if cmd == nil {
		t.Fatal("confirm returned no command")
	}
	next.Update(cmd())
	acts := f.acts()
	if len(acts) != 1 || acts[0] != (proto.Act{RecID: "r9", Which: "secondary", Confirm: "y"}) {
		t.Fatalf("sent %+v", acts)
	}
}

func TestPanelKillNamesTheOrphan(t *testing.T) {
	st := fixtureState()
	st.Orphans = []sample.Orphan{{PID: 4242, Start: 7, Comm: "bash", Kill: []string{"kill", "4242"}}}
	m, f := loadedWith(t, 100, st)
	m.tab = tabProcs
	next, _ := key(m, "x")
	_, cmd := key(next, "y")
	cmd()
	if acts := f.acts(); len(acts) != 1 || acts[0].RecID != "orphan:4242" || acts[0].Confirm != "y" {
		t.Fatalf("sent %+v", acts)
	}
}

func TestResultShowsInTheFooter(t *testing.T) {
	m := loaded(t, 100)
	next, _ := m.Update(resultMsg(proto.Result{Skipped: []string{"already done"}}))
	if out := next.(Model).render(); !strings.Contains(out, "already done") {
		t.Fatalf("footer lacks the result:\n%s", out)
	}
}

func TestConnectionLostKeepsStateAndRedials(t *testing.T) {
	again := newFake()
	m := New(Options{NoMotion: true, Conn: newFake(), Redial: func() (transport.Conn, error) { return again, nil }})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	next, _ = next.Update(stateMsg(fixtureState()))
	next, cmd := next.Update(lostMsg{err: io.EOF})
	out := next.(Model).render()
	if !strings.Contains(out, "connection lost") || !strings.Contains(out, "app-review") {
		t.Fatalf("lost connection must keep the last state and say so:\n%s", out)
	}
	if cmd == nil {
		t.Fatal("no redial scheduled")
	}
	next, _ = next.Update(redialMsg{})
	next, _ = next.Update(connMsg{conn: again})
	if next.(Model).conn != again {
		t.Fatal("the console did not take the new connection")
	}
}

func TestGitRescanAsksTheService(t *testing.T) {
	m, f := loadedWith(t, 100, fixtureState())
	m.tab = tabGit
	_, cmd := key(m, "r")
	if cmd == nil {
		t.Fatal("r sent nothing")
	}
	cmd()
	if acts := f.acts(); len(acts) != 1 || acts[0].RecID != "rescan:git" {
		t.Fatalf("sent %+v", acts)
	}
}

func TestOlderLocalServiceIsReplaced(t *testing.T) {
	for _, h := range []proto.Hello{
		{Version: proto.Version, Binary: "v0.1.0", Host: "ws-1"}, // same wire, older build
		{Version: proto.Version - 1, Binary: "v0.0.1", Host: "ws-1"},
	} {
		f := newFake()
		m := New(Options{NoMotion: true, Conn: f, Local: true, Binary: "v0.2.0", Redial: func() (transport.Conn, error) { return newFake(), nil }})
		next, cmd := m.Update(helloMsg(h))
		drain(cmd)
		if acts := f.acts(); len(acts) != 1 || acts[0].RecID != "service:replace" {
			t.Fatalf("%+v: sent %+v", h, acts)
		}
		if out := next.(Model).render(); strings.Contains(out, "curl") {
			t.Fatalf("%+v: a local console must not ask the person to update what it can replace:\n%s", h, out)
		}
		// The old service goes away; the console connects again.
		if _, cmd := next.Update(lostMsg{err: io.EOF, conn: f}); cmd == nil {
			t.Fatalf("%+v: no redial after the replace", h)
		}
	}
}

func TestReplaceAtMostOnce(t *testing.T) {
	f := newFake()
	g := newFake()
	m := New(Options{NoMotion: true, Conn: f, Local: true, Binary: "v0.2.0", Redial: func() (transport.Conn, error) { return g, nil }})
	next, cmd := m.Update(helloMsg(proto.Hello{Version: proto.Version, Binary: "v0.1.0"}))
	drain(cmd)
	if len(f.acts()) != 1 {
		t.Fatal("the first hello did not replace")
	}
	next, _ = next.Update(connMsg{conn: g})
	_, cmd = next.Update(helloMsg(proto.Hello{Version: proto.Version, Binary: "v0.1.0"}))
	drain(cmd)
	if acts := g.acts(); len(acts) != 0 {
		t.Fatalf("two consoles of two versions would replace each other forever: %+v", acts)
	}
}

func TestRemoteHostIsNeverReplaced(t *testing.T) {
	f := newFake()
	m := New(Options{NoMotion: true, Conn: f, Binary: "v0.2.0"})
	next, _ := m.Update(helloMsg(proto.Hello{Version: proto.Version - 1, Binary: "v0.0.1", Host: "ws-1"}))
	if acts := f.acts(); len(acts) != 0 {
		t.Fatalf("sent %+v", acts)
	}
	if out := next.(Model).render(); !strings.Contains(out, "update ws-1") {
		t.Fatalf("a remote host shows the fix:\n%s", out)
	}
}

func TestTwoLostEventsRedialOnce(t *testing.T) {
	x := newFake()
	dials := 0
	m := New(Options{NoMotion: true, Conn: x, Redial: func() (transport.Conn, error) { dials++; return newFake(), nil }})
	next, cmd := m.Update(lostMsg{err: io.EOF, conn: x})
	if cmd == nil {
		t.Fatal("no redial scheduled")
	}
	if !x.closed {
		t.Fatal("the lost connection was not closed")
	}
	// The send and the recv of the same connection both report the loss.
	if _, cmd := next.Update(lostMsg{err: io.EOF, conn: x}); cmd != nil {
		t.Fatal("a second loss of the same connection scheduled a second redial")
	}
}

func TestMessagesFromAnOldConnAreIgnored(t *testing.T) {
	x, y := newFake(), newFake()
	m := New(Options{NoMotion: true, Conn: x, Redial: func() (transport.Conn, error) { return y, nil }})
	next, _ := m.Update(lostMsg{err: io.EOF, conn: x})
	next, _ = next.Update(connMsg{conn: y})
	next, cmd := next.Update(fromConn{conn: x, msg: stateMsg(fixtureState())})
	if cmd != nil || next.(Model).have {
		t.Fatal("a message from the old connection was handled; it would start a second reader on the new one")
	}
}

// drain runs cmd and every command it batches, skipping any that block
// (a recv on an empty connection).
func drain(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	out := make(chan tea.Msg, 1)
	go func() { out <- cmd() }()
	select {
	case msg := <-out:
		if b, ok := msg.(tea.BatchMsg); ok {
			for _, c := range b {
				drain(c)
			}
		}
	case <-time.After(50 * time.Millisecond):
	}
}

// A service that accepts but never answers (stopped, wedged) must not
// leave an empty screen.
func TestSilentServiceShowsFix(t *testing.T) {
	f := newFake()
	m := New(Options{NoMotion: true, Conn: f, Owner: func() int { return 4321 }})
	next, _ := m.Update(silentMsg{conn: f})
	out := next.(Model).render()
	if !strings.Contains(out, "does not answer") || !strings.Contains(out, "kill 4321") {
		t.Fatalf("view lacks the fix:\n%s", out)
	}
	// Once the state arrives, a late timer changes nothing.
	m2, _ := loadedWith(t, 100, fixtureState())
	next, _ = m2.Update(silentMsg{conn: m2.conn})
	if strings.Contains(next.(Model).render(), "does not answer") {
		t.Fatal("a late timer claimed a live service is silent")
	}
}

func TestSilentAfterReconnectShowsFix(t *testing.T) {
	m, _ := loadedWith(t, 100, fixtureState())
	m.opt.Owner = func() int { return 77 }
	y := newFake()
	next, _ := m.Update(connMsg{conn: y})
	next, _ = next.Update(silentMsg{conn: y})
	if !strings.Contains(next.(Model).render(), "kill 77") {
		t.Fatalf("a silent service after a reconnect went unnoticed:\n%s", next.(Model).render())
	}
}

// Nav steps run here without a typed y, so only tmux moves may run, even
// if a service sends something else.
func TestNavRunsOnlyTmuxMoves(t *testing.T) {
	st := fixtureState()
	st.Recommendations = []advice.Rec{{ID: "r1", Title: "x", Primary: &advice.Action{Nav: true, Steps: [][]string{{"rm", "-rf", "/tmp/x"}}}}}
	m, _ := loadedWith(t, 100, st)
	m.opt.Run = func(argv []string) error { t.Fatalf("ran %v", argv); return nil }
	m.tab = tabRecs
	next, _ := key(m, "enter")
	next, cmd := key(next, "enter")
	if cmd != nil {
		next, _ = next.Update(cmd())
	}
	if !strings.Contains(next.(Model).render(), "refused") {
		t.Fatalf("no refusal shown:\n%s", next.(Model).render())
	}
}

// tmux runs a trailing shell command for new-window and splits commands on
// a ";" argument: a Nav step must have exactly the shape advice builds.
func TestNavAllowlistIsExact(t *testing.T) {
	ok := [][]string{
		{"tmux", "switch-client", "-t", "%1"},
		{"tmux", "new-window", "-c", "/w/app"},
	}
	bad := [][]string{
		{"tmux", "new-window", "-c", "/w", "rm -rf ~"},
		{"tmux", "switch-client", "-t", "%1", ";", "run-shell", "rm -rf ~"},
		{"tmux", "new-window", "-c", "/w", ";", "run-shell", "x"},
		{"tmux", "switch-client", "-t", "%1;run-shell x"},
		{"tmux", "switch-client", "-E", "-t", "%1"},
		{"tmux", "new-window", "-c", "-e"},
		{"tmux", "switch-client"},
		{"tmux", "run-shell", "x"},
		// tmux format-expands these: #() runs a shell command.
		{"tmux", "new-window", "-c", "#(rm -rf ~)"},
		{"tmux", "new-window", "-c", "/w/#(id)"},
		{"tmux", "switch-client", "-t", "#(id)"},
		{"tmux", "switch-client", "-t", "work:1"},
		{"tmux", "new-window", "-c", "relative/dir"},
	}
	for _, a := range ok {
		if !navAllowed(a) {
			t.Errorf("refused %v", a)
		}
	}
	for _, a := range bad {
		if navAllowed(a) {
			t.Errorf("allowed %v", a)
		}
	}
}

func TestHeaderShowsHost(t *testing.T) {
	m := loaded(t, 100)
	next, _ := m.Update(helloMsg{Version: proto.Version, Host: "ws-1"})
	first := strings.Split(ansi.Strip(next.(Model).render()), "\n")[0]
	if !strings.HasPrefix(first, " ▰ matchblox · ws-1") {
		t.Fatalf("header %q, want it to start with the mark, the name and the host", first)
	}
}

func TestFooterUsesClock(t *testing.T) {
	st := fixtureState()
	f := newFake()
	m := New(Options{NoMotion: true, Conn: f, Now: func() time.Time { return st.At.Add(5 * time.Second) }})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	next, _ = next.Update(stateMsg(st))
	lines := strings.Split(ansi.Strip(next.(Model).render()), "\n")
	if last := strings.TrimRight(lines[len(lines)-1], " "); !strings.HasSuffix(last, "sampled 5s ago") {
		t.Fatalf("footer %q, want it to end with sampled 5s ago", last)
	}
}
