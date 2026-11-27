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

// TestParseJumpAPC: the jump binding's APC carries a digit 1 to 9, the
// client's tty and the pane; anything less is not a jump.
func TestParseJumpAPC(t *testing.T) {
	apc := JumpPrefix(4) + "/dev/pts/3 %171\x1b\\"
	if kind, arg, ok := ParseAPC(apc); !ok || kind != "jump" || arg != "4 /dev/pts/3 %171" {
		t.Errorf("ParseAPC(%q) = %q, %q, %v; want jump, 4 /dev/pts/3 %%171", apc, kind, arg, ok)
	}
	for _, arg := range []string{"0 /dev/pts/3 %1", "10 /dev/pts/3 %1", "x /dev/pts/3 %1", "4 /dev/pts/3", "4", "", "4 /dev/pts/3 "} {
		if kind, got, ok := ParseAPC(FlipPrefix("jump") + arg + "\x1b\\"); ok {
			t.Errorf("ParseAPC(jump=%q) = %q, %q, true; want false", arg, kind, got)
		}
	}
}
