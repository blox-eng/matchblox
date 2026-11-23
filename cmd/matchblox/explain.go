package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/user"
	"slices"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/menu"
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
	Rule    string `json:"rule"`
	Line    int    `json:"line"`
	Text    string `json:"text"`
	Tried   bool   `json:"tried"`
	Decides bool   `json:"decides"`
}

type explainRow struct {
	N     int    `json:"n"`
	Label string `json:"label"`
}

// explainOut is what explain prints, scrubbed, as text or JSON. A saved
// screen fills no Session, Agent, Version or Hook.
type explainOut struct {
	Session string       `json:"session"`
	Agent   string       `json:"agent"`
	Version string       `json:"version"`
	Hook    string       `json:"hook"`
	Verdict string       `json:"verdict"`
	Changed bool         `json:"screen_changed"`
	Rules   string       `json:"rules"`
	Hits    []explainHit `json:"hits"`
	MenuOn  bool         `json:"-"`
	Options []explainRow `json:"options"`
	Screen  []string     `json:"screen"`
	Unknown bool         `json:"unknown_screen"`
	File    bool         `json:"-"`
}

const (
	explainFix     = "Fix: report it at github.com/blox-eng/matchblox/issues with this output"
	explainChanged = "the screen changed since the note: run it again"
	maxScreenFile  = 1 << 20
)

func cliExplain(words []string, w io.Writer, path, file string, asJSON bool) error {
	switch {
	case file != "" && len(words) > 0:
		return errors.New("explain takes a session or --file, not both")
	case file == "" && len(words) == 0:
		return errors.New("explain needs a session: matchblox agents explain <who>")
	case len(words) > 1:
		return errors.New("explain needs one session: matchblox agents explain <who>")
	}
	home, _ := os.UserHomeDir()
	host, _ := os.Hostname()
	name := os.Getenv("USER")
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	x := explainEnv{screenOf: sample.CaptureScreen, now: time.Now(), home: home, host: host, user: name}
	if file != "" {
		return explainFile(w, file, x, asJSON)
	}
	doc, err := readDoc(path, staleAfter)
	if err != nil {
		return err
	}
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
	out := traceOut(t, screen, scrub)
	out.Session = scrub(s.Name)
	out.Agent = sample.AgentLabel(s.Agent)
	out.Version = s.Version
	out.Hook = hookOf(s, doc.Queue, x.now)
	out.Verdict = scrub(verdictOf(s))
	out.Changed = screenChanged(s, t)
	out.MenuOn = out.MenuOn || s.UnknownScreen || s.MenuUnread
	out.Unknown = s.UnknownScreen
	return printExplain(w, out, asJSON)
}

// screenChanged: the note and the screen read now disagree. An unknown or
// unread note disagrees with a read menu; any other note disagrees with a
// screen of another kind, or with another approval prompt.
func screenChanged(s sample.Session, t said.Trace) bool {
	if s.UnknownScreen || s.MenuUnread {
		return t.MenuRead
	}
	note := said.None
	switch {
	case s.Ask != nil && len(s.Ask.Questions) > 0:
		note = said.Asks
	case s.Permits:
		note = said.Permits
	case s.Asks:
		note = said.Asks
	}
	return note != t.Kind || note == said.Permits && s.LastLine != t.Line
}

// explainFile prints which rule reads a screen saved in the file at path.
func explainFile(w io.Writer, path string, x explainEnv, asJSON bool) error {
	scrub := func(text string) string {
		return said.Scrub(text, []string{x.home}, []string{x.host, x.user})
	}
	screen, err := readScreenFile(path)
	if err != nil {
		return errors.New(scrub(err.Error()))
	}
	t := said.Explain(screen)
	out := traceOut(t, screen, scrub)
	out.Verdict = scrub(traceVerdict(t))
	out.File = true
	return printExplain(w, out, asJSON)
}

func readScreenFile(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // the person names the file to read
	if err != nil {
		return "", unreadable(path, err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxScreenFile+1))
	if err != nil {
		return "", unreadable(path, err)
	}
	if len(data) > maxScreenFile {
		return "", fmt.Errorf("the file %s is over 1 MiB: save only the screen", path)
	}
	return string(data), nil
}

// unreadable names the path once: an *fs.PathError would name it again.
func unreadable(path string, err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	return fmt.Errorf("the file %s cannot be read: %w", path, err)
}

// traceOut is the part of the output the rules give, scrubbed. Its menu
// rows are what menu.Read reads, as the console does before a digit goes.
func traceOut(t said.Trace, screen string, scrub func(string) string) explainOut {
	out := explainOut{
		Rules:   "built into matchblox " + version,
		MenuOn:  t.Kind == said.Permits,
		Hits:    []explainHit{},
		Options: []explainRow{},
		Screen:  []string{},
	}
	for _, h := range t.Hits {
		out.Hits = append(out.Hits, explainHit{Rule: h.Rule, Line: h.Line, Text: scrub(h.Text), Tried: h.Tried, Decides: h.Decides})
	}
	if t.Kind == said.Permits {
		rows := menu.Read(screen)
		for n := 1; n <= len(rows); n++ {
			out.Options = append(out.Options, explainRow{N: n, Label: scrub(rows[n])})
		}
	}
	for _, l := range t.Lines {
		out.Screen = append(out.Screen, scrub(l))
	}
	return out
}

func printExplain(w io.Writer, out explainOut, asJSON bool) error {
	if asJSON {
		return writeJSON(w, out)
	}
	_, err := io.WriteString(w, explainText(out))
	return err
}

// traceVerdict is the verdict the rules alone give, for a saved screen.
func traceVerdict(t said.Trace) string {
	switch t.Kind {
	case said.Permits:
		return "waits for an approval: " + t.Line
	case said.Asks:
		return "asks: " + t.Line
	}
	return "none: " + t.Line
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
	if !o.File {
		session := o.Session + " · " + o.Agent
		if o.Version != "" {
			session += " " + o.Version
		}
		fmt.Fprintf(&b, "Session  %s\nHook     %s\n", session, o.Hook)
	}
	fmt.Fprintf(&b, "Verdict  %s\n", o.Verdict)
	if o.Changed {
		b.WriteString("         " + explainChanged + "\n")
	}
	fmt.Fprintf(&b, "Rules    %s\n\n", o.Rules)
	fmt.Fprintf(&b, "Rules on the last %d lines with words\n", said.ScreenWithin)
	for _, h := range o.Hits {
		hit := "no match"
		switch {
		case !h.Tried:
			hit = "not tried"
		case h.Line > 0:
			hit = fmt.Sprintf("line %d  %s", h.Line, h.Text)
		}
		if h.Decides {
			hit += "  (decides)"
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
