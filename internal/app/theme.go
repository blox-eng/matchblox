package app

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme is near-monochrome and warm. Color is an event: the accent marks the
// selected row and the active panel only; status uses muted tints and always
// carries a glyph and a word, never hue alone.
type Theme struct {
	Text, Muted, Faint, Hair, Accent, AccentWash, Warn, Neg color.Color
}

var darkTheme = Theme{
	Text:       lipgloss.Color("#EBE6DC"),
	Muted:      lipgloss.Color("#A69D8D"),
	Faint:      lipgloss.Color("#776F60"),
	Hair:       lipgloss.Color("#2E2921"),
	Accent:     lipgloss.Color("#C3A56A"),
	AccentWash: lipgloss.Color("#221D14"),
	Warn:       lipgloss.Color("#B98F44"),
	Neg:        lipgloss.Color("#B0563F"),
}

var lightTheme = Theme{
	Text:       lipgloss.Color("#1C1813"),
	Muted:      lipgloss.Color("#5F5A4C"),
	Faint:      lipgloss.Color("#8A8170"),
	Hair:       lipgloss.Color("#DDD5C3"),
	Accent:     lipgloss.Color("#9A7B3F"),
	AccentWash: lipgloss.Color("#ECE3CF"),
	Warn:       lipgloss.Color("#8A6420"),
	Neg:        lipgloss.Color("#9E4A34"),
}

type styles struct {
	theme                                 Theme
	text, muted, faint, hair, accent      lipgloss.Style
	label, selected, warn, neg, tabActive lipgloss.Style
}

func newStyles(dark bool) styles {
	t := lightTheme
	if dark {
		t = darkTheme
	}
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	return styles{
		theme:     t,
		text:      fg(t.Text),
		muted:     fg(t.Muted),
		faint:     fg(t.Faint),
		hair:      fg(t.Hair),
		accent:    fg(t.Accent),
		label:     fg(t.Faint),
		selected:  lipgloss.NewStyle().Foreground(t.Text).Background(t.AccentWash),
		warn:      fg(t.Warn),
		neg:       fg(t.Neg),
		tabActive: fg(t.Accent).Underline(true),
	}
}

// The matchbox of the start screen, in the colours of the site's mark.
type markColours struct {
	Top, Left, Right, Tray, Striker, Stick, Flame, Core color.Color
	// The match of each session: the head at rest, the head as it breathes
	// (toward the ground, three steps), the ember of a dying or spent match,
	// and the amber of a flame between its orange and its core.
	Head, Ember, Amber color.Color
	Breath             [3]color.Color
}

var darkMark = markColours{
	Top: lipgloss.Color("#3A3328"), Left: lipgloss.Color("#1C1914"), Right: lipgloss.Color("#2A251D"),
	Tray: lipgloss.Color("#15120E"), Striker: lipgloss.Color("#5A4128"), Stick: lipgloss.Color("#C9A877"),
	Flame: lipgloss.Color("#E8822E"), Core: lipgloss.Color("#FFD27A"),
	Head: lipgloss.Color("#C2452D"), Ember: lipgloss.Color("#9C5A46"), Amber: lipgloss.Color("#F5A54A"),
	Breath: [3]color.Color{lipgloss.Color("#9E3A26"), lipgloss.Color("#7A2F1F"), lipgloss.Color("#572318")},
}

var lightMark = markColours{
	Top: lipgloss.Color("#EFE9DC"), Left: lipgloss.Color("#CFC6AF"), Right: lipgloss.Color("#DED6C3"),
	Tray: lipgloss.Color("#C4BAA2"), Striker: lipgloss.Color("#8C6A43"), Stick: lipgloss.Color("#B8925B"),
	// In daylight the pale core vanishes on the cream ground: a deeper one.
	Flame: lipgloss.Color("#E8822E"), Core: lipgloss.Color("#D69A2A"),
	Head: lipgloss.Color("#B23A24"), Ember: lipgloss.Color("#9E5E4A"), Amber: lipgloss.Color("#DF8E2C"),
	Breath: [3]color.Color{lipgloss.Color("#BF5F4B"), lipgloss.Color("#CD8372"), lipgloss.Color("#DAA899")},
}

// Tokens are the colours the console draws with, by name. The replay maps
// each colour of a frame to its token, so the site can draw it in its theme.
func Tokens(dark bool) map[string]color.Color {
	t, mk, bg := lightTheme, lightMark, lipgloss.Color("#F5F1E7")
	if dark {
		t, mk, bg = darkTheme, darkMark, lipgloss.Color("#0F0D0A")
	}
	return map[string]color.Color{
		"bg": bg, "fg": t.Text, "muted": t.Muted, "faint": t.Faint, "hair": t.Hair,
		"accent": t.Accent, "wash": t.AccentWash, "warn": t.Warn, "neg": t.Neg,
		"mark-top": mk.Top, "mark-left": mk.Left, "mark-right": mk.Right, "mark-tray": mk.Tray,
		"mark-striker": mk.Striker, "mark-stick": mk.Stick, "flame": mk.Flame, "flame-core": mk.Core,
		"flame-amber": mk.Amber, "match-head": mk.Head, "match-ember": mk.Ember,
		"match-breath-1": mk.Breath[0], "match-breath-2": mk.Breath[1], "match-breath-3": mk.Breath[2],
	}
}
