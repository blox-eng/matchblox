// Package keycmd is the one table of agent slash commands. The console and the
// iOS app read the same JSON (linked at testdata/keys/commands.json).
package keycmd

import (
	"cmp"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/blox-eng/matchblox/internal/sample"
)

type Command struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Agent string `json:"agent"`
	Mark  string `json:"mark"`
	Run   string `json:"run"` // "compact" or "type"
	Text  string `json:"text"`
	Slot  int    `json:"slot"`
}

//go:embed commands.json
var raw []byte

var table = func() []Command {
	var cs []Command
	if err := json.Unmarshal(raw, &cs); err != nil {
		panic("keycmd: commands.json: " + err.Error())
	}
	return cs
}()

func All() []Command { return append([]Command(nil), table...) }

func ForAgent(agent string) []Command {
	var out []Command
	for _, c := range table {
		if c.Agent == agent {
			out = append(out, c)
		}
	}
	return out
}

func ByID(id string) (Command, bool) {
	for _, c := range table {
		if c.ID == id {
			return c, true
		}
	}
	return Command{}, false
}

func Sequence(id string) string { return "\x1b_matchblox;cmd=" + id + "\x1b\\" }

func AgentSignal(agent string) string { return "\x1b_matchblox;agent=" + agent + "\x1b\\" }

// PaneSignal tells the app which tmux pane agent's chat is in.
func PaneSignal(agent, pane string) string { return AgentSignal(agent + ";pane=" + pane) }

// MarksSignal is the console's announce that it reads the marker.
func MarksSignal() string { return AgentSignal(";marks") }

const (
	// MarkKey is what the app sends before a Return or a press, so tmux
	// hands the console the tty of the client that sent it.
	MarkKey = "\x1b_matchblox;mark\x1b\\"
	// MarkSlot is MarkKey's user-keys slot (900 is home, 901-914 commands).
	MarkSlot = 950
	// MarkedPrefix is what the binding hands the console, then the tty, then ESC \.
	MarkedPrefix = "\x1b_matchblox;from="
)

// OtherAgent is the agent signal's id for a chat whose agent has no
// commands: the app's command keys still go, and the console says why
// each does not run there.
const OtherAgent = "other"

// agentNames are the agents of the table, as a key line names them.
var agentNames = map[string]string{"claude": "Claude Code", "codex": "Codex", "opencode": "OpenCode", "cursor": "Cursor"}

// AgentOf is the table's id of the agent a session runs.
func AgentOf(agent string) string {
	if agent == sample.CursorAgent {
		return "cursor"
	}
	return agent
}

// Refusal says why c does not run on session, whose agent is not c's.
func Refusal(c Command, session, agent string) string {
	runs := cmp.Or(agentNames[AgentOf(agent)], agent, "no agent matchblox knows")
	return fmt.Sprintf("/%s is %s's: %s runs %s", c.Label, agentNames[c.Agent], session, runs)
}
