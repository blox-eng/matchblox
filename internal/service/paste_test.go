package service

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/mux"
	"github.com/blox-eng/matchblox/internal/mux/muxtest"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
)

func flintAuth() sample.Session {
	return sample.Session{PID: 9, Start: 3, Pane: "%3", Name: "flint-auth", SessionID: "s1", Status: "busy", Busy: true}
}

func pasteAct(pane, text string) proto.Act {
	return proto.Act{RecID: proto.PastePrefix + pane, Confirm: "y", Text: text, FromPlaced: true}
}

// TestPasteGoesInAsBracketedPaste: the text goes to tmux on stdin, into a
// buffer of its own, and comes out as one bracketed paste that deletes the
// buffer. Then Enter sends it.
func TestPasteGoesInAsBracketedPaste(t *testing.T) {
	s, _ := holdsTest(t, flintAuth())
	var ran [][]string
	var stdin []string
	s.Actions.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	s.Mux = mux.Tmux{
		Run:   func(argv []string) (string, error) { ran = append(ran, argv); return "", nil },
		RunIn: func(argv []string, in string) error { ran = append(ran, argv); stdin = append(stdin, in); return nil },
	}
	text := "fix the build\nthen run the e2e"
	res := s.act(t.Context(), pasteAct("%3", text))
	if res.Err != "" || len(res.Skipped) != 0 {
		t.Fatalf("%+v", res)
	}
	if len(ran) != 3 || len(stdin) != 1 || stdin[0] != text {
		t.Fatalf("ran %q, stdin %q", ran, stdin)
	}
	load, paste := ran[0], ran[1]
	if !slices.Equal(load[:3], []string{"tmux", "load-buffer", "-b"}) || load[4] != "-" || len(load) != 5 {
		t.Fatalf("load %q", load)
	}
	name := load[3]
	if !strings.HasPrefix(name, "matchblox-paste-") {
		t.Fatalf("buffer %q", name)
	}
	if want := []string{"tmux", "paste-buffer", "-p", "-d", "-b", name, "-t", "%3"}; !slices.Equal(paste, want) {
		t.Fatalf("paste %q, want %q", paste, want)
	}
	if want := []string{"tmux", "send-keys", "-t", "%3", "Enter"}; !slices.Equal(ran[2], want) {
		t.Fatalf("enter %q, want %q", ran[2], want)
	}
	ran = nil
	if res := s.act(t.Context(), pasteAct("%3", "another\ntext")); res.Err != "" {
		t.Fatalf("%+v", res)
	}
	if ran[0][3] == name {
		t.Fatalf("two pastes share the buffer %q", name)
	}
}

// TestPasteTakesOutControlCharacters: Review Focus 2. A paste that holds
// the end of a bracketed paste or an OSC 52 reaches the pane as text,
// never as keys: ESC, every other C0 character but \n and \t, DEL and the
// C1 characters are taken out.
func TestPasteTakesOutControlCharacters(t *testing.T) {
	s, ran := holdsTest(t, flintAuth())
	fake := &muxtest.Fake{}
	s.Mux = fake
	in := "one\x1b[201~two\r\n\tthree\x1b]52;c;aGk=\x07\x00\x7f\u009b\u0085four\xff ü"
	if res := s.act(t.Context(), pasteAct("%3", in)); res.Err != "" {
		t.Fatalf("%+v", res)
	}
	loads := fake.Loads()
	if want := "one[201~two\n\tthree]52;c;aGk=four ü"; len(loads) != 1 || loads[0].Text != want {
		t.Fatalf("loaded %q, want %q", loads, want)
	}
	if loads[0].Pane != "%3" {
		t.Fatalf("loaded on %q", loads[0].Pane)
	}
	if want := fmt.Sprint([][]string{fake.Paste("%3", loads[0].Name), fake.Keys("%3", "Enter")}); fmt.Sprint(*ran) != want {
		t.Fatalf("ran %q, want %q", *ran, want)
	}
	*ran = nil
	if res := s.act(t.Context(), pasteAct("%3", "\x1b\x07 \r\x00")); res.Err != "the paste is empty" || len(*ran) != 0 {
		t.Fatalf("a paste of controls only: %+v, ran %q", res, *ran)
	}
}

// TestPasteOverTheCapIsRefused: the words say the size and the cap. The
// size is the text that would go in, after the controls are out.
func TestPasteOverTheCapIsRefused(t *testing.T) {
	s, ran := holdsTest(t, flintAuth())
	fake := &muxtest.Fake{}
	s.Mux = fake
	res := s.act(t.Context(), pasteAct("%3", strings.Repeat("a", proto.MaxPaste+1)))
	if want := "the paste is 16385 bytes: the most is 16384"; res.Err != want {
		t.Fatalf("err %q, want %q", res.Err, want)
	}
	if len(*ran) != 0 || len(fake.Loads()) != 0 {
		t.Fatalf("ran %q, loaded %q", *ran, fake.Loads())
	}
	if res := s.act(t.Context(), pasteAct("%3", strings.Repeat("a", proto.MaxPaste)+"\x1b")); res.Err != "" {
		t.Fatalf("at the cap after the ESC is out: %+v", res)
	}
}

// TestPasteKeepsTheMessageGuard: a paste goes where a message goes, and
// nowhere else. Held, parked and asking sessions are refused with the
// words of a message; a guard that fails leaves no buffer behind.
func TestPasteKeepsTheMessageGuard(t *testing.T) {
	asks := sample.Session{PID: 10, Start: 4, Pane: "%4", Name: "spark-docs", SessionID: "s2", Status: "idle", Ask: &said.Ask{ID: "toolu_1"}}
	s, ran := holdsTest(t, flintAuth(), asks)
	fake := &muxtest.Fake{}
	s.Mux = fake
	ctx := t.Context()
	if res := s.act(ctx, pasteAct("%4", "a\nb")); res.Err != "spark-docs asks a question now: open it again to pick" {
		t.Fatalf("asking: %q", res.Err)
	}
	if res := s.act(ctx, pasteAct("%5", "a\nb")); res.Err != "no agent session in %5 any more" {
		t.Fatalf("no session: %q", res.Err)
	}
	if res := s.act(ctx, proto.Act{RecID: "hold:%3", Confirm: "y", Text: "not priority"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	*ran = nil
	if res := s.act(ctx, pasteAct("%3", "a\nb")); res.Err != "flint-auth is held by the stoker: z lets it go" {
		t.Fatalf("held: %q", res.Err)
	}
	if res := s.act(ctx, proto.Act{Console: true, RecID: "release:%3", Confirm: "y"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if res := s.act(ctx, proto.Act{Console: true, RecID: "park:%3"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	*ran = nil
	if res := s.act(ctx, pasteAct("%3", "a\nb")); res.Err != "flint-auth is parked: z lets it go" {
		t.Fatalf("parked: %q", res.Err)
	}
	if res := s.act(ctx, proto.Act{RecID: proto.PastePrefix + "%3", Text: "a\nb", FromPlaced: true}); res.Err == "" {
		t.Fatal("a paste ran without a y")
	}
	if len(*ran) != 0 || len(fake.Loads()) != 0 {
		t.Fatalf("ran %q, loaded %q", *ran, fake.Loads())
	}
	// The agent changes between the load and the paste: the step's guard
	// stops it, and the buffer goes. A second z lets the session go.
	if res := s.act(ctx, proto.Act{Console: true, RecID: "park:%3"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	g := s.Actions.Check
	s.Actions.Check = func(advice.Guard) error { return errors.New("another agent runs in %3 now") }
	defer func() { s.Actions.Check = g }()
	res := s.act(ctx, pasteAct("%3", "a\nb"))
	if len(res.Ran) != 0 || len(res.Skipped) != 1 {
		t.Fatalf("%+v", res)
	}
	if loads := fake.Loads(); len(loads) != 1 || !slices.Equal(fake.Dropped(), []string{loads[0].Name}) {
		t.Fatalf("loaded %q, dropped %q", loads, fake.Dropped())
	}
}

// TestPasteIsAPaneActLikeAMessage: an unplaced caller and an agent's pane
// that is neither the stoker's nor driven are refused, as for a message.
func TestPasteIsAPaneActLikeAMessage(t *testing.T) {
	s, ran := holdsTest(t, flintAuth())
	s.Mux = &muxtest.Fake{}
	if res := s.act(t.Context(), proto.Act{RecID: proto.PastePrefix + "%3", Confirm: "y", Text: "a\nb"}); res.Err != unplacedSender {
		t.Fatalf("unplaced: %q", res.Err)
	}
	if paneOf(proto.PastePrefix+"%3") != "%3" || typedPane(proto.PastePrefix+"%3") != "%3" {
		t.Fatal("paste: names no pane")
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

// TestAMessageOfTwoLinesIsStillRefused: matchblox send and the stoker
// send message:, and it still takes one line only.
func TestAMessageOfTwoLinesIsStillRefused(t *testing.T) {
	s, ran := holdsTest(t, flintAuth())
	res := s.act(t.Context(), proto.Act{RecID: "message:%3", Confirm: "y", Text: "fix the build\nthen run the e2e", FromPlaced: true})
	if res.Err != "the message must be one line, without control keys" || len(*ran) != 0 {
		t.Fatalf("%+v, ran %q", res, *ran)
	}
}
