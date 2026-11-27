package queue

import (
	"fmt"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/sample"
)

func prompt(id string, s int, text string) hooks.Event {
	return hooks.Event{Name: "UserPromptSubmit", Agent: hooks.CursorAgent, SessionID: id, Pane: "%6", At: at(s), Text: text}
}

func reply(id string, s int, text string) hooks.Event {
	return hooks.Event{Name: hooks.Reply, Agent: hooks.CursorAgent, SessionID: id, Pane: "%6", At: at(s), Text: text}
}

// TestTalkIsDroppedWithItsSession: a session's words leave memory with the
// session, on every path the queue drops it by.
func TestTalkIsDroppedWithItsSession(t *testing.T) {
	q := New()
	q.Apply(prompt("forgot", 0, "the secret plan"))
	q.Apply(reply("forgot", 1, "noted"))
	if len(q.Talk("forgot")) != 2 {
		t.Fatalf("talk %+v before Forget, want 2 turns", q.Talk("forgot"))
	}
	q.Forget([]string{"forgot"}, at(2))
	q.Apply(reply("forgot", 3, "a late reply"))
	if talk := q.Talk("forgot"); len(talk) != 0 {
		t.Fatalf("talk %+v after Forget, want none", talk)
	}

	q.Apply(prompt("unseen", 0, "the secret plan"))
	q.Apply(reply("unseen", 1, "noted"))
	q.Apply(reply("replied", 0, "a reply with no session"))
	q.Merge(nil, at(10))
	if len(q.Talk("unseen")) != 2 || len(q.Talk("replied")) != 1 {
		t.Fatalf("talk dropped within UnseenGrace: %+v %+v", q.Talk("unseen"), q.Talk("replied"))
	}
	q.Merge([]sample.Session{}, at(40))
	for _, id := range []string{"unseen", "replied"} {
		if talk := q.Talk(id); len(talk) != 0 {
			t.Fatalf("talk of %s %+v past UnseenGrace, want none", id, talk)
		}
	}
}

func TestTalkKeepsTurnsInOrder(t *testing.T) {
	text := func(role string, n int) string {
		head := fmt.Sprintf("%s %02d ", role, n)
		return head + strings.Repeat("x", 4096-len(head))
	}
	q := New()
	for n := 1; n <= 30; n++ {
		q.Apply(prompt("c1", 2*n, text("prompt", n)))
		q.Apply(reply("c1", 2*n+1, text("reply", n)))
	}
	talk := q.Talk("c1")
	if len(talk) < 3 || talk[0] != (Turn{Role: "user", Text: text("prompt", 1)}) {
		t.Fatalf("talk starts %+v, want prompt 01 first", talk[:min(len(talk), 1)])
	}
	size := 0
	for _, turn := range talk {
		size += len(turn.Text)
	}
	if size > TalkLimit {
		t.Fatalf("talk holds %d bytes, over TalkLimit %d", size, TalkLimit)
	}
	if size+2*4096 <= TalkLimit {
		t.Fatalf("talk holds %d bytes: it dropped turns it had room for", size)
	}
	rest := talk[1:]
	for i, turn := range rest {
		k := 60 - len(rest) + i // 0-based index of the turn among the 60
		n, role, want := k/2+1, "user", "prompt"
		if k%2 == 1 {
			role, want = "assistant", "reply"
		}
		if turn != (Turn{Role: role, Text: text(want, n)}) {
			t.Fatalf("turn %d is %.10q (%s), want %s %02d", i+1, turn.Text, turn.Role, want, n)
		}
	}
	if len(rest) >= 59 {
		t.Fatalf("talk kept %d turns after the first: none dropped", len(rest))
	}
}

func TestTalkTakesRepliesBeforeTheirEarlyReturn(t *testing.T) {
	q := New()
	q.Apply(prompt("c1", 0, "fix the bug"))
	q.Apply(hooks.Event{Name: "Stop", Agent: hooks.CursorAgent, SessionID: "c1", Pane: "%6", At: at(1), Turn: "g1"})
	q.Apply(hooks.Event{Name: hooks.Reply, Agent: hooks.CursorAgent, SessionID: "c1", Pane: "%6", At: at(1), Turn: "g1", Message: "Fixed.", Text: "Fixed. The bug was in the parser."})
	q.Apply(hooks.Event{Name: hooks.Reply, Agent: hooks.CursorAgent, SessionID: "c1", Pane: "%6", At: at(2), Message: "no text"})
	want := []Turn{{Role: "user", Text: "fix the bug"}, {Role: "assistant", Text: "Fixed. The bug was in the parser."}}
	if got := q.Talk("c1"); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("talk %+v, want %+v", got, want)
	}
}
