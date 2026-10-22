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
	if Cells("zen") != "" || Cells("gemini") != "" { // OpenCode names Gemini's provider google
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
// client on the console's session is kitty or Ghostty; a phone, a client
// that names no terminal, or any other terminal attached there turns them
// off. The key changes when the clients do, and when one comes to the
// console's window, so the logos are sent again to a client that can now
// show them.
func TestViewersDrawOnlyWhenEveryClientCan(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	inTmux := env(map[string]string{"TMUX": "/tmp/tmux-501/default,1,0", "TMUX_PANE": "%7"})
	session, clients := "my work\t@1\n", ""
	tmux := func(args ...string) ([]byte, error) {
		if args[0] == "display" {
			return []byte(session), nil
		}
		return []byte(clients), nil
	}
	clients = "/dev/ttys002\t100\txterm-kitty\tmy work\t@1\n/dev/ttys009\t109\txterm-256color\tdev\t@4\n"
	can, here, _ := Viewers(inTmux, tmux)
	if !can {
		t.Fatal("kitty alone on the console's session (named with a space) cannot draw")
	}
	clients = "/dev/ttys002\t100\txterm-kitty\tmy work\t@2\n"
	if can, away, _ := Viewers(inTmux, tmux); !can || away == here {
		t.Fatalf("a client on another window: can %v, key %q unchanged", can, away)
	}
	clients = "/dev/ttys002\t100\txterm-kitty\tmy work\t@1\n/dev/ttys004\t104\txterm-ghostty\tmy work\t@1\n"
	if can, two, _ := Viewers(inTmux, tmux); !can || two == here {
		t.Fatalf("kitty and Ghostty: can %v, key %q unchanged", can, two)
	}
	// A new kitty client on a departed one's tty needs the images too.
	clients = "/dev/ttys002\t200\txterm-kitty\tmy work\t@1\n"
	if can, again, _ := Viewers(inTmux, tmux); !can || again == here {
		t.Fatalf("a new client on a reused tty: can %v, key %q unchanged", can, again)
	}
	for name, c := range map[string]string{
		"a phone":           "/dev/ttys002\t100\txterm-kitty\tmy work\t@1\n/dev/ttys005\t105\txterm-256color\tmy work\t@1\n",
		"a nameless client": "/dev/ttys002\t100\txterm-kitty\tmy work\t@1\n/dev/ttys006\t106\t\tmy work\t@1\n",
		"no client":         "",
	} {
		clients = c
		can, _, watched := Viewers(inTmux, tmux)
		if can {
			t.Errorf("%s on the console's session: logos drawn", name)
		}
		if watched != (name != "no client") {
			t.Errorf("%s: watched %v", name, watched)
		}
	}
	if can, _, _ := Viewers(env(map[string]string{"TERM": "xterm-kitty"}), nil); !can {
		t.Fatal("kitty outside tmux cannot draw")
	}
	if can, _, _ := Viewers(env(map[string]string{"TERM": "xterm-256color"}), nil); can {
		t.Fatal("a plain terminal draws")
	}
	// NO_COLOR strips the colour that names the image: blank cells.
	if can, _, _ := Viewers(env(map[string]string{"TERM": "xterm-kitty", "NO_COLOR": "1"}), nil); can {
		t.Fatal("logos drawn under NO_COLOR")
	}
}

// TestEveryChunkIsQuiet: a terminal that reads q from the last chunk
// still answers nothing into the console's input.
func TestEveryChunkIsQuiet(t *testing.T) {
	for _, apc := range strings.Split(Transmit(true, false), "\x1b_G")[1:] {
		if head, _, _ := strings.Cut(apc, ";"); !strings.Contains(head, "q=2") {
			t.Fatalf("a chunk without q=2: %q", head)
		}
	}
}
