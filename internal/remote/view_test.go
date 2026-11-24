package remote

import (
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/sample"
)

func TestViewAllowed(t *testing.T) {
	ok := Jump{Pane: "%3", Prefix: "C-b", ID: "0123abcd"}
	cases := []struct {
		name   string
		j      Jump
		agent  string
		socket string
		want   bool
	}{
		{"this machine", ok, "kindle-web", "/tmp/tmux-1000/default", true},
		{"a dir is not a pane", Jump{Dir: "/tmp", Prefix: "C-b", ID: "0123abcd"}, "a", "/s", false},
		{"no pane", Jump{Prefix: "C-b", ID: "0123abcd"}, "a", "/s", false},
		{"a bad pane", Jump{Pane: "3", Prefix: "C-b", ID: "0123abcd"}, "a", "/s", false},
		{"a bad id", Jump{Pane: "%3", Prefix: "C-b", ID: "x"}, "a", "/s", false},
		{"a control char in the name", ok, "a\nb", "/s", false},
		{"no name", ok, "", "/s", false},
		{"a relative socket", ok, "a", "tmux/default", false},
		{"no socket", ok, "a", "", false},
		{"a hash in the socket", ok, "a", "/tmp/#x", false},
		{"a semicolon in the socket", ok, "a", "/tmp/a;b", false},
		{"a newline in the socket", ok, "a", "/tmp/a\nb", false},
	}
	for _, c := range cases {
		if got := ViewAllowed(c.j, c.agent, c.socket); got != c.want {
			t.Errorf("%s: ViewAllowed = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestPeerViewArgvKeepsTheNameAnArgument(t *testing.T) {
	argv := PeerViewArgv("kindle-web", []string{"ssh", "-t", "--", "ws-1", "a ; b"})
	if argv[0] != "/bin/sh" || argv[1] != "-c" || argv[3] != "sh" || argv[4] != "kindle-web" || argv[5] != "ssh" {
		t.Fatalf("argv %q", argv)
	}
	if strings.Contains(argv[2], "kindle-web") {
		t.Fatalf("the name is in the script: %q", argv[2])
	}
}

func TestOnlyAViewEndsWithItsAgentsPane(t *testing.T) {
	j := Jump{Pane: "%3", Prefix: "C-b", ID: "0123abcd", Outer: true}
	if got := strings.Join(j.Remote(), " "); strings.Contains(got, "set-hook") || strings.Contains(got, sample.ViewOption) {
		t.Fatalf("a ctrl+o jump changed: %q", got)
	}
	j.View = true
	got := strings.Join(j.Remote(), " ")
	for _, want := range []string{"set-option -t =matchblox-jump-0123abcd: " + sample.ViewOption + " %3", "set-hook -t =matchblox-jump-0123abcd: session-window-changed"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in %q", want, got)
		}
	}
}

// A name that ends in ";" would end tmux's command on the split's command
// line: the view's argv would break there.
func TestAViewNameEndingInASemicolonIsRefused(t *testing.T) {
	for _, name := range []string{"foo;", ";", "a b;"} {
		if ViewNameAllowed(name) {
			t.Errorf("%q allowed", name)
		}
	}
	for _, name := range []string{"foo;bar", "a;b c", `it's "$(x)" 100%`} {
		if !ViewNameAllowed(name) {
			t.Errorf("%q refused", name)
		}
	}
}

// TestAPeerViewIsToldItDrawsTrueColour: matchblox's screen draws true
// colour, which no terminfo tells a view's tmux on the peer, so its ssh
// line runs tmux -T RGB; a ctrl+o jump draws in the owner's own terminal
// and is told nothing.
func TestAPeerViewIsToldItDrawsTrueColour(t *testing.T) {
	j := Jump{Host: "ws-1", Pane: "%3", Prefix: "C-b", ID: "0123abcd", View: true}
	if got := JumpSSH("ws-1", j, nil); !strings.Contains(got[len(got)-1], " tmux -T RGB bind-key ") {
		t.Errorf("the view's ssh line runs %q, want tmux -T RGB", got[len(got)-1])
	}
	j.View = false
	if got := JumpSSH("ws-1", j, nil); strings.Contains(got[len(got)-1], "RGB") {
		t.Errorf("a jump's ssh line runs %q: it is told RGB", got[len(got)-1])
	}
}
