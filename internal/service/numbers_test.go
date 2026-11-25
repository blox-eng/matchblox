package service

import (
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/numbers"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

func numbersTest(t *testing.T) *Service {
	t.Helper()
	s := newTest(t, time.Hour)
	st, err := numbers.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.Numbers = st
	return s
}

func numbered(id string, pid int) sample.Session {
	return sample.Session{PID: pid, Start: 3, Pane: "%" + id, Name: id, SessionID: "s" + id, Status: "idle"}
}

func numbersOf(s *Service) map[string]int {
	got := map[string]int{}
	for _, x := range s.State().Sessions {
		got[x.Name] = x.Number
	}
	return got
}

func publishSessions(s *Service, errs []string, sessions ...sample.Session) {
	s.publish(sample.Snapshot{At: time.Now(), Sessions: sessions, Errors: errs}, func(state.Doc) {})
}

func TestPublishNumbersTheSessions(t *testing.T) {
	s := numbersTest(t)
	publishSessions(s, nil, numbered("1", 11), numbered("2", 12), numbered("3", 13))
	if got := numbersOf(s); got["1"] != 1 || got["2"] != 2 || got["3"] != 3 {
		t.Fatalf("numbers %v", got)
	}
}

func TestAMissedSampleKeepsTheNumbers(t *testing.T) {
	s := numbersTest(t)
	publishSessions(s, nil, numbered("1", 11), numbered("2", 12), numbered("3", 13))
	publishSessions(s, []string{"tmux: no server"}, numbered("3", 13))
	publishSessions(s, nil, numbered("2", 12), numbered("1", 11), numbered("3", 13))
	if got := numbersOf(s); got["1"] != 1 || got["2"] != 2 || got["3"] != 3 {
		t.Fatalf("numbers %v", got)
	}
}

func TestTheStokerTakesNoNumber(t *testing.T) {
	s := numbersTest(t)
	s.StokerServer = "mbtest"
	stoker := numbered("9", 19)
	stoker.Pane = "%9@mbtest"
	publishSessions(s, nil, stoker, numbered("1", 11))
	if got := numbersOf(s); got["9"] != 0 || got["1"] != 1 {
		t.Fatalf("numbers %v", got)
	}
}

func TestNoStoreNoNumbers(t *testing.T) {
	s := newTest(t, time.Hour)
	publishSessions(s, nil, numbered("1", 11), numbered("2", 12))
	if got := numbersOf(s); got["1"] != 0 || got["2"] != 0 {
		t.Fatalf("numbers %v", got)
	}
}
