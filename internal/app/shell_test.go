package app

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

func TestTNotReached(t *testing.T) {
	home, gpu := newFake(), newFake()
	var next tea.Model = New(Options{
		NoMotion: true, Conn: home, Now: func() time.Time { return demoAt },
		Peers: []Peer{{Host: "gpu-1", Conn: gpu}}, Beside: true, Pane: "%1",
	})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	next, _ = next.Update(stateMsg(hostState()))
	ux := hostState(sess(7, "ux-pass", "%7", func(s *sample.Session) { s.Cwd = "/wt/ux-pass" }))
	next, _ = next.Update(peerMsg{host: "gpu-1", conn: gpu, msg: stateMsg(ux)})
	m := next.(Model)
	m.splashDone = true
	m.deskFrame = true
	m.toList()
	m.peers[m.peerAt("gpu-1")].lost = true
	for _, s := range m.all.sessions {
		if s.Name == "ux-pass" {
			m.chooseRow(s)
		}
	}
	m = pressOn(m, "t")
	if m.noticeText != "not opened: gpu-1 is not reached" {
		t.Fatalf("notice %q", m.noticeText)
	}
}

// TestTReachedPeerSplitsThere: gpu-1 is reached and its pane id is also a
// local pane. t splits on gpu-1, never on the console's tmux, and the
// word shell is only on gpu-1's row.
func TestTReachedPeerSplitsThere(t *testing.T) {
	home, gpu := newFake(), newFake()
	var local []string
	var next tea.Model = New(Options{
		NoMotion: true, Conn: home, Now: func() time.Time { return demoAt },
		Peers: []Peer{{Host: "gpu-1", Conn: gpu}}, Beside: true, Pane: "%1",
		Tmux: func(args ...string) (string, error) {
			local = append(local, strings.Join(args, " "))
			if len(args) > 0 && args[0] == "display" {
				return "%3", nil
			}
			return "", nil
		},
	})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	next, _ = next.Update(stateMsg(hostState(sess(1, "local-pane", "%3", func(s *sample.Session) { s.Cwd = "/tmp/local" }))))
	ux := hostState(sess(7, "ux-pass", "%3", func(s *sample.Session) { s.Cwd = "/wt/ux-pass" }))
	next, _ = next.Update(peerMsg{host: "gpu-1", conn: gpu, msg: stateMsg(ux)})
	m := next.(Model)
	m.splashDone = true
	m.deskFrame = true
	m.toList()
	m.width, m.height = 160, 40
	for _, s := range m.all.sessions {
		if s.Host == "gpu-1" && s.Name == "ux-pass" {
			m.chooseRow(s)
		}
	}
	var remote [][]string
	prev := hostRun
	hostRun = func(argv []string) (string, error) {
		remote = append(remote, append([]string(nil), argv...))
		if strings.Contains(strings.Join(argv, " "), "split-window") {
			return "%9", nil
		}
		return "", nil
	}
	t.Cleanup(func() { hostRun = prev })
	got, cmd := key(m, "t")
	m = got.(Model)
	for _, msg := range cmdMsgs(cmd) {
		if _, ok := msg.(shellMsg); !ok {
			continue
		}
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	joined := strings.Join(local, "\n")
	if strings.Contains(joined, "split-window") || strings.Contains(joined, "@matchblox_shell") || strings.Contains(joined, "%3") {
		t.Fatalf("local split:\n%s", joined)
	}
	var saw bool
	for _, argv := range remote {
		line := strings.Join(argv, " ")
		if strings.Contains(line, "gpu-1") && strings.Contains(line, "split-window") && strings.Contains(line, "%3") && strings.Contains(line, "/wt/ux-pass") {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("no split on gpu-1: %q", remote)
	}
	var flip bool
	for _, argv := range remote {
		line := strings.Join(argv, " ")
		if strings.Contains(line, "@matchblox_desk") || (strings.Contains(line, "set-option") && strings.Contains(line, "%1")) {
			t.Fatalf("peer t writes the desk pane: %s", line)
		}
		if strings.Contains(line, "select-pane") && strings.Contains(line, "@matchblox_agent") {
			flip = true
		}
	}
	if !flip {
		t.Fatalf("peer t does not select the agent: %q", remote)
	}
	var localS, peerS sample.Session
	for _, s := range m.all.sessions {
		switch {
		case s.Name == "local-pane":
			localS = s
		case s.Host == "gpu-1" && s.Name == "ux-pass":
			peerS = s
		}
	}
	localRow := ansi.Strip(strings.Join(m.sideRow(localS, 80, false, sideLink{}), "\n"))
	peerRow := strings.Join(m.sideRow(peerS, 80, true, sideLink{}), "\n")
	if strings.Contains(localRow, "shell") {
		t.Fatalf("local row says shell: %q", localRow)
	}
	if !strings.Contains(peerRow, m.st.muted.Render("shell")) {
		t.Fatalf("gpu-1 row has no shell: %q", ansi.Strip(peerRow))
	}
}

func TestTNoFolder(t *testing.T) {
	st := hostState(sess(7, "ux-pass", "%7", func(s *sample.Session) { s.Cwd = "" }))
	m := boardShot(t, st, 80, 30, func(m *Model) { m.deskFrame = true })
	m = pressOn(m, "t")
	if m.noticeText != "not opened: ux-pass has no folder" {
		t.Fatalf("notice %q", m.noticeText)
	}
}

// TestTWithoutTmuxSaysWhy: t on a console with no tmux (the phone, the demo)
// says why, as D does (review of 2026-11-27, Important 6).
func TestTWithoutTmuxSaysWhy(t *testing.T) {
	st := hostState(sess(7, "ux-pass", "%7", func(s *sample.Session) { s.Cwd = "/wt/ux-pass" }))
	m := boardShot(t, st, 80, 30, func(m *Model) { m.deskFrame = true; m.opt.Pane = "" })
	next, cmd := key(m, "t")
	m = next.(Model)
	if m.noticeText == "" && cmd == nil {
		t.Fatal("t with no tmux did nothing and said nothing")
	}
	want := "not opened: the shell needs the agent's tmux · ⏎ goes into the session"
	if m.noticeText != want {
		t.Fatalf("notice %q, want %q", m.noticeText, want)
	}
}

// TestTDeadPaneSaysWhy: the agent's pane is gone, and t says so.
func TestTDeadPaneSaysWhy(t *testing.T) {
	st := hostState(sess(7, "ux-pass", "%7", func(s *sample.Session) { s.Cwd = "/wt/ux-pass" }))
	m := boardShot(t, st, 80, 30, func(m *Model) {
		m.deskFrame, m.opt.Pane = true, "%1"
		m.opt.Tmux = func(args ...string) (string, error) { return "", nil } // display answers no pane
	})
	m = pressOn(m, "t")
	if want := "not opened: ux-pass's pane is gone"; m.noticeText != want {
		t.Fatalf("notice %q, want %q", m.noticeText, want)
	}
}

// peerShellModel is the console with gpu-1 reached and three of its agents
// in the list, the desk frame on. It returns the model before gpu-1's state.
func peerShellModel(t *testing.T) (Model, *fakeConn, proto.State) {
	t.Helper()
	home, gpu := newFake(), newFake()
	var next tea.Model = New(Options{
		NoMotion: true, Conn: home, Now: func() time.Time { return demoAt },
		Peers: []Peer{{Host: "gpu-1", Conn: gpu}}, Beside: true, Pane: "%1",
		Tmux: func(args ...string) (string, error) { return "", nil },
	})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	next, _ = next.Update(stateMsg(hostState()))
	m := next.(Model)
	m.deskFrame = true
	m.peers[m.peerAt("gpu-1")].hello.Tmux = "tmux"
	cwd := func(s *sample.Session) { s.Cwd = "/wt/" + s.Name }
	ux := hostState(sess(7, "ux-pass", "%7", cwd), sess(8, "api", "%8", cwd), sess(9, "docs", "%9", cwd))
	return m, gpu, ux
}

// TestPeerShellLookupIsACmd: finding the shell panes of a peer is one tmux
// call over ssh for the host, run as a command and never inside Update.
// A failed call is not made again on the next snapshot (review of
// 2026-11-27, Important 4).
func TestPeerShellLookupIsACmd(t *testing.T) {
	m, gpu, ux := peerShellModel(t)
	var mu sync.Mutex
	var calls []string
	release := make(chan struct{})
	fail := false
	prev := hostRun
	hostRun = func(argv []string) (string, error) {
		mu.Lock()
		calls = append(calls, strings.Join(argv, " "))
		mu.Unlock()
		<-release
		if fail {
			return "", errors.New("ssh: connect timed out")
		}
		return "%11 %7 1\n%7  \n%8  \n%9  \n", nil
	}
	t.Cleanup(func() { hostRun = prev })
	count := func() int { mu.Lock(); defer mu.Unlock(); return len(calls) }

	type result struct {
		m   tea.Model
		cmd tea.Cmd
	}
	update := func(m Model, msg tea.Msg) (Model, tea.Cmd) {
		done := make(chan result, 1)
		go func() { next, cmd := m.Update(msg); done <- result{next, cmd} }()
		select {
		case r := <-done:
			return r.m.(Model), r.cmd
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatal("Update waited for ssh: the lookup runs inside Update")
			return m, nil
		}
	}
	snapshot := peerMsg{host: "gpu-1", conn: gpu, msg: stateMsg(ux)}

	m, cmd := update(m, snapshot)
	if n := count(); n != 0 {
		t.Fatalf("Update made %d ssh calls, want 0", n)
	}
	if cmd == nil {
		t.Fatal("no command")
	}
	var lookup tea.Msg
	finished := make(chan struct{})
	go func() {
		for _, msg := range cmdMsgs(cmd) {
			if l, ok := msg.(shellLookupMsg); ok {
				lookup = l
			}
		}
		close(finished)
	}()
	release <- struct{}{}
	<-finished
	if n := count(); n != 1 {
		t.Fatalf("%d ssh calls for 3 sessions on one host, want 1: %q", n, calls)
	}
	if line := calls[0]; !strings.Contains(line, "list-panes") || !strings.Contains(line, "-a") {
		t.Fatalf("the call is not one list-panes -a: %s", line)
	}
	if lookup == nil {
		t.Fatal("the command made no shellLookupMsg")
	}
	next, _ := m.Update(lookup)
	m = next.(Model)
	if got := m.shells[shellKey("gpu-1", "%7")]; got != "%11" {
		t.Fatalf("shell of %%7 is %q, want %%11", got)
	}

	// A failed lookup is marked tried: the next snapshot asks nothing.
	close(release)
	fail = true
	m2, gpu2, ux2 := peerShellModel(t)
	m2, cmd = update(m2, peerMsg{host: "gpu-1", conn: gpu2, msg: stateMsg(ux2)})
	for _, msg := range cmdMsgs(cmd) {
		if l, ok := msg.(shellLookupMsg); ok {
			next, _ := m2.Update(l)
			m2 = next.(Model)
		}
	}
	before := count()
	_, cmd = update(m2, peerMsg{host: "gpu-1", conn: gpu2, msg: stateMsg(ux2)})
	for _, msg := range cmdMsgs(cmd) {
		if _, ok := msg.(shellLookupMsg); ok {
			t.Fatal("a failed lookup was asked again on the next snapshot")
		}
	}
	if n := count(); n != before {
		t.Fatalf("ssh ran again after a failure: %d calls, was %d", n, before)
	}
}

// TestShellWordMakesNoTmuxCall: drawing the list asks tmux nothing. The
// liveness of a shell pane is read on the 1 s poll and kept
// (review of 2026-11-27, Minor 1).
func TestShellWordMakesNoTmuxCall(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	live := "%11\n%7\n"
	st := hostState(sess(7, "ux-pass", "%7", func(s *sample.Session) { s.Cwd = "/wt/ux-pass" }))
	m := boardShot(t, st, 80, 30, func(m *Model) {
		m.deskFrame, m.opt.Pane = true, "%1"
		m.opt.Tmux = func(args ...string) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			calls = append(calls, strings.Join(args, " "))
			if len(args) > 0 && args[0] == "list-panes" {
				return live, nil
			}
			return "", nil
		}
		m.shells = map[string]string{shellKey("", "%7"): "%11"}
	})
	s, _ := m.sideChosen()
	for range 10 {
		_ = m.View()
		_ = m.sideRow(s, 80, false, sideLink{})
	}
	if len(calls) != 0 {
		t.Fatalf("10 draws made %d tmux calls: %q", len(calls), calls)
	}
	if !strings.Contains(ansi.Strip(strings.Join(m.sideRow(s, 80, false, sideLink{}), "\n")), "shell") {
		t.Fatal("the row does not say shell while the pane lives")
	}
	// The pane dies: the next poll reads it, and the row stops saying shell.
	live = "%7\n"
	m, cmd := func() (Model, tea.Cmd) { n, c := m.Update(deskPollMsg{}); return n.(Model), c }()
	for _, msg := range cmdMsgs(cmd) {
		if l, ok := msg.(deskModeMsg); ok {
			n, _ := m.Update(l)
			m = n.(Model)
		}
	}
	if strings.Contains(ansi.Strip(strings.Join(m.sideRow(s, 80, false, sideLink{}), "\n")), "shell") {
		t.Fatal("the row still says shell after its pane is gone")
	}
}
