package stoker

import (
	"reflect"
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
	m, changed := m.Observe([]sample.Session{calm("%1"), full("%2")}, nil, 85)
	if !changed {
		t.Fatal("a new session and a full one changed nothing")
	}
	asks := calm("%3")
	asks.Asks = true
	m, _ = m.Observe([]sample.Session{calm("%1"), full("%2"), asks, calm("%4")}, map[string]bool{"%4": true}, 85)
	if _, changed = m.Observe([]sample.Session{calm("%1"), full("%2")}, nil, 85); changed {
		t.Fatal("the same sessions again changed the mode")
	}
	if !reflect.DeepEqual(m.Watched, []string{"s%1", "s%2", "s%3", "s%4"}) ||
		!reflect.DeepEqual(m.Full, []string{"s%2"}) || !reflect.DeepEqual(m.Waited, []string{"s%3", "s%4"}) {
		t.Fatalf("watched %v full %v waited %v", m.Watched, m.Full, m.Waited)
	}
	if _, changed := (Mode{}).Observe([]sample.Session{full("%1")}, nil, 85); changed {
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
			Card{Watched: 3, Ready: 2, Of: 3, NotReady: []Miss{{"work%3", "gone"}}}},
		{"a session that asks is not ready", []sample.Session{calm("%1"), calm("%2"), asks}, nil, nil,
			Card{Watched: 3, Waited: 1, Ready: 2, Of: 3, NotReady: []Miss{{"work%3", "waits for you"}}}},
		{"a session in the queue is not ready", []sample.Session{calm("%1"), calm("%2"), calm("%3")}, map[string]bool{"%1": true}, nil,
			Card{Watched: 3, Waited: 1, Ready: 2, Of: 3, NotReady: []Miss{{"work%1", "waits for you"}}}},
		{"a full session is not ready", []sample.Session{calm("%1"), full("%2"), calm("%3")}, nil, nil,
			Card{Watched: 3, Full: 1, Ready: 2, Of: 3, NotReady: []Miss{{"work%2", "full · 88%"}}}},
		{"compacts read back count, with the tokens they gave back", []sample.Session{calm("%1"), calm("%2"), calm("%3")}, nil,
			[]Entry{compacted, noCounts, unread, skipped},
			Card{Watched: 3, Compacted: 2, Given: 166600, Ready: 3, Of: 3}},
		{"a session that came later is watched, not scored", []sample.Session{calm("%1"), calm("%2"), calm("%3"), full("%9")}, nil, nil,
			Card{Watched: 4, Full: 1, Ready: 3, Of: 3}},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := on().Begin([]sample.Session{calm("%1"), calm("%2"), calm("%3")})
			m, _ = m.Observe(c.end, c.waiting, 85)
			m = m.Off(end).Score(c.end, c.waiting, 85, c.run)
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
	if c := m.Score(nil, nil, 85, nil).Card; c == nil || !c.Night {
		t.Fatalf("card %+v", c)
	}
	if c := on().Off(t0.Add(time.Hour)).Score(nil, nil, 85, nil).Card; c == nil || c.Night {
		t.Fatalf("a run of the day: %+v", c)
	}
}

func TestTurnOnStartsAFreshRun(t *testing.T) {
	m := on().Begin([]sample.Session{full("%1")})
	m, _ = m.Observe([]sample.Session{full("%1")}, nil, 85)
	m = m.Off(t0.Add(time.Hour)).Score(nil, nil, 85, nil)
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
	if got := Plan(m, []sample.Session{full("%1")}, nil, 85, nil, t0); len(got) != 0 {
		t.Fatalf("a resting stoker planned %+v", got)
	}
	if got := Plan(m.Wake(), []sample.Session{full("%1")}, nil, 85, nil, t0); len(got) != 1 {
		t.Fatalf("woken, it planned %+v", got)
	}
}

func TestARestEndsWhenTheBuilderLeaves(t *testing.T) {
	m := on().Rest(t0)
	for _, c := range []struct {
		name     string
		at       time.Time
		consoles int
		wakes    bool
	}{
		{"a console open, 14 min idle", t0.Add(14 * time.Minute), 1, false},
		{"15 min with no key", t0.Add(RestIdle), 1, true},
		{"no console open", t0.Add(time.Minute), 0, true},
	} {
		got, woke := m.Awake(c.at, c.consoles)
		if woke != c.wakes || got.Rests.IsZero() != c.wakes {
			t.Errorf("%s: woke %v, %+v", c.name, woke, got)
		}
	}
	touched := m.Touch(t0.Add(10 * time.Minute))
	if _, woke := touched.Awake(t0.Add(20*time.Minute), 1); woke {
		t.Error("a key 10 min ago: woke after 20")
	}
	if _, woke := on().Awake(t0.Add(time.Hour), 0); woke {
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
	c := m.Off(t0.Add(time.Minute)).Score(end, nil, 85, nil).Card
	if c.Full != 1 || c.Waited != 1 || c.Ready != 1 {
		t.Fatalf("card %+v", c)
	}
}
