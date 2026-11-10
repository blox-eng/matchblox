package service

import (
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/tmuxtest"
)

// fakeWindows is a tmux that lists the windows of listed and notes each
// argv it is asked to run.
type fakeWindows struct {
	mu     sync.Mutex
	ran    [][]string
	listed string // what list-windows prints
}

func newFakeWindows() *fakeWindows { return &fakeWindows{} }

func (f *fakeWindows) tmux(argv []string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ran = append(f.ran, slices.Clone(argv))
	if slices.Contains(argv, "list-windows") {
		return f.listed, nil
	}
	return "", nil
}

// steps are the argv the fake ran since the last call.
func (f *fakeWindows) steps() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.ran
	f.ran = nil
	return out
}

// until waits for ok, at most waitLimit.
func until(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(waitLimit); !ok(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("no %s in %v", what, waitLimit)
		}
	}
}

// A service that starts puts back each window a service before it left
// sized: on the builder's server and on matchblox's own.
func TestStartPutsBackWindowsLeftSized(t *testing.T) {
	s := newTest(t, time.Hour)
	s.StokerServer = "matchblox-test"
	f := newFakeWindows()
	f.listed = "@1\t- 120x40\n@2\t\n@3\tlatest 80x24\n"
	s.TmuxOut = f.tmux
	s.restoreSized()
	want := [][]string{
		panes.SizedWindows(""), panes.RestoreWindow("", "@1", "- 120x40"), panes.RestoreWindow("", "@3", "latest 80x24"),
		panes.SizedWindows("matchblox-test"), panes.RestoreWindow("matchblox-test", "@1", "- 120x40"), panes.RestoreWindow("matchblox-test", "@3", "latest 80x24"),
	}
	if got := f.steps(); !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("ran %q\nwant %q", got, want)
	}
}

// TestAWatchNeverSizesTheAgentsWindow runs a console's watch on a tmux
// server of the test's own: the agent's window keeps its size and sets no
// window-size, so the agent never redraws for the view (#351). The screen
// says if the agent draws on the alternate screen. A start puts back a
// window an older service left sized.
func TestAWatchNeverSizesTheAgentsWindow(t *testing.T) {
	server := tmuxtest.Server(t)
	tmux := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", server, "-f", "/dev/null"}, args...)...).CombinedOutput() //nolint:gosec // the test's own server
		if err != nil {
			t.Fatalf("tmux %q: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	tmux("new-session", "-d", "-s", "text", "-x", "120", "-y", "40", "printf 'a text agent'; sleep 300")
	tmux("new-session", "-d", "-s", "full", "-x", "120", "-y", "40", `printf '\033[?1049ha full-screen agent'; sleep 300`)
	text := sample.OnServer(tmux("display-message", "-p", "-t", "text", "#{pane_id}"), server)
	full := sample.OnServer(tmux("display-message", "-p", "-t", "full", "#{pane_id}"), server)
	s, _, ctx := watchTest(t, map[string]string{text: "", full: ""})
	s.Capture, s.ScreenOf = sample.CapturePanesStyled, sample.CaptureScreen
	r := read(connectAs(t, ctx, s, proto.ClientConsole))
	r.actWith(t, "w", proto.Act{RecID: proto.WatchPrefix + text})
	r.screenThat(t, func(sc proto.Screen) bool {
		return sc.Pane == text && strings.Contains(sc.Text, "a text agent") && !sc.Alt
	})
	r.actWith(t, "w2", proto.Act{RecID: proto.WatchPrefix + full})
	r.screenThat(t, func(sc proto.Screen) bool {
		return sc.Pane == full && strings.Contains(sc.Text, "a full-screen agent") && sc.Alt
	})
	time.Sleep(1500 * time.Millisecond) // past any dwell an older console waited
	for _, name := range []string{"text", "full"} {
		if got := tmux("display-message", "-p", "-t", name, "#{window_width}x#{window_height}"); got != "120x40" {
			t.Errorf("%s's window is %s, want 120x40 as the agent had it", name, got)
		}
		if got := tmux("show-options", "-w", "-v", "-t", name, "window-size"); got != "" {
			t.Errorf("%s's window-size is %q, want none of its own", name, got)
		}
	}

	tmux("set-option", "-w", "-t", "text", "window-size", "manual", ";", "set-option", "-w", "-t", "text", panes.SizedMark, "latest 120x40")
	again := newTest(t, time.Hour)
	again.StokerServer = server
	again.restoreSized()
	if got := tmux("show-options", "-w", "-v", "-t", "text", "window-size"); got != "latest" {
		t.Fatalf("window-size %q after a start, want latest", got)
	}
	if got := tmux("display-message", "-p", "-t", "text", "#{"+panes.SizedMark+"}"); got != "" {
		t.Fatalf("mark %q left after a start", got)
	}
}
