package service

import (
	"reflect"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

func keysTest(t *testing.T) (*Service, *[][]string) {
	t.Helper()
	s, ran := holdsTest(t, sample.Session{PID: 42, Pane: "%1", Name: "flint", SessionID: "s1", Status: "idle"})
	return s, ran
}

func key(text string) proto.Act {
	return proto.Act{Console: true, RecID: proto.KeysPrefix + "%1", Text: text, PID: 42}
}

func TestKeysSendsANamedKey(t *testing.T) {
	s, _ := keysTest(t)
	res := s.act(t.Context(), key("Down"))
	if want := [][]string{panes.Keys("%1", "Down")}; res.Err != "" || !reflect.DeepEqual(res.Ran, want) {
		t.Fatalf("res %+v, want ran %q", res, want)
	}
}

func TestKeysSendsLiteralText(t *testing.T) {
	s, _ := keysTest(t)
	res := s.act(t.Context(), key("lit:yes please"))
	if want := [][]string{panes.Keys("%1", "-l", "--", "yes please")}; res.Err != "" || !reflect.DeepEqual(res.Ran, want) {
		t.Fatalf("res %+v, want ran %q", res, want)
	}
}

func TestKeysTwiceIsTwice(t *testing.T) {
	s, ran := keysTest(t)
	for range 2 {
		if res := s.act(t.Context(), key("Down")); res.Err != "" || len(res.Skipped) != 0 {
			t.Fatalf("res %+v", res)
		}
	}
	if len(*ran) != 2 {
		t.Fatalf("ran %q", *ran)
	}
}

func TestKeysRefusesWhatIsNotAKey(t *testing.T) {
	for text, name := range map[string]string{"C-x": "C-x", "lit:a\x1b[A": "control", "lit:": "empty", "lit:" + string(make([]byte, 201)): "long"} {
		s, ran := keysTest(t)
		if res := s.act(t.Context(), key(text)); res.Err == "" || len(*ran) != 0 {
			t.Fatalf("%s: res %+v ran %q", name, res, *ran)
		}
	}
	s, _ := keysTest(t)
	if res := s.act(t.Context(), key("C-x")); res.Err != "not sent: C-x is not a key" {
		t.Fatalf("err %q", res.Err)
	}
}

func TestKeysRefusedForANewAgentInThePane(t *testing.T) {
	for _, pid := range []int{41, 0} {
		s, ran := keysTest(t)
		a := key("Down")
		a.PID = pid
		if res := s.act(t.Context(), a); res.Err != "not sent: %1 runs a different agent now" || len(*ran) != 0 {
			t.Fatalf("pid %d: res %+v ran %q", pid, res, *ran)
		}
	}
}

func TestEnterRefusedForAQuestionNotSeen(t *testing.T) {
	s, ran := keysTest(t)
	since := time.Now()
	s.cur.Queue = []queue.Item{{Pane: "%1", State: queue.StateQuestion, Since: since}}
	a := key("Enter")
	a.Seen = since.Add(-time.Second)
	if res := s.act(t.Context(), a); res.Err != "not sent: flint asks something new — look again" || len(*ran) != 0 {
		t.Fatalf("res %+v ran %q", res, *ran)
	}
	a.Seen = since.Add(time.Second)
	if res := s.act(t.Context(), a); res.Err != "" || len(*ran) != 1 {
		t.Fatalf("res %+v ran %q", res, *ran)
	}
}

func TestEnterAtAPermissionPromptRuns(t *testing.T) {
	s, ran := keysTest(t)
	since := time.Now()
	s.cur.Queue = []queue.Item{{Pane: "%1", State: queue.StatePermission, Since: since}}
	a := key("Enter")
	a.Seen = since.Add(time.Second)
	if res := s.act(t.Context(), a); res.Err != "" || len(*ran) != 1 {
		t.Fatalf("res %+v ran %q", res, *ran)
	}
}

func TestKeysOnlyFromAConsole(t *testing.T) {
	s, ran := keysTest(t)
	s.StokerServer = "mbtest"
	a := key("Down")
	a.Console = false
	if res := s.act(t.Context(), a); res.Err != "keys go only from a console" {
		t.Fatalf("err %q", res.Err)
	}
	a = key("Down")
	a.From = "%7@mbtest"
	if res := s.act(t.Context(), a); res.Err != "keys go only from a console" {
		t.Fatalf("stoker: err %q", res.Err)
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}
