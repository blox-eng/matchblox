package mux

import (
	"bytes"
	"errors"
	"log"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/panes"
)

// listFormat is today's pane list plus the tty and the dead flag.
const listFormat = "#{pane_id}\t#{session_name}:#{window_index}.#{pane_index}\t#{window_name}\t#{pane_pid}\t#{pane_current_command}\t#{pane_current_path}\t#{socket_path}\t#{pid}\t#{@matchblox-jump}\t#{start_time}\t#{pane_tty}\t#{pane_dead}"

func TestTmuxTypeEscapesAFinalSemicolon(t *testing.T) {
	if got, want := (Tmux{}).Type("%1", "a;"), panes.Literal("%1", "a;"); !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTmuxKeysPressKeyNames(t *testing.T) {
	if got, want := (Tmux{}).Keys("%1", "C-u", "Enter"), panes.Keys("%1", "C-u", "Enter"); !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTmuxInModeReadsDisplayMessage(t *testing.T) {
	var ran []string
	m := Tmux{Run: func(argv []string) (string, error) { ran = argv; return "1\n", nil }}
	in, err := m.InMode("%1")
	if err != nil || !in {
		t.Fatalf("in = %v, %v", in, err)
	}
	if want := []string{"tmux", "display-message", "-p", "-t", "%1", "#{pane_in_mode}"}; !slices.Equal(ran, want) {
		t.Fatalf("ran %q, want %q", ran, want)
	}
	m.Run = func([]string) (string, error) { return "0\n", nil }
	if in, err := m.InMode("%1"); err != nil || in {
		t.Fatalf("not in a mode: %v, %v", in, err)
	}
}

func TestTmuxScreensUsesItsCapture(t *testing.T) {
	m := Tmux{Capture: func(ps []string) map[string]string { return map[string]string{ps[0]: "screen"} }}
	if got := m.Screens([]string{"%1"}); got["%1"] != "screen" {
		t.Fatalf("got %q", got)
	}
}

func TestTmuxMarkSetsAndTakesOffThePaneOption(t *testing.T) {
	var ran [][]string
	m := Tmux{Run: func(argv []string) (string, error) { ran = append(ran, argv); return "", nil }}
	if err := m.Mark("%1", true); err != nil {
		t.Fatal(err)
	}
	if err := m.Mark("%4@matchblox", false); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"tmux", "set-option", "-p", "-t", "%1", "@matchblox_match", "1"},
		{"tmux", "-L", "matchblox", "set-option", "-p", "-u", "-t", "%4", "@matchblox_match"},
	}
	if !slices.EqualFunc(ran, want, slices.Equal) {
		t.Fatalf("ran %q, want %q", ran, want)
	}
}

func TestTmuxLoadPutsTheTextOnStdinAndPasteIsBracketed(t *testing.T) {
	var ran [][]string
	var in string
	m := Tmux{
		Run:   func(argv []string) (string, error) { ran = append(ran, argv); return "", nil },
		RunIn: func(argv []string, stdin string) error { ran = append(ran, argv); in = stdin; return nil },
	}
	if err := m.Load("%4@matchblox", "matchblox-paste-1", "a\nb;"); err != nil {
		t.Fatal(err)
	}
	if err := m.Drop("%1", "matchblox-paste-1"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"tmux", "-L", "matchblox", "load-buffer", "-b", "matchblox-paste-1", "-"},
		{"tmux", "delete-buffer", "-b", "matchblox-paste-1"},
	}
	if !slices.EqualFunc(ran, want, slices.Equal) || in != "a\nb;" {
		t.Fatalf("ran %q with %q, want %q", ran, in, want)
	}
	if got, want := m.Paste("%1", "matchblox-paste-1"), []string{"tmux", "paste-buffer", "-p", "-d", "-b", "matchblox-paste-1", "-t", "%1"}; !slices.Equal(got, want) {
		t.Fatalf("paste %q, want %q", got, want)
	}
}

func TestTmuxTypedAtIsTheLatestKeyOfAClientOnThePane(t *testing.T) {
	var ran []string
	m := Tmux{Run: func(argv []string) (string, error) {
		ran = argv
		return "1700000000 %1\n1700000100 %1\n1700000500 %2\ncan't find session: gone\n", nil
	}}
	if got := m.TypedAt("%1"); !got.Equal(time.Unix(1700000100, 0)) {
		t.Fatalf("TypedAt = %v", got)
	}
	if want := []string{"tmux", "list-clients", "-F", "#{client_activity} #{pane_id}"}; !slices.Equal(ran, want) {
		t.Fatalf("ran %q, want %q", ran, want)
	}
	if got := m.TypedAt("%3"); !got.IsZero() {
		t.Fatalf("no client on %%3: %v", got)
	}
	m.TypedAt("%4@matchblox")
	if want := []string{"tmux", "-L", "matchblox", "list-clients", "-F", "#{client_activity} #{pane_id}"}; !slices.Equal(ran, want) {
		t.Fatalf("ran %q, want %q", ran, want)
	}
	m.Run = func([]string) (string, error) { return "", errors.New("no server") }
	if got := m.TypedAt("%1"); !got.IsZero() {
		t.Fatalf("no server: %v", got)
	}
}

func TestKillStepsTermOnlyThePanePid(t *testing.T) {
	got := (Tmux{}).Kill("%1", 99)
	want := [][]string{
		{"tmux", "kill-pane", "-t", "%1"},
		{"kill", "-TERM", "99"},
	}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("got %q, want %q", got, want)
	}
	only := (Tmux{}).Kill("%1", 0)
	if !slices.EqualFunc(only, [][]string{want[0]}, slices.Equal) {
		t.Fatalf("pid 0 is the pane only: %q", only)
	}
	if neg := (Tmux{}).Kill("%1", -1); !slices.EqualFunc(neg, [][]string{want[0]}, slices.Equal) {
		t.Fatalf("negative pid is the pane only: %q", neg)
	}
	on := (Tmux{}).Kill("%4@matchblox", 99)
	wantOn := [][]string{
		{"tmux", "-L", "matchblox", "kill-pane", "-t", "%4"},
		{"kill", "-TERM", "99"},
	}
	if !slices.EqualFunc(on, wantOn, slices.Equal) {
		t.Fatalf("server: got %q, want %q", on, wantOn)
	}
}

func TestStartErrorOmitsEnvValues(t *testing.T) {
	const secret = "sk-test"
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	ran := false
	m := Tmux{Run: func([]string) (string, error) {
		ran = true
		return "", errors.New("start failed: " + secret)
	}}
	steps := m.Start(Start{
		Name:     "api",
		Dir:      "/work",
		Env:      []string{"ANTHROPIC_API_KEY=" + secret},
		Argv:     []string{"claude", "--name", "api"},
		Detached: true,
	})
	if ran {
		t.Fatal("Start returns steps for Actions.Do; it does not run")
	}
	if strings.Contains(logs.String(), secret) {
		t.Fatalf("log holds the env value: %q", logs.String())
	}
	for _, step := range steps {
		for _, word := range step {
			if strings.Contains(word, secret) && word != "ANTHROPIC_API_KEY="+secret {
				t.Fatalf("env value leaked into %q", word)
			}
		}
	}
	want := [][]string{{
		"env", "-u", "GIT_TERMINAL_PROMPT", "-u", "GIT_SSH_COMMAND", "-u", "MATCHBLOX_HOME",
		"tmux", "new-session", "-d", "-s", "api", "-n", "api", "-c", "/work",
		"-e", "ANTHROPIC_API_KEY=" + secret,
		"claude", "--name", "api",
	}}
	if !slices.EqualFunc(steps, want, slices.Equal) {
		t.Fatalf("got %q, want %q", steps, want)
	}

	stoker := (Tmux{Server: "matchblox"}).Start(Start{
		Name: "stoker", Dir: "/home/builder", Argv: []string{"claude"},
		Detached: true, EmptyExit: true,
	})
	wantStoker := [][]string{{
		"env", "-u", "GIT_TERMINAL_PROMPT", "-u", "GIT_SSH_COMMAND", "-u", "MATCHBLOX_HOME",
		"tmux", "-L", "matchblox", "-f", os.DevNull,
		"new-session", "-d", "-s", "stoker", "-c", "/home/builder",
		"claude", ";", "set-option", "-s", "exit-empty", "on",
	}}
	if !slices.EqualFunc(stoker, wantStoker, slices.Equal) {
		t.Fatalf("stoker: got %q, want %q", stoker, wantStoker)
	}
}

func TestRespawnKeepsPaneID(t *testing.T) {
	got := (Tmux{}).Respawn("%3", "/work", nil, []string{"codex"})
	want := [][]string{{
		"tmux", "respawn-pane", "-k", "-t", "%3", "-c", "/work",
		"env", "-u", "GIT_TERMINAL_PROMPT", "-u", "GIT_SSH_COMMAND", "-u", "MATCHBLOX_HOME",
		"codex",
	}}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for _, step := range got {
		if slices.Contains(step, "new-session") {
			t.Fatalf("respawn opened a session: %q", got)
		}
	}
	on := (Tmux{}).Respawn("%3@matchblox", "/work", []string{"FOO=bar"}, []string{"claude"})
	wantOn := [][]string{{
		"tmux", "-L", "matchblox", "respawn-pane", "-k", "-t", "%3", "-c", "/work",
		"env", "-u", "GIT_TERMINAL_PROMPT", "-u", "GIT_SSH_COMMAND", "-u", "MATCHBLOX_HOME",
		"FOO=bar", "claude",
	}}
	if !slices.EqualFunc(on, wantOn, slices.Equal) {
		t.Fatalf("server: got %q, want %q", on, wantOn)
	}
}

func TestRenameNamesTheWindow(t *testing.T) {
	got := (Tmux{}).Rename("%1", "api")
	want := [][]string{{"tmux", "rename-window", "-t", "%1", "api"}}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("got %q, want %q", got, want)
	}
	on := (Tmux{}).Rename("%4@matchblox", "api")
	wantOn := [][]string{{"tmux", "-L", "matchblox", "rename-window", "-t", "%4", "api"}}
	if !slices.EqualFunc(on, wantOn, slices.Equal) {
		t.Fatalf("server: got %q, want %q", on, wantOn)
	}
	for _, step := range append(got, on...) {
		if slices.Contains(step, "send-keys") || slices.Contains(step, "rename-session") {
			t.Fatalf("rename did more than the window: %q", step)
		}
	}
}

func TestKillSessionEndsThatName(t *testing.T) {
	if got, want := (Tmux{}).KillSession("api"), []string{"tmux", "kill-session", "-t", "=api"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	got := (Tmux{Server: "matchblox"}).KillSession("stoker")
	want := []string{"tmux", "-L", "matchblox", "kill-session", "-t", "=stoker"}
	if !slices.Equal(got, want) {
		t.Fatalf("server: got %q, want %q", got, want)
	}
	got = (Tmux{Server: "matchblox", Socket: "/tmp/mb.sock"}).KillSession("api")
	want = []string{"tmux", "-S", "/tmp/mb.sock", "kill-session", "-t", "=api"}
	if !slices.Equal(got, want) {
		t.Fatalf("socket: got %q, want %q", got, want)
	}
}

func TestListWithNoTmuxIsEmpty(t *testing.T) {
	ran := false
	missing := Tmux{
		LookPath: func(string) (string, error) { return "", errors.New("tmux not found") },
		Run:      func([]string) (string, error) { ran = true; return "pane", nil },
	}
	panes, err := missing.List()
	if err != nil || len(panes) != 0 || ran {
		t.Fatalf("no tmux: %d panes, err %v, ran %v", len(panes), err, ran)
	}

	exitErr := exec.Command("false").Run()
	var got [][]string
	gone := Tmux{
		LookPath: func(string) (string, error) { return "tmux", nil },
		Run: func(argv []string) (string, error) {
			got = append(got, slices.Clone(argv))
			return "no server", exitErr
		},
	}
	panes, err = gone.List()
	if err != nil || len(panes) != 0 {
		t.Fatalf("no server: %d panes, err %v", len(panes), err)
	}
	want := [][]string{
		{"tmux", "-u", "list-panes", "-a", "-F", listFormat},
	}
	gone.Server = "matchblox"
	if _, err := gone.List(); err != nil {
		t.Fatal(err)
	}
	want = append(want, []string{"tmux", "-u", "-L", "matchblox", "list-panes", "-a", "-F", listFormat})
	gone.Socket = "/tmp/mb.sock"
	if _, err := gone.List(); err != nil {
		t.Fatal(err)
	}
	want = append(want, []string{"tmux", "-u", "-S", "/tmp/mb.sock", "list-panes", "-a", "-F", listFormat})
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("ran %q, want %q", got, want)
	}

	gone.Run = func([]string) (string, error) { return "", errors.New("boom") }
	if _, err := gone.List(); err == nil {
		t.Fatal("a run error that is not an exit must stay")
	}
}

func TestListIsPerSocket(t *testing.T) {
	ownTmux(t)
	endServer(t, "")
	endServer(t, "matchblox")
	tmuxCmd(t, "", "new-session", "-d", "-s", "only-builder", "sleep", "60")
	tmuxCmd(t, "matchblox", "new-session", "-d", "-s", "only-stoker", "sleep", "60")

	stoker, err := (Tmux{Server: "matchblox"}).List()
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := paneSession(stoker, "only-builder"); ok {
		t.Fatalf("builder pane on the stoker socket: %+v", p)
	}
	p, ok := paneSession(stoker, "only-stoker")
	if !ok || !strings.HasSuffix(p.ID, "@matchblox") {
		t.Fatalf("stoker pane: %+v ok=%v", p, ok)
	}
	builder, err := (Tmux{}).List()
	if err != nil {
		t.Fatal(err)
	}
	home, ok := paneSession(builder, "only-builder")
	if !ok || strings.Contains(home.ID, "@") {
		t.Fatalf("builder pane: %+v ok=%v", home, ok)
	}
	if p, ok := paneSession(builder, "only-stoker"); ok {
		t.Fatalf("stoker pane on the builder socket: %+v", p)
	}
}

func TestListFieldsMatchPaneFormat(t *testing.T) {
	ownTmux(t)
	const server = "mb-fields"
	endServer(t, server)
	dir := t.TempDir()
	for _, k := range []string{"LANG", "LC_ALL", "LC_CTYPE", "LC_MESSAGES"} {
		t.Setenv(k, "")
	}
	tmuxCmd(t, server, "new-session", "-d", "-s", "fields", "-n", "desk", "-c", dir, "sleep", "60")
	tmuxCmd(t, server, "set-option", "-t", "=fields:", "@matchblox-jump", "1")
	tmuxCmd(t, server, "set-option", "-t", "fields", "remain-on-exit", "on")
	tmuxCmd(t, server, "split-window", "-t", "fields", "-d", "true")

	var panes []Pane
	deadline := time.Now().Add(2 * time.Second)
	for {
		var err error
		panes, err = (Tmux{Server: server}).List()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := paneByDead(panes, false); ok {
			if _, ok := paneByDead(panes, true); ok {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("panes = %+v", panes)
		}
		time.Sleep(20 * time.Millisecond)
	}
	live, _ := paneByDead(panes, false)
	dead, _ := paneByDead(panes, true)
	assertPaneFormat(t, server, live, dir)
	if live.Command != "sleep" || live.Window != "desk" || live.Target != "fields:0.0" || live.Jump != "1" || live.Dead {
		t.Fatalf("live pane: %+v", live)
	}
	if !dead.Dead || !strings.HasSuffix(dead.ID, "@"+server) || dead.ServerPID != live.ServerPID {
		t.Fatalf("dead pane: %+v", dead)
	}
}

func assertPaneFormat(t *testing.T, server string, p Pane, dir string) {
	t.Helper()
	bare, ok := strings.CutSuffix(p.ID, "@"+server)
	if !ok || !strings.HasPrefix(bare, "%") {
		t.Fatalf("id %q", p.ID)
	}
	got := tmuxCmd(t, server, "display-message", "-p", "-t", bare, listFormat)
	f := strings.Split(got, "\t")
	if len(f) != 12 {
		t.Fatalf("format fields %d: %q", len(f), got)
	}
	pid, err := strconv.Atoi(f[3])
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != OnServer(f[0], server) || p.Target != f[1] || p.Window != f[2] || p.PID != pid || p.Command != f[4] || p.Path != f[5] || p.Socket != f[6] || p.ServerPID != f[7] || p.Jump != f[8] || p.ServerStart != f[9] || p.TTY != f[10] || p.Dead != (f[11] == "1") {
		t.Fatalf("pane %+v, format %q", p, f)
	}
	if p.PID <= 0 || p.Path != dir || p.Socket == "" || p.ServerPID == "" || p.ServerPID == strconv.Itoa(p.PID) || p.TTY == "" || p.Jump != "1" {
		t.Fatalf("pane %+v", p)
	}
	if _, err := strconv.ParseInt(p.ServerStart, 10, 64); err != nil {
		t.Fatalf("start %q", p.ServerStart)
	}
}

func paneSession(panes []Pane, session string) (Pane, bool) {
	for _, p := range panes {
		name, _, _ := strings.Cut(p.Target, ":")
		if name == session {
			return p, true
		}
	}
	return Pane{}, false
}

func paneByDead(panes []Pane, dead bool) (Pane, bool) {
	for _, p := range panes {
		if p.Dead == dead {
			return p, true
		}
	}
	return Pane{}, false
}

func ownTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
}

func endServer(t *testing.T, server string) {
	t.Helper()
	t.Cleanup(func() {
		argv := []string{"kill-server"}
		if server != "" {
			argv = append([]string{"-L", server}, argv...)
		}
		_ = exec.Command("tmux", argv...).Run()
	})
}

func tmuxCmd(t *testing.T, server string, args ...string) string {
	t.Helper()
	argv := []string{"-f", os.DevNull}
	if server != "" {
		argv = append([]string{"-L", server}, argv...)
	}
	argv = append(argv, args...)
	out, err := exec.Command("tmux", argv...).CombinedOutput()
	if err != nil {
		t.Fatalf("tmux %q: %v\n%s", argv, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestSplitReturnsNewPaneID(t *testing.T) {
	ownTmux(t)
	const server = "mb-split"
	endServer(t, server)
	tmuxCmd(t, server, "new-session", "-d", "-s", "splitme", "sleep", "60")

	got, err := (Tmux{Server: server}).Split("splitme", []string{"sleep", "60"})
	if err != nil {
		t.Fatal(err)
	}
	_, id := SplitPane(got)
	if !strings.HasPrefix(id, "%") || strings.ContainsAny(got, " \n") {
		t.Fatalf("got %q, want a %% pane id", got)
	}
	if n := len(strings.Fields(tmuxCmd(t, server, "list-panes", "-t", "splitme", "-F", "#{pane_id}"))); n != 2 {
		t.Fatalf("session has %d panes, want 2", n)
	}
}

func TestSplitBuildsTheSplitWindowArgv(t *testing.T) {
	var got []string
	tm := Tmux{Server: "matchblox", Run: func(argv []string) (string, error) {
		got = argv
		return "%7\n", nil
	}}
	id, err := tm.Split("work", []string{"claude", "--resume"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"tmux", "-L", "matchblox", "split-window", "-t", "work", "-P", "-F", "#{pane_id}", "--", "claude", "--resume"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	if id != "%7@matchblox" {
		t.Fatalf("got %q, want %q", id, "%7@matchblox")
	}
}

func TestSplitErrorsWithoutAPaneID(t *testing.T) {
	tm := Tmux{Run: func([]string) (string, error) { return "\n", nil }}
	if id, err := tm.Split("work", nil); err == nil {
		t.Fatalf("got id %q and no error", id)
	}
	tm = Tmux{Run: func([]string) (string, error) { return "", errors.New("no server") }}
	if _, err := tm.Split("work", nil); err == nil {
		t.Fatal("run error was swallowed")
	}
}

func TestFocusArgvSelectsThePane(t *testing.T) {
	got := (Tmux{}).Focus("%3")
	want := [][]string{
		{"tmux", "select-window", "-t", "%3"},
		{"tmux", "select-pane", "-t", "%3"},
	}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("got %q, want %q", got, want)
	}
	on := (Tmux{}).Focus("%3@matchblox")
	wantOn := [][]string{
		{"tmux", "-L", "matchblox", "select-window", "-t", "%3"},
		{"tmux", "-L", "matchblox", "select-pane", "-t", "%3"},
	}
	if !slices.EqualFunc(on, wantOn, slices.Equal) {
		t.Fatalf("server: got %q, want %q", on, wantOn)
	}
	for _, step := range append(got, on...) {
		if slices.Contains(step, "attach-session") {
			t.Fatalf("focus attached: %q", step)
		}
	}
}

func TestAttachNamesTheSession(t *testing.T) {
	if got, want := (Tmux{}).Attach("api"), []string{"tmux", "attach-session", "-t", "api"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	got := (Tmux{Server: "matchblox"}).Attach("stoker")
	if want := []string{"tmux", "-L", "matchblox", "attach-session", "-t", "stoker"}; !slices.Equal(got, want) {
		t.Fatalf("server: got %q, want %q", got, want)
	}
	got = (Tmux{Socket: "/tmp/s"}).Attach("api")
	if want := []string{"tmux", "-S", "/tmp/s", "attach-session", "-t", "api"}; !slices.Equal(got, want) {
		t.Fatalf("socket: got %q, want %q", got, want)
	}
}

func TestCallerReadsTMUX_PANE(t *testing.T) {
	t.Setenv("TMUX_PANE", "%9")
	if got := (Tmux{}).Caller(); got != "%9" {
		t.Fatalf("got %q, want %%9", got)
	}
	t.Setenv("TMUX_PANE", "")
	if got := (Tmux{}).Caller(); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestLastClientPicksTheLatestActivity(t *testing.T) {
	var got []string
	tm := Tmux{Run: func(argv []string) (string, error) {
		got = argv
		return "100 50 /dev/pts/1\n300 200 /dev/pts/2\nbad line\n200 150 /dev/pts/3\n", nil
	}}
	tty, at, err := tm.LastClient("console")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"tmux", "list-clients", "-t", "=console", "-F", "#{client_activity} #{client_created} #{client_tty}"}
	if !slices.Equal(got, want) {
		t.Fatalf("argv: got %q, want %q", got, want)
	}
	if tty != "/dev/pts/2" || !at.Equal(time.Unix(200, 0)) {
		t.Fatalf("got %q %v, want /dev/pts/2 and the created time 200", tty, at)
	}
}

func TestLastClientSwallowsATmuxError(t *testing.T) {
	tm := Tmux{Run: func([]string) (string, error) { return "", errors.New("no server running") }}
	tty, at, err := tm.LastClient("console")
	if err != nil || tty != "" || !at.IsZero() {
		t.Fatalf("got %q %v %v, want empty, zero and nil", tty, at, err)
	}
}
