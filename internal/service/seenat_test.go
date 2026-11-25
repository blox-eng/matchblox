package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

var seenAt0 = time.Date(2026, 11, 23, 10, 9, 0, 0, time.UTC)

func seenTest(t *testing.T, statePath string, sess sample.Session) *Service {
	t.Helper()
	s := newTest(t, time.Hour)
	s.StatePath = statePath
	s.Now = func() time.Time { return seenAt0 }
	s.sample = func() sample.Snapshot { return sample.Snapshot{At: seenAt0, Sessions: []sample.Session{sess}} }
	return s
}

func seenAct(pane string) proto.Act {
	return proto.Act{RecID: proto.SeenPrefix + pane, Console: true}
}

func TestSeenIsKeptPerOwner(t *testing.T) {
	for _, sid := range []string{"s1", ""} { // no conversation id: the tmux session's name keys it
		statePath := filepath.Join(t.TempDir(), "state.json")
		sess := flintAuth()
		sess.SessionID, sess.Target = sid, "flint:1.0"
		s := seenTest(t, statePath, sess)
		ctx := run(t, s)
		one := connectAs(t, ctx, s, proto.ClientConsole)
		two := connectAs(t, ctx, s, proto.ClientConsole)
		snapshot(t, one)
		if r := act(t, one, "s1", seenAct("%3")); r.Err != "" {
			t.Fatalf("session %q: seen:%%3 answered %+v", sid, r)
		}
		for st := snapshot(t, two); len(st.Sessions) != 1 || !st.Sessions[0].SeenAt.Equal(seenAt0); st = snapshot(t, two) {
		}
		if fi, err := os.Stat(filepath.Join(filepath.Dir(statePath), "seen.json")); err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("session %q: seen.json: %v, %v", sid, fi, err)
		}

		recovered := sess
		recovered.Pane = "%4" // a recover gives the session a new pane
		again := seenTest(t, statePath, recovered)
		again.Now = func() time.Time { return seenAt0.Add(time.Hour) }
		c, _ := connect(t, run(t, again), again)
		if st := snapshot(t, c); len(st.Sessions) != 1 || !st.Sessions[0].SeenAt.Equal(seenAt0) {
			t.Fatalf("session %q: after a restart the state reads %+v, want SeenAt %s", sid, st.Sessions, seenAt0)
		}
	}
}

func TestSeenRefusesAPaneThatIsGone(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	s := seenTest(t, statePath, flintAuth())
	c, _ := connect(t, run(t, s), s)
	snapshot(t, c)
	for _, c := range []struct{ pane, want string }{
		{"%99", `not kept: no session "%99"`},
		{"../x", `not kept: no session "../x"`},
		{"%3\n", `not kept: no session "%3\n"`},
		{"%3\x1b]0;x\x07", `not kept: no session "%3\x1b]0;x\a"`},
	} {
		r := s.act(t.Context(), seenAct(c.pane))
		if r.Err != c.want || !r.Refused || strings.ContainsAny(r.Err, "\n\x1b\x07") {
			t.Fatalf("seen:%q answered %+v, want %q", c.pane, r, c.want)
		}
	}
	agents := seenAct("%3")
	agents.From = "%3" // an agent's own pane
	stranger := seenAct("%3")
	stranger.Console = false // a CLI or a peer
	for _, a := range []proto.Act{agents, stranger} {
		if r := s.act(t.Context(), a); r.Err != seenOnlyFromConsole {
			t.Fatalf("seen:%%3 from %+v answered %+v, want %q", a, r, seenOnlyFromConsole)
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(statePath))
	for _, e := range entries {
		if e.Name() != "state.json" {
			t.Fatalf("a refused seen wrote %s", e.Name())
		}
	}
	if x := s.State().Sessions[0]; !x.SeenAt.IsZero() {
		t.Fatalf("a refused seen stamped %+v", x)
	}
}

func TestSeenDropsASessionGoneForAWeek(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	gone, here := flintAuth(), flintAuth()
	here.Pane, here.SessionID = "%5", "s2"
	s := seenTest(t, statePath, gone)
	sampleAt(s, 0, gone, here)
	for _, pane := range []string{"%3", "%5"} {
		if r := s.act(t.Context(), seenAct(pane)); r.Err != "" {
			t.Fatalf("seen:%s answered %+v", pane, r)
		}
	}
	sampleAt(s, time.Minute, here)
	for _, c := range []struct {
		after time.Duration
		kept  bool
	}{{7 * 24 * time.Hour, true}, {7*24*time.Hour + time.Minute, false}} {
		s.Now = func() time.Time { return seenAt0.Add(c.after) }
		if r := s.act(t.Context(), seenAct("%5")); r.Err != "" {
			t.Fatalf("seen:%%5 answered %+v", r)
		}
		b, _ := os.ReadFile(filepath.Join(filepath.Dir(statePath), "seen.json"))
		var by map[string]seenEntry
		if err := json.Unmarshal(b, &by); err != nil {
			t.Fatal(err)
		}
		if _, ok := by["sid:s1"]; ok != c.kept || len(by) == 0 {
			t.Fatalf("at +%s seen.json holds %s, want the gone session kept %v", c.after, b, c.kept)
		}
	}
}

func TestSeenSetsABadFileAside(t *testing.T) {
	for _, older := range []bool{false, true} {
		dir := t.TempDir()
		path := filepath.Join(dir, "seen.json")
		if err := os.WriteFile(path, []byte("{garbage"), 0o600); err != nil {
			t.Fatal(err)
		}
		if older {
			if err := os.WriteFile(path+".bad", []byte("older"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		log := stderrTo(t)
		s := seenTest(t, filepath.Join(dir, "state.json"), flintAuth())
		if st := snapshot(t, connectAs(t, run(t, s), s, proto.ClientConsole)); !st.Sessions[0].SeenAt.IsZero() {
			t.Fatalf("a bad seen.json stamped %+v", st.Sessions[0])
		}
		bad := path + ".bad"
		if older {
			if b, _ := os.ReadFile(bad); string(b) != "older" {
				t.Fatalf("seen.json.bad was overwritten: %q", b)
			}
			found, _ := filepath.Glob(path + ".bad-*")
			if len(found) != 1 {
				t.Fatalf("set aside as %q, want one seen.json.bad-*", found)
			}
			bad = found[0]
		}
		if b, _ := os.ReadFile(bad); string(b) != "{garbage" {
			t.Fatalf("%s holds %q, want the garbage", bad, b)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("seen.json stays: %v", err)
		}
		if line := log(); !strings.Contains(line, "matchblox: seen.json did not read") || !strings.Contains(line, bad) {
			t.Fatalf("log %q names no %s", line, bad)
		}
	}
}

// stderrTo sends os.Stderr to a file until the test ends; the func reads it.
func stderrTo(t *testing.T) func() string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = f
	t.Cleanup(func() { os.Stderr = old; f.Close() })
	return func() string { b, _ := os.ReadFile(f.Name()); return string(b) }
}
