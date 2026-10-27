package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/accounts"
	"github.com/blox-eng/matchblox/internal/gitremote"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/service"
	"github.com/blox-eng/matchblox/internal/state"
)

// lifeAct is the act of matchblox kill <who> (--anyway: it works or waits,
// and you kill it all the same) or matchblox rename <who> <name>. The
// service holds every guard, and the go the stoker needs for a kill.
func lifeAct(doc state.Doc, cmd string, words []string, anyway bool, from string) (proto.Act, error) {
	if len(words) == 0 {
		return proto.Act{}, fmt.Errorf("%s needs a session: matchblox %s <who>", cmd, cmd)
	}
	a := proto.Act{Confirm: "y", From: from}
	switch cmd {
	case "kill":
		if len(words) > 1 {
			return a, errors.New("kill takes one session: matchblox kill <who>")
		}
		a.RecID = "kill:"
		if anyway {
			a.Text = service.KillAnyway
		}
	case "rename":
		switch {
		case len(words) == 1:
			return a, errors.New("rename needs a name: matchblox rename <who> <name>")
		case len(words) > 2:
			return a, errors.New("a name is one word: letters, digits, - and _")
		}
		if err := proto.CheckName(words[1]); err != nil {
			return a, err
		}
		a.RecID, a.Text = "rename:", words[1]
	}
	sess, err := resolve(doc, words[0])
	if err != nil {
		return a, err
	}
	a.RecID += sess.Pane
	return a, nil
}

// switchOpts are matchblox switch's flags: agent is "" and handoff nil
// when the flag was not given.
type switchOpts struct {
	agent   string
	handoff *string
	anyway  bool
	preview bool
}

// switchAct is the act of matchblox switch <who> --agent <name>: the
// session's pane runs another agent, started from the handoff line (the
// default one unless --handoff gives another, or none when it is empty).
// The service holds every guard.
func switchAct(doc state.Doc, words []string, o switchOpts, from string) (proto.Act, error) {
	const usage = "matchblox switch <who> --agent <name>"
	switch {
	case len(words) == 0:
		return proto.Act{}, errors.New("switch needs a session: " + usage)
	case o.agent == "":
		return proto.Act{}, errors.New("switch needs an agent: --agent claude, codex, opencode or cursor-agent")
	case len(words) > 1:
		return proto.Act{}, errors.New("switch takes one session: " + usage)
	}
	sess, err := resolve(doc, words[0])
	if err != nil {
		return proto.Act{}, err
	}
	if sess.Pane == "" {
		return proto.Act{}, fmt.Errorf("%s is outside tmux: nothing to switch", words[0])
	}
	sw := proto.SwitchAgent{Agent: o.agent, Handoff: proto.DefaultHandoff, Anyway: o.anyway}
	sw.ExpectedPID, sw.ExpectedStart = sess.PID, sess.Start
	if o.handoff != nil {
		sw.Handoff = *o.handoff
	}
	// The account the console would show: the last one if it is still
	// listed, else default.
	agent := o.agent
	if agent == "cursor" {
		agent = sample.CursorAgent
	}
	if list := doc.Accounts[agent]; len(list) > 0 {
		sw.Account = "default"
		if a := doc.LastAccount[agent]; a != "default" && slices.Contains(list, a) {
			sw.Account = a
		}
	}
	b, err := json.Marshal(sw)
	if err != nil {
		return proto.Act{}, err
	}
	return proto.Act{RecID: "switch:" + sess.Pane, Confirm: "y", Text: string(b), From: from}, nil
}

// newOpts are matchblox new's flags.
type newOpts struct {
	where, agent, branch, name string
	account, setup             string
	newAccount                 bool
}

// newSessionAct is the act of matchblox new: an agent (claude by default)
// in a tmux session of its own, in --where (here by default), with the
// words as its first message. --account picks an account of the agent;
// --new-account makes it, set up as --setup says (like-default by default).
func newSessionAct(words []string, o newOpts, from, here string) (proto.Act, error) {
	n := proto.NewSession{Where: o.where, Agent: o.agent, Branch: o.branch, Name: o.name, First: strings.Join(words, " ")}
	switch {
	case n.Where == "":
		n.Where = here
	case isCloneURL(o.where):
		// a git URL goes as typed: the service clones it
	case !filepath.IsAbs(n.Where):
		n.Where = filepath.Join(here, n.Where)
	}
	if n.Agent == "" {
		n.Agent = "claude"
	}
	if o.newAccount && o.account == "" {
		return proto.Act{}, errors.New("new: --new-account needs --account <name>")
	}
	if o.newAccount && len(words) > 0 {
		return proto.Act{}, errors.New("new: a new account starts without a first message")
	}
	if o.setup != "" && !o.newAccount {
		return proto.Act{}, errors.New("new: --setup needs --new-account")
	}
	if o.newAccount {
		n.Setup = o.setup
		if n.Setup == "" {
			n.Setup = string(accounts.LikeDefault)
		}
		switch accounts.Mode(n.Setup) {
		case accounts.LikeDefault, accounts.Copy, accounts.Empty:
		default:
			return proto.Act{}, fmt.Errorf("new: --setup is like-default, copy or empty, not %q", o.setup)
		}
	}
	n.Account, n.NewAccount = o.account, o.newAccount
	b, err := json.Marshal(n)
	if err != nil {
		return proto.Act{}, err
	}
	return proto.Act{RecID: service.NewSessionID, Confirm: "y", Text: string(b), From: from}, nil
}

// cloneWait is how long matchblox new waits for a clone: a big repo is slow.
const cloneWait = 30 * time.Minute

// isCloneURL tells if where is a git URL to clone, as the service reads it.
func isCloneURL(where string) bool { return gitremote.Explicit(where) }

func cliLife(cmd string, words []string, anyway bool, w io.Writer, path string) error {
	doc, err := readDoc(path, staleAfter)
	if err != nil {
		return err
	}
	a, err := lifeAct(doc, cmd, words, anyway, os.Getenv("TMUX_PANE"))
	if err != nil {
		return err
	}
	res, err := doAct(path, a)
	if err == nil && cmd == "rename" && res.Err == "" && len(res.Skipped) == 1 && strings.HasSuffix(res.Skipped[0], "renames when it rests") {
		_, err = fmt.Fprintln(w, res.Skipped[0])
		return err
	}
	return handOutcome(w, cmd, words[0], res, err)
}

func cliNew(words []string, o newOpts, w io.Writer, path string) error {
	here, err := os.Getwd()
	if err != nil {
		return err
	}
	a, err := newSessionAct(words, o, os.Getenv("TMUX_PANE"), here)
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	wait := newWaitFor(o, home)
	if isCloneURL(o.where) {
		if r, ok := gitremote.Parse(o.where, "https"); ok {
			if _, err := fmt.Fprintf(w, "cloning %s…\n", r.Slug()); err != nil {
				return err
			}
		}
	}
	res, err := doActFor(path, a, wait)
	if err != nil {
		if errors.Is(err, errNoResult) {
			return errors.New("new: not known whether it started: matchblox sessions shows it within seconds if it did")
		}
		return fmt.Errorf("new: did not run: %w", err)
	}
	if res.Err == "" && res.Note != "" {
		if _, err := fmt.Fprintln(w, res.Note); err != nil {
			return err
		}
	}
	if err := newOutcome(w, res, o.newAccount); err != nil {
		switch {
		case res.Err != "" && !res.Refused:
			return fmt.Errorf("new: failed: %w", err)
		case res.Err == "" && len(res.Ran) > 0:
			// A guard stopped a later step: what ran stays.
			return fmt.Errorf("new: stopped after %s: %w", strings.Join(res.Ran[len(res.Ran)-1], " "), err)
		}
		return fmt.Errorf("new: refused: %w", err)
	}
	return nil
}

// newWaitFor is newWait, and a git URL waits for its clone as well.
func newWaitFor(o newOpts, home string) time.Duration {
	if isCloneURL(o.where) {
		return max(newWait(o, home), cloneWait)
	}
	return newWait(o, home)
}

// newWait is how long matchblox new waits for its result: a start that may
// copy the default (a new account, or a copy account, which goes on with a
// copy that did not end) waits the copy's time limit on top of an act's.
func newWait(o newOpts, home string) time.Duration {
	agent := o.agent
	if agent == "" {
		agent = "claude"
	}
	// A hand-made folder (~/… or a path) is never a copy account.
	named := o.account != "" && !strings.HasPrefix(o.account, "~") && !strings.ContainsRune(o.account, filepath.Separator)
	copies := o.newAccount || named && accounts.SetupOf(accounts.Folder(home, agent, o.account)) == accounts.Copy
	if copies {
		return accounts.CopyLimit.Time + actWait
	}
	return actWait
}

// switchWait is how long matchblox switch waits for its result: a switch
// starts agent on its last account, and a copy account may go on with its
// copy, as at a start (newWait).
func switchWait(doc state.Doc, agent, home string) time.Duration {
	if agent == "cursor" {
		agent = sample.CursorAgent
	}
	return newWait(newOpts{agent: agent, account: doc.LastAccount[agent]}, home)
}

// newOutcome is reportResult for a new session: a start with notes on what
// its account shared or skipped ran, and the notes follow its steps. A new
// account (signIn) then names the session to sign in in.
func newOutcome(w io.Writer, res proto.Result, signIn bool) error {
	notes, ran := proto.StartNotes(res)
	if ran {
		res.Skipped = nil
	}
	if err := reportResult(w, res); err != nil {
		return err
	}
	for _, note := range notes {
		if _, err := fmt.Fprintln(w, note); err != nil {
			return err
		}
	}
	if !signIn || len(res.Ran) == 0 {
		return nil
	}
	_, err := fmt.Fprintf(w, "sign in in %s\n", sessionOf(res.Ran))
	return err
}

// sessionOf is the name of the tmux session the steps of a new session
// made: the -s of its new-session.
func sessionOf(ran [][]string) string {
	for _, argv := range slices.Backward(ran) {
		if i := slices.Index(argv, "new-session"); i >= 0 {
			if j := slices.Index(argv[i:], "-s"); j >= 0 && i+j+1 < len(argv) {
				return argv[i+j+1]
			}
		}
	}
	return "the session"
}

// cliSwitch switches the session's agent and, when jump is not nil, takes
// the tmux client to its pane: only after the switch ran.
func cliSwitch(words []string, o switchOpts, w io.Writer, path string, jump func(argv []string) error) error {
	doc, err := readDoc(path, staleAfter)
	if err != nil {
		return err
	}
	a, err := switchAct(doc, words, o, os.Getenv("TMUX_PANE"))
	if err != nil {
		return err
	}
	if o.preview {
		a.RecID = "switch-preview:" + strings.TrimPrefix(a.RecID, "switch:")
		a.Confirm = ""
		res, err := doAct(path, a)
		if err != nil {
			return err
		}
		if res.Err != "" {
			return errors.New(res.Err)
		}
		if res.Preview == "" {
			return errors.New("the service returns no handover preview: update matchblox before switching")
		}
		_, err = fmt.Fprintln(w, res.Preview)
		return err
	}
	home, _ := os.UserHomeDir()
	res, err := doActFor(path, a, switchWait(doc, o.agent, home))
	if err == nil && res.Err != "" && !res.Refused {
		return fmt.Errorf("switch %s: failed: %s", words[0], res.Err)
	}
	if err != nil || res.Err != "" || len(res.Ran) == 0 {
		return handOutcome(w, "switch", words[0], res, err)
	}
	if _, err := fmt.Fprintf(w, "switched %s to %s\n", words[0], o.agent); err != nil {
		return err
	}
	notes, _ := proto.StartNotes(res)
	for _, note := range notes {
		if _, err := fmt.Fprintln(w, note); err != nil {
			return err
		}
	}
	if jump == nil {
		return nil
	}
	server, bare := sample.SplitPane(strings.TrimPrefix(a.RecID, "switch:"))
	if err := jump(slices.Concat(sample.TmuxOn(server), []string{"switch-client", "-t", bare})); err != nil {
		_, err = fmt.Fprintf(w, "switched, but not jumped: %v\n", err)
		return err
	}
	return nil
}
