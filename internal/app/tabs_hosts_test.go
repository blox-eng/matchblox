package app

import (
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// shipOn is a ship numbered n that the session in pane with id drives.
func shipOn(n int, pane, id, kind string) proto.Ship {
	return proto.Ship{Repo: "o/r", Number: n, Title: "ship " + string(rune('0'+n)), Kind: kind, Stage: 1,
		Session: proto.ShipSession{Pane: pane, ID: id}, Merge: []string{"gh", "pr", "merge"}}
}

func shipsOfIn(st *proto.State, ss ...proto.Ship) {
	st.Ships = &proto.Ships{Ships: ss, InFlight: len(ss)}
}

func TestShipsOfEveryHost(t *testing.T) {
	home, peer := hostState(sess(1, "api-auth", "%1", nil)), hostState(sess(2, "train-loop", "%2", nil))
	shipsOfIn(&home, shipOn(1, "%1", "", shipRunning))
	shipsOfIn(&peer, shipOn(2, "%2", "", shipWaits))
	m, _ := twoHosts(t, home, peer)
	hosts := map[int]string{}
	for _, sh := range m.ships() {
		hosts[sh.Number] = sh.Host
	}
	if len(hosts) != 2 || hosts[1] != "" || hosts[2] != "ws-1" {
		t.Fatalf("the ships are %v, want #1 on home and #2 on ws-1", hosts)
	}
	if m.ships()[0].Number != 2 {
		t.Fatalf("the ship that waits for you is not first: %v", m.ships())
	}
	if got := m.tabLabel(tabShips, true); !strings.Contains(ansi.Strip(got), "2") {
		t.Fatalf("the tab counts %q, want both hosts' ships", ansi.Strip(got))
	}
	m.setTab(tabShips)
	frame := frameOf(m)
	if l := lineWith(frame, "#2"); !strings.Contains(l, "ws-1") {
		t.Fatalf("the peer's ship does not name its host: %q", l)
	}
	if l := lineWith(frame, "#1"); !strings.Contains(l, "this machine") {
		t.Fatalf("the home ship does not name its host: %q", l)
	}
}

func TestOneHostShipsAreToday(t *testing.T) {
	st := hostState(sess(1, "api-auth", "%1", nil))
	shipsOfIn(&st, shipOn(1, "%1", "", shipRunning), shipOn(2, "%1", "", shipDone))
	m, _ := loadedWith(t, 132, st)
	if !slices.EqualFunc(m.ships(), st.Ships.Ships, func(a, b proto.Ship) bool { return a.Number == b.Number && a.Host == b.Host }) {
		t.Fatalf("one host's ships changed: %v", m.ships())
	}
}

// TestDriverMatchesWithinItsHost: two hosts whose sessions share a pane
// and a session id; each ship's driver is its own host's.
func TestDriverMatchesWithinItsHost(t *testing.T) {
	home, peer := hostState(sess(1, "home-12", "%12", nil)), hostState(sess(1, "peer-12", "%12", nil))
	home.Sessions[0].SessionID, peer.Sessions[0].SessionID = "same-id", "same-id"
	shipsOfIn(&home, shipOn(1, "%12", "same-id", shipRunning))
	shipsOfIn(&peer, shipOn(2, "%12", "same-id", shipRunning))
	m, _ := twoHosts(t, home, peer)
	for _, sh := range m.ships() {
		d, ok := m.driver(sh)
		if !ok || d.Host != sh.Host {
			t.Fatalf("ship #%d on %q is driven by %s on %q", sh.Number, sh.Host, d.Name, d.Host)
		}
	}
	for _, s := range m.snap.Sessions {
		ss := m.shipsOf(s)
		if len(ss) != 1 || ss[0].Host != s.Host {
			t.Fatalf("%s on %q drives %v", s.Name, s.Host, ss)
		}
	}
}

// selectShip selects the ship numbered n on the Ships tab.
func selectShip(t *testing.T, m Model, n int) Model {
	t.Helper()
	m.setTab(tabShips)
	for i, sh := range m.ships() {
		if sh.Number == n {
			m.shipSel = i
			return m
		}
	}
	t.Fatalf("no ship #%d", n)
	return m
}

// TestPeerRowActsGoToItsHost: a merge on a peer's ship, a worktree's
// removal and a pane's jump go to that row's host.
func TestPeerRowActsGoToItsHost(t *testing.T) {
	home, peer := hostState(sess(1, "home-12", "%12", nil)), hostState(sess(1, "peer-12", "%12", nil))
	shipsOfIn(&home, shipOn(1, "%12", "", shipWaits))
	shipsOfIn(&peer, shipOn(2, "%12", "", shipWaits))
	safe := proto.Worktree{Path: "/src/api/wt/done", Branch: "done", Merged: true, Safe: true, Remove: []string{"git", "worktree", "remove", "/src/api/wt/done"}}
	home.Git = &proto.GitReport{Repos: []gitscan.Repo{{Path: "/src/api", Worktrees: []proto.Worktree{{Path: "/src/api"}, safe}}}}
	peer.Git = &proto.GitReport{Repos: []gitscan.Repo{{Path: "/src/api", Worktrees: []proto.Worktree{{Path: "/src/api"}, safe}}}}
	m, homeConn, wsConn := peered(nil)
	var next tea.Model = m
	next, _ = next.Update(tea.WindowSizeMsg{Width: 132, Height: 30})
	next, _ = next.Update(stateMsg(home))
	next, _ = next.Update(peerMsg{host: "ws-1", conn: wsConn, msg: stateMsg(peer)})
	m = next.(Model)

	got := pressAll(selectShip(t, m, 2), "x", "y")
	if acts := wsConn.acts(); len(acts) != 1 || acts[0].RecID != "ship:o/r#2" {
		t.Fatalf("the peer got %v; flash %q", acts, got.(Model).flash)
	}

	m.setTab(tabGit)
	rows := m.worktreeRows()
	at := slices.IndexFunc(rows, func(r wtRow) bool { return r.host == "ws-1" && !r.main })
	if at < 0 {
		t.Fatalf("no peer worktree row: %v", rows)
	}
	if l := lineWith(frameOf(m), "ws-1"); l == "" {
		t.Fatal("the Git tab does not name the peer")
	}
	m.gitSel = at
	if a := m.secondary(); a == nil || m.hostOf(*a) != "ws-1" {
		t.Fatalf("x on the peer's worktree is %+v", a)
	}
	marked := pressAll(m, "space")
	if mm := marked.(Model); !mm.picked[wtKey{"ws-1", safe.Path}] || mm.picked[wtKey{"", safe.Path}] {
		t.Fatalf("the marks are %v", mm.picked)
	}
	pressAll(marked, "x", "y")
	if acts := wsConn.acts(); len(acts) != 2 || acts[1].RecID != "worktree:"+safe.Path {
		t.Fatalf("the peer got %v", acts)
	}
	if acts := homeConn.acts(); len(acts) != 0 {
		t.Fatalf("home got %v", acts)
	}
	if a := m.primary(); a == nil || !a.remote || a.host != "ws-1" {
		t.Fatalf("Enter on the peer's worktree is %+v", a)
	}

	m.setTab(tabPanes)
	pr := m.paneRows()
	at = slices.IndexFunc(pr, func(r paneRow) bool { return r.host == "ws-1" })
	if at < 0 {
		t.Fatalf("no peer pane row: %v", pr)
	}
	m.paneSel = at
	if l := lineWith(frameOf(m), "peer-12"); !strings.Contains(l, "ws-1") {
		t.Fatalf("the peer's pane does not name its host: %q", l)
	}
	if a := m.primary(); a == nil || !a.remote || a.host != "ws-1" {
		t.Fatalf("Enter on the peer's pane is %+v", a)
	}
}

func TestUsageRowsPerHostNeverMerged(t *testing.T) {
	at := time.Now()
	win := []limits.Window{{UsedPct: 41, Minutes: limits.BoxMinutes, ResetsAt: at.Add(2 * time.Hour)}}
	acct := func(dir string, pct float64) limits.Account {
		w := slices.Clone(win)
		w[0].UsedPct = pct
		return limits.Account{Provider: "anthropic", Account: "you@example.com · Max", Dir: dir, State: limits.Measured, At: at, Windows: w}
	}
	work := "/home/u/.matchblox/accounts/claude/work"
	home, peer := hostState(), hostState()
	home.Limits = []limits.Account{acct(work, 41), acct("/home/u/.matchblox/accounts/claude/side", 10)}
	peer.Limits = []limits.Account{acct(work, 77)}
	m, _ := twoHosts(t, home, peer)
	out := m.usageLines(100, true, "")
	frame := ansi.Strip(strings.Join(out, "\n"))
	var rows []string
	for _, l := range out {
		if l = ansi.Strip(l); strings.Contains(l, "Claude work") {
			rows = append(rows, l)
		}
	}
	if len(rows) != 1 {
		t.Fatalf("work has %d rows, want one with a cell per host:\n%s", len(rows), frame)
	}
	if row := rows[0]; !strings.Contains(row, "this machine 41% · ws-1 77%") || strings.Contains(row, "118") {
		t.Fatalf("the work row is %q, want each host's own percent", row)
	}
	if l := lineWith(out, "Claude side"); !strings.Contains(ansi.Strip(l), "this machine 10%") || strings.Contains(l, "ws-1") {
		t.Fatalf("a label of one host is %q", ansi.Strip(l))
	}
}

// TestUsageCellOfAStaleHost: a stale host's cell says so, not its old
// reading.
func TestUsageCellOfAStaleHost(t *testing.T) {
	m := lostPeer(t, 40*time.Second)
	acct := limits.Account{Provider: "anthropic", Account: "you@example.com · Max", State: limits.Measured, At: lostAt,
		Windows: []limits.Window{{UsedPct: 30, Minutes: limits.BoxMinutes}}}
	m.snap.Limits = []limits.Account{acct}
	m.peers[0].st.Limits = []limits.Account{acct}
	if l := ansi.Strip(lineWith(m.usageLines(100, true, ""), "Claude")); !strings.Contains(l, "this machine 30% · ws-1 stale") {
		t.Fatalf("the row is %q", l)
	}
}

// machineHosts is a console of this machine, gpu-1 and ws-1, each with a
// state; ws-1's comes first, gpu-1 is lost after its state. The clock is
// at lostAt+after.
func machineHosts(t *testing.T, after time.Duration, w, h int) Model {
	t.Helper()
	now := lostAt
	m, _, conns := hostsAt(t, &now, "", w, h, "gpu-1", "ws-1")
	cpu := func(st proto.State, v float64) proto.State {
		st.Machine.CPU = v
		return st
	}
	m = upd(t, m, stateMsg(stateAt(cpu(hostState(sess(1, "api-auth", "%1", nil)), 12), now)))
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(cpu(hostState(sess(3, "docs", "%3", nil)), 55))})
	m = upd(t, m, peerMsg{host: "gpu-1", conn: conns["gpu-1"], msg: stateMsg(cpu(hostState(sess(2, "train", "%2", nil)), 87))})
	m = upd(t, m, peerMsg{host: "gpu-1", conn: conns["gpu-1"], msg: lostMsg{err: io.EOF, conn: conns["gpu-1"]}})
	now = lostAt.Add(after)
	return upd(t, m, stateMsg(stateAt(cpu(hostState(sess(1, "api-auth", "%1", nil)), 12), now)))
}

func TestDockMachineLinePerHost(t *testing.T) {
	m := machineHosts(t, 10*time.Second, 132, 40)
	least, _ := m.panel(panelMachine, 60, "")
	if len(least) != 4 {
		t.Fatalf("the machine panel has %d lines, want its label and one per host:\n%s", len(least), ansi.Strip(strings.Join(least, "\n")))
	}
	for i, want := range [][2]string{{"this machine", "12%"}, {"gpu-1", "87%"}, {"ws-1", "55%"}} {
		l := ansi.Strip(least[i+1])
		if !strings.Contains(l, want[0]) || !strings.Contains(l, want[1]) || strings.Contains(l, "stale") {
			t.Errorf("line %d is %q, want %s at %s", i+1, l, want[0], want[1])
		}
	}
}

func TestStaleMachineLine(t *testing.T) {
	m := machineHosts(t, 40*time.Second, 132, 40)
	least, _ := m.panel(panelMachine, 60, "")
	if l := ansi.Strip(lineWith(least, "gpu-1")); !strings.Contains(l, "stale · as of 14:02") || strings.Contains(l, "87%") {
		t.Fatalf("the stale host's line is %q", l)
	}
	if l := ansi.Strip(lineWith(least, "ws-1")); strings.Contains(l, "stale") {
		t.Fatalf("a live host's line is %q", l)
	}
}

// heads are the lines of a tab's body that head a host's section, in order.
func heads(lines []string) []string {
	var out []string
	for _, l := range lines {
		l = strings.TrimSpace(ansi.Strip(l))
		for _, h := range []string{"this machine", "gpu-1", "ws-1"} {
			if l == h || strings.HasPrefix(l, h+"  ") {
				out = append(out, l)
			}
		}
	}
	return out
}

func TestMachineTabSectionsInHostsOrder(t *testing.T) {
	m := machineHosts(t, 40*time.Second, 104, 38)
	want := []string{"this machine", "gpu-1  stale · as of 14:02", "ws-1"}
	if got := heads(m.machinePanel(104).lines); !slices.Equal(got, want) {
		t.Fatalf("the Machine tab's sections are %q, want %q", got, want)
	}
	if got := heads(m.historyBody(104).lines); !slices.Equal(got, want) {
		t.Fatalf("the History tab's sections are %q, want %q", got, want)
	}
	body := ansi.Strip(strings.Join(m.machinePanel(104).lines, "\n"))
	if strings.Count(body, "MACHINE") != 3 {
		t.Fatalf("the Machine tab has %d machines:\n%s", strings.Count(body, "MACHINE"), body)
	}
}

func TestOneHostMachineTabHasNoSections(t *testing.T) {
	m, _ := loadedWith(t, 104, hostState(sess(1, "api-auth", "%1", nil)))
	if got := heads(m.machinePanel(104).lines); len(got) != 0 {
		t.Fatalf("one host's Machine tab has sections %q", got)
	}
	least, _ := m.panel(panelMachine, 60, "")
	if len(least) != 5 {
		t.Fatalf("one host's dock machine has %d lines", len(least))
	}
}

// TestRunsCountTheirHostsAgents: a host's machine counts its own agents.
func TestRunsCountTheirHostsAgents(t *testing.T) {
	claude := func(s *sample.Session) { s.Agent = "claude" }
	m, _ := twoHosts(t, hostState(sess(1, "a", "%1", claude)), hostState(sess(2, "b", "%2", claude), sess(3, "c", "%3", claude)))
	if got := m.viewOf("ws-1").runs(); got != "2 claude" {
		t.Fatalf("ws-1 runs %q", got)
	}
	if got := m.viewOf("").runs(); got != "1 claude" {
		t.Fatalf("home runs %q", got)
	}
}

// TestShortUsageLineKeepsItsWindow: short of room each line still names
// its window, and every cell shown is whole and of its own host.
func TestShortUsageLineKeepsItsWindow(t *testing.T) {
	acct := func(box, week float64) limits.Account {
		return limits.Account{Provider: "anthropic", Account: "you@example.com · Max", State: limits.Measured, At: time.Now(),
			Windows: []limits.Window{{UsedPct: box, Minutes: limits.BoxMinutes}, {UsedPct: week, Minutes: limits.WeekMinutes}}}
	}
	home, peer := hostState(), hostState()
	home.Limits, peer.Limits = []limits.Account{acct(84, 82)}, []limits.Account{acct(61, 12)}
	m, _ := twoHosts(t, home, peer)
	want := map[string][]string{"5h": {"this machine 84%", "ws-1 61%"}, "wk": {"this machine 82%", "ws-1 12%"}}
	for _, w := range []int{40, 50, 100} {
		lines := m.usageLines(w, false, "")[1:]
		if len(lines) != 2 {
			t.Fatalf("width %d: %d lines:\n%s", w, len(lines), ansi.Strip(strings.Join(lines, "\n")))
		}
		for i, win := range []string{"5h", "wk"} {
			l := strings.TrimSpace(ansi.Strip(lines[i]))
			_, cells, ok := strings.Cut(l, win+"   ")
			if !ok {
				t.Fatalf("width %d: line %q does not name %s", w, l, win)
			}
			cells = strings.TrimSuffix(strings.TrimSpace(cells), " …")
			got := strings.Split(cells, " · ")
			for j, c := range got {
				if j >= len(want[win]) || c != want[win][j] {
					t.Fatalf("width %d, %s: cell %q, want a prefix of %q", w, win, c, want[win])
				}
			}
			if w == 100 && len(got) != 2 {
				t.Fatalf("width 100, %s: cells %q", win, got)
			}
		}
	}
}

// gitHosts is a console of this machine and ws-1, each with a safe
// worktree at the same path, on the Git tab.
func gitHosts(t *testing.T) (Model, *fakeConn, *fakeConn) {
	t.Helper()
	home, peer := hostState(sess(1, "home-12", "%12", nil)), hostState(sess(1, "peer-12", "%12", nil))
	safe := proto.Worktree{Path: "/src/api/wt/done", Branch: "done", Merged: true, Safe: true, Remove: []string{"git", "worktree", "remove", "/src/api/wt/done"}}
	for _, st := range []*proto.State{&home, &peer} {
		st.Git = &proto.GitReport{Repos: []gitscan.Repo{{Path: "/src/api", Worktrees: []proto.Worktree{{Path: "/src/api"}, safe}}}}
	}
	m, homeConn, wsConn := peered(nil)
	m = upd(t, m, tea.WindowSizeMsg{Width: 132, Height: 30})
	m = upd(t, m, stateMsg(home))
	m = upd(t, m, peerMsg{host: "ws-1", conn: wsConn, msg: stateMsg(peer)})
	m.setTab(tabGit)
	return m, homeConn, wsConn
}

// selectWorktree selects the worktree row of host that is not main.
func selectWorktree(t *testing.T, m Model, host string) Model {
	t.Helper()
	at := slices.IndexFunc(m.worktreeRows(), func(r wtRow) bool { return r.host == host && !r.main })
	if at < 0 {
		t.Fatalf("no worktree row on %q", host)
	}
	m.gitSel = at
	return m
}

func TestLostPeerEndsItsBatch(t *testing.T) {
	m, _, wsConn := gitHosts(t)
	m = pressAll(selectWorktree(t, m, "ws-1"), "space", "x", "y").(Model)
	if len(wsConn.acts()) != 1 || m.batch == nil {
		t.Fatalf("the peer got %v, batch %+v", wsConn.acts(), m.batch)
	}
	m = upd(t, m, peerMsg{host: "ws-1", conn: wsConn, msg: lostMsg{err: io.EOF, conn: wsConn}})
	if m.batch != nil || m.flash != "removed 0, failed 1: ws-1: connection lost" {
		t.Fatalf("batch %+v, flash %q", m.batch, m.flash)
	}
}

func TestMixedBatchEndsByResultAndLoss(t *testing.T) {
	m, homeConn, wsConn := gitHosts(t)
	m = pressAll(selectWorktree(t, m, ""), "space").(Model)
	m = pressAll(selectWorktree(t, m, "ws-1"), "space", "x", "y").(Model)
	if len(homeConn.acts()) != 1 || len(wsConn.acts()) != 1 || m.batch == nil || len(m.batch.ids) != 2 {
		t.Fatalf("home got %v, the peer %v, batch %+v", homeConn.acts(), wsConn.acts(), m.batch)
	}
	var homeID string
	for id := range m.batch.ids {
		if !slices.Contains(m.peers[0].acts, id) {
			homeID = id
		}
	}
	m = upd(t, m, resultMsg(proto.Result{ActID: homeID, Ran: [][]string{{"git", "worktree", "remove", "/src/api/wt/done"}}}))
	if m.batch == nil || len(m.batch.ids) != 1 {
		t.Fatalf("one result ended the batch: %+v, flash %q", m.batch, m.flash)
	}
	m = upd(t, m, peerMsg{host: "ws-1", conn: wsConn, msg: lostMsg{err: io.EOF, conn: wsConn}})
	if m.batch != nil || m.flash != "removed 1, failed 1: ws-1: connection lost" {
		t.Fatalf("batch %+v, flash %q", m.batch, m.flash)
	}
}

func TestRescanOnThePeersRow(t *testing.T) {
	m, homeConn, wsConn := gitHosts(t)
	m = pressAll(selectWorktree(t, m, "ws-1"), "r").(Model)
	if m.flash != "scanning git on ws-1…" {
		t.Fatalf("flash %q", m.flash)
	}
	if acts := wsConn.acts(); len(acts) != 1 || acts[0].RecID != "rescan:git" {
		t.Fatalf("the peer got %v", acts)
	}
	if acts := homeConn.acts(); len(acts) != 0 {
		t.Fatalf("home got %v", acts)
	}
}

// TestHomeLossKeepsThePeersPartOfABatch: the home host's loss counts only
// its own removals; the peer's result still ends the batch.
func TestHomeLossKeepsThePeersPartOfABatch(t *testing.T) {
	m, homeConn, wsConn := gitHosts(t)
	m = pressAll(selectWorktree(t, m, ""), "space").(Model)
	m = pressAll(selectWorktree(t, m, "ws-1"), "space", "x", "y").(Model)
	if m.batch == nil || len(m.batch.ids) != 2 {
		t.Fatalf("batch %+v", m.batch)
	}
	m = upd(t, m, lostMsg{err: io.EOF, conn: homeConn})
	if m.batch == nil || len(m.batch.ids) != 1 {
		t.Fatalf("the home host's loss ended the peer's part: batch %+v, flash %q", m.batch, m.flash)
	}
	wsID := m.peers[0].acts[0]
	m = upd(t, m, peerMsg{host: "ws-1", conn: wsConn, msg: resultMsg(proto.Result{ActID: wsID, Ran: [][]string{{"git", "worktree", "remove", "/src/api/wt/done"}}})})
	if want := "removed 1, failed 1: this machine: connection lost"; m.batch != nil || m.flash != want {
		t.Fatalf("batch %+v, flash %q, want %q", m.batch, m.flash, want)
	}
}
