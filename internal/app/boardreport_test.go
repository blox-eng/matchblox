package app

import (
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/stoker"
)

// reported is a match whose agent reports its own state (spec "Where a
// lane comes from"): Claude Code's session file says status, and for a
// wait the reason.
func reported(name, status, waitingFor string) sample.Session {
	s := boardMatch(name, time.Hour)
	s.Agent, s.Reported, s.Status, s.WaitingFor = "claude", true, status, waitingFor
	s.Busy = status == "busy"
	if s.Busy {
		s.Idle = 0
	}
	return s
}

// laneOf is the column and the card's word and text of the one match of st.
func laneOf(t *testing.T, st proto.State) (boardCol, string, string) {
	t.Helper()
	m := boardModel(t, st)
	if len(m.all.sessions) != 1 {
		t.Fatalf("the board has %d matches", len(m.all.sessions))
	}
	s := m.all.sessions[0]
	word, says := m.cardSays(s)
	return m.columnOf(s), word, says
}

// TestAWaitingReportNeedsYou: Claude Code says it waits at a permission
// prompt, and no hook told the queue (the service restarted): the card is
// in NEEDS YOU and says the reason.
func TestAWaitingReportNeedsYou(t *testing.T) {
	s := reported("flint-auth", "waiting", "permission prompt")
	col, word, says := laneOf(t, boardState(s))
	if col != colNeed || word != "asks" || says != "permission prompt" {
		t.Fatalf("got %s %q %q, want NEEDS YOU asks permission prompt", col.name(), word, says)
	}
}

// TestABusyReportBeatsAnOldAsk: the agent says it works; a question in its
// transcript and a question the queue still holds are old guesses.
func TestABusyReportBeatsAnOldAsk(t *testing.T) {
	s := reported("hearth-api", "busy", "")
	s.Asks, s.LastLine = true, "Should I also update the docs?"
	st := boardState(s)
	st.Queue = []queue.Item{waitFor(s, queue.StateQuestion, 20*time.Minute)}
	if col, word, _ := laneOf(t, st); col != colWork || word != "busy" {
		t.Fatalf("got %s %q, want WORKING busy", col.name(), word)
	}
	if w := stoker.Word(s); w != "busy" { // matchblox sessions agrees with the board
		t.Fatalf("stoker.Word %q, want busy", w)
	}
}

// TestAShellReportIsWorking: the turn ended and its background shells or
// agents run; matchblox may see no shell of its own.
func TestAShellReportIsWorking(t *testing.T) {
	s := reported("kindle-web", "shell", "")
	s.Asks = true // the turn that started the shells ended on a question
	if col, word, _ := laneOf(t, boardState(s)); col != colWork || word != "shell" {
		t.Fatalf("got %s %q, want WORKING shell", col.name(), word)
	}
	if !s.Works() {
		t.Fatal("a shell report does not work")
	}
}

// TestAStokerGoStillNeedsYouWhileBusy: a stoker step that waits for the
// owner's go is matchblox's own wait, whatever the agent says.
func TestAStokerGoStillNeedsYouWhileBusy(t *testing.T) {
	s := reported("ember-billing", "busy", "")
	st := boardState(s)
	st.Stoker = &proto.StokerView{Mode: proto.StokerMode{On: true}, Run: []proto.StokerEntry{goFor(s, 2*time.Minute)}}
	if col, _, _ := laneOf(t, st); col != colNeed {
		t.Fatalf("got %s, want NEEDS YOU", col.name())
	}
}

// TestAnIdleReportKeepsTodaysRules: an idle agent is read as before: a turn
// that ended on a question asks, and one that asked nothing is QUIET.
func TestAnIdleReportKeepsTodaysRules(t *testing.T) {
	s := reported("spark-docs", "idle", "")
	if col, _, _ := laneOf(t, boardState(s)); col != colQuiet {
		t.Fatalf("idle, nothing: got %s, want QUIET", col.name())
	}
	s.Asks, s.LastLine = true, "Should I also update the docs?"
	if col, _, says := laneOf(t, boardState(s)); col != colNeed || says != s.LastLine {
		t.Fatalf("idle on a question: got %s %q, want NEEDS YOU", col.name(), says)
	}
}
