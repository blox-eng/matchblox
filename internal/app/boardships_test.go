package app

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// The board's tests of the ships (Task 12 step 0c).

// trackOf is line 3 of the card of the match named name, plain.
func trackOf(t *testing.T, m Model, name string) string {
	t.Helper()
	s := matchOf(t, m, name)
	lines, _ := m.boardCard(s, 60, cardOpts{})
	if len(lines) < 3 {
		t.Fatalf("%s's card has %d lines", name, len(lines))
	}
	return strings.TrimSpace(plainLines(lines)[2])
}

// TestACardCountsItsOtherShips: a card leads with the ship that most
// needs you (stuck, then waits, then in flight) and counts the others: by
// what they need, else by the furthest stage, an unknown stage too; ships
// of one rank keep the report's order, and a ship of an earlier agent in
// the same pane is not the match's.
func TestACardCountsItsOtherShips(t *testing.T) {
	two := twoShipsState(t)
	m := boardShot(t, two, 200, 40, nil)
	if got := trackOf(t, m, "hearth-api"); !strings.HasPrefix(got, "#4551 ●━◉┄○┄○┄○ +1 merged") {
		t.Errorf("hearth-api's card: %q", got)
	}
	if got := trackOf(t, m, "spark-docs"); !strings.HasPrefix(got, "#4547 ●━◉┄○┄○┄○ +1 live") {
		t.Errorf("spark-docs's card: %q", got)
	}
	for _, c := range []struct {
		name string
		more []proto.Ship
		want string
	}{
		{"stuck leads", []proto.Ship{{Number: 4561, Title: "Retry the webhook", Kind: shipStuck, Note: "CI failed · lint", Stage: 1}},
			"#4561 ●━✕┄○┄○┄○ +1 ci"},
		{"an unknown stage", []proto.Ship{{Number: 219, Title: "Future", Kind: shipRunning, Stage: 7}}, "#4551 ●━◉┄○┄○┄○ +1"},
		{"the need first", []proto.Ship{{Number: 4561, Title: "Retry the webhook", Kind: shipStuck, Note: "CI failed · lint", Stage: 1},
			{Number: 4562, Title: "Docs", Kind: shipWaits, Note: "green · waits for your merge", Stage: 2}}, "#4561 ●━✕┄○┄○┄○ +2 waits"},
	} {
		m := boardShot(t, withShips(screensState(t), "hearth-api", c.more...), 200, 40, nil)
		if got := trackOf(t, m, "hearth-api"); !strings.HasPrefix(got, c.want) {
			t.Errorf("%s: hearth-api's card %q, want %q", c.name, got, c.want)
		}
	}

	m = boardShot(t, withShips(screensState(t), "spark-docs",
		proto.Ship{Number: 4550, Title: "First", Kind: shipRunning, Note: "CI", Stage: 1},
		proto.Ship{Number: 4549, Title: "Second", Kind: shipRunning, Note: "green", Stage: 2}), 200, 40, nil)
	if ss := m.shipsOf(matchOf(t, m, "spark-docs")); len(ss) < 2 || ss[0].Number != 4550 {
		t.Errorf("spark-docs's ships of one rank: %+v, want #4550 first, as the report", ss)
	}

	st := screensState(t)
	docs := matchOf(t, boardShot(t, st, 120, 40, nil), "spark-docs")
	r := *st.Ships
	r.Ships = append(slices.Clone(r.Ships), proto.Ship{Repo: "hearth/spark-docs", Number: 4400, Title: "Old", Kind: shipDone, Note: "live", Stage: 4,
		Session: proto.ShipSession{Pane: docs.Pane, ID: "an-earlier-agent"}})
	st.Ships = &r
	if got := trackOf(t, boardShot(t, st, 200, 40, nil), "spark-docs"); strings.Contains(got, "+1") {
		t.Errorf("spark-docs counts the earlier agent's ship: %q", got)
	}
}

// demoBoard is the board of st at w by 40, on the demo's clock.
func demoBoard(t *testing.T, w int, st proto.State) Model {
	t.Helper()
	var next tea.Model = New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }})
	next, _ = next.Update(tea.WindowSizeMsg{Width: w, Height: 40})
	next, _ = next.Update(stateMsg(st))
	return boardOn(next.(Model))
}

// TestTheStripSaysWhyShipsAreBehind: the strip's row says plainly why the
// ships may be behind: GitHub's rate limit, gh that cannot read, and a
// repository whose poll failed.
func TestTheStripSaysWhyShipsAreBehind(t *testing.T) {
	st := screensState(t)
	r := *st.Ships
	r.Paused, r.Unknown, r.Errors = time.Date(2026, 10, 1, 23, 33, 0, 0, time.Local), "gh not signed in", []string{"hearth/hearth-api: HTTP 502"}
	st.Ships = &r
	for _, w := range []int{80, 132} { // a notice keeps three lines: at 44 the last reason goes
		m := demoBoard(t, w, st)
		out := strings.Join(strings.Fields(plainFrame(m)), " ")
		for _, want := range []string{"ships read again at 23:33", "GitHub cannot be read: gh not signed in", "not read: hearth/hearth-api: HTTP 502"} {
			if !strings.Contains(out, want) {
				t.Errorf("width %d: the strip does not say %q:\n%s", w, want, plainFrame(m))
			}
		}
	}
	if out := plainFrame(demoBoard(t, 132, screensState(t))); strings.Contains(out, "read again") || strings.Contains(out, "cannot be read") {
		t.Errorf("the strip says the ships are behind when they are not:\n%s", out)
	}
}

// TestAShipWithNoSessionIsACard (ruling 31, #80): a ship no session owns
// is a card in SHIPPING, named by its branch, never "outside tmux" or
// blank; ⏎ on it opens New session in the ship's checkout, the folder
// picked, its first message naming the ship, and sends nothing. With no
// checkout of it here, ⏎ says so. Its card takes no key that needs a pane.
func TestAShipWithNoSessionIsACard(t *testing.T) {
	m0, _ := loadedWith(t, 100, fixtureState())
	if got := m0.driverName(proto.Ship{Number: 7}); got != "no session" {
		t.Errorf("a ship with no session is by %q", got)
	}
	if got := m0.driverGone(proto.Ship{Number: 7}); got != "no session owns #7" {
		t.Errorf("%q", got)
	}

	orphan := func(where string) Model {
		st := screensState(t)
		st.Agents = []string{"claude"}
		r := *st.Ships
		r.Ships = append(slices.Clone(r.Ships), proto.Ship{Repo: "hearth/hearth-api", Number: 4570, Title: "Cache the session store",
			Kind: shipRunning, Note: "CI · 3 of 9 jobs", Stage: 1, Where: where, Since: demoAt.Add(-time.Hour),
			URL: "https://github.com/hearth/hearth-api/pull/4570"})
		st.Ships = &r
		st.Git = &proto.GitReport{Repos: []gitscan.Repo{{Path: "/w/hearth-api", Worktrees: []gitscan.Worktree{
			{Path: "/w/hearth-api", Branch: "main"}, {Path: "/w/hearth-api-cache", Branch: "cache-store"}}}}}
		return boardShot(t, st, 120, 40, nil)
	}
	m := orphan("/w/hearth-api-cache")
	b, _ := m.boardDrawOf()
	i := slices.IndexFunc(b.idx[colShip], func(s sample.Session) bool { return s.Name == "cache-store" })
	if i < 0 {
		t.Fatalf("no card of the ship nobody owns in SHIPPING:\n%s", plainFrame(m))
	}
	card := b.idx[colShip][i]
	if word, says := m.cardSays(card); strings.Contains(says, "outside tmux") || word == "" || says == "" {
		t.Errorf("the ship's card says %q %q", word, says)
	}
	if got := trackOf(t, m, "cache-store"); !strings.HasPrefix(got, "#4570 ") {
		t.Errorf("the ship's card's track is %q", got)
	}
	m.boardChooseCard(colShip, card)
	f := boardFake(m)
	for _, k := range []string{"x", "z", "r", "A", "y"} {
		if nm := pressOn(m, k); nm.pending != nil || nm.form != nil || nm.held != nil {
			t.Errorf("%s on the ship's card opened %+v %+v", k, nm.pending, nm.form)
		}
	}
	nm := pressOn(m, "enter")
	n := strconv.Itoa(4570)
	switch {
	case nm.form == nil:
		t.Fatalf("⏎ did not open New session: %q", nm.flash)
	case nm.whereSelectedIn(nm.form).path() != "/w/hearth-api-cache":
		t.Errorf("New session opens in %q", nm.whereSelectedIn(nm.form).path())
	case nm.form.at != fieldAgent || !nm.form.picked:
		t.Errorf("the folder is not picked with the cursor on agent: at %v", nm.form.at)
	case !strings.Contains(nm.form.first, "#"+n):
		t.Errorf("the first message %q does not name #%s", nm.form.first, n)
	}
	if acts := f.acts(); len(acts) != 0 {
		t.Errorf("the ship's card sent %+v", acts)
	}

	m = orphan("")
	b, _ = m.boardDrawOf()
	i = slices.IndexFunc(b.idx[colShip], func(s sample.Session) bool { return strings.Contains(s.Name, "#4570") })
	if i < 0 {
		t.Fatalf("no card of the ship with no checkout here:\n%s", plainFrame(m))
	}
	m.boardChooseCard(colShip, b.idx[colShip][i])
	if nm := pressOn(m, "enter"); nm.form != nil || !strings.Contains(plainFrame(nm), "no checkout of") {
		t.Errorf("⏎ with no checkout here: form %v, %q", nm.form != nil, noticeRow(nm))
	}
}
