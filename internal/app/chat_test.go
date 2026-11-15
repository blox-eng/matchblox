package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

func chatText(m Model, w, h int) string {
	return strings.Join(plainLines(m.chatPanel(w, h)), "\n")
}

// TestChatWatchesAPeersPane: the chosen session is on ws-2, so the watch
// goes on ws-2's own conn and the home conn gets none. A screen of ws-2
// shows; a screen of the same pane id from home does not.
func TestChatWatchesAPeersPane(t *testing.T) {
	home, ws := newFake(), newFake()
	var next tea.Model = New(Options{NoMotion: true, Conn: home, Peers: []Peer{{Host: "ws-2", Conn: ws}}})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 132, Height: 30})
	next, _ = next.Update(stateMsg(hostState()))
	peer := hostState(sess(4242, "flint-auth", "%7", func(s *sample.Session) { s.Agent = "claude" }))
	next, cmd := next.Update(peerMsg{host: "ws-2", conn: ws, msg: stateMsg(peer)})
	drain(cmd)

	if !hasAct(ws, proto.WatchPrefix+"%7") {
		t.Fatalf("ws-2 got %+v, no watch of %%7", ws.allActs())
	}
	for _, a := range home.allActs() {
		if strings.HasPrefix(a.RecID, proto.WatchPrefix) {
			t.Fatalf("the home conn got %q", a.RecID)
		}
	}

	next, _ = next.Update(peerMsg{host: "ws-2", conn: ws, msg: screenMsg{Pane: "%7", At: time.Now(), Text: "● the screen of ws-2"}})
	next, _ = next.Update(screenMsg{Pane: "%7", At: time.Now(), Text: "● the screen of home"})
	m := next.(Model)
	if got := chatText(m, 80, 12); !strings.Contains(got, "the screen of ws-2") || strings.Contains(got, "the screen of home") {
		t.Fatalf("chat:\n%s", got)
	}

	m.peers[m.peerAt("ws-2")].lost = true
	if got := chatText(m, 80, 12); !strings.Contains(got, "ws-2 is not reached.") || strings.Contains(got, "the screen of ws-2") {
		t.Fatalf("chat of a lost host:\n%s", got)
	}
}

// TestAPeersRefusedWatchIsHeld: a peer that refuses the watch of a pane
// that went is not asked again until its next state.
func TestAPeersRefusedWatchIsHeld(t *testing.T) {
	home, ws := newFake(), newFake()
	var next tea.Model = New(Options{NoMotion: true, Conn: home, Peers: []Peer{{Host: "ws-2", Conn: ws}}})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 132, Height: 30})
	next, _ = next.Update(stateMsg(hostState()))
	next, cmd := next.Update(peerMsg{host: "ws-2", conn: ws, msg: stateMsg(hostState(sess(4242, "flint-auth", "%7", nil)))})
	drain(cmd)
	next, cmd = next.Update(peerMsg{host: "ws-2", conn: ws, msg: resultMsg{ActID: watchActID + "%7", Err: "no pane %7"}})
	drain(cmd)
	if m := next.(Model); m.watchHeld != "%7" || m.watching != "" || m.flash != "" {
		t.Fatalf("held %q watching %q flash %q", m.watchHeld, m.watching, m.flash)
	}
	if n := len(ws.allActs()); n != 1 {
		t.Fatalf("ws-2 got %d acts, want the one watch: %+v", n, ws.allActs())
	}
}

func hasAct(f *fakeConn, recID string) bool {
	for _, a := range f.allActs() {
		if a.RecID == recID {
			return true
		}
	}
	return false
}

func TestChatHasNoChainLineOutsideAChain(t *testing.T) {
	m := loginChain(t, nil)
	m.sideAt = paneKey{m.opt.Host, "%flint-auth"}
	lines := plainLines(m.chatPanel(100, 12))
	if got, want := strings.TrimSpace(lines[1]), "chain login · step 2 of 3   kindle-web done → flint-auth needs you → wick-mobile works"; got != want {
		t.Fatalf("chain line %q, want %q", got, want)
	}

	m = boardModel(t, boardState(boardMatch("hearth-api", time.Hour)))
	for _, l := range plainLines(m.chatPanel(100, 12)) {
		if strings.HasPrefix(strings.TrimSpace(l), "chain ") {
			t.Fatalf("a session in no chain has the line %q", l)
		}
	}
}

func TestChatHeadAndLine(t *testing.T) {
	s := boardMatch("flint-auth", time.Hour)
	s.Context, s.ContextPct = "known", 58
	m := boardModel(t, boardState(s))
	lines := plainLines(m.chatPanel(100, 12))
	head := lines[0]
	for _, want := range []string{"flint-auth", harnessMark(s), "ctx 58%", m.keys.main.Jump.Help().Key + " expand · " + m.keys.board.Menu.Help().Key + " menu"} {
		if !strings.Contains(head, want) {
			t.Fatalf("head %q lacks %q", head, want)
		}
	}
	last := lines[len(lines)-1]
	if !strings.HasPrefix(strings.TrimSpace(last), "›") || !strings.HasSuffix(strings.TrimRight(last, " "), "send to flint-auth") {
		t.Fatalf("line %q", last)
	}
	for i, l := range m.chatPanel(100, 12) {
		if w := ansi.StringWidth(l); w != 100 {
			t.Fatalf("line %d is %d wide: %q", i, w, l)
		}
	}
}

// TestChatCutsTheAgentsInputBox: each agent's own input box and the
// status lines under it go; the conversation stays, up to its last line.
func TestChatCutsTheAgentsInputBox(t *testing.T) {
	for _, c := range []struct {
		agent, file, last string
		gone              []string
	}{
		{"claude", "claude.txt", "● The handler keeps the token in memory. I add a store for the refresh token.", []string{"? for shortcuts", "│ >"}},
		{"codex", "codex.txt", "• The link points at /docs. I change it to /help.", []string{"Ask Codex to do anything", "context left"}},
		{"opencode", "opencode.txt", "⚙ edit docs/index.md", []string{"Ask anything", "Build  Claude", "ctrl+p commands", "▀▀▀"}},
	} {
		t.Run(c.agent, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "chat", c.file))
			if err != nil {
				t.Fatal(err)
			}
			got := fitScreen(chatCut(c.agent, string(b)), 100, 40)
			if len(got) == 0 || strings.TrimSpace(got[len(got)-1]) != c.last {
				t.Fatalf("the cut ends %q, want %q:\n%s", got[len(got)-1], c.last, strings.Join(got, "\n"))
			}
			for _, g := range c.gone {
				if strings.Contains(strings.Join(got, "\n"), g) {
					t.Fatalf("the cut keeps %q:\n%s", g, strings.Join(got, "\n"))
				}
			}

			s := boardMatch("flint-auth", time.Hour)
			s.Agent = c.agent
			m := boardModel(t, boardState(s))
			next, _ := m.Update(screenMsg{Pane: s.Pane, At: time.Now(), Text: string(b)})
			lines := plainLines(next.(Model).chatPanel(100, 30))
			if conv := strings.TrimSpace(lines[len(lines)-2]); conv != c.last {
				t.Fatalf("the chat's last conversation line %q, want %q:\n%s", conv, c.last, strings.Join(lines, "\n"))
			}
		})
	}
}

func TestChatCutKeepsAnUnknownAgentsPane(t *testing.T) {
	text := "● one\n╭──╮\n│ > │\n╰──╯\n  footer"
	if got := chatCut("cursor", text); got != text {
		t.Fatalf("cut %q", got)
	}
}

func TestNoSessionsSaysHowToStart(t *testing.T) {
	m := boardModel(t, boardState())
	want := "No sessions yet. + new session starts one here (N). matchblox setup finds the agents you already run."
	if got := chatText(m, 140, 10); !strings.Contains(got, want) {
		t.Fatalf("chat:\n%s", got)
	}
}
