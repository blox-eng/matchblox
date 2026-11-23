package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"slices"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// explainEnv is what explain reads beyond the state doc: the pane, the
// clock, and the names Scrub takes out of what it prints.
type explainEnv struct {
	screenOf         func(pane string) (text string, alt, ok bool)
	now              time.Time
	home, host, user string
}

type explainHit struct {
	Rule string `json:"rule"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type explainRow struct {
	N     int    `json:"n"`
	Label string `json:"label"`
}

// explainOut is what explain prints, scrubbed, as text or JSON.
type explainOut struct {
	Session string       `json:"session"`
	Agent   string       `json:"agent"`
	Version string       `json:"version"`
	Hook    string       `json:"hook"`
	Verdict string       `json:"verdict"`
	Rules   string       `json:"rules"`
	Hits    []explainHit `json:"hits"`
	MenuOn  bool         `json:"-"`
	Options []explainRow `json:"options"`
	Screen  []string     `json:"screen"`
	Unknown bool         `json:"unknown_screen"`
}

const explainFix = "Fix: report it at github.com/blox-eng/matchblox/issues with this output"

func cliExplain(words []string, w io.Writer, path string, asJSON bool) error {
	if len(words) != 1 {
		return errors.New("explain needs one session: matchblox agents explain <who>")
	}
	doc, err := readDoc(path, staleAfter)
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	host, _ := os.Hostname()
	name := os.Getenv("USER")
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	x := explainEnv{screenOf: sample.CaptureScreen, now: time.Now(), home: home, host: host, user: name}
	return writeExplain(w, doc, words[0], x, asJSON)
}

// writeExplain prints what the service decided for the session who names,
// and which rule read its screen as it is now.
func writeExplain(w io.Writer, doc state.Doc, who string, x explainEnv, asJSON bool) error {
	s, err := resolve(doc, who)
	if errors.Is(err, errNoSession) {
		return fmt.Errorf("no session %s: matchblox sessions lists them", who)
	} else if err != nil {
		return err
	}
	screen, _, ok := x.screenOf(s.Pane)
	if !ok {
		return fmt.Errorf("the pane %s of %s cannot be read: matchblox sessions lists the live ones", s.Pane, who)
	}
	// Explain cleans the styles away itself, as Menu does for the service.
	t := said.Explain(screen)
	scrub := func(text string) string {
		return said.Scrub(text, []string{s.Cwd, x.home}, []string{x.host, x.user})
	}
	out := explainOut{
		Session: scrub(s.Name),
		Agent:   sample.AgentLabel(s.Agent),
		Version: s.Version,
		Hook:    hookOf(s, doc.Queue, x.now),
		Verdict: scrub(verdictOf(s)),
		Rules:   "built into matchblox " + version,
		MenuOn:  t.Kind == said.Permits || s.UnknownScreen || s.MenuUnread,
		Unknown: s.UnknownScreen,
		Hits:    []explainHit{},
		Options: []explainRow{},
		Screen:  []string{},
	}
	for _, h := range t.Hits {
		out.Hits = append(out.Hits, explainHit{Rule: h.Rule, Line: h.Line, Text: scrub(h.Text)})
	}
	for _, r := range t.Options {
		out.Options = append(out.Options, explainRow{N: r.N, Label: scrub(r.Label)})
	}
	for _, l := range t.Lines {
		out.Screen = append(out.Screen, scrub(l))
	}
	if asJSON {
		return writeJSON(w, out)
	}
	_, err = io.WriteString(w, explainText(out))
	return err
}

func verdictOf(s sample.Session) string {
	switch {
	case s.UnknownScreen:
		return "unknown screen: no rule reads a menu"
	case s.MenuUnread:
		return "menu not read yet"
	case s.Ask != nil && len(s.Ask.Questions) > 0:
		return "asks: " + firstOf(s.Ask.Questions[0].Question)
	case s.Permits:
		return "waits for an approval: " + s.LastLine
	}
	return "none: " + s.LastLine
}

// hookOf is what the session's hook says, from its queue item, matched as
// the console's peek matches it.
func hookOf(s sample.Session, items []queue.Item, now time.Time) string {
	if slices.Contains(hookless, s.Agent) {
		return "no hooks: screen only"
	}
	for _, it := range items {
		if s.Pane != "" && it.Pane == s.Pane || s.Pane == "" && s.SessionID != "" && it.SessionID == s.SessionID {
			if it.State == queue.StatePermission {
				return "waits for a permission, for " + now.Sub(it.Since).Round(time.Second).String()
			}
			break
		}
	}
	return "no wait"
}

func explainText(o explainOut) string {
	var b strings.Builder
	session := o.Session + " · " + o.Agent
	if o.Version != "" {
		session += " " + o.Version
	}
	fmt.Fprintf(&b, "Session  %s\nHook     %s\nVerdict  %s\nRules    %s\n\n", session, o.Hook, o.Verdict, o.Rules)
	fmt.Fprintf(&b, "Rules on the last %d lines with words\n", said.ScreenWithin)
	for _, h := range o.Hits {
		hit := "no match"
		if h.Line > 0 {
			hit = fmt.Sprintf("line %d  %s", h.Line, h.Text)
		}
		fmt.Fprintf(&b, "  %-19s %s\n", h.Rule, hit)
	}
	switch {
	case len(o.Options) > 0:
		rows := make([]string, len(o.Options))
		for i, r := range o.Options {
			rows[i] = fmt.Sprintf("%d %s", r.N, r.Label)
		}
		fmt.Fprintf(&b, "Menu     %s\n", strings.Join(rows, " · "))
	case o.MenuOn:
		b.WriteString("Menu     not read\n")
	}
	b.WriteString("\nScreen (home path, host and user removed; check it before you post it)\n")
	for _, l := range o.Screen {
		fmt.Fprintf(&b, "  │ %s\n", l)
	}
	if o.Unknown {
		b.WriteString("\n" + explainFix + "\n")
	}
	return b.String()
}
