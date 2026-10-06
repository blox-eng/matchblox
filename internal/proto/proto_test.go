package proto

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/history"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

func TestRoundTripEveryKind(t *testing.T) {
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		kind Kind
		body any
		into any
	}{
		{KindHello, Hello{Version: Version, Binary: "v0.1.0", Host: "ws-1", OS: "linux", Arch: "amd64"}, &Hello{}},
		{KindSnapshot, State{
			Doc: state.Doc{
				Snapshot:        sample.Snapshot{At: at},
				Recommendations: []advice.Rec{{ID: "r1", Level: "warn", Title: "t", Evidence: "e"}},
			},
			History: &history.Series{Points: []history.Point{{At: at, CPU: 12.5}}},
		}, &State{}},
		{KindEvent, Event{Type: "alert", Text: "swap", Pane: "%1"}, &Event{}},
		{KindAct, Act{RecID: "r1", Which: "secondary", Confirm: "y"}, &Act{}},
		{KindResult, Result{ActID: "a1", Ran: [][]string{{"kill", "1"}}, Skipped: []string{"already done"}, Err: ""}, &Result{}},
		{KindStoker, map[string]string{"mode": "off"}, &map[string]string{}},
		{KindError, Error{Text: "bad"}, &Error{}},
	}
	var buf bytes.Buffer
	for _, c := range cases {
		if err := Encode(&buf, c.kind, "id-"+string(c.kind), c.body); err != nil {
			t.Fatalf("%s: encode: %v", c.kind, err)
		}
	}
	r := bufio.NewReader(&buf)
	for _, c := range cases {
		env, err := Decode(r)
		if err != nil {
			t.Fatalf("%s: decode: %v", c.kind, err)
		}
		if env.Kind != c.kind || env.ID != "id-"+string(c.kind) {
			t.Fatalf("got kind %q id %q, want %q", env.Kind, env.ID, c.kind)
		}
		if err := json.Unmarshal(env.Body, c.into); err != nil {
			t.Fatalf("%s: body: %v", c.kind, err)
		}
		if got := reflect.ValueOf(c.into).Elem().Interface(); !reflect.DeepEqual(got, c.body) {
			t.Fatalf("%s: got %#v, want %#v", c.kind, got, c.body)
		}
	}
}

func TestDecodeRejectsHugeLine(t *testing.T) {
	line := `{"kind":"event","body":"` + strings.Repeat("a", 17<<20) + "\"}\n"
	_, err := Decode(bufio.NewReader(strings.NewReader(line)))
	if !errors.Is(err, ErrLineTooLong) {
		t.Fatalf("got %v, want ErrLineTooLong", err)
	}
}

func TestEachMessageIsOneLine(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, KindEvent, "", Event{Text: "a\nb"}); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(buf.String(), "\n"); n != 1 || !strings.HasSuffix(buf.String(), "\n") {
		t.Fatalf("want exactly one trailing newline, got %q", buf.String())
	}
}
