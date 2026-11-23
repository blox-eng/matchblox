package app

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/queue"
)

func TestTheHeaderCountsDone(t *testing.T) {
	ask, asks := boardMatch("flint-auth", time.Hour), boardMatch("ember-billing", time.Hour)
	waits := []queue.Item{waitFor(ask, queue.StateQuestion, time.Minute), waitFor(asks, queue.StatePermission, time.Minute)}
	st := boardState(ask, asks, doneMatch("kindle-web", time.Minute), doneMatch("ux-pass", 2*time.Minute), doneMatch("spark-rates", 3*time.Minute))
	st.Queue = waits
	line, hit, _ := boardModel(t, st).header(160)
	plain := ansi.Strip(line)
	at := strings.Index(plain, "● 2 need you · ✓ 3 done")
	if at < 0 {
		t.Fatalf("header %q, want ● 2 need you · ✓ 3 done", plain)
	}
	x := ansi.StringWidth(plain[:at])
	if hit.x0 != x || hit.x1 != x+len([]rune("● 2 need you")) {
		t.Errorf("a tap on who waits spans %d..%d, want %d..%d: only the waits", hit.x0, hit.x1, x, x+12)
	}
	cells := inkedCells(t, line)[0]
	for i, want := range map[int]string{x: "accent", x + 11: "accent", x + 13: "faint", x + 15: "match-ember", x + 19: "match-ember"} {
		if cells[i].fg != want {
			t.Errorf("cell %d %q is in %s, want %s", i, cells[i].ch, cells[i].fg, want)
		}
	}
	for _, w := range []int{44, 80} {
		line, _, _ := boardModel(t, st).header(w)
		if plain := ansi.Strip(line); !strings.Contains(plain, "● 2") || ansi.StringWidth(plain) > w {
			t.Errorf("at %d cells the header is %q (%d cells): want ● 2, and no wider", w, plain, ansi.StringWidth(plain))
		}
	}

	st = boardState(doneMatch("kindle-web", time.Minute), doneMatch("ux-pass", 2*time.Minute))
	line, _, _ = boardModel(t, st).header(160)
	if plain := ansi.Strip(line); !strings.Contains(plain, "✓ 2 done") || strings.Contains(plain, "need you") || strings.Contains(plain, "nothing") {
		t.Errorf("with nothing waiting and two done the header is %q: want ✓ 2 done alone", plain)
	}

	st = boardState(ask, asks, boardMatch("cinder-ci", time.Hour))
	st.Queue = waits
	line, _, _ = boardModel(t, st).header(160)
	if plain := ansi.Strip(line); !strings.Contains(plain, "● 2 need you") || strings.Contains(plain, "✓") {
		t.Fatalf("with nothing done the header is %q: want ● 2 need you and no ✓", plain)
	}
}
