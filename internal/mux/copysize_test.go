package mux

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"testing"
	"time"
)

func TestTmuxListBuffersReadsTheServerAtItsSocket(t *testing.T) {
	var ran []string
	m := Tmux{Socket: "/tmp/s", Run: func(argv []string) (string, error) {
		ran = argv
		return "buffer1\t200\t5\nbuffer0\t100\t3\n", nil
	}}
	got, err := m.ListBuffers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"tmux", "-S", "/tmp/s", "list-buffers", "-F", "#{buffer_name}\t#{buffer_created}\t#{buffer_size}"}
	if !slices.Equal(ran, want) {
		t.Fatalf("ran %q, want %q", ran, want)
	}
	if len(got) != 2 || got[0] != (Buffer{Name: "buffer1", Created: "200", Size: "5"}) || got[1].Name != "buffer0" {
		t.Fatalf("buffers %+v", got)
	}
}

func TestTmuxListBuffersWithNoServerIsAnError(t *testing.T) {
	m := Tmux{Socket: "/tmp/s", Run: func([]string) (string, error) { return "", errors.New("no server") }}
	if got, err := m.ListBuffers(t.Context()); err == nil || got != nil {
		t.Fatalf("got %v, %v: no server must be an error, not an empty list", got, err)
	}
}

func TestTmuxShowBufferNamesTheBuffer(t *testing.T) {
	var ran []string
	m := Tmux{Socket: "/tmp/s", Run: func(argv []string) (string, error) { ran = argv; return "text\n", nil }}
	got, err := m.ShowBuffer(t.Context(), "buffer1")
	if err != nil || got != "text\n" {
		t.Fatalf("got %q, %v", got, err)
	}
	if want := []string{"tmux", "-S", "/tmp/s", "show-buffer", "-b", "buffer1"}; !slices.Equal(ran, want) {
		t.Fatalf("ran %q, want %q", ran, want)
	}
}

func TestTmuxNoticeShowsOnlyToPeoplesClients(t *testing.T) {
	var ran [][]string
	m := Tmux{Socket: "/tmp/s", Run: func(argv []string) (string, error) {
		ran = append(ran, slices.Clone(argv))
		if slices.Contains(argv, "list-clients") {
			return "1 ctl\n0 /dev/pts/3\n0 /dev/pts/4\n", nil
		}
		return "", nil
	}}
	m.Notice(t.Context(), "too big")
	want := [][]string{
		{"tmux", "-S", "/tmp/s", "list-clients", "-F", "#{client_control_mode} #{client_name}"},
		{"tmux", "-S", "/tmp/s", "display-message", "-c", "/dev/pts/3", "-d", "4000", "too big"},
		{"tmux", "-S", "/tmp/s", "display-message", "-c", "/dev/pts/4", "-d", "4000", "too big"},
	}
	if !slices.EqualFunc(ran, want, slices.Equal) {
		t.Fatalf("ran %q\nwant %q", ran, want)
	}
}

func TestTmuxNoticeSaysNothingWhenTheListFails(t *testing.T) {
	var ran [][]string
	m := Tmux{Socket: "/tmp/s", Run: func(argv []string) (string, error) {
		ran = append(ran, argv)
		return "", errors.New("no server")
	}}
	m.Notice(t.Context(), "x")
	if len(ran) != 1 {
		t.Fatalf("ran %q: after a failed list nothing is shown", ran)
	}
}

// A read with no Run ends with its context: a watcher dies with its ctx.
func TestTmuxReadsEndWithTheirContext(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	start := time.Now()
	_, err := Tmux{Socket: t.TempDir() + "/none"}.ListBuffers(ctx)
	if err == nil {
		t.Fatal("a read on a cancelled context must fail")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("took %v", time.Since(start))
	}
}

func TestTmuxSizedWindowsListsEveryWindowWithItsMark(t *testing.T) {
	var ran []string
	m := Tmux{Server: "matchblox-test", Run: func(argv []string) (string, error) {
		ran = argv
		return "@1\t- 120x40\n@2\t\n@3\tlatest 80x24\n", nil
	}}
	got, err := m.SizedWindows()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"tmux", "-L", "matchblox-test", "list-windows", "-a", "-F", "#{window_id}\t#{@matchblox-sized}"}
	if !slices.Equal(ran, want) {
		t.Fatalf("ran %q, want %q", ran, want)
	}
	if !slices.Equal(got, []SizedWindow{{"@1", "- 120x40"}, {"@3", "latest 80x24"}}) {
		t.Fatalf("got %+v: a window with no mark is not sized", got)
	}
}

func TestTmuxSizedWindowsWithNoServerIsAnError(t *testing.T) {
	m := Tmux{Run: func([]string) (string, error) { return "", errors.New("no server") }}
	if _, err := m.SizedWindows(); err == nil {
		t.Fatal("no server must be an error")
	}
}

func TestTmuxRestoreWindow(t *testing.T) {
	unset := []string{"tmux", "set-option", "-w", "-u", "-t", "@5", "window-size", ";", "set-option", "-w", "-u", "-t", "@5", "@matchblox-sized"}
	for _, c := range []struct {
		server, mark string
		want         []string
	}{
		{"", "- 120x40", unset},
		{"", "nonsense 1x1", unset}, // a value tmux would refuse: tmux sizes the window by its clients again
		{"", "latest 120x40", []string{"tmux", "set-option", "-w", "-t", "@5", "window-size", "latest", ";", "set-option", "-w", "-u", "-t", "@5", "@matchblox-sized"}},
		{"", "manual 120x40", []string{"tmux", "set-option", "-w", "-t", "@5", "window-size", "manual", ";", "resize-window", "-t", "@5", "-x", "120", "-y", "40", ";", "set-option", "-w", "-u", "-t", "@5", "@matchblox-sized"}},
		// manual with a size tmux would refuse: the value comes back, no resize
		{"", "manual 0x40", []string{"tmux", "set-option", "-w", "-t", "@5", "window-size", "manual", ";", "set-option", "-w", "-u", "-t", "@5", "@matchblox-sized"}},
		{"matchblox-test", "- 120x40", append([]string{"tmux", "-L", "matchblox-test"}, unset[1:]...)},
	} {
		var ran []string
		m := Tmux{Server: c.server, Run: func(argv []string) (string, error) { ran = argv; return "", nil }}
		if err := m.RestoreWindow("@5", c.mark); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(ran, c.want) {
			t.Errorf("RestoreWindow(%q, %q) ran %q, want %q", c.server, c.mark, ran, c.want)
		}
	}
}
