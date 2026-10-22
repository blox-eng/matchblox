package logos

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestCellsAreTwoColumnsNamingTheImage: a logo is two placeholder cells
// whose colour is its image id, so a layout counts it as two columns.
func TestCellsAreTwoColumnsNamingTheImage(t *testing.T) {
	for _, p := range []string{"anthropic", "openai", "cursor", "opencode", "google"} {
		c := Cells(p)
		if w := ansi.StringWidth(c); w != 2 {
			t.Errorf("%s: width %d, want 2", p, w)
		}
		if !strings.Contains(c, "\x1b[38;5;") || strings.Count(c, string(placeholder)) != 2 {
			t.Errorf("%s: cells %q", p, c)
		}
	}
	if Cells("zen") != "" {
		t.Error("a provider without a logo has no cells")
	}
	if Cells("anthropic") == Cells("openai") {
		t.Error("two providers share an image")
	}
}

// TestTransmitSendsEveryLogoInChunks: each image goes in chunks of at most
// 4096 bytes of base64, as kitty wants; in tmux each chunk is wrapped for
// passthrough with its escapes doubled.
func TestTransmitSendsEveryLogoInChunks(t *testing.T) {
	raw := Transmit(true, false)
	for _, p := range []string{"anthropic", "openai", "cursor", "opencode", "google"} {
		id := idOf(p)
		if !strings.Contains(raw, "\x1b_Ga=T,U=1,f=100,i="+id+",") {
			t.Errorf("%s (image %s) is not sent", p, id)
		}
	}
	for _, apc := range strings.Split(raw, "\x1b_G")[1:] {
		_, data, _ := strings.Cut(apc, ";")
		data, _, _ = strings.Cut(data, "\x1b\\")
		if len(data) > 4096 {
			t.Fatalf("a chunk of %d bytes", len(data))
		}
	}
	tmux := Transmit(true, true)
	if !strings.HasPrefix(tmux, "\x1bPtmux;\x1b\x1b_G") || strings.Count(tmux, "\x1bPtmux;") != strings.Count(raw, "\x1b_G") {
		t.Fatalf("tmux wrapping: %q", tmux[:40])
	}
	if Transmit(true, false) == Transmit(false, false) {
		t.Error("a light terminal gets the same logos as a dark one")
	}
}

// TestViewersDrawOnlyWhenEveryClientCan: in tmux, logos draw while every
// client on the console's session is kitty or Ghostty; a phone, or any
// other terminal, attached there turns them off. The key changes when
// the clients do, so the logos are sent again to a new one.
func TestViewersDrawOnlyWhenEveryClientCan(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	inTmux := env(map[string]string{"TMUX": "/tmp/tmux-501/default,1,0", "TMUX_PANE": "%7"})
	clients := ""
	tmux := func(args ...string) ([]byte, error) {
		if args[0] == "display" {
			return []byte("matchblox\n"), nil
		}
		return []byte(clients), nil
	}
	clients = "/dev/ttys002 xterm-kitty matchblox\n/dev/ttys009 xterm-256color dev\n"
	can, key := Viewers(inTmux, tmux)
	if !can {
		t.Fatal("kitty alone on the console's session cannot draw")
	}
	clients = "/dev/ttys002 xterm-kitty matchblox\n/dev/ttys004 xterm-ghostty matchblox\n"
	if can, key2 := Viewers(inTmux, tmux); !can || key2 == key {
		t.Fatalf("kitty and Ghostty: can %v, key %q unchanged", can, key2)
	}
	clients = "/dev/ttys002 xterm-kitty matchblox\n/dev/ttys005 xterm-256color matchblox\n"
	if can, _ := Viewers(inTmux, tmux); can {
		t.Fatal("a phone on the console's session would see boxes")
	}
	clients = ""
	if can, _ := Viewers(inTmux, tmux); can {
		t.Fatal("no client: nothing to draw on")
	}
	if can, _ := Viewers(env(map[string]string{"TERM": "xterm-kitty"}), nil); !can {
		t.Fatal("kitty outside tmux cannot draw")
	}
	if can, _ := Viewers(env(map[string]string{"TERM": "xterm-256color"}), nil); can {
		t.Fatal("a plain terminal draws")
	}
}
