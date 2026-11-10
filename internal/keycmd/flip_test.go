package keycmd

import "testing"

func TestFlipAPCsRoundTrip(t *testing.T) {
	for _, c := range []struct {
		apc, kind, arg string
	}{
		{FlipAPC("/dev/pts/3 %171"), "flip", "/dev/pts/3 %171"},
		{NextAPC("%171"), "next", "%171"},
		{FlipAPC(""), "flip", ""},
		{OpenAPC("ws-1", 3), "open", "ws-1/3"},
	} {
		kind, arg, ok := ParseAPC(c.apc)
		if !ok || kind != c.kind || arg != c.arg {
			t.Errorf("ParseAPC(%q) = %q, %q, %v; want %q, %q", c.apc, kind, arg, ok, c.kind, c.arg)
		}
	}
	if got, want := FlipAPC("%171"), "\x1b_matchblox;flip=%171\x1b\\"; got != want {
		t.Errorf("FlipAPC = %q, want %q", got, want)
	}
	if got, want := NextAPC("%171"), "\x1b_matchblox;next=%171\x1b\\"; got != want {
		t.Errorf("NextAPC = %q, want %q", got, want)
	}
	if got, want := OpenAPC("ws-1", 3), "\x1b_matchblox;open=ws-1/3\x1b\\"; got != want {
		t.Errorf("OpenAPC = %q, want %q", got, want)
	}
	if FlipPrefix("flip")+"x\x1b\\" != FlipAPC("x") {
		t.Errorf("FlipPrefix does not start FlipAPC")
	}
}

func TestParseAPCRefusesOtherStrings(t *testing.T) {
	for _, s := range []string{"\x1b_matchblox\x1b\\", "\x1b_matchblox;flip\x1b\\", "\x1b_matchblox;burn=%1\x1b\\", "flip=%1", "\x1b_other;flip=%1\x1b\\"} {
		if kind, arg, ok := ParseAPC(s); ok {
			t.Errorf("ParseAPC(%q) = %q, %q, true; want false", s, kind, arg)
		}
	}
}
