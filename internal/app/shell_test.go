package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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
