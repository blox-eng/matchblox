package app

import (
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// cardState is the fixture machine where flint-auth asks "Which way do we
// fix the login?" with two options, drives #4551 in CI, and is in the
// chain login with flint-ui.
func cardState(edit func(*sample.Session)) (proto.State, sample.Session) {
	s := boardMatch("flint-auth", time.Hour)
	s.Agent, s.Context, s.ContextPct, s.Tokens = "claude", "known", 62, 124000
	s.LastLine = "I can fix the login in two ways."
	s.Ask = &said.Ask{ID: "toolu_login", Questions: []said.Question{{Question: "Which way do we fix the login?",
		Options: []said.Option{{Label: "Patch the token check"}, {Label: "Move to the new session store"}}}}}
	if edit != nil {
		edit(&s)
	}
	ui := boardMatch("flint-ui", 2*time.Hour)
	st := boardState(s, ui)
	st.Queue = []queue.Item{waitFor(s, queue.StateQuestion, 2*time.Minute)}
	st.Ships.Ships = []proto.Ship{shipFor(s, 4551, shipRunning, "CI · 6 of 9 jobs", 1, 4*time.Minute)}
	st.Chains = &state.ChainsView{Chains: []chains.Chain{{ID: "c1", Name: "login", Repo: "/home/user/src/hearth",
		Members: []chains.Member{memberOf(s), memberOf(ui)}}}}
	return st, s
}

func plainLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = ansi.Strip(l)
	}
	return out
}
