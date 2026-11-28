package service

import (
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

func TestKeysAtRecordsWhenTheAgentGetsTheKeys(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	s, _ := holdsTest(t, sample.Session{PID: 42, Pane: "%1", Name: "flint", SessionID: "s1", Status: "idle"})
	s.Now = func() time.Time { return now }
	res := s.act(t.Context(), proto.Act{Console: true, RecID: proto.KeysPrefix + "s1"})
	if res.Err != "" {
		t.Fatalf("err %q", res.Err)
	}
	got := s.State().KeysAt["s1"]
	if !got.Equal(now) {
		t.Fatalf("KeysAt %v, want %v", got, now)
	}
}

func TestKeysAtRefusesANonConsole(t *testing.T) {
	s, ran := holdsTest(t, sample.Session{PID: 42, Pane: "%1", Name: "flint", SessionID: "s1", Status: "idle"})
	res := s.act(t.Context(), proto.Act{RecID: proto.KeysPrefix + "s1"})
	if res.Err == "" {
		t.Fatal("a non-console wrote KeysAt")
	}
	if _, ok := s.State().KeysAt["s1"]; ok {
		t.Fatal("KeysAt was written")
	}
	if len(*ran) != 0 {
		t.Fatalf("typed into the pane: %q", *ran)
	}
}
