package sample

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/limits"
)

// claudeOut is a transcript as Claude Code 2.1 writes a turn refused by the
// plan's limit: a real turn, then a synthetic reply that carries the reset.
const claudeOut = `{"type":"assistant","isSidechain":false,"timestamp":"2026-10-11T12:45:00Z","message":{"model":"claude-opus-5-5","content":[{"type":"text","text":"Three decisions recorded."}],"usage":{"input_tokens":10,"cache_read_input_tokens":438000,"cache_creation_input_tokens":0,"output_tokens":90}}}
{"type":"user","isSidechain":false,"timestamp":"2026-10-11T14:38:50Z","message":{"role":"user","content":"go on"}}
{"type":"assistant","isSidechain":false,"timestamp":"2026-10-11T14:38:54.500Z","apiError":"usage_limit_reached","apiErrorParams":{"rate_limit_info":{"status":"rejected","resetsAt":1791737400,"rateLimitType":"five_hour"}},"error":"rate_limit","isApiErrorMessage":true,"message":{"model":"<synthetic>","content":[{"type":"text","text":"You've hit your session limit · resets 7:50pm (Europe/Sofia)"}],"usage":{"input_tokens":0,"output_tokens":0}}}
`

// A turn the limit refused leaves the session out until the reset it
// names; the context is still the last real turn's.
func TestATurnTheLimitRefusedTellsWhenTheSessionIsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	writeFile(t, path, claudeOut)
	u, ok := lastUsage(path, int64(len(claudeOut)))
	if !ok || u.Tokens != 438100 {
		t.Fatalf("usage %+v ok=%v", u, ok)
	}
	if want := time.Unix(1791737400, 0); !u.Out.Equal(want) {
		t.Fatalf("out %v, want %v", u.Out, want)
	}
}

// A real turn after the refusal: the limit came back, the session is not out.
func TestATurnAfterTheRefusalIsBack(t *testing.T) {
	body := claudeOut + `{"type":"assistant","isSidechain":false,"timestamp":"2026-10-11T16:51:00Z","message":{"model":"claude-opus-5-5","content":[{"type":"text","text":"Back at it."}],"usage":{"input_tokens":10,"cache_read_input_tokens":440000,"cache_creation_input_tokens":0,"output_tokens":90}}}` + "\n"
	path := filepath.Join(t.TempDir(), "s.jsonl")
	writeFile(t, path, body)
	if u, _ := lastUsage(path, int64(len(body))); !u.Out.IsZero() {
		t.Fatalf("out %v after a real turn", u.Out)
	}
}

// Out is a state of a session the builder would pick up: an idle one. A
// cold or stale session keeps its age (and its x end), a busy one works.
func TestOnlyAnIdleSessionIsStopped(t *testing.T) {
	out := fixtureNow.Add(time.Hour)
	for _, c := range []struct {
		s           Session
		age         string
		stopped     bool
	}{
		{Session{Out: out, Idle: 2 * time.Hour}, "idle", true},
		{Session{Out: out, Busy: true}, "busy", false},
		{Session{Out: out, Idle: ColdAfter}, "cold", false},
		{Session{Out: out, Idle: StaleAfter}, "stale", false},
		{Session{Idle: time.Hour}, "idle", false},
	} {
		if c.s.Age() != c.age || c.s.Stopped() != c.stopped {
			t.Errorf("%+v: age %q stopped %v", c.s, c.s.Age(), c.s.Stopped())
		}
	}
}

// Lines newer than the refusal: only a real turn or a compact ends it; a
// reply that names the boundary, or another local line, does not.
func TestWhatEndsARefusal(t *testing.T) {
	real := `{"type":"assistant","isSidechain":false,"timestamp":"2026-10-11T12:40:00Z","message":{"model":"claude-opus-5-5","content":[{"type":"text","text":"Look for \"compact_boundary\" in the file."}],"usage":{"input_tokens":10,"cache_read_input_tokens":400000,"cache_creation_input_tokens":0,"output_tokens":90}}}` + "\n"
	local := `{"type":"assistant","isSidechain":false,"timestamp":"2026-10-11T14:40:00Z","message":{"model":"<synthetic>","content":[{"type":"text","text":"No response requested."}],"usage":{"input_tokens":0,"output_tokens":0}}}` + "\n"
	boundary := `{"type":"system","subtype":"compact_boundary","isSidechain":false,"timestamp":"2026-10-11T14:41:00Z","compactMetadata":{"trigger":"manual","preTokens":438100,"postTokens":9400}}` + "\n"
	for _, c := range []struct {
		name string
		body string
		out  bool
	}{
		{"a local line after it", claudeOut + local, true},
		{"a reply naming the boundary before it", real + claudeOut, true},
		{"a compact after it", claudeOut + boundary, false},
	} {
		path := filepath.Join(t.TempDir(), "s.jsonl")
		writeFile(t, path, c.body)
		if u, _ := lastUsage(path, int64(len(c.body))); u.Out.IsZero() == c.out {
			t.Errorf("%s: out %v", c.name, u.Out)
		}
	}
}

// A turn after the tap's reading says the session still works (extra
// usage, a reading the reset has not reached): the tap does not stop it.
func TestATurnAfterTheReadingVetoesTheTap(t *testing.T) {
	root := copyFixture(t)
	stateDir := filepath.Join(root, "state")
	tap := `{"rate_limits":{"five_hour":{"used_percentage":100,"resets_at":` + itoa(fixtureNow.Add(2*time.Hour).Unix()) + `}}}`
	if err := limits.Tap([]byte(tap), "/home/user/.claude", stateDir, time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	smp := newFixtureSampler(root)
	smp.LimitsDir = stateDir
	for _, s := range smp.Sample().Sessions {
		if s.Agent == "claude" && s.Context == "known" && !s.Out.IsZero() {
			t.Errorf("%s with a turn after the reading is out until %v", s.Name, s.Out)
		}
	}
}

// Each Claude session on an account whose window is spent is out until
// its reset, even before its own next turn is refused.
func TestASpentAccountPutsItsSessionsOut(t *testing.T) {
	root := copyFixture(t)
	stateDir := filepath.Join(root, "state")
	resets := fixtureNow.Add(2 * time.Hour).Truncate(time.Second)
	tap := `{"rate_limits":{"five_hour":{"used_percentage":100,"resets_at":` + itoa(resets.Unix()) +
		`},"seven_day":{"used_percentage":40,"resets_at":` + itoa(fixtureNow.Add(4*24*time.Hour).Unix()) + `}}}`
	// Read after every turn of the fixture (its transcripts end past fixtureNow).
	if err := limits.Tap([]byte(tap), "/home/user/.claude", stateDir, fixtureNow.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	smp := newFixtureSampler(root)
	smp.LimitsDir = stateDir
	snap := smp.Sample()
	n := 0
	for _, s := range snap.Sessions {
		if s.Agent != "claude" || s.SessionID == "" {
			continue
		}
		n++
		if !s.Out.Equal(resets) {
			t.Errorf("%s out %v, want %v", s.Name, s.Out, resets)
		}
	}
	if n == 0 {
		t.Fatal("the fixture has no Claude session")
	}
}

func TestOutOfAReading(t *testing.T) {
	now := fixtureNow
	five := limits.Window{UsedPct: 100, Minutes: 300, ResetsAt: now.Add(time.Hour)}
	week := limits.Window{UsedPct: 100, Minutes: 10080, ResetsAt: now.Add(48 * time.Hour)}
	for _, c := range []struct {
		name string
		r    *limits.Reading
		want time.Time
	}{
		{"none", nil, time.Time{}},
		{"under", &limits.Reading{Windows: []limits.Window{{UsedPct: 99, Minutes: 300, ResetsAt: now.Add(time.Hour)}}}, time.Time{}},
		{"spent", &limits.Reading{Windows: []limits.Window{five}}, five.ResetsAt},
		{"reset passed", &limits.Reading{Windows: []limits.Window{{UsedPct: 100, Minutes: 300, ResetsAt: now.Add(-time.Minute)}}}, time.Time{}},
		{"both spent: the later", &limits.Reading{Windows: []limits.Window{five, week}}, week.ResetsAt},
	} {
		if got := c.r.Out(now); !got.Equal(c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// A 1M window rarely reaches a percentage: the token count fills it first.
func TestFullAtPercentOrTokens(t *testing.T) {
	f := Full{Pct: 85, Tokens: 350_000}
	for _, c := range []struct {
		s    Session
		want bool
	}{
		{Session{Tokens: 360_000, Window: 1_000_000, ContextPct: 36}, true},
		{Session{Tokens: 180_000, Window: 200_000, ContextPct: 90}, true},
		{Session{Tokens: 300_000, Window: 1_000_000, ContextPct: 30}, false},
		{Session{}, false},
	} {
		if got := f.Reached(c.s); got != c.want {
			t.Errorf("%+v: %v", c.s, got)
		}
	}
	if (Full{Pct: 85}).Reached(Session{Tokens: 900_000, ContextPct: 84}) {
		t.Error("no token count: only the percentage fills")
	}
	if got := f.String(); got != "85% or 350k" {
		t.Errorf("text %q", got)
	}
}
