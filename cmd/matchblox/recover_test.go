package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/proto"
)

func TestRecoverActReadsTheCommandLine(t *testing.T) {
	for _, c := range []struct {
		words []string
		all   bool
		dir   string
		want  proto.Recover
		err   string
	}{
		{want: proto.Recover{}},
		{all: true, want: proto.Recover{All: true}},
		{words: []string{"api", "web"}, want: proto.Recover{Names: []string{"api", "web"}}},
		{words: []string{"api"}, dir: "/src/api2", want: proto.Recover{Names: []string{"api"}, Dirs: map[string]string{"api": "/src/api2"}}},
		{words: []string{"api"}, all: true, err: "--all or names"},
		{dir: "/x", err: "matchblox recover <name> --dir <path>"},
		{words: []string{"a", "b"}, dir: "/x", err: "matchblox recover <name> --dir <path>"},
	} {
		a, err := recoverAct(c.words, c.all, c.dir, "%3")
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%+v: err %v, want %q", c, err, c.err)
			}
			continue
		}
		var got proto.Recover
		if err != nil || json.Unmarshal([]byte(a.Text), &got) != nil {
			t.Fatalf("%+v: %v %q", c, err, a.Text)
		}
		gb, _ := json.Marshal(got)
		wb, _ := json.Marshal(c.want)
		if string(gb) != string(wb) || a.From != "%3" || a.RecID != "recover" {
			t.Errorf("act %+v %s, want %s", a, gb, wb)
		}
	}
}

func TestRecoverPrintsEachEntryAndItsFix(t *testing.T) {
	rows := []proto.Recovered{
		{Name: "api", Agent: "claude", State: proto.RecoverReady, Dir: "/src/api"},
		{Name: "web", Agent: "codex", State: proto.RecoverLive, Dir: "/src/web"},
		{Name: "gone", Agent: "claude", State: proto.RecoverCannot, Dir: "/src/gone", Why: "the folder /src/gone is gone: run matchblox recover gone --dir <its new place>"},
	}
	var b bytes.Buffer
	if err := printRecover(&b, rows, false); err != nil {
		t.Fatalf("a preview fails: %v", err)
	}
	out := b.String()
	for _, want := range []string{"api", "can reopen", "web", "live", "cannot reopen", "matchblox recover gone --dir", "matchblox recover --all reopens the 1 that can reopen"} {
		if !strings.Contains(out, want) {
			t.Errorf("the preview lacks %q:\n%s", want, out)
		}
	}

	rows[0].State, rows[0].As = proto.RecoverReopened, "api-2"
	b.Reset()
	err := printRecover(&b, rows, true)
	if err == nil || !strings.Contains(err.Error(), "1 could not reopen") {
		t.Fatalf("a reopen with a cannot: err %v", err)
	}
	if !strings.Contains(b.String(), "reopened as api-2") || strings.Contains(b.String(), "--all reopens") {
		t.Fatalf("the reopen prints:\n%s", b.String())
	}

	b.Reset()
	if err := printRecover(&b, nil, false); err != nil || !strings.Contains(b.String(), "The recovery record is empty") {
		t.Fatalf("an empty record: %v %q", err, b.String())
	}
}
