package app

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/askpass"
	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/procfs"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

var update = flag.Bool("update", false, "TestScreens: write the golden screens in testdata/screens")

// demoAt is the demo machine's clock, as the replay starts it.
var demoAt = time.Date(2026, 10, 1, 22, 50, 0, 0, time.UTC)

// screensState is the demo machine on a still clock: kindle-web at a
// permission prompt, flint-auth asking with the question tool, and a ship
// of each kind.
func screensState(t *testing.T) proto.State {
	t.Helper()
	cfg := config.Default()
	s := &sample.Sampler{
		Cfg: cfg, Agents: []string{"claude", "codex", "opencode"},
		FS: procfs.FS{Root: filepath.Join(demo, "proc"), PageSize: procfs.FixturePage}, Sys: procfs.Sys{Root: filepath.Join(demo, "sys")},
		Home: filepath.Join(demo, "home"), HomeAs: "/home/user",
		Tmux:      func() ([]byte, error) { return os.ReadFile(filepath.Join(demo, "tmux-panes.txt")) },
		Capture:   sample.CaptureDir(filepath.Join(demo, "panes")),
		LimitsDir: filepath.Join(demo, "state"),
		Disks:     sample.DiskFile(filepath.Join(demo, "disks.txt")),
		Rules:     sample.Rules{CompactAt: cfg.Sessions.CompactAt, CompactTokens: cfg.Sessions.CompactTokens, Windows: cfg.Sessions.Windows},
		Now:       func() time.Time { return demoAt },
	}
	s.Sample()
	snap := s.Sample()
	q := queue.New()
	ref := map[string]string{}
	for i, x := range snap.Sessions {
		ref[x.Name] = x.Pane
		if x.Name == "flint-auth" {
			x.Busy, x.Status, x.Idle = false, "idle", 2*time.Minute
			x.Said = "The refresh endpoint is in and its tests pass. One choice is yours before I wire the rotation: what should a refresh do with the token it was given?"
			x.Ask = &said.Ask{ID: "toolu_demo", Questions: []said.Question{
				{Question: "Should a refresh also rotate the token?", Header: "Rotate", Options: []said.Option{
					{Label: "Rotate it", Description: "the old one stops at once"},
					{Label: "Rotate, keep the old one a minute"},
					{Label: "Keep the token"}}},
				{Question: "Which checks run before the PR?", Header: "Checks", Multi: true, Options: []said.Option{
					{Label: "unit"}, {Label: "e2e"}, {Label: "lint"}}},
			}}
			snap.Sessions[i] = x
		}
	}
	q.Apply(hooks.Event{Name: "Notification", Kind: hooks.KindPermission, SessionID: "5ea1c0de-0000-4000-8000-000000000004",
		Pane: "%5", Message: "Claude needs your permission to use Bash", At: demoAt.Add(-90 * time.Second)})
	q.Merge(snap.Sessions, snap.At)
	ship := func(n int, name, title, kind, note string, stage int, age time.Duration) proto.Ship {
		sh := proto.Ship{Repo: "hearth/" + name, Number: n, Title: title, URL: "https://github.com/hearth/" + name + "/pull/" + strconv.Itoa(n),
			Session: proto.ShipSession{Pane: ref[name], Name: name}, Stage: stage, Kind: kind, Note: note, Since: demoAt.Add(-age)}
		for i := 0; i <= stage; i++ {
			sh.Stages[i].At = demoAt.Add(-age - time.Duration(stage-i)*25*time.Minute)
		}
		sh.Stages[stage].Note = note
		return sh
	}
	waits := ship(4552, "ember-billing", "Invoice export as CSV and PDF", shipWaits, "green · waits for your merge", 2, 6*time.Minute)
	waits.Merge = []string{"gh", "pr", "merge", "4552", "--squash", "-R", "hearth/ember-billing"}
	ships := &proto.Ships{At: demoAt, InFlight: 3, Ships: []proto.Ship{
		ship(4551, "hearth-api", "Rate limits on the public API", shipRunning, "CI · 6 of 9 jobs", 1, 4*time.Minute),
		waits,
		ship(4545, "flint-auth", "Refresh tokens that rotate", shipStuck, "conflicts with main", 1, 40*time.Minute),
		ship(4540, "spark-docs", "The 1.0 docs", shipDone, "live", 4, 2*time.Hour),
	}}
	return proto.State{Doc: state.Doc{Snapshot: snap, Recommendations: advice.Build(snap, nil), Queue: q.Items(), Ships: ships}}
}

// TestScreens holds the console on the demo machine at the three sizes
// of design/0006 §10 against the frames in testdata/screens: Sessions with
// the one that waits opened, the answer peek with its options, Ships with
// a ship of each kind, Sessions where two sessions drive two ships each,
// and the dock folded on a short phone. -update
// writes them again.
func TestScreens(t *testing.T) {
	// Times on the screen are local: the frames are drawn in UTC, which
	// TestMain sets for the whole package (writing time.Local here races
	// the timers other tests leave running).
	st := screensState(t)
	type shot struct {
		name string
		keys []string
	}
	shots := []shot{
		{"sessions", nil},
		{"peek", []string{"a"}},
		{"ships", []string{"2"}},
	}
	sizes := [][2]int{{44, 31}, {86, 30}, {107, 60}, {132, 32}}
	for _, size := range sizes {
		for _, sh := range shots {
			check(t, st, sh.name, size[0], size[1], sh.keys)
		}
	}
	check(t, st, "folded", 44, 16, nil)
	for _, size := range [][2]int{{44, 31}, {132, 32}} {
		check(t, twoShipsState(t), "sessionships", size[0], size[1], nil)
	}
	night := append([]string{"n"}, strings.Split("merge them on green", "")...)
	for _, size := range [][2]int{{44, 31}, {104, 38}, {132, 32}} {
		check(t, sheetScreen(st, false), "sheet", size[0], size[1], append(night, "tab", "space", "tab"))
	}
	for _, size := range [][2]int{{44, 31}, {104, 38}, {132, 32}} {
		check(t, tabNight(st), "stoker", size[0], size[1], []string{"c"})
		check(t, tabMorning(st), "stokermorning", size[0], size[1], []string{"c"})
	}
	check(t, sheetScreen(st, true), "sheetday", 104, 38, []string{"o"})
	life := st
	life.Agents = []string{"claude", "codex", "opencode"}
	newKeys := append([]string{"N", "tab"}, strings.Split("feat/88-rate-limits", "")...)
	newKeys = append(newKeys, "tab", "tab", "tab")
	newKeys = append(newKeys, strings.Split("fix the flaky ships test", "")...)
	for _, size := range [][2]int{{44, 31}, {104, 38}, {132, 32}} {
		check(t, life, "new", size[0], size[1], newKeys)
		check(t, life, "rename", size[0], size[1], append(append([]string{"r"}, slices.Repeat([]string{"backspace"}, len("spark-docs"))...), strings.Split("docs-1-0", "")...))
		check(t, life, "kill", size[0], size[1], []string{"x"})
	}
	pw := askMsg{prompt: askpass.Prompt{Host: "ws-1", Text: "builder@ws-1's password: "}, answer: func(string, bool) {}}
	hostKey := askMsg{prompt: askpass.Prompt{Host: "ws-1", Confirm: true, Text: "The authenticity of host 'ws-1' can't be established.\n" +
		"ED25519 key fingerprint is SHA256:Hx1T4cGQ0bZp6Yd3lq9m0f8aJk2sWvR7uNcE5yXoLtI.\n" +
		"+--[ED25519 256]--+\n|    .o+=*o.      |\n|     o=B.+ .     |\n|      +.X o      |\n+----[SHA256]-----+\n" +
		"This key is not known by any other names.\nAre you sure you want to continue connecting (yes/no/[fingerprint])? "}, answer: func(string, bool) {}}
	for _, size := range [][2]int{{44, 31}, {104, 38}} {
		check(t, st, "ask", size[0], size[1], strings.Split("abc", ""), pw)
		check(t, st, "askhostkey", size[0], size[1], []string{"y", "enter"}, hostKey)
	}
	for _, size := range [][2]int{{44, 31}, {86, 30}, {132, 32}} {
		golden(t, threeHosts(t, st, size[0], size[1], "fine"), "sessions3", size[0], size[1])
	}
	golden(t, threeHosts(t, st, 86, 30, "stale"), "stale3", 86, 30)
	golden(t, threeHosts(t, st, 86, 30, "blocked"), "blocked3", 86, 30)
	for _, size := range [][2]int{{44, 31}, {86, 30}} {
		golden(t, hostsTicks(st, size[0], size[1]), "hoststicks", size[0], size[1])
	}
}

// threeHosts is the demo machine with two peers: gpu-1, whose train-loop
// waits, and ws-1, whose api-auth asks on the work account while
// docs-sweep edits. gpu-1 is fine, stale (lost 40 s ago, the next try in
// 8 s) or blocked (not connected, so it sent nothing).
func threeHosts(t *testing.T, st proto.State, w, h int, gpu string) Model {
	t.Helper()
	now := demoAt.Add(-40 * time.Second)
	conns := map[string]*fakeConn{"gpu-1": newFake(), "ws-1": newFake()}
	m := New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return now },
		Peers: []Peer{{Host: "gpu-1", Conn: conns["gpu-1"]}, {Host: "ws-1", Conn: conns["ws-1"]}}})
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	next, _ = next.Update(stateMsg(st))
	gpuState, wsState := peerStates(st)
	switch gpu {
	case "blocked":
		next, _ = next.Update(peerMsg{host: "gpu-1", conn: conns["gpu-1"], msg: lostMsg{err: fmt.Errorf("gpu-1: %w", remote.ErrNotConnected), conn: conns["gpu-1"]}})
	default:
		next, _ = next.Update(peerMsg{host: "gpu-1", conn: conns["gpu-1"], msg: stateMsg(gpuState)})
	}
	if gpu == "stale" {
		next, _ = next.Update(peerMsg{host: "gpu-1", conn: conns["gpu-1"], msg: lostMsg{err: io.EOF, conn: conns["gpu-1"]}})
	}
	now = demoAt
	next, _ = next.Update(peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(wsState)})
	nm := next.(Model)
	if gpu == "stale" {
		nm.peers[0].redialing, nm.peers[0].backoff = true, 8*time.Second
	}
	nm.splashDone = true
	return nm
}

// peerStates are the states of gpu-1, whose train-loop waits, and ws-1,
// whose api-auth asks on the work account while docs-sweep edits.
func peerStates(st proto.State) (gpu, ws proto.State) {
	base := st.Sessions[0]
	base.SessionID, base.Busy, base.Status, base.Idle, base.Do, base.Shells, base.Agents, base.Progress, base.AccountDir =
		"", false, "idle", 3*time.Minute, "", nil, 0, nil, ""
	on := func(name, pane string, f func(*sample.Session)) sample.Session {
		s := base
		s.Name, s.Pane, s.Target = name, pane, "w:"+name
		if f != nil {
			f(&s)
		}
		return s
	}
	peer := func(ss []sample.Session, q []queue.Item) proto.State {
		snap := st.Snapshot
		snap.Sessions, snap.Children, snap.IdlePanes = ss, nil, nil
		return proto.State{Doc: state.Doc{Snapshot: snap, Queue: q}}
	}
	gpu = peer([]sample.Session{on("train-loop", "%1", func(s *sample.Session) { s.Do = "clear" })},
		[]queue.Item{{Pane: "%1", Name: "train-loop", State: queue.StatePermission, Since: demoAt.Add(-4 * time.Minute)}})
	ws = peer([]sample.Session{
		on("api-auth", "%1", func(s *sample.Session) { s.AccountDir = "/home/user/.matchblox/accounts/claude/work" }),
		on("docs-sweep", "%2", func(s *sample.Session) { s.Busy, s.Status = true, "edits" }),
	}, []queue.Item{{Pane: "%1", Name: "api-auth", State: queue.StateQuestion, Since: demoAt.Add(-time.Minute)}})
	return gpu, ws
}

// hostsTicks is Hosts over the console of this machine and ws-1, with
// gpu-1 hidden: the ticks, and the live notes of the ticked hosts.
func hostsTicks(st proto.State, w, h int) Shell {
	conns := map[string]*fakeConn{}
	sh := NewShell(ShellOptions{
		Base:   Options{NoMotion: true, Now: func() time.Time { return demoAt }},
		Hosts:  func() []string { return []string{"ws-1", "gpu-1"} },
		Hidden: func() []string { return []string{"gpu-1"} },
		Open: func(host string) Options {
			conns[host] = newFake()
			return Options{Conn: conns[host], Host: host}
		},
	})
	_, ws := peerStates(st)
	var next tea.Model = sh
	for _, msg := range []tea.Msg{
		tea.WindowSizeMsg{Width: w, Height: h},
		stateMsg(st),
		peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(ws)},
		layerMsg{},
	} {
		next, _ = next.Update(msg)
	}
	return next.(Shell)
}

// sheetScreen is the demo machine with a stoker agent, and the launch
// sheet: a night's with its order typed and merges on (n, the text, tab,
// space, tab), or by day the sheet o opens on a running order, its chips as
// the run's goes.
func sheetScreen(st proto.State, day bool) proto.State {
	st.StokerServer = "matchblox"
	v := proto.StokerView{Recipes: []string{"merge-order", "priority", "release-train", "triage"}, Agent: "claude", PaneUp: true, Ready: true}
	if day {
		o := proto.StokerOrder{
			ID: "r0", Words: "pause what waits on review", At: demoAt.Add(-4 * time.Hour), State: "running",
			Plan:   []proto.StokerPlan{{N: 1, Session: "hearth-api", Text: "hold: not priority (ways: priority)"}},
			Grants: []string{"merge"},
		}
		v.On, v.Since = true, demoAt.Add(-4*time.Hour)
		v.Order = &o
	}
	st.Stoker = &v
	return st
}

// check draws the console with msgs, then keys, against its golden frame.
func check(t *testing.T, st proto.State, name string, w, h int, keys []string, msgs ...tea.Msg) {
	t.Helper()
	f := newFake()
	m := New(Options{NoMotion: true, Conn: f, Now: func() time.Time { return demoAt }})
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	next, _ = next.Update(stateMsg(st))
	nm := next.(Model)
	nm.splashDone = true
	if name == "peek" {
		for i, s := range nm.snap.Sessions {
			if s.Name == "flint-auth" {
				nm.chose, nm.selPID = true, nm.snap.Sessions[i].PID
			}
		}
	}
	next = nm
	for _, msg := range msgs {
		next, _ = next.Update(msg)
	}
	for _, k := range keys {
		next, _ = key(next, k)
	}
	golden(t, next.(Model), name, w, h)
}

// golden checks the frame of m, a console or Hosts, against
// testdata/screens/name-wxh.txt; -update writes it.
func golden(t *testing.T, m interface{ render() string }, name string, w, h int) {
	t.Helper()
	got := ansi.Strip(m.render())
	lines := strings.Split(got, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	got = strings.Join(lines, "\n") + "\n"
	path := filepath.Join("..", "..", "testdata", "screens", name+"-"+strconv.Itoa(w)+"x"+strconv.Itoa(h)+".txt")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v: run go test ./internal/app -run TestScreens -update", err)
	}
	if got != string(want) {
		t.Errorf("%s is out of date: run go test ./internal/app -run TestScreens -update\nwant:\n%s\ngot:\n%s", path, want, got)
	}
}
