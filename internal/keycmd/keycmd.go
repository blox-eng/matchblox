// Package keycmd is the one table of agent slash commands. The console and the
// iOS app read the same JSON (linked at testdata/keys/commands.json).
package keycmd

import (
	_ "embed"
	"encoding/json"
)

type Command struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Agent string `json:"agent"`
	Mark  string `json:"mark"`
	Run   string `json:"run"` // "compact", "clear" or "type"
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
