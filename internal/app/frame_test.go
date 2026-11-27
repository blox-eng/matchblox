package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/transport"
)

func frameAt(f frameModel, msg tea.Msg) (frameModel, tea.Cmd) {
	next, cmd := f.Update(msg)
	return next.(frameModel), cmd
}

// TestFrameHeaderNoServiceSaysSo: a header with no service draws no count
// (review of 2026-11-27, Important 2): a zero would say nobody waits.
func TestFrameHeaderNoServiceSaysSo(t *testing.T) {
	f := newFrame("header", New(Options{NoMotion: true, DialErr: errors.New("no socket"), Now: func() time.Time { return demoAt }}))
	f, _ = frameAt(f, tea.WindowSizeMsg{Width: 100, Height: 1})
	for _, step := range []string{"before any message", "after the dial error"} {
		text := headerText(f)
		if strings.Contains(text, "● 0") || strings.Contains(text, "✓ 0") {
			t.Fatalf("%s: header draws a zero count: %q", step, text)
		}
		if !strings.Contains(text, "service not reached") {
			t.Fatalf("%s: header %q does not say the service is not reached", step, text)
		}
		f, _ = frameAt(f, lostMsg{err: errors.New("no socket")})
	}
}

// TestFrameHeaderRedials: a header whose connection drops dials again, shows
// no number while it is lost, and shows the counts once a snapshot comes.
func TestFrameHeaderRedials(t *testing.T) {
	first, again := newFake(), newFake()
	dials := 0
	m := New(Options{NoMotion: true, Conn: first, Now: func() time.Time { return demoAt },
		Redial: func() (transport.Conn, error) { dials++; return again, nil }})
	f := newFrame("header", m)
	f, _ = frameAt(f, tea.WindowSizeMsg{Width: 100, Height: 1})
	f, _ = frameAt(f, stateMsg(boardState(boardMatch("hearth-api", time.Hour))))
	if text := headerText(f); !strings.Contains(text, "● 0 need you") {
		t.Fatalf("a snapshot: header %q, want its count", text)
	}
	f, cmd := frameAt(f, lostMsg{err: errors.New("EOF"), conn: first})
	if cmd == nil || !f.redialing {
		t.Fatalf("lost: cmd nil %v, redialing %v, want a redial scheduled", cmd == nil, f.redialing)
	}
	text := headerText(f)
	if strings.Contains(text, "● 0") || !strings.Contains(text, "service not reached") {
		t.Fatalf("lost: header %q, want no number and the words", text)
	}
	f, _ = frameAt(f, stateMsg(proto.State{}))
	if text := headerText(f); !strings.Contains(text, "● 0 need you") {
		t.Fatalf("after the redial's snapshot: header %q, want its count", text)
	}
}
