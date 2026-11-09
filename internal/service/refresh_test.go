package service

import (
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
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
