package panes

import (
	"strconv"
	"strings"
)

// FlipKey goes between a session and its chain's room; NextKey walks the
// chain's matches, then its stoker. No agent uses them (the key check,
// spec § The flip), and every pane that is not an agent's gets them as typed.
const (
	FlipKey = "M-s"
	NextKey = "M-n"
)

// The flip's APC strings carry one word and its argument to the console, as
// HomeKey carries the way back: no keyboard types an APC.
const (
	apcHead = "\x1b_" + Home + ";"
	apcTail = "\x1b\\"
)

// FlipAPC tells the console to flip from pane; "" means from the selected
// session, as the phone's button sends it.
func FlipAPC(pane string) string { return apc("flip", pane) }

// NextAPC tells the console to walk on from pane.
func NextAPC(pane string) string { return apc("next", pane) }

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
