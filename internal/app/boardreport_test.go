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

// laneOf is the list's group and the row's word and text of the one
// session of st.
func laneOf(t *testing.T, st proto.State) (sideState, string, string) {
	t.Helper()
	m := boardModel(t, st)
	if len(m.all.sessions) != 1 {
		t.Fatalf("the list has %d sessions", len(m.all.sessions))
	}
	s := m.all.sessions[0]
	word, says := m.cardSays(s)
	return m.sideStateOf(s), word, says
}

// TestAWaitingReportNeedsYou: Claude Code says it waits at a permission
// prompt, and no hook told the queue (the service restarted): the card is
// in NEEDS YOU and says the reason.
func TestAWaitingReportNeedsYou(t *testing.T) {
	s := reported("flint-auth", "waiting", "permission prompt")
	col, word, says := laneOf(t, boardState(s))
	if col != sideNeed || word != "asks" || says != "permission prompt" {
		t.Fatalf("got %s %q %q, want NEEDS YOU asks permission prompt", col.head(), word, says)
	}
	if w := stoker.Word(s); w != "asks" { // matchblox sessions and peek say the board's word
		t.Fatalf("stoker.Word %q, want asks", w)
	}
}

// TestAReportedWaitBeatsAFinishedRow: a queue row of a turn that ended
// (its word: done) is older than the report that the agent waits: the card
// says one thing, asks and why.
func TestAReportedWaitBeatsAFinishedRow(t *testing.T) {
	s := reported("flint-auth", "waiting", "permission prompt")
	st := boardState(s)
	st.Queue = []queue.Item{waitFor(s, queue.StateFinished, 20*time.Minute)}
	if col, word, says := laneOf(t, st); col != sideNeed || word != "asks" || says != "permission prompt" {
		t.Fatalf("got %s %q %q, want NEEDS YOU asks permission prompt", col.head(), word, says)
	}
}

// TestABusyReportBeatsAnOldAsk: the agent says it works; a question in its
// transcript and a question the queue still holds are old guesses.
func TestABusyReportBeatsAnOldAsk(t *testing.T) {
	s := reported("hearth-api", "busy", "")
	s.Asks, s.LastLine = true, "Should I also update the docs?"
	st := boardState(s)
	st.Queue = []queue.Item{waitFor(s, queue.StateQuestion, 20*time.Minute)}
	if col, word, _ := laneOf(t, st); col != sideWork || word != "busy" {
		t.Fatalf("got %s %q, want WORKING busy", col.head(), word)
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
	if col, word, _ := laneOf(t, boardState(s)); col != sideWork || word != "shell" {
		t.Fatalf("got %s %q, want WORKING shell", col.head(), word)
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
	if col, _, _ := laneOf(t, st); col != sideNeed {
		t.Fatalf("got %s, want NEEDS YOU", col.head())
	}
}

// TestAnIdleReportKeepsTodaysRules: an idle agent is read as before: a turn
// that ended on a question asks, and one that asked nothing is QUIET.
func TestAnIdleReportKeepsTodaysRules(t *testing.T) {
	s := reported("spark-docs", "idle", "")
	if col, _, _ := laneOf(t, boardState(s)); col != sideIdle {
		t.Fatalf("idle, nothing: got %s, want QUIET", col.head())
	}
	s.Asks, s.LastLine = true, "Should I also update the docs?"
	if col, _, says := laneOf(t, boardState(s)); col != sideNeed || says != s.LastLine {
		t.Fatalf("idle on a question: got %s %q, want NEEDS YOU", col.head(), says)
	}
}

// TestAnOpencodeReportDecidesItsLane: OpenCode's plugin reports in the same
// words, and its lanes are Claude Code's. A retry works: OpenCode tries
// again by itself, and the card says which attempt.
func TestAnOpencodeReportDecidesItsLane(t *testing.T) {
	opencode := func(status, why string) sample.Session {
		s := reported("wick-mobile", status, why)
		s.Agent, s.SessionID = "opencode", ""
		s.Busy = status == "busy" || status == "retry"
		return s
	}
	for _, c := range []struct {
		status, why string
		want        sideState
		says        string
	}{
		{"waiting", "permission", sideNeed, "permission"},
		{"waiting", "question", sideNeed, "question"},
		{"busy", "", sideWork, ""},
		{"idle", "", sideIdle, ""},
	} {
		s := opencode(c.status, c.why)
		s.Asks = c.status == "busy" // an old question while it works is no ask
		col, _, says := laneOf(t, boardState(s))
		if col != c.want || c.says != "" && says != c.says {
			t.Errorf("%s %s: got %s %q, want %s %q", c.status, c.why, col.head(), says, c.want.head(), c.says)
		}
	}
	s := opencode("retry", "")
	s.Retry = 3
	if col, word, says := laneOf(t, boardState(s)); col != sideWork || word != "busy" || says != "retrying · attempt 3" {
		t.Fatalf("retry: got %s %q %q, want WORKING busy retrying · attempt 3", col.head(), word, says)
	}
}
