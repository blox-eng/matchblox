package service

import (
	"fmt"
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

// fakeWindows is a tmux of one window per pane: it answers the steps a
// chat's size takes, keeps the window's mark and size, and notes each argv.
type fakeWindows struct {
	mu     sync.Mutex
	ran    [][]string
	panes  map[string]int    // how many panes the window of each pane has
	prev   map[string]string // the window-size each window sets itself
	marks  map[string]string // by window
	sizes  map[string]string // by window
	listed string            // what list-windows prints
}

func newFakeWindows() *fakeWindows {
	return &fakeWindows{panes: map[string]int{}, prev: map[string]string{}, marks: map[string]string{}, sizes: map[string]string{}}
}

// fakeWindow is the fake's window id of a pane: "%1" is in "@1".
func fakeWindow(pane string) string { return "@" + strings.TrimPrefix(pane, "%") }

func (f *fakeWindows) tmux(argv []string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ran = append(f.ran, slices.Clone(argv))
	switch {
	case slices.Contains(argv, "list-windows"):
		return f.listed, nil
	case slices.Contains(argv, "show-options"):
		return f.prev[argv[len(argv)-2]] + "\n", nil
	case slices.Contains(argv, "resize-window") && slices.Contains(argv, "display-message"):
		w := argv[slices.Index(argv, "resize-window")+2]
		if i := slices.Index(argv, panes.SizedMark); i > 0 {
			f.marks[w] = argv[i+1]
		}
		x, y := argv[slices.Index(argv, "-x")+1], argv[slices.Index(argv, "-y")+1]
		f.sizes[w] = x + "x" + y
		return x + " " + y + "\n", nil
	case slices.Contains(argv, "display-message"):
		pane := argv[slices.Index(argv, "-t")+1]
		w, n := fakeWindow(pane), f.panes[pane]
		if n == 0 {
			n = 1
		}
		size := f.sizes[w]
		if size == "" {
			size = "120x40"
		}
		return fmt.Sprintf("%s\t%d\t%s\t%s\n", w, n, size, f.marks[w]), nil
	case slices.Contains(argv, panes.SizedMark): // a restore
		delete(f.marks, argv[slices.Index(argv, "-t")+1])
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

// sizeOf is the size the window of pane has now, and its mark.
func (f *fakeWindows) sizeOf(pane string) (string, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sizes[fakeWindow(pane)], f.marks[fakeWindow(pane)]
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

func sizeTest(t *testing.T, screens map[string]string) (*Service, *fakeWindows, func() *reader) {
	t.Helper()
	s, _, ctx := watchTest(t, screens)
	f := newFakeWindows()
	s.TmuxOut = f.tmux
	return s, f, func() *reader { return read(connectAs(t, ctx, s, proto.ClientConsole)) }
}

// watchSized watches pane with a chat of cols by rows.
func (r *reader) watchSized(t *testing.T, id, pane string, cols, rows int) {
	t.Helper()
	if res := r.actWith(t, id, proto.Act{RecID: proto.WatchPrefix + pane, Cols: cols, Rows: rows}); res.Err != "" {
		t.Fatal(res.Err)
	}
}

// The watch of a chat sizes the window of the agent's pane to the chat:
// the window is marked with what it had, its window-size goes manual, and
// the screens say the pane's size. The watch's end puts it back.
func TestAChatSizesTheAgentsWindowAndPutsItBack(t *testing.T) {
	s, f, console := sizeTest(t, map[string]string{"%1": idleScreen})
	_ = s
	r := console()
	r.watchSized(t, "w", "%1", 60, 20)
	until(t, "window sized", func() bool { size, _ := f.sizeOf("%1"); return size == "60x20" })
	if _, mark := f.sizeOf("%1"); mark != "- 120x40" {
		t.Fatalf("mark %q, want - 120x40", mark)
	}
	want := [][]string{panes.WindowOf("%1"), panes.WindowSizeOf("", "@1"), panes.SizeWindow("%1", "@1", "- 120x40", 60, 20)}
	if got := f.steps(); !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("ran %q\nwant %q", got, want)
	}
	r.screenThat(t, func(sc proto.Screen) bool { return sc.Cols == 60 && sc.Rows == 20 && sc.Text == idleScreen })

	r.watchSized(t, "w2", "%1", 60, 20) // the same size again: tmux is not asked
	time.Sleep(200 * time.Millisecond)
	if got := f.steps(); len(got) != 0 {
		t.Fatalf("the same size ran %q", got)
	}

	if res := r.act(t, "stop", proto.WatchPrefix); res.Err != "" {
		t.Fatal(res.Err)
	}
	until(t, "window put back", func() bool { _, mark := f.sizeOf("%1"); return mark == "" })
	if got, want := f.steps(), [][]string{panes.RestoreWindow("", "@1", "- 120x40")}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("ran %q\nwant %q", got, want)
	}
}

// A window that set its own window-size gets it back.
func TestTheWindowGetsItsOwnWindowSizeBack(t *testing.T) {
	_, f, console := sizeTest(t, map[string]string{"%1": idleScreen})
	f.prev["@1"] = "latest"
	r := console()
	r.watchSized(t, "w", "%1", 60, 20)
	until(t, "window sized", func() bool { _, mark := f.sizeOf("%1"); return mark == "latest 120x40" })
	f.steps()
	r.c.Close() // the console goes
	until(t, "window put back", func() bool { _, mark := f.sizeOf("%1"); return mark == "" })
	got := f.steps()
	if want := panes.RestoreWindow("", "@1", "latest 120x40"); len(got) != 1 || !slices.Equal(got[0], want) || !slices.Contains(want, "latest") {
		t.Fatalf("ran %q, want %q", got, want)
	}
}

// A new size of the chat sizes the window again and keeps the first mark;
// moving to another pane puts the first window back.
func TestAResizedChatAndAMovedSelection(t *testing.T) {
	_, f, console := sizeTest(t, map[string]string{"%1": idleScreen, "%2": idleScreen})
	r := console()
	r.watchSized(t, "w", "%1", 60, 20)
	until(t, "window sized", func() bool { size, _ := f.sizeOf("%1"); return size == "60x20" })
	r.watchSized(t, "w2", "%1", 70, 22)
	until(t, "window sized again", func() bool { size, _ := f.sizeOf("%1"); return size == "70x22" })
	if _, mark := f.sizeOf("%1"); mark != "- 120x40" {
		t.Fatalf("mark %q, want the first", mark)
	}
	r.watchSized(t, "w3", "%2", 70, 22)
	until(t, "the second window sized and the first put back", func() bool {
		size, _ := f.sizeOf("%2")
		_, mark := f.sizeOf("%1")
		return size == "70x22" && mark == ""
	})
}

// Two consoles on one pane: the one that took the keys last sizes it,
// else the one that watched last; when it goes the other's size holds,
// and when both go the window is put back.
func TestTwoChatsOnOnePane(t *testing.T) {
	_, f, console := sizeTest(t, map[string]string{"%1": idleScreen})
	desk, phone := console(), console()
	desk.watchSized(t, "w", "%1", 100, 40)
	until(t, "desk's size", func() bool { size, _ := f.sizeOf("%1"); return size == "100x40" })
	phone.watchSized(t, "w", "%1", 44, 25)
	until(t, "phone's size: it watched last", func() bool { size, _ := f.sizeOf("%1"); return size == "44x25" })
	if res := desk.actWith(t, "k", proto.Act{RecID: proto.KeysPrefix + "%1", Text: "Down", PID: 100}); res.Err != "" {
		t.Fatal(res.Err)
	}
	until(t, "desk's size: it took the keys", func() bool { size, _ := f.sizeOf("%1"); return size == "100x40" })
	phone.watchSized(t, "w2", "%1", 44, 26) // a new size of the phone's chat is not a new watch
	time.Sleep(200 * time.Millisecond)
	if size, _ := f.sizeOf("%1"); size != "100x40" {
		t.Fatalf("size %s, want the desk's", size)
	}
	desk.c.Close()
	until(t, "phone's size once the desk goes", func() bool { size, _ := f.sizeOf("%1"); return size == "44x26" })
	if res := phone.act(t, "stop", proto.WatchPrefix); res.Err != "" {
		t.Fatal(res.Err)
	}
	until(t, "window put back", func() bool { _, mark := f.sizeOf("%1"); return mark == "" })
}

// A window of several panes is not sized: it would reflow the others. Its
// screens say no size, and the chat wraps them.
func TestAWindowOfSeveralPanesIsNotSized(t *testing.T) {
	_, f, console := sizeTest(t, map[string]string{"%1": idleScreen})
	f.panes["%1"] = 2
	r := console()
	r.watchSized(t, "w", "%1", 60, 20)
	until(t, "the window read", func() bool { f.mu.Lock(); defer f.mu.Unlock(); return len(f.ran) > 0 })
	time.Sleep(200 * time.Millisecond)
	if got, want := f.steps(), [][]string{panes.WindowOf("%1")}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("ran %q, want only %q", got, want)
	}
	r.screenThat(t, func(sc proto.Screen) bool { return sc.Text == idleScreen })
	for _, sc := range r.got() {
		if sc.Cols != 0 || sc.Rows != 0 {
			t.Fatalf("screen %+v says a size", sc)
		}
	}
}

// A console with no size, as one before sizes was, sizes nothing.
func TestAWatchWithNoSizeSizesNothing(t *testing.T) {
	_, f, console := sizeTest(t, map[string]string{"%1": idleScreen})
	r := console()
	r.act(t, "w", proto.WatchPrefix+"%1")
	r.screenThat(t, func(sc proto.Screen) bool { return sc.Text == idleScreen })
	time.Sleep(200 * time.Millisecond)
	if got := f.steps(); len(got) != 0 {
		t.Fatalf("ran %q", got)
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

// A service that stops puts back every window its chats sized.
func TestStopPutsBackEveryWindow(t *testing.T) {
	s, f, console := sizeTest(t, map[string]string{"%1": idleScreen, "%2": idleScreen})
	console().watchSized(t, "w", "%1", 60, 20)
	console().watchSized(t, "w", "%2", 60, 20)
	until(t, "windows sized", func() bool {
		a, _ := f.sizeOf("%1")
		b, _ := f.sizeOf("%2")
		return a == "60x20" && b == "60x20"
	})
	s.unsizeAll()
	_, a := f.sizeOf("%1")
	_, b := f.sizeOf("%2")
	if a != "" || b != "" {
		t.Fatalf("marks %q %q left after the stop", a, b)
	}
}

// TestAChatSizesARealWindow runs the steps on a tmux server of the test's
// own: the window takes the chat's size and is marked, and the watch's end
// gives it its window-size back. A start puts back a window left marked.
func TestAChatSizesARealWindow(t *testing.T) {
	server := tmuxtest.Server(t)
	tmux := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", server, "-f", "/dev/null"}, args...)...).CombinedOutput() //nolint:gosec // the test's own server
		if err != nil {
			t.Fatalf("tmux %q: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	tmux("new-session", "-d", "-s", "agent", "-x", "120", "-y", "40", "sleep 300")
	pane := sample.OnServer(tmux("display-message", "-p", "-t", "agent", "#{pane_id}"), server)
	s, _, ctx := watchTest(t, map[string]string{pane: idleScreen})
	r := read(connectAs(t, ctx, s, proto.ClientConsole))
	r.watchSized(t, "w", pane, 60, 20)
	until(t, "the window at 60x20", func() bool {
		return tmux("display-message", "-p", "-t", "agent", "#{window_width}x#{window_height}") == "60x20"
	})
	if got := tmux("show-options", "-w", "-v", "-t", "agent", "window-size"); got != "manual" {
		t.Fatalf("window-size %q, want manual", got)
	}
	if got := tmux("display-message", "-p", "-t", "agent", "#{"+panes.SizedMark+"}"); got != "- 120x40" {
		t.Fatalf("mark %q", got)
	}
	r.screenThat(t, func(sc proto.Screen) bool { return sc.Cols == 60 && sc.Rows == 20 })
	if res := r.act(t, "stop", proto.WatchPrefix); res.Err != "" {
		t.Fatal(res.Err)
	}
	until(t, "the window put back", func() bool { return tmux("show-options", "-w", "-v", "-t", "agent", "window-size") == "" })
	if got := tmux("display-message", "-p", "-t", "agent", "#{"+panes.SizedMark+"}"); got != "" {
		t.Fatalf("mark %q left", got)
	}

	tmux("set-option", "-w", "-t", "agent", "window-size", "manual", ";", "set-option", "-w", "-t", "agent", panes.SizedMark, "latest 120x40")
	again := newTest(t, time.Hour)
	again.StokerServer = server
	again.restoreSized()
	if got := tmux("show-options", "-w", "-v", "-t", "agent", "window-size"); got != "latest" {
		t.Fatalf("window-size %q after a start, want latest", got)
	}
	if got := tmux("display-message", "-p", "-t", "agent", "#{"+panes.SizedMark+"}"); got != "" {
		t.Fatalf("mark %q left after a start", got)
	}
}
