package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

var spaceRepos = &proto.GitReport{Repos: []gitscan.Repo{
	{Path: "/dev/matchblox", Worktrees: []gitscan.Worktree{{Path: "/dev/matchblox"}}},
	{Path: "/dev/base", Worktrees: []gitscan.Worktree{{Path: "/dev/base"}}},
	{Path: "/dev/ledge", Worktrees: []gitscan.Worktree{{Path: "/dev/ledge"}}},
}}

func matchIn(name, cwd string) sample.Session {
	s := boardMatch(name, time.Hour)
	s.Cwd = cwd
	return s
}

// spacesDesk is the mockup's desk: this machine holds matchblox (two ask),
// base, ledge (one asks) and a folder outside git; gpu-1 holds one more
// matchblox session and one ledge session, each at its own path there.
func spacesDesk(t *testing.T) Model {
	t.Helper()
	now := boardAt
	m, _, conns := hostsAt(t, &now, "", 160, 40, "gpu-1")
	m.viewPrefs.CopyCheckedDesk, m.viewPrefs.CopyCheckedPhone = true, true
	flint, kindle := matchIn("flint-auth", "/dev/matchblox"), matchIn("kindle-web", "/dev/matchblox/www")
	ember := matchIn("ember-billing", "/dev/ledge")
	home := boardState(flint, kindle, matchIn("spark-rates", "/dev/base"), ember, matchIn("scratch", "/tmp/x"))
	home.Git = spaceRepos
	home.Queue = []queue.Item{waitFor(flint, queue.StateQuestion, time.Minute), waitFor(kindle, queue.StatePermission, time.Minute),
		waitFor(ember, queue.StateQuestion, time.Minute)}
	gpu := boardState(matchIn("hearth-api", "/srv/matchblox"), matchIn("wick-mobile", "/srv/ledge"))
	gpu.Git = &proto.GitReport{Repos: []gitscan.Repo{
		{Path: "/srv/matchblox", Worktrees: []gitscan.Worktree{{Path: "/srv/matchblox"}}},
		{Path: "/srv/ledge", Worktrees: []gitscan.Worktree{{Path: "/srv/ledge"}}},
	}}
	m = upd(t, m, stateMsg(home))
	m = upd(t, m, peerMsg{host: "gpu-1", conn: conns["gpu-1"], msg: stateMsg(gpu)})
	m.toList()
	return m
}

func spaceWords(m Model) string {
	words := []string{"all"}
	for _, sp := range m.spaces() {
		w := sp.name
		if sp.need > 0 {
			w += " ●" + string(rune('0'+sp.need))
		}
		words = append(words, w)
	}
	return strings.Join(words, " · ")
}

func listed(m Model) []string {
	var names []string
	for _, g := range m.sideGroups() {
		for _, u := range g.units {
			for _, s := range u.members {
				names = append(names, s.Name)
			}
		}
	}
	return names
}

func TestSpacesAreTheReposWithLiveSessions(t *testing.T) {
	if got, want := spaceWords(spacesDesk(t)), "all · matchblox ●2 · base · ledge ●1 · x"; got != want {
		t.Errorf("the spaces read %q, want %q", got, want)
	}
}

func TestOneRepoShowsNoSpaces(t *testing.T) {
	st := boardState(matchIn("flint-auth", "/dev/matchblox"), matchIn("kindle-web", "/dev/matchblox/www"))
	st.Git = spaceRepos
	m := boardModel(t, st)
	if sps := m.spaces(); len(sps) != 0 {
		t.Errorf("one repo gives the spaces %v, want none", sps)
	}
	if m = pressOn(m, "]"); m.space != "" {
		t.Errorf("] with one repo opens the space %q, want all", m.space)
	}
}

func TestASpaceNeverHidesAWait(t *testing.T) {
	m := spacesDesk(t)
	m.space = "base"
	if got := listed(m); strings.Join(got, " ") != "spark-rates" {
		t.Errorf("space base lists %v, want spark-rates alone", got)
	}
	if n := m.needCount(); n != 3 {
		t.Errorf("in space base needCount is %d, want 3: a space never hides a wait", n)
	}
}

func TestAnEmptySpaceSaysWhy(t *testing.T) {
	m := spacesDesk(t)
	m.space, m.boardHost = "base", "gpu-1"
	var said []string
	for _, l := range m.sideWords(40) { // the list's width on a 160-cell desk
		said = append(said, strings.TrimSpace(ansi.Strip(l)))
	}
	if got, want := strings.Join(said, " "), "Nothing in base on gpu-1. esc shows all."; got != want {
		t.Errorf("base on gpu-1 with no session says %q, want %q", got, want)
	}
	frame := frameOf(m)
	if lineWith(frame, "Nothing in") == "" || lineWith(frame, "No sessions yet") != "" {
		t.Error("an empty space draws no words in the list, or the chat says no session exists")
	}
	if m = pressOn(m, "esc"); m.space != "" || m.boardHost != "gpu-1" {
		t.Fatalf("esc leaves space %q on host %q, want all spaces on gpu-1", m.space, m.boardHost)
	}
	if m = pressOn(m, "esc"); m.boardHost != "" {
		t.Errorf("a second esc leaves host %q, want all hosts", m.boardHost)
	}
}

func TestBracketsTurnTheSpaces(t *testing.T) {
	m := spacesDesk(t)
	if got := pressOn(m, "]").space; got != "matchblox" {
		t.Errorf("] from all goes to %q, want matchblox", got)
	}
	if got := pressOn(m, "[").space; got != "x" {
		t.Errorf("[ from all goes to %q, want x, the last", got)
	}
	if got := pressOn(m, "[", "]").space; got != "" {
		t.Errorf("] from the last goes to %q, want all", got)
	}
}

func TestBracketsTurnOnlyTheSpacesOfTheHostShown(t *testing.T) {
	m := spacesDesk(t)
	m.boardHost = "gpu-1"
	for _, want := range []string{"matchblox", "ledge", ""} {
		if m = pressOn(m, "]"); m.space != want {
			t.Fatalf("] on gpu-1 goes to %q, want %q: gpu-1 holds matchblox and ledge", m.space, want)
		}
	}
	if m = pressOn(m, "["); m.space != "ledge" {
		t.Fatalf("[ from all on gpu-1 goes to %q, want ledge", m.space)
	}
	if m = pressOn(m, "esc"); m.space != "" || m.boardHost != "gpu-1" {
		t.Fatalf("esc leaves space %q on host %q, want all spaces on gpu-1", m.space, m.boardHost)
	}
	if m = pressOn(m, "esc"); m.boardHost != "" {
		t.Errorf("a second esc leaves host %q, want all hosts", m.boardHost)
	}
}

// TestASpaceKeepsTheChainWhole: the owner's ruling, don't break the
// chains. A chain with a member in the space shows every member, one in
// another repo and one that waits for recover too.
func TestASpaceKeepsTheChainWhole(t *testing.T) {
	st := loginState(func(st *proto.State, ss map[string]*sample.Session) {
		ss["kindle-web"].Cwd, ss["flint-auth"].Cwd, ss["wick-mobile"].Cwd = "/dev/matchblox", "/dev/base", "/dev/matchblox"
	})
	st.Git = spaceRepos
	st.Sessions = slices.DeleteFunc(st.Sessions, func(s sample.Session) bool {
		if s.Name != "wick-mobile" {
			return false
		}
		st.Chains.Mates = append(st.Chains.Mates, state.MateView{Member: memberOf(s), Wait: &state.WaitView{Name: s.Name, Agent: "claude"}})
		return true
	})
	st.Sessions = append(st.Sessions, matchIn("ember-billing", "/dev/ledge"))
	m := boardModel(t, st)
	shown := func() []string {
		var names []string
		for _, g := range m.sideGroups() {
			for _, u := range g.units {
				names = append(names, unitNames(u)...)
				for _, v := range u.waiting {
					names = append(names, v.Wait.Name)
				}
			}
		}
		slices.Sort(names)
		return names
	}
	m.space = "matchblox"
	if got := shown(); !slices.Equal(got, []string{"flint-auth", "kindle-web", "wick-mobile"}) {
		t.Errorf("space matchblox shows %v, want the whole chain: flint-auth, kindle-web, wick-mobile", got)
	}
	m.space = "ledge"
	if got := shown(); !slices.Equal(got, []string{"ember-billing"}) {
		t.Errorf("space ledge shows %v, want ember-billing alone: no member of the chain works there", got)
	}
}

func TestATapOnTheCountShowsEveryRepo(t *testing.T) {
	m := spacesDesk(t)
	m.space = "base"
	next, cmd := tap(m, m.draw(160).waits.x0, 0)
	drain(cmd)
	m = next.(Model)
	if m.space != "" {
		t.Fatalf("a tap on the count leaves space %q open, want all", m.space)
	}
	got := listed(m)
	for _, want := range []string{"flint-auth", "kindle-web", "ember-billing"} {
		if !slices.Contains(got, want) {
			t.Errorf("after a tap on the count the list %v lacks %s, which waits", got, want)
		}
	}
}

func TestTheHeaderNamesEveryWaitWhateverTheSpaceOrHost(t *testing.T) {
	for _, c := range []struct{ space, host string }{{"", ""}, {"base", ""}, {"", "gpu-1"}, {"base", "gpu-1"}} {
		m := pressOn(spacesDesk(t), "ctrl+l")
		if m.listShown() {
			t.Fatal("ctrl+l left the list on the screen")
		}
		m.space, m.boardHost = c.space, c.host
		names := m.headNames()
		for _, want := range []string{"flint-auth", "kindle-web", "ember-billing"} {
			if !slices.ContainsFunc(names, func(n string) bool { return strings.HasPrefix(n, want) }) {
				t.Errorf("space %q, host %q: the header names %v, want %s among them: the count says 3", c.space, c.host, names, want)
			}
		}
	}
}
