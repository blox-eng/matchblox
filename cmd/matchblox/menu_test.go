package main

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/transport"
)

func pocketDoc() state.Doc {
	return state.Doc{At: time.Now(), Sessions: []sample.Session{{Pane: "%3", Name: "s", PID: 9, Start: 3, Status: "waiting"}}}
}

func TestChooseCommand(t *testing.T) {
	a, err := pocketAct(pocketDoc(), "choose", []string{"s", "2"}, "Yes", "abc", "%7")
	if err != nil {
		t.Fatal(err)
	}
	want := proto.Act{RecID: proto.ChoosePrefix + "%3", Text: "2\x00Yes\x00abc", PID: 9, Start: 3, Confirm: "y", From: "%7"}
	if a != want {
		t.Fatalf("%+v, want %+v", a, want)
	}
	for _, words := range [][]string{{"s"}, {"s", "0"}, {"s", "10"}, {"s", "two"}, {"nobody", "2"}} {
		if _, err := pocketAct(pocketDoc(), "choose", words, "Yes", "abc", ""); err == nil {
			t.Errorf("choose %q built an act", words)
		}
	}
	if _, err := pocketAct(pocketDoc(), "choose", []string{"s", "2"}, "", "abc", ""); err == nil {
		t.Error("a choose with no --text built an act")
	}
	if _, err := pocketAct(pocketDoc(), "choose", []string{"s", "2"}, "Yes", "", ""); err == nil {
		t.Error("a choose with no --sign built an act")
	}
}

// TestMenuCommandJSON runs `matchblox menu s --json` against a service
// that sees it say hello as Pocket.
func TestMenuCommandJSON(t *testing.T) {
	sock := isolate(t)
	l, err := transport.Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	clients := make(chan string, 2)
	go func() {
		for {
			nc, err := l.Accept()
			if err != nil {
				return
			}
			c := transport.NewConn(nc)
			c.Send(proto.KindHello, "", proto.Hello{Version: proto.Version})
			env, err := c.Recv()
			var h proto.Hello
			if err != nil || json.Unmarshal(env.Body, &h) != nil {
				c.Close()
				return
			}
			clients <- h.Client
			c.Send(proto.KindSnapshot, "", proto.State{Doc: pocketDoc()})
			if env, err = c.Recv(); err == nil && env.Kind == proto.KindAct {
				var a proto.Act
				json.Unmarshal(env.Body, &a)
				m := &proto.Menu{Options: []proto.Option{{N: 1, Text: "Yes"}, {N: 2, Text: "No"}}, Chosen: 2, Sign: "f00d"}
				if a.RecID != proto.MenuPrefix+"%3" {
					m = nil
				}
				c.Send(proto.KindResult, env.ID, proto.Result{ActID: env.ID, Menu: m})
			}
			c.Close()
		}
	}()
	var out bytes.Buffer
	if err := pocketCmd("menu", []string{"s", "--json"}, &out, sock); err != nil {
		t.Fatal(err)
	}
	if read, act := <-clients, <-clients; act != proto.ClientPocket {
		t.Fatalf("hellos %q then %q, want the act's as %q", read, act, proto.ClientPocket)
	}
	var got proto.Menu
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if len(got.Options) != 2 || got.Options[0] != (proto.Option{N: 1, Text: "Yes"}) || got.Chosen != 2 || got.Sign != "f00d" {
		t.Fatalf("%+v", got)
	}
}

// TestMenuOfNoMenuIsEmptyJSON: a pane with no menu prints no options, as
// a list, not null.
func TestMenuOfNoMenuIsEmptyJSON(t *testing.T) {
	var out bytes.Buffer
	if err := writeMenu(&out, &proto.Menu{}); err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil || string(raw["options"]) != "[]" {
		t.Fatalf("%v: %s", err, out.String())
	}
}
