package said

import (
	"strings"
	"testing"
)

func TestScrubRemovesAShortUserAndHost(t *testing.T) {
	const screen = "al@box:~/work$ ls /home/al/work/x.go\nAllow this?\n 1. Yes\n"
	got := Scrub(screen, []string{"/home/al/work", "/home/al"}, []string{"box", "al"})
	for _, s := range []string{"/home/al", "al@", "@box"} {
		if strings.Contains(got, s) {
			t.Errorf("Scrub left %q in:\n%s", s, got)
		}
	}
	for _, s := range []string{"<user>@<host>", "<dir>/x.go", "Allow this?", "1. Yes"} {
		if !strings.Contains(got, s) {
			t.Errorf("Scrub wrote no %q in:\n%s", s, got)
		}
	}
}

func TestScrubKeepsWordsThatHoldTheName(t *testing.T) {
	const screen = " Do you want to proceed?\n ❯ 1. Yes\n"
	for _, name := range []string{"ye", "o"} {
		if got := Scrub(screen, nil, []string{name, name}); got != screen {
			t.Errorf("Scrub with the name %q changed the screen:\n%s", name, got)
		}
	}
}

// TestScrubRemovesTheShortHost: a prompt prints the host name only up to
// its first dot, so a dotted host name leaves that part on the screen.
func TestScrubRemovesTheShortHost(t *testing.T) {
	got := Scrub("al@box:~/dev$ ssh box.lan\n", nil, []string{"box.lan", "al"})
	if want := "<user>@<host>:~/dev$ ssh <host>\n"; got != want {
		t.Errorf("Scrub = %q, want %q", got, want)
	}
}
