package main

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// agentRow is one agent at one version. It carries no name, pane or folder:
// `matchblox agents` prints no home path, host name or user name.
type agentRow struct {
	Agent    string `json:"agent"`
	Version  string `json:"version"`
	Why      string `json:"why,omitempty"`
	Sessions int    `json:"sessions"`
	Unknown  int    `json:"unknown"`
}

// hookless agents tell matchblox nothing, so their sessions never show
// `unknown screen`.
var hookless = []string{"codex", "opencode"}

const agentsFix = "a screen matchblox cannot read: update matchblox, or report it with this table"

// agentRows groups the sessions by agent and version; an installed agent
// with no session is a row of 0.
func agentRows(doc state.Doc) []agentRow {
	var rows []agentRow
	for _, s := range doc.Sessions {
		i := slices.IndexFunc(rows, func(r agentRow) bool { return r.Agent == s.Agent && r.Version == s.Version })
		if i < 0 {
			rows = append(rows, agentRow{Agent: s.Agent, Version: s.Version})
			i = len(rows) - 1
		}
		r := &rows[i]
		r.Sessions++
		if s.UnknownScreen {
			r.Unknown++
		}
		if s.Version == "" && s.VersionWhy != "" && !slices.Contains(strings.Split(r.Why, "; "), s.VersionWhy) {
			r.Why = strings.TrimPrefix(r.Why+"; "+s.VersionWhy, "; ")
		}
	}
	for _, a := range doc.Agents {
		if !slices.ContainsFunc(rows, func(r agentRow) bool { return r.Agent == a }) {
			rows = append(rows, agentRow{Agent: a})
		}
	}
	slices.SortFunc(rows, func(a, b agentRow) int {
		return cmp.Or(cmp.Compare(sample.AgentLabel(a.Agent), sample.AgentLabel(b.Agent)), cmp.Compare(a.Version, b.Version))
	})
	return rows
}

func writeAgents(w io.Writer, doc state.Doc, asJSON bool) error {
	rows := agentRows(doc)
	if asJSON {
		if rows == nil {
			rows = []agentRow{}
		}
		return writeJSON(w, rows)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-12s %-9s %-9s %s\n", "AGENT", "VERSION", "SESSIONS", "UNKNOWN SCREEN")
	var notes []string
	unknown := false
	for _, r := range rows {
		version := r.Version
		switch {
		case r.Sessions == 0:
			version = "-"
		case version == "":
			version = "not read"
		}
		count := fmt.Sprint(r.Unknown)
		if slices.Contains(hookless, r.Agent) {
			count = "no hooks: screen only"
		}
		unknown = unknown || r.Unknown > 0
		fmt.Fprintf(&b, "%-12s %-9s %-9d %s\n", sample.AgentLabel(r.Agent), version, r.Sessions, count)
		for _, why := range strings.Split(r.Why, "; ") {
			if note := sample.AgentLabel(r.Agent) + ": " + why; why != "" && !slices.Contains(notes, note) {
				notes = append(notes, note)
			}
		}
	}
	if unknown {
		notes = append(notes, agentsFix)
	}
	for _, n := range notes {
		b.WriteString(n + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}
