//go:build unix

package remote

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// These tests run a real jump (newJumpEnv: a desk server, a host server and
// a fake ssh) whose agent pane is a recorder: it asks for the mouse in a
// given mode and appends every byte it reads to a file. The test writes
// mouse reports to the desk client, as a terminal emulator does.

const (
	sgrMouse    = `\033[?1000h\033[?1006h`
	normalMouse = `\033[?1000h`
	noMouse     = ``
)

type mouseRig struct {
	jumpEnv
	t   *testing.T
	c   *client
	log string
}

// newMouseRig shows text in the agent pane, turns on the mouse in modes and
// records what the agent reads, then jumps into it.
func newMouseRig(t *testing.T, modes, text string) *mouseRig {
	t.Helper()
	e := newJumpEnv(t)
	r := &mouseRig{jumpEnv: e, t: t, log: filepath.Join(t.TempDir(), "mouse")}
	rec := fmt.Sprintf(`printf '%%b' %q; printf '%s'; stty raw -echo; exec cat > %s`, text, modes, r.log)
	mustTmux(t, e.inner, "respawn-pane", "-k", "-t", e.pane, "sh", "-c", rec)
	if modes != noMouse {
		waitFor(t, "the recorder asks for the mouse", func() bool {
			out, _ := tmuxOut(e.inner, "display-message", "-p", "-t", e.pane, "#{mouse_any_flag}")
			return out == "1"
		})
	}
	r.c = attach(t, e.outer, "matchblox")
	j := e.newJump("0a1b2c3d")
	e.jump(t, r.c, j, e.ssh)
	r.c.waitSession(HostSessionName(j.Host, j.ID))
	waitFor(t, "the jump is attached", func() bool { return len(clientsOf(e.inner, JumpSessionName(j.ID))) == 1 })
	waitFor(t, "the jump asks the desk for the mouse", func() bool {
		out, _ := tmuxOut(e.outer, "display-message", "-p", "-t", "="+HostSessionName(j.Host, j.ID)+":", "#{mouse_any_flag}")
		return out == "1"
	})
	return r
}

// mouse writes one SGR report for 0-based cell x, y; up ends it with m.
func (r *mouseRig) mouse(b, x, y int, up bool) {
	r.t.Helper()
	end := "M"
	if up {
		end = "m"
	}
	r.c.type_(fmt.Sprintf("\x1b[<%d;%d;%d%s", b, x+1, y+1, end))
	time.Sleep(50 * time.Millisecond)
}

// got is what the agent has read, once tmux has had time to pass it on.
func (r *mouseRig) got() string {
	time.Sleep(400 * time.Millisecond)
	b, _ := os.ReadFile(r.log)
	return string(b)
}

// TestAClickReachesTheAgentWholeOnRelease: the press is held back; the
// agent gets press and release together when the button comes up, in the
// encoding it asked for (spec R3, D8). A pane that reads no mouse gets none.
func TestAClickReachesTheAgentWholeOnRelease(t *testing.T) {
	t.Run("sgr", func(t *testing.T) {
		r := newMouseRig(t, sgrMouse, "")
		r.mouse(0, 4, 2, false)
		if got := r.got(); got != "" {
			t.Fatalf("the press reached the agent before the release: %q", got)
		}
		r.mouse(0, 4, 2, true)
		if got, want := r.got(), "\x1b[<0;5;3M\x1b[<0;5;3m"; got != want {
			t.Fatalf("agent got %q, want %q", got, want)
		}
	})
	t.Run("normal", func(t *testing.T) {
		r := newMouseRig(t, normalMouse, "")
		r.mouse(0, 4, 2, false)
		r.mouse(0, 4, 2, true)
		// Normal encoding: ESC [ M, then button+32, x+33, y+33; a release is button 3.
		want := "\x1b[M" + string(rune(32)) + string(rune(4+33)) + string(rune(2+33)) +
			"\x1b[M" + string(rune(3+32)) + string(rune(4+33)) + string(rune(2+33))
		if got := r.got(); got != want {
			t.Fatalf("agent got %q, want %q", got, want)
		}
	})
	t.Run("no mouse", func(t *testing.T) {
		r := newMouseRig(t, noMouse, "")
		r.mouse(0, 4, 2, false)
		r.mouse(0, 4, 2, true)
		if got := r.got(); got != "" {
			t.Fatalf("a pane that reads no mouse got %q", got)
		}
	})
}
