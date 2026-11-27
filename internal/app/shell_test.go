package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

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

func TestTNoFolder(t *testing.T) {
	st := hostState(sess(7, "ux-pass", "%7", func(s *sample.Session) { s.Cwd = "" }))
	m := boardShot(t, st, 80, 30, func(m *Model) { m.deskFrame = true })
	m = pressOn(m, "t")
	if m.noticeText != "not opened: ux-pass has no folder" {
		t.Fatalf("notice %q", m.noticeText)
	}
}
