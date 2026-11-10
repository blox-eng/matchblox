package keycmd

import (
	"strconv"
	"strings"
)

// The flip's APC strings carry one word and its argument to the console, as
// HomeKey carries the way back: no keyboard types an APC.
const (
	apcHead = "\x1b_matchblox;"
	apcTail = "\x1b\\"
)

// FlipAPC tells the console to flip from pane, for the client whose tty it
// names: "<tty> <pane>". "" means from the selected session, as the
// phone's button sends it at the console.
func FlipAPC(from string) string { return apc("flip", from) }

// NextAPC tells the console to walk on from a pane, as FlipAPC.
func NextAPC(from string) string { return apc("next", from) }

// FlipPrefix is the start of FlipAPC and NextAPC: the tmux binding sends
// it, then the tty and pane, then ESC \.
func FlipPrefix(kind string) string { return apcHead + kind + "=" }

// FlipSlot and NextSlot are the user-keys slots of the phone's flip and
// next buttons, below the home key's 900.
const (
	FlipSlot = 898
	NextSlot = 899
)

// OpenAPC tells the console to open the room of chain n of host: the phone
// knows chains by number only (push.Numbers).
func OpenAPC(host string, n int) string { return apc("open", host+"/"+strconv.Itoa(n)) }

func apc(kind, arg string) string { return apcHead + kind + "=" + arg + apcTail }

// ParseAPC reads back a string FlipAPC, NextAPC or OpenAPC made.
func ParseAPC(s string) (kind, arg string, ok bool) {
	body, ok := strings.CutPrefix(s, apcHead)
	if !ok {
		return "", "", false
	}
	if body, ok = strings.CutSuffix(body, apcTail); !ok {
		return "", "", false
	}
	kind, arg, ok = strings.Cut(body, "=")
	switch {
	case !ok:
		return "", "", false
	case kind == "flip", kind == "next", kind == "open":
		return kind, arg, true
	}
	return "", "", false
}

// FeelNeeds and FeelDone tell the phone to play a haptic: a match on the
// console's host started to ask, or one finished. The console writes them as
// it writes the agent signal; the app reads them and plays at most one each
// 2 s.
const (
	FeelNeeds = "\x1b_matchblox;feel=needs\x1b\\"
	FeelDone  = "\x1b_matchblox;feel=done\x1b\\"
)
