package app

import (
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/procfs"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
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
// a ship of each kind, and the dock folded on a short phone. -update
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
}

func check(t *testing.T, st proto.State, name string, w, h int, keys []string) {
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
	for _, k := range keys {
		next, _ = key(next, k)
	}
	got := ansi.Strip(next.(Model).render())
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
