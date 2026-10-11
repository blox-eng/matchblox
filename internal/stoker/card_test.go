package stoker

import (
	"fmt"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/sample"
)

func calm(pane string) sample.Session {
	s := full(pane)
	s.ContextPct, s.Tokens = 40, 80000
	return s
}

func TestBeginSeatsTheSessionsAtWork(t *testing.T) {
	cold := calm("%3")
	cold.Idle = sample.ColdAfter
	anon := calm("%4")
	anon.SessionID = ""
	codex := calm("%5")
	codex.Agent = "codex"
	m := on().Begin([]sample.Session{calm("%1"), full("%2"), cold, anon, codex})
	want := []Seat{{"s%1", "work%1"}, {"s%2", "work%2"}, {"s%5", "work%5"}}
	if !reflect.DeepEqual(m.Start, want) {
		t.Fatalf("start %+v", m.Start)
	}
	if !reflect.DeepEqual(m.Watched, []string{"s%1", "s%2", "s%5"}) {
		t.Fatalf("watched %+v", m.Watched)
	}
}

func TestObserveCountsEachSessionOnce(t *testing.T) {
	m := on().Begin([]sample.Session{calm("%1")})
	m, changed := m.Observe([]sample.Session{calm("%1"), full("%2")}, nil, at85, t0)
	if !changed {
		t.Fatal("a new session and a full one changed nothing")
	}
	asks := calm("%3")
	asks.Asks = true
	m, _ = m.Observe([]sample.Session{calm("%1"), full("%2"), asks, calm("%4")}, map[string]bool{"%4": true}, at85, t0)
	if _, changed = m.Observe([]sample.Session{calm("%1"), full("%2")}, nil, at85, t0); changed {
		t.Fatal("the same sessions again changed the mode")
	}
	if !reflect.DeepEqual(m.Watched, []string{"s%1", "s%2", "s%3", "s%4"}) ||
		!reflect.DeepEqual(m.Full, []string{"s%2"}) || !reflect.DeepEqual(m.Waited, []string{"s%3", "s%4"}) {
		t.Fatalf("watched %v full %v waited %v", m.Watched, m.Full, m.Waited)
	}
	if _, changed := (Mode{}).Observe([]sample.Session{full("%1")}, nil, at85, t0); changed {
		t.Fatal("a stoker that is off observed")
	}
}

func TestScore(t *testing.T) {
	end := t0.Add(8 * time.Hour)
	compacted := Entry{ID: "1", At: t0.Add(time.Hour), Session: "s%2", Result: Sent, Before: 176000, After: 9400, Back: t0.Add(time.Hour + time.Minute)}
	noCounts := Entry{ID: "2", At: t0.Add(2 * time.Hour), Session: "s%2", Result: Sent, Before: 170000, Back: t0.Add(2*time.Hour + time.Minute)}
	unread := Entry{ID: "3", At: t0.Add(3 * time.Hour), Session: "s%1", Result: Sent, Before: 170000}
	skipped := Entry{ID: "4", At: t0.Add(4 * time.Hour), Session: "s%1", Result: Skipped + ": busy", Before: 170000}
	asks := calm("%3")
	asks.Asks = true
	gone := sample.Session{}

	for _, c := range []struct {
		name    string
		end     []sample.Session
		waiting map[string]bool
		run     []Entry
		want    Card
	}{
		{"a quiet night: all ready, nothing done", []sample.Session{calm("%1"), calm("%2"), calm("%3")}, nil, nil,
			Card{Watched: 3, Ready: 3, Of: 3}},
		{"a gone session is not ready", []sample.Session{calm("%1"), calm("%2"), gone}, nil, nil,
			Card{Watched: 3, Ready: 2, Of: 3, NotReady: []Miss{{Name: "work%3", Why: "gone"}}}},
		{"a session that asks is not ready", []sample.Session{calm("%1"), calm("%2"), asks}, nil, nil,
			Card{Watched: 3, Waited: 1, Ready: 2, Of: 3, NotReady: []Miss{{Name: "work%3", Why: "waits for you"}}}},
		{"a session in the queue is not ready", []sample.Session{calm("%1"), calm("%2"), calm("%3")}, map[string]bool{"%1": true}, nil,
			Card{Watched: 3, Waited: 1, Ready: 2, Of: 3, NotReady: []Miss{{Name: "work%1", Why: "waits for you"}}}},
		{"a full session is not ready", []sample.Session{calm("%1"), full("%2"), calm("%3")}, nil, nil,
			Card{Watched: 3, Full: 1, Ready: 2, Of: 3, NotReady: []Miss{{Name: "work%2", Why: "full · 176k"}}}},
		{"compacts read back count, with the tokens they gave back", []sample.Session{calm("%1"), calm("%2"), calm("%3")}, nil,
			[]Entry{compacted, noCounts, unread, skipped},
			Card{Watched: 3, Compacted: 2, Given: 166600, Ready: 3, Of: 3}},
		{"a session that came later is watched, not scored", []sample.Session{calm("%1"), calm("%2"), calm("%3"), full("%9")}, nil, nil,
			Card{Watched: 4, Full: 1, Ready: 3, Of: 3}},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := on().Begin([]sample.Session{calm("%1"), calm("%2"), calm("%3")})
			m, _ = m.Observe(c.end, c.waiting, at85, t0)
			m = m.Off(end).Score(c.end, c.waiting, at85, c.run)
			want := c.want
			want.Since, want.Ended = t0, end
			if m.Card == nil || !reflect.DeepEqual(*m.Card, want) {
				t.Fatalf("card %+v\nwant %+v", m.Card, want)
			}
		})
	}
}

func TestANightCardSaysItWasANight(t *testing.T) {
	m, err := on().Night(t0, "07:00")
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.Expire(m.Until)
	if c := m.Score(nil, nil, at85, nil).Card; c == nil || !c.Night {
		t.Fatalf("card %+v", c)
	}
	if c := on().Off(t0.Add(time.Hour)).Score(nil, nil, at85, nil).Card; c == nil || c.Night {
		t.Fatalf("a run of the day: %+v", c)
	}
}

func TestTurnOnStartsAFreshRun(t *testing.T) {
	m := on().Begin([]sample.Session{full("%1")})
	m, _ = m.Observe([]sample.Session{full("%1")}, nil, at85, t0)
	m = m.Off(t0.Add(time.Hour)).Score(nil, nil, at85, nil)
	m.Asked = true
	n := m.TurnOn(t0.Add(2 * time.Hour))
	if n.Card != nil || n.Start != nil || n.Watched != nil || n.Full != nil || n.Asked {
		t.Fatalf("the new run kept the old one: %+v", n)
	}
}

func TestARestingStokerPlansNothing(t *testing.T) {
	m := on().Rest(t0)
	if !m.Asked || m.Rests.IsZero() {
		t.Fatalf("rest %+v", m)
	}
	if got := Plan(m, []sample.Session{full("%1")}, nil, at85, nil, t0); len(got) != 0 {
		t.Fatalf("a resting stoker planned %+v", got)
	}
	if got := Plan(m.Wake(), []sample.Session{full("%1")}, nil, at85, nil, t0); len(got) != 1 {
		t.Fatalf("woken, it planned %+v", got)
	}
}

func TestARestEndsWhenTheBuilderLeaves(t *testing.T) {
	m := on().Rest(t0)
	for _, c := range []struct {
		name  string
		at    time.Time
		alone time.Time // zero: a console is open
		wakes bool
	}{
		{"a console open, 14 min idle", t0.Add(14 * time.Minute), time.Time{}, false},
		{"15 min with no key", t0.Add(RestIdle), time.Time{}, true},
		{"the last console closed a moment ago", t0.Add(time.Minute), t0.Add(30 * time.Second), false},
		{"the last console closed minutes ago", t0.Add(5 * time.Minute), t0.Add(time.Minute), true},
	} {
		got, woke := m.Awake(c.at, c.alone)
		if woke != c.wakes || got.Rests.IsZero() != c.wakes {
			t.Errorf("%s: woke %v, %+v", c.name, woke, got)
		}
	}
	touched := m.Touch(t0.Add(10 * time.Minute))
	if _, woke := touched.Awake(t0.Add(20*time.Minute), time.Time{}); woke {
		t.Error("a key 10 min ago: woke after 20")
	}
	if _, woke := on().Awake(t0.Add(time.Hour), t0); woke {
		t.Error("a stoker that does not rest woke")
	}
	if off := m.Off(t0.Add(time.Minute)); !off.Rests.IsZero() {
		t.Error("off kept the rest")
	}
}

// A run that ends before any sample still counts what its end shows: the
// card never names a full session under "0 were full".
func TestScoreCountsTheEnd(t *testing.T) {
	asks := calm("%3")
	asks.Asks = true
	end := []sample.Session{calm("%1"), full("%2"), asks}
	m := on().Begin([]sample.Session{calm("%1"), calm("%2"), calm("%3")})
	c := m.Off(t0.Add(time.Minute)).Score(end, nil, at85, nil).Card
	if c.Full != 1 || c.Waited != 1 || c.Ready != 1 {
		t.Fatalf("card %+v", c)
	}
}

// Two writers at once: each change lands, none undoes another.
func TestUpdateNeverLosesAWrite(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Set(on()); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Go(func() {
			if _, err := st.Update(func(m Mode) (Mode, bool) {
				m.Watched = append(slices.Clip(m.Watched), fmt.Sprint(i))
				return m, true
			}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if n := len(st.Mode().Watched); n != 40 {
		t.Fatalf("%d of 40 writes kept", n)
	}
}

// A run the previous build started has no start: the first sample seats it.
func TestObserveSeatsARunWithoutAStart(t *testing.T) {
	m, changed := on().Observe([]sample.Session{calm("%1")}, nil, at85, t0)
	if !changed || len(m.Start) != 1 {
		t.Fatalf("start %+v", m.Start)
	}
}

// A session the plan's limit stops is not ready, and says until when; a
// limit that ran out during the run is on the card with when it comes back.
func TestTheCardCountsTheLimit(t *testing.T) {
	back := t0.Add(3 * time.Hour)
	out := func(s sample.Session) sample.Session { s.Out = back; return s }
	m := on().Begin([]sample.Session{calm("%1"), calm("%2")})
	m, changed := m.Observe([]sample.Session{out(calm("%1")), out(calm("%2"))}, nil, at85, t0.Add(41*time.Minute))
	if !changed {
		t.Fatal("a limit running out changed nothing")
	}
	// Later samples of the same lapse keep its start.
	m, _ = m.Observe([]sample.Session{out(calm("%1")), out(calm("%2"))}, nil, at85, t0.Add(2*time.Hour))
	end := t0.Add(2 * time.Hour)
	m = m.Off(end).Score([]sample.Session{out(calm("%1")), out(calm("%2"))}, nil, at85, nil)
	c := m.Card
	if c.Ready != 0 || c.Of != 2 {
		t.Fatalf("ready %d/%d", c.Ready, c.Of)
	}
	why := "out until " + back.Format("15:04")
	if !reflect.DeepEqual(c.NotReady, []Miss{{Name: "work%1", Why: why, Out: back}, {Name: "work%2", Why: why, Out: back}}) {
		t.Fatalf("not ready %+v", c.NotReady)
	}
	if !reflect.DeepEqual(c.Lapses, []Lapse{{From: t0.Add(41 * time.Minute), Until: back}}) {
		t.Fatalf("lapses %+v", c.Lapses)
	}
}

// The token count fills a 1M window long before its percentage.
func TestAFullSessionByTokens(t *testing.T) {
	big := calm("%1")
	big.Tokens, big.ContextPct = 445000, 44.5
	f := sample.Full{Pct: 85, Tokens: 350_000}
	m := on().Begin([]sample.Session{big})
	m, _ = m.Observe([]sample.Session{big}, nil, f, t0)
	m = m.Off(t0.Add(time.Hour)).Score([]sample.Session{big}, nil, f, nil)
	if m.Card.Full != 1 || !reflect.DeepEqual(m.Card.NotReady, []Miss{{Name: "work%1", Why: "full · 445k"}}) {
		t.Fatalf("card %+v", m.Card)
	}
}

// A session out and full needs both after its reset; one that waits says
// that first; a busy one works, whatever the tap says.
func TestWhyASessionIsNotReady(t *testing.T) {
	back := t0.Add(3 * time.Hour)
	outFull := full("%1")
	outFull.Out = back
	waitsOut := calm("%2")
	waitsOut.Asks, waitsOut.Out = true, back
	busyOut := calm("%3")
	busyOut.Busy, busyOut.Status, busyOut.Out = true, "busy", back
	ss := []sample.Session{outFull, waitsOut, busyOut}
	m := on().Begin(ss)
	m = m.Off(t0.Add(time.Hour)).Score(ss, nil, at85, nil)
	want := []Miss{
		{Name: "work%1", Why: "out until " + back.Format("15:04") + " · full · 176k", Out: back},
		{Name: "work%2", Why: "waits for you"},
	}
	if !reflect.DeepEqual(m.Card.NotReady, want) || m.Card.Ready != 1 {
		t.Fatalf("not ready %+v ready %d", m.Card.NotReady, m.Card.Ready)
	}
}

// A sample after the night's end (a host that slept past it) adds nothing
// to the night.
func TestASampleAfterTheNightIsNotTheNights(t *testing.T) {
	m, _ := on().Night(t0, "07:00")
	m = m.Begin([]sample.Session{calm("%1")})
	late := full("%1")
	late.Out = m.Until.Add(5 * time.Hour)
	if m2, changed := m.Observe([]sample.Session{late}, nil, at85, m.Until.Add(2*time.Hour)); changed || len(m2.Full) != 0 || len(m2.Lapses) != 0 {
		t.Fatalf("a late sample changed the night: %+v", m2)
	}
}

// Two readings of one reset a second apart are one lapse.
func TestOneResetIsOneLapse(t *testing.T) {
	back := t0.Add(3 * time.Hour)
	a, b := calm("%1"), calm("%2")
	a.Out, b.Out = back, back.Add(time.Second)
	m := on().Begin([]sample.Session{a, b})
	m, _ = m.Observe([]sample.Session{a, b}, nil, at85, t0)
	if len(m.Lapses) != 1 {
		t.Fatalf("lapses %+v", m.Lapses)
	}
}

// A stale session the limit stops is still stale: the run does not seat it.
func TestAStaleOutSessionIsNotSeated(t *testing.T) {
	s := calm("%1")
	s.Idle, s.Out = sample.StaleAfter, t0.Add(time.Hour)
	if m := on().Begin([]sample.Session{s}); len(m.Start) != 0 {
		t.Fatalf("seated %+v", m.Start)
	}
}
