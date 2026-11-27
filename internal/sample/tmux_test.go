package sample_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/tmuxtest"
)

// TestTmuxPanesKeepTabsWhenLangUnset: with LANG unset, tmux in the C locale
// replaces tabs in list-panes -F with "_", and ParsePanes finds nothing.
// The sampler's list must still yield panes (tmux -u), or Sessions is empty.
func TestTmuxPanesKeepTabsWhenLangUnset(t *testing.T) {
	server := tmuxtest.Server(t)
	cmd := exec.Command("tmux", "-L", server, "-f", os.DevNull, "new-session", "-d", "-s", "base", "sleep", "60") //nolint:gosec // test argv
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("new-session: %v\n%s", err, out)
	}
	for _, k := range []string{"LANG", "LC_ALL", "LC_CTYPE", "LC_MESSAGES"} {
		t.Setenv(k, "")
	}
	out, err := sample.TmuxPanesOn(server)
	if err != nil {
		t.Fatal(err)
	}
	panes := sample.ParsePanes(out)
	if len(panes) == 0 {
		t.Fatalf("no panes with LANG unset; list-panes was %q", out)
	}
	if panes[0].ID == "" || panes[0].Target == "" {
		t.Fatalf("pane incomplete: %+v (raw %q)", panes[0], out)
	}
}

// TestTmuxPanesOnUsesMuxList: mux lists the panes. No tmux, or a tmux that
// exits, is no panes and no error. A listing is the PaneFormat text with
// the bare pane id, and ParsePanes still skips a jump's session.
func TestTmuxPanesOnUsesMuxList(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if out, err := sample.TmuxPanesOn("matchblox"); out != nil || err != nil {
		t.Fatalf("no tmux: %q, %v; want nil, nil", out, err)
	}

	bin := t.TempDir()
	script := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\n"+body), 0o755); err != nil { //nolint:gosec // test script
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	script("exit 1\n")
	if out, err := sample.TmuxPanesOn(""); out != nil || err != nil {
		t.Fatalf("tmux exits: %q, %v; want nil, nil", out, err)
	}

	script(`printf '%s\n' "$*" > "$0.args"
printf '%%7\tmain:1.2\tw\t42\tclaude\t/x\t/tmp/s\t9\t\t100\t/dev/pts/1\t0\n'
printf '%%8\tmatchblox-jump-0a1b2c3d:1.1\tw\t43\tssh\t/y\t/tmp/s\t9\t1\t100\t/dev/pts/2\t0\n'
`)
	out, err := sample.TmuxPanesOn("matchblox")
	if err != nil {
		t.Fatal(err)
	}
	if want := "%7\tmain:1.2\tw\t42\tclaude\t/x\t/tmp/s\t9\t\t100\n%8\tmatchblox-jump-0a1b2c3d:1.1\tw\t43\tssh\t/y\t/tmp/s\t9\t1\t100\n"; string(out) != want {
		t.Fatalf("out = %q, want %q", out, want)
	}
	if got := sample.ParsePanes(out); len(got) != 1 || got[0].ID != "%7" || got[0].Server != "9" || got[0].ServerStart != "100" {
		t.Fatalf("ParsePanes = %+v", got)
	}
	args, _ := os.ReadFile(filepath.Join(bin, "tmux.args"))
	if !strings.HasPrefix(string(args), "-u -L matchblox list-panes -a -F ") {
		t.Fatalf("tmux ran %q", args)
	}
}

// TestParsePanesSkipsJumpSessions: a jump's session on either side groups
// the agent's: its listing is the same pane, and `list-panes -a` sorts it
// first by name. The pane's own session stays, and so does a builder's
// session whose name looks like a jump's: the jump's mark says which.
func TestParsePanesSkipsJumpSessions(t *testing.T) {
	got := sample.ParsePanes([]byte("%1\tmatchblox-jump-0a1b2c3d:1.1\tw\t10\tclaude\t/x\n" +
		"%9\tmatchblox-ws-1-0a1b2c3d:1.1\tw\t30\tssh\t/y\t/s\t1\t1\n" +
		"%1\twork:1.1\tw\t10\tclaude\t/x\n" +
		"%5\tmatchblox-api-deadbeef:0.0\tw\t50\tclaude\t/z\t/s\t1\t\n"))
	if len(got) != 2 || got[0].Target != "work:1.1" || got[1].Target != "matchblox-api-deadbeef:0.0" {
		t.Fatalf("got %+v", got)
	}
}

func TestPaneMouseOnTmux(t *testing.T) {
	server := tmuxtest.Server(t)
	start := func(session, script string) string {
		t.Helper()
		cmd := exec.Command("tmux", "-L", server, "-f", os.DevNull, "new-session", "-d", "-s", session, "-x", "80", "-y", "24", script)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("new-session: %v\n%s", err, out)
		}
		out, err := exec.Command("tmux", "-L", server, "list-panes", "-t", session, "-F", "#{pane_id}").Output()
		if err != nil {
			t.Fatal(err)
		}
		return sample.OnServer(strings.TrimSpace(string(out)), server)
	}
	reads := start("reads", `printf '\033[?1049h\033[?1002h\033[?1006h'; sleep 30`)
	plain := start("plain", `sleep 30`)
	deadline := time.Now().Add(5 * time.Second)
	cols, rows, ok := sample.PaneMouse(reads)
	for !ok && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		cols, rows, ok = sample.PaneMouse(reads)
	}
	if !ok || cols != 80 || rows != 24 {
		t.Fatalf("reads: %d %d %v", cols, rows, ok)
	}
	if _, _, ok := sample.PaneMouse(plain); ok {
		t.Fatal("plain reads no mouse")
	}
	if _, _, ok := sample.PaneMouse(sample.OnServer("%999", server)); ok {
		t.Fatal("a gone pane reads no mouse")
	}
}

func TestCaptureBackOnTmux(t *testing.T) {
	server := tmuxtest.Server(t)
	cmd := exec.Command("tmux", "-L", server, "-f", os.DevNull, "new-session", "-d", "-s", "hist", "-x", "80", "-y", "24", "seq 1 60; sleep 30")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("new-session: %v\n%s", err, out)
	}
	out, err := exec.Command("tmux", "-L", server, "list-panes", "-t", "hist", "-F", "#{pane_id}").Output()
	if err != nil {
		t.Fatal(err)
	}
	pane := sample.OnServer(strings.TrimSpace(string(out)), server)
	first := func(text string) string { return strings.Fields(text + " none")[0] }
	deadline := time.Now().Add(5 * time.Second)
	live := sample.CapturePanes([]string{pane})[pane]
	for !strings.Contains(live, "60") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		live = sample.CapturePanes([]string{pane})[pane]
	}
	hist, rows, ok := sample.PaneHistory(pane)
	if !ok || rows != 24 || hist < 30 {
		t.Fatalf("history %d, rows %d, ok %v", hist, rows, ok)
	}
	back, ok := sample.CaptureBackStyled(pane, 10, rows)
	if !ok {
		t.Fatal("no capture back")
	}
	var at, want int
	fmt.Sscan(first(live), &at)
	fmt.Sscan(first(back), &want)
	if want != at-10 || len(strings.Split(strings.TrimRight(back, "\n"), "\n")) != rows {
		t.Fatalf("live starts at %d, 10 back starts at %q:\n%s", at, first(back), back)
	}
}
