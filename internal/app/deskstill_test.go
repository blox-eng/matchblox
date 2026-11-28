package app

import (
	"bytes"
	"flag"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/replay"
)

var updateStill = flag.Bool("update-still", false, "write the four-pane desk still into www/index.html")

const (
	deskStillCols  = 104 // the landing's open Duo is 104 columns
	deskStillListW = 32
	deskStillRows  = 28
)

// TestDeskStillIsFourPanes: the landing still is the header, the list, the
// agent's own pane capture, and the bar — not a drawn sidebar chat.
func TestDeskStillIsFourPanes(t *testing.T) {
	view := deskStillView(t)
	plain := ansi.Strip(view)
	for _, want := range []string{"? keys", "NEEDS YOU", "Do you want to proceed"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("desk still lacks %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "send to") {
		t.Fatalf("desk still drew the sidebar chat:\n%s", plain)
	}
	tokens := Tokens(true)
	lines, err := replay.Parse(view, tokens)
	if err != nil {
		t.Fatal(err)
	}
	html := replay.Still(replay.Frame{Lines: lines})
	index := "../../www/index.html"
	page, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	const open, closing = "<!-- replay:still -->", "<!-- /replay:still -->"
	i, j := bytes.Index(page, []byte(open)), bytes.Index(page, []byte(closing))
	if i < 0 || j < i {
		t.Fatalf("%s needs the still markers", index)
	}
	if *updateStill {
		out := append(append(append([]byte{}, page[:i+len(open)]...), "\n"+html+"\n"...), page[j:]...)
		if err := os.WriteFile(index, out, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got := string(page[i+len(open) : j])
	if !strings.Contains(got, "? keys") || strings.Contains(got, "send to") {
		t.Fatal("www/index.html still is not the desk: run go test ./internal/app -run TestDeskStillIsFourPanes -update-still")
	}
}

func deskStillView(t *testing.T) string {
	t.Helper()
	st := boardFixture(t)
	keysLongAgo(&st)
	list := boardShot(t, st, deskStillListW, deskStillRows, func(m *Model) {
		m.deskFrame = true
	})
	h := newFrame("header", New(Options{NoMotion: true, Conn: newFake(), Now: list.opt.Now}))
	h, _ = frameAt(h, tea.WindowSizeMsg{Width: deskStillCols, Height: 1})
	h, _ = frameAt(h, stateMsg(proto.State{Snapshot: list.snap, KeysAt: st.KeysAt}))
	bar := newFrame("bar", list)
	bar.width = deskStillCols
	bar.barMode = "LIST"

	agentW := deskStillCols - deskStillListW - 1
	raw, err := os.ReadFile("../../testdata/demo/panes/5.txt")
	if err != nil {
		t.Fatal(err)
	}
	agent := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	left := strings.Split(list.render(), "\n")
	body := joinDesk(left, agent, deskStillListW, agentW, deskStillRows)
	return h.View().Content + "\n" + strings.Join(body, "\n") + "\n" + bar.View().Content
}

func joinDesk(left, right []string, leftW, rightW, rows int) []string {
	out := make([]string, rows)
	div := "│"
	for i := range rows {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out[i] = fit(l, leftW) + div + fit(r, rightW)
	}
	return out
}
