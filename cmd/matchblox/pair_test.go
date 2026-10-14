package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/pair"
	"github.com/blox-eng/matchblox/internal/setup"
)

type pairFixture struct {
	HostKeyPub  string            `json:"hostkey_pub"`
	HostKeyFP   string            `json:"hostkey_fp"`
	RequestLine string            `json:"request_line"`
	Replies     map[string]string `json:"replies"`
}

func loadPairFixture(t *testing.T) pairFixture {
	t.Helper()
	b, err := os.ReadFile("../../testdata/pair/v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var f pairFixture
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

const testExe = "/usr/local/bin/matchblox"

func homeWithPairKey(t *testing.T) (home, id string) {
	t.Helper()
	_, authorized, id, err := pair.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	home = t.TempDir()
	if err := setup.AddPairKey(home, testExe, authorized, id, time.Now().Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	return home, id
}

func useHostLabel(t *testing.T, name string) {
	t.Helper()
	old := hostLabel
	hostLabel = func() string { return name }
	t.Cleanup(func() { hostLabel = old })
}

func TestPairMainAuthorizesAndRemovesTheCode(t *testing.T) {
	f := loadPairFixture(t)
	useHostLabel(t, "ws-1")
	home, id := homeWithPairKey(t)
	var out bytes.Buffer
	if err := pairMain(home, testExe, strings.NewReader(f.RequestLine), &out, id); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != f.Replies["ok"] {
		t.Fatalf("reply = %s, want %s", got, f.Replies["ok"])
	}
	devs, err := setup.Devices(home)
	if err != nil || len(devs) != 1 || devs[0].Name != "Jordan’s iPhone" {
		t.Fatalf("Devices = %+v, %v", devs, err)
	}
	if present, _ := setup.PairKeyPresent(home, id); present {
		t.Fatal("the one-time line is still there")
	}
}

func TestPairMainRefuses(t *testing.T) {
	f := loadPairFixture(t)
	useHostLabel(t, "ws-1")
	good := f.RequestLine
	for name, c := range map[string]struct{ in, reply string }{
		"bad name":     {strings.Replace(good, "Jordan’s iPhone", `a\"b`, 1), f.Replies["bad name"]},
		"bad key":      {`{"name":"iPhone","key":"ssh-ed25519 AAAA"}` + "\n", f.Replies["bad key"]},
		"not json":     {"hello\n", `{"ok":false,"error":"bad request"}`},
		"empty":        {"", `{"ok":false,"error":"bad request"}`},
		"too long":     {strings.Repeat("x", 10000) + "\n", `{"ok":false,"error":"bad request"}`},
		"unterminated": {strings.Repeat("{", 5000), `{"ok":false,"error":"bad request"}`},
	} {
		t.Run(name, func(t *testing.T) {
			home, id := homeWithPairKey(t)
			before, _ := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
			var out bytes.Buffer
			if err := pairMain(home, testExe, strings.NewReader(c.in), &out, id); err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(out.String()); got != c.reply {
				t.Fatalf("reply = %s, want %s", got, c.reply)
			}
			after, _ := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
			if !bytes.Equal(before, after) {
				t.Fatalf("authorized_keys changed:\n%s", after)
			}
		})
	}
}

func TestPairMainRefusesASlowPhoneAndKeepsTheCode(t *testing.T) {
	f := loadPairFixture(t)
	old := pairReadTimeout
	pairReadTimeout = 50 * time.Millisecond
	t.Cleanup(func() { pairReadTimeout = old })
	home, id := homeWithPairKey(t)
	before, _ := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))

	r, w := io.Pipe() // like the stdin of sshd: open, and silent
	var out bytes.Buffer
	if err := pairMain(home, testExe, r, &out, id); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != f.Replies["too slow"] {
		t.Fatalf("reply = %s, want %s", got, f.Replies["too slow"])
	}
	_, _ = w.Write([]byte(f.RequestLine)) // a request after the refusal claims nothing
	_ = w.Close()
	time.Sleep(20 * time.Millisecond)
	after, _ := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	if !bytes.Equal(before, after) {
		t.Fatalf("authorized_keys changed:\n%s", after)
	}
}

func TestPairMainNeverEchoesInput(t *testing.T) {
	useHostLabel(t, "ws-1")
	home, id := homeWithPairKey(t)
	var out bytes.Buffer
	_ = pairMain(home, testExe, strings.NewReader("SECRET-TOKEN\n"), &out, id)
	if strings.Contains(out.String(), "SECRET") {
		t.Fatalf("reply echoes input: %s", out.String())
	}
}

func TestPairIDIsTwelveLowercaseHex(t *testing.T) {
	for id, ok := range map[string]bool{
		"0123456789ab": true, "0123456789AB": false, "0123456789a": false,
		"0123456789abc": false, "../../etc/pa": false, "": false,
	} {
		if validPairID(id) != ok {
			t.Errorf("validPairID(%q) != %v", id, ok)
		}
	}
}

func TestPairCmdExpires(t *testing.T) {
	f := loadPairFixture(t)
	home := t.TempDir()
	hostKey := filepath.Join(t.TempDir(), "host.pub")
	if err := os.WriteFile(hostKey, []byte(f.HostKeyPub+" root@ws-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := pairSettings
	pairSettings = pairEnv{home: home, exe: testExe, hostKeyPath: hostKey, poll: time.Millisecond, limit: 5 * time.Minute}
	t.Cleanup(func() { pairSettings = old })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	now := time.Date(2026, 10, 14, 14, 32, 0, 0, time.Local)
	var out bytes.Buffer
	if err := pairCmd(ctx, []string{"--address", "ws-1.tail1234.ts.net", "--name", "ws-1"}, &out, func() time.Time { return now }); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"matchblox://pair?p=", "Scan with the iPhone camera. The code works once, until 14:37."} {
		if !strings.Contains(s, want) {
			t.Errorf("output lacks %q:\n%s", want, s)
		}
	}
	if !strings.HasSuffix(s, "The code expired. Run matchblox pair again.\n") {
		t.Errorf("output does not end with the expired line:\n%s", s)
	}
	b, _ := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	if strings.Contains(string(b), "gate pair") {
		t.Fatalf("the pair line is still there:\n%s", b)
	}
}

func TestPairCmdSaysWhenTheHostKeyIsMissing(t *testing.T) {
	old := pairSettings
	pairSettings = pairEnv{home: t.TempDir(), exe: testExe, hostKeyPath: filepath.Join(t.TempDir(), "none.pub"), poll: time.Millisecond, limit: 5 * time.Minute}
	t.Cleanup(func() { pairSettings = old })
	err := pairCmd(context.Background(), []string{"--address", "ws-1"}, &bytes.Buffer{}, time.Now)
	if err == nil || !strings.Contains(err.Error(), "sudo ssh-keygen -A") {
		t.Fatalf("err = %v", err)
	}
}

func TestPairCmdPairedWhenTheLineGoes(t *testing.T) {
	f := loadPairFixture(t)
	home := t.TempDir()
	hostKey := filepath.Join(t.TempDir(), "host.pub")
	if err := os.WriteFile(hostKey, []byte(f.HostKeyPub+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := pairSettings
	pairSettings = pairEnv{home: home, exe: testExe, hostKeyPath: hostKey, poll: time.Millisecond, limit: 5 * time.Minute}
	t.Cleanup(func() { pairSettings = old })
	deadline := time.Now().Add(10 * time.Second)
	go func() { // the phone's gate pair removes the line
		for time.Now().Before(deadline) {
			b, _ := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
			if strings.Contains(string(b), "gate pair") {
				id := string(b[strings.Index(string(b), "gate pair ")+len("gate pair "):][:12])
				_, _ = setup.RemovePairKey(home, id)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	var out bytes.Buffer
	if err := pairCmd(context.Background(), []string{"--address", "ws-1"}, &out, time.Now); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out.String(), "Paired. matchblox devices lists it.\n") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func TestPairMainWorksOnce(t *testing.T) {
	f := loadPairFixture(t)
	useHostLabel(t, "ws-1")
	home, id := homeWithPairKey(t)
	for i, want := range []string{f.Replies["ok"], f.Replies["code used"]} {
		var out bytes.Buffer
		if err := pairMain(home, testExe, strings.NewReader(f.RequestLine), &out, id); err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(out.String()); got != want {
			t.Fatalf("call %d: reply = %s, want %s", i, got, want)
		}
	}
	if devs, _ := setup.Devices(home); len(devs) != 1 {
		t.Fatalf("Devices = %+v", devs)
	}
}

func TestPairMainWithoutALineAddsNothing(t *testing.T) {
	f := loadPairFixture(t)
	home := t.TempDir()
	var out bytes.Buffer
	if err := pairMain(home, testExe, strings.NewReader(f.RequestLine), &out, "0123456789ab"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != f.Replies["code used"] {
		t.Fatalf("reply = %s", got)
	}
	if devs, _ := setup.Devices(home); len(devs) != 0 {
		t.Fatalf("Devices = %+v", devs)
	}
}
