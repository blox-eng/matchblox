package app

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/remote"
)

// withLayers puts the named layers on m, as their state alone: what focus reads.
func withLayers(m Model, layers ...string) Model {
	for _, l := range layers {
		switch l {
		case "pair":
			m.pair = &pairPage{} // beat 0: the page owns the keys
		case "strip":
			m.pair = &pairPage{beat: 3}
		case "sheet":
			m.sheet = &sheet{}
		case "form":
			m.form = &form{}
		case "search":
			m.searching = true
		case "held":
			m.held = func(m Model) (tea.Model, tea.Cmd) { return m, nil }
		case "pending":
			m.pending = &action{label: "close", say: "close it"}
		case "blocked":
			m.blocked = errors.New("unreachable")
		case "hostkey":
			m.blocked = remote.ErrHostKeyChanged
		case "scan":
			m.scanAsk = &scanAsk{}
		case "mismatch":
			m.mismatch = true
		default:
			panic("no layer " + l)
		}
	}
	return m
}

func TestFocusOrder(t *testing.T) {
	everything := []string{"pair", "sheet", "form", "search", "held", "pending", "blocked", "mismatch"}
	type row struct {
		name   string
		from   func(t *testing.T) Model
		layers []string
		key    string
		want   layerID
	}
	main := func(t *testing.T) Model { return routeAt(t, queueState()) }
	night := func(t *testing.T) Model { return routeAt(t, nightState()) }
	preview := func(t *testing.T) Model {
		m := routeAt(t, doorState(longDoor()), "enter")
		if m.pending == nil || m.previewSeen() {
			t.Fatal("no unread diff")
		}
		return m
	}
	rows := []row{
		{"nothing open: main", main, nil, "x", layerMain},
		{"nothing open, ctrl+c: main", main, nil, "ctrl+c", layerMain},
		{"every layer at once: pair", main, everything, "esc", layerPair},
		{"every layer at once, ctrl+c: pair", main, everything, "ctrl+c", layerPair},
		{"every layer at night: pair", night, everything, "enter", layerPair},
		{"pair over sheet", main, []string{"pair", "sheet"}, "x", layerPair},
		{"pair strip, esc: the strip", main, []string{"strip"}, "esc", layerPairStripEsc},
		{"pair strip, another key: main", main, []string{"strip"}, "x", layerMain},
		{"pair strip under a sheet, esc: the sheet", main, []string{"strip", "sheet"}, "esc", layerSheet},
		{"pair strip under search, esc: search", main, []string{"strip", "search"}, "esc", layerSearch},
		{"pair strip under pending, esc: pending", main, []string{"strip", "pending"}, "esc", layerPending},
		{"pair strip under held, esc: held", main, []string{"strip", "held"}, "esc", layerHeld},
		{"sheet and form: sheet", main, []string{"sheet", "form"}, "esc", layerSheet},
		{"sheet and search: sheet", main, []string{"sheet", "search"}, "x", layerSheet},
		{"form and search: form", main, []string{"form", "search"}, "x", layerForm},
		{"search, held and pending: search", main, []string{"search", "held", "pending"}, "y", layerSearch},
		{"held and pending: held", main, []string{"held", "pending"}, "y", layerHeld},
		{"held, ctrl+c: held", main, []string{"held"}, "ctrl+c", layerHeld},
		{"held and blocked, enter: held", main, []string{"held", "blocked"}, "enter", layerHeld},
		{"night, ctrl+o: take over", night, nil, "ctrl+o", layerTakeOver},
		{"night, a look: main", night, nil, "j", layerMain},
		{"night and blocked, ctrl+o: take over", night, []string{"blocked"}, "ctrl+o", layerTakeOver},
		{"night and mismatch, ctrl+o: take over", night, []string{"mismatch"}, "ctrl+o", layerTakeOver},
		{"night with pending, enter: pending", night, []string{"pending"}, "enter", layerPending},
		{"night with held, enter: held", night, []string{"held"}, "enter", layerHeld},
		{"preview, j: preview", preview, nil, "j", layerPreview},
		{"preview, n: pending", preview, nil, "n", layerPending},
		{"preview, ctrl+c: pending", preview, nil, "ctrl+c", layerPending},
		{"preview with held, j: held", preview, []string{"held"}, "j", layerHeld},
		{"blocked, enter: blocked", main, []string{"blocked"}, "enter", layerBlocked},
		{"host key changed, enter: blocked", main, []string{"hostkey"}, "enter", layerBlocked},
		{"mismatch, enter: blocked", main, []string{"mismatch"}, "enter", layerBlocked},
		{"blocked, another key: main", main, []string{"blocked"}, "x", layerMain},
		{"blocked and pending, enter: pending", main, []string{"blocked", "pending"}, "enter", layerPending},
		{"pending, ctrl+c: pending", main, []string{"pending"}, "ctrl+c", layerPending},
		{"scan, y: scan", main, []string{"scan"}, "y", layerScan},
		{"scan and pending, y: pending", main, []string{"scan", "pending"}, "y", layerPending},
		{"scan and pending, enter: pending", main, []string{"scan", "pending"}, "enter", layerPending},
		{"night with scan, ctrl+o: take over", night, []string{"scan"}, "ctrl+o", layerTakeOver},
		{"scan and blocked, enter: scan", main, []string{"scan", "blocked"}, "enter", layerScan},
		{"held and scan: held", main, []string{"held", "scan"}, "y", layerHeld},
		{"search and scan: search", main, []string{"search", "scan"}, "y", layerSearch},
		{"form and scan: form", main, []string{"form", "scan"}, "y", layerForm},
		{"sheet and scan: sheet", main, []string{"sheet", "scan"}, "y", layerSheet},
		{"pair and scan: pair", main, []string{"pair", "scan"}, "y", layerPair},
		{"pair strip and scan, esc: scan", main, []string{"strip", "scan"}, "esc", layerScan},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			m := withLayers(r.from(t), r.layers...)
			k := keyPress(r.key)
			if r.key == "ctrl+c" {
				k = ctrlC
			}
			if got := m.focus(k); got != r.want {
				t.Fatalf("focus %v, want %v", got, r.want)
			}
		})
	}
}
