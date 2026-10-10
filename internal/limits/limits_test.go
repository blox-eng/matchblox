package limits

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mon is the Monday 09:00 a week starts in these tests: it resets the next
// Monday 09:00.
var mon = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

var quiet = mustQuiet("23:00-08:00", []string{"Sat", "sun"})

func mustQuiet(h string, d []string) Quiet {
	q, err := ParseQuiet(h, d)
	if err != nil {
		panic(err)
	}
	return q
}

func week(used float64) Window {
	return Window{UsedPct: used, Minutes: WeekMinutes, ResetsAt: mon.Add(7 * 24 * time.Hour)}
}

// TestMatchesAndSparksCountWhatIsLeft: 5 matches are a 5-hour window, 10
// sparks a week; a part of one that is left still counts as one.
func TestMatchesAndSparksCountWhatIsLeft(t *testing.T) {
	now := mon.Add(time.Hour)
	for _, c := range []struct {
		used          float64
		matches, sprk int
	}{{0, 5, 10}, {28, 4, 8}, {80, 1, 2}, {99.5, 1, 1}, {100, 0, 0}, {130, 0, 0}} {
		a := Build("anthropic", "a@example.com · Max", SourceClaude, &Reading{At: now, Windows: []Window{
			{UsedPct: c.used, Minutes: BoxMinutes, ResetsAt: now.Add(2 * time.Hour)},
			{UsedPct: c.used, Minutes: WeekMinutes, ResetsAt: mon.Add(7 * 24 * time.Hour)},
		}}, now, quiet)
		if a.Matches == nil || *a.Matches != c.matches || a.Sparks == nil || *a.Sparks != c.sprk {
			t.Errorf("used %v%%: matches %v sparks %v, want %d and %d", c.used, deref(a.Matches), deref(a.Sparks), c.matches, c.sprk)
		}
		if a.State != Measured {
			t.Errorf("used %v%%: state %q", c.used, a.State)
		}
	}
}

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// TestAWindowIsKnownByItsLength: Codex names its windows primary and
// secondary, and a plan can have a month. The length decides: 5 hours are
// matches, a week sparks, any other window shows its percent left.
func TestAWindowIsKnownByItsLength(t *testing.T) {
	now := mon.Add(time.Hour)
	a := Build("openai", "b@example.com · Go", SourceCodex, &Reading{At: now, Windows: []Window{
		{UsedPct: 47, Minutes: 43200, ResetsAt: now.Add(9 * 24 * time.Hour)},
	}}, now, quiet)
	if a.Matches != nil || a.Sparks != nil {
		t.Fatalf("a month became matches or sparks: %+v", a)
	}
	if len(a.Other) != 1 || a.Other[0].Text != "53% left this month" {
		t.Fatalf("other windows: %+v", a.Other)
	}
}

// TestAnOldReadingSaysHowOld: a reading over an hour old is stale; a window
// whose reset has passed starts full again and is known.
func TestAnOldReadingSaysHowOld(t *testing.T) {
	now := mon.Add(5 * time.Hour)
	r := &Reading{At: now.Add(-2 * time.Hour), Windows: []Window{
		{UsedPct: 90, Minutes: BoxMinutes, ResetsAt: now.Add(-time.Minute)},
		{UsedPct: 30, Minutes: WeekMinutes, ResetsAt: mon.Add(7 * 24 * time.Hour)},
	}}
	a := Build("anthropic", "a@example.com · Max", SourceClaude, r, now, quiet)
	if a.State != Stale || !a.At.Equal(r.At) {
		t.Fatalf("state %q at %v", a.State, a.At)
	}
	if *a.Matches != 5 || !a.MatchesReset.IsZero() {
		t.Fatalf("a window past its reset: %d matches, reset %v", *a.Matches, a.MatchesReset)
	}
	// Every window reset since: the figures are known, not old.
	all := &Reading{At: now.Add(-2 * time.Hour), Windows: []Window{{UsedPct: 90, Minutes: BoxMinutes, ResetsAt: now.Add(-time.Minute)}}}
	if a := Build("anthropic", "", SourceClaude, all, now, quiet); a.State != Measured {
		t.Fatalf("all reset since: %q", a.State)
	}
	r.At = now.Add(-10 * time.Minute)
	if a := Build("anthropic", "", SourceClaude, r, now, quiet); a.State != Measured {
		t.Fatalf("a fresh reading: %q", a.State)
	}
}

// TestNoReadingSaysWhatToDo: a missing source is not a blank: it names the
// fix for its agent.
func TestNoReadingSaysWhatToDo(t *testing.T) {
	for src, fix := range map[string]string{
		SourceClaude:   "open the door “Show your limits”",
		SourceCodex:    "Codex reports them after its next turn",
		SourceOpenCode: "OpenCode keeps no limits",
	} {
		a := Build("x", "", src, nil, mon, quiet)
		if a.State != Unmeasured || !strings.Contains(a.Fix, fix) {
			t.Errorf("%s: state %q fix %q", src, a.State, a.Fix)
		}
	}
	a := Build("openai", "API key", SourceCodex, &Reading{At: mon}, mon, quiet)
	if a.State != Unmeasured || a.Fix != "an API key has no plan limits" {
		t.Errorf("an API key: %q %q", a.State, a.Fix)
	}
}

// TestTheForecastDiscountsQuietHours: the week is forecast from the rate of
// the hours worked so far, and the quiet hours and days ahead count a
// tenth. A busy Friday evening is not a crisis: the weekend is quiet.
func TestTheForecastDiscountsQuietHours(t *testing.T) {
	at := func(d time.Duration) time.Time { return mon.Add(d) }
	day := 24 * time.Hour
	for _, c := range []struct {
		name  string
		now   time.Time
		used  float64
		lasts bool
		out   time.Time
	}{
		{"a calm Tuesday", at(day), 10, true, time.Time{}},
		{"a hot Tuesday", at(day), 40, false, time.Date(2026, 10, 14, 17, 0, 0, 0, time.UTC)},
		{"a busy Friday evening", at(4*day + 13*time.Hour), 80, true, time.Time{}},
		{"spent", at(2 * day), 100, false, at(2 * day)},
		{"out after the reset is not out", at(6 * day), 95, true, time.Time{}},
	} {
		f := Forecast(week(c.used), c.now, quiet)
		if f == nil {
			t.Errorf("%s: no forecast", c.name)
			continue
		}
		if f.Lasts != c.lasts || !f.Out.Equal(c.out) {
			t.Errorf("%s: lasts %v out %v, want %v %v", c.name, f.Lasts, f.Out, c.lasts, c.out)
		}
	}
	if f := Forecast(week(5), mon.Add(30*time.Minute), quiet); f != nil {
		t.Errorf("half an hour into the week is too early to tell: %+v", f)
	}
}

func TestParseQuiet(t *testing.T) {
	q := mustQuiet("23:00-08:00", []string{"Sat", "sun"})
	for _, c := range []struct {
		at   time.Time
		want bool
	}{
		{time.Date(2026, 10, 13, 23, 30, 0, 0, time.UTC), true},
		{time.Date(2026, 10, 13, 7, 59, 0, 0, time.UTC), true},
		{time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC), true}, // Saturday
	} {
		if got := q.Quiet(c.at); got != c.want {
			t.Errorf("%v: quiet %v", c.at, got)
		}
	}
	for _, bad := range []string{"23-08", "25:00-08:00", "08:00"} {
		if _, err := ParseQuiet(bad, nil); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
	if _, err := ParseQuiet("", []string{"Someday"}); err == nil {
		t.Error("a day that is not a day parsed")
	}
	if q, err := ParseQuiet("", nil); err != nil || q.Quiet(mon) {
		t.Errorf("no quiet hours: %v %v", q, err)
	}
}

// statusLine is what Claude Code gives a status line on stdin (shortened).
const statusLine = `{"session_id":"s-1","transcript_path":"/home/dev/.claude/projects/x/s-1.jsonl","cwd":"/home/dev/app",
"model":{"id":"claude-opus","display_name":"Opus"},"workspace":{"current_dir":"/home/dev/app"},
"cost":{"total_cost_usd":1.2},
"rate_limits":{"five_hour":{"used_percentage":28,"resets_at":1791673800},"seven_day":{"used_percentage":2,"resets_at":1792267200}}}`

// TestTheStatusLineTapKeepsOnlyTheLimits: the tap stores the two windows
// for its config directory, and nothing else the status line was given.
func TestTheStatusLineTapKeepsOnlyTheLimits(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1791667916, 0)
	if err := Tap([]byte(statusLine), "/home/dev/.claude", dir, now); err != nil {
		t.Fatal(err)
	}
	r, ok := ReadTap(dir, "/home/dev/.claude")
	if !ok || !r.At.Equal(now) || len(r.Windows) != 2 {
		t.Fatalf("read back: %+v %v", r, ok)
	}
	if w := r.Windows[0]; w.Minutes != BoxMinutes || w.UsedPct != 28 || !w.ResetsAt.Equal(time.Unix(1791673800, 0)) {
		t.Fatalf("five hours: %+v", w)
	}
	if w := r.Windows[1]; w.Minutes != WeekMinutes || w.UsedPct != 2 {
		t.Fatalf("the week: %+v", w)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "limits", "*"))
	if len(files) != 1 {
		t.Fatalf("files: %v", files)
	}
	b, _ := os.ReadFile(files[0])
	for _, leak := range []string{"s-1", "transcript", "cost", "/home/dev/app"} {
		if strings.Contains(string(b), leak) {
			t.Fatalf("the tap kept %q: %s", leak, b)
		}
	}
	if _, ok := ReadTap(dir, "/home/other/.claude"); ok {
		t.Fatal("another config directory read this one's limits")
	}
	// An idle session reports its last answer's figures: the same window
	// keeps the higher use.
	idle := strings.Replace(statusLine, `"seven_day":{"used_percentage":2,`, `"seven_day":{"used_percentage":1,`, 1)
	if err := Tap([]byte(idle), "/home/dev/.claude", dir, now.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if r, _ := ReadTap(dir, "/home/dev/.claude"); r.Windows[1].UsedPct != 2 {
		t.Fatalf("an idle session lowered the week: %+v", r.Windows[1])
	}
	// No resets_at is not a reset.
	if err := Tap([]byte(`{"rate_limits":{"five_hour":{"used_percentage":50}}}`), "/home/x/.claude", dir, now); err != nil {
		t.Fatal(err)
	}
	if r, _ := ReadTap(dir, "/home/x/.claude"); !r.Windows[0].ResetsAt.IsZero() {
		t.Fatalf("resets_at 0 became a time: %v", r.Windows[0].ResetsAt)
	}
	// An API key session has no rate_limits: the last reading stays.
	if err := Tap([]byte(`{"session_id":"s-2"}`), "/home/dev/.claude", dir, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if r, _ := ReadTap(dir, "/home/dev/.claude"); !r.At.Equal(now.Add(30 * time.Second)) {
		t.Fatalf("a status line without limits replaced the reading: %+v", r)
	}
}

// TestTheStoreKeepsFiveWeeksOfChanges: one line per account when its
// reading changes; lines older than 35 days go.
func TestTheStoreKeepsFiveWeeksOfChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "limits.jsonl")
	s := &Store{Path: path}
	a := Account{Provider: "anthropic", Account: "a@example.com · Max", State: Measured, At: mon, Windows: []Window{week(10)}}
	old := a
	old.At = mon.Add(-20 * 24 * time.Hour)
	old.Windows = []Window{week(5)}
	for _, x := range []Account{old, a, a} {
		if err := s.Append([]Account{x}, x.At); err != nil {
			t.Fatal(err)
		}
	}
	unmeasured := Account{Provider: "openai", State: Unmeasured}
	if err := s.Append([]Account{unmeasured}, mon); err != nil {
		t.Fatal(err)
	}
	if n := lines(t, path); n != 2 {
		t.Fatalf("%d lines, want the old one and one for the same reading twice", n)
	}
	// The tap stamps every status refresh: the same figures later are not
	// a new sample.
	same := a
	same.At = mon.Add(time.Minute)
	if err := s.Append([]Account{same}, same.At); err != nil {
		t.Fatal(err)
	}
	if n := lines(t, path); n != 2 {
		t.Fatalf("%d lines: the same figures were stored again", n)
	}
	b := a
	b.At = mon.Add(time.Hour)
	b.Windows = []Window{week(12)}
	if err := s.Append([]Account{b}, b.At.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n := lines(t, path); n != 3 {
		t.Fatalf("%d lines after a new reading", n)
	}
	fresh := &Store{Path: path} // a new service learns what is stored
	if err := fresh.Append([]Account{b}, mon.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n := lines(t, path); n != 3 {
		t.Fatalf("%d lines: a new service stored a known reading again", n)
	}
	// A service that runs for weeks prunes each day.
	late := b
	late.At = mon.Add(40 * 24 * time.Hour)
	late.Windows = []Window{week(1)}
	if err := fresh.Append([]Account{late}, late.At); err != nil {
		t.Fatal(err)
	}
	if n := lines(t, path); n != 1 {
		t.Fatalf("%d lines: a long-running service did not prune", n)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode())
	}
}

func lines(t *testing.T, path string) int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	n := 0
	for sc := bufio.NewScanner(f); sc.Scan(); {
		n++
	}
	return n
}

// TestTheLineSaysWhatIsLeftAndWhetherItLasts: the counts first, a reset only
// when it is near, the forecast last; a quiet week is muted, a short one a
// warning; an old reading says its time; no source says its fix.
func TestTheLineSaysWhatIsLeftAndWhetherItLasts(t *testing.T) {
	now := mon.Add(24*time.Hour + 5*time.Hour) // Tue 14:00
	build := func(box, wk float64, at time.Time) Account {
		return Build("anthropic", "", SourceClaude, &Reading{At: at, Windows: []Window{
			{UsedPct: box, Minutes: BoxMinutes, ResetsAt: now.Add(2*time.Hour + 40*time.Minute)},
			{UsedPct: wk, Minutes: WeekMinutes, ResetsAt: mon.Add(7 * 24 * time.Hour)},
		}}, now, quiet)
	}
	for _, c := range []struct {
		name string
		a    Account
		want string
		tone string // of the last part
	}{
		{"a calm week", build(28, 12, now), "4 matches · 9 sparks · lasts the week", Muted},
		{"near the edge", build(85, 82, now), "1 match until 16:40 · 2 sparks until Mon 09:00 · out ~19:00", Warn},
		{"a hot week", build(10, 50, now), "5 matches · 5 sparks · out Wed ~19:00", Warn},
		{"old", build(10, 12, now.Add(-90*time.Minute)), "5 matches · 9 sparks · lasts the week · as of 12:30", Warn},
		{"no source", Build("anthropic", "", SourceClaude, nil, now, quiet), "not measured · open the door “Show your limits”", Warn},
		{"a month", Build("openai", "", SourceCodex, &Reading{At: now, Windows: []Window{{UsedPct: 47, Minutes: 43200, ResetsAt: now.Add(240 * time.Hour)}}}, now, quiet), "53% left this month", ""},
	} {
		ps := Parts(c.a, now)
		if got := Plain(ps); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
		if tone := ps[len(ps)-1].Tone; tone != c.tone {
			t.Errorf("%s: tone %q, want %q", c.name, tone, c.tone)
		}
	}
}
