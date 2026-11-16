package remote

import (
	"strings"
	"testing"
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
