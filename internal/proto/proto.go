// Package proto is the wire between the service and a console: one JSON
// envelope on each line, hello first from each side.
package proto

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/history"
	"github.com/blox-eng/matchblox/internal/holds"
	"github.com/blox-eng/matchblox/internal/idle"
	"github.com/blox-eng/matchblox/internal/ships"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/stoker"
)

// Version changes only when an older peer cannot read the messages.
const Version = 1

// MaxLine bounds one message, so a broken peer cannot grow our memory.
const MaxLine = 16 << 20

var ErrLineTooLong = errors.New("proto: message longer than 16 MiB")

type Kind string

const (
	KindHello    Kind = "hello"
	KindSnapshot Kind = "snapshot"
	KindEvent    Kind = "event"
	KindAct      Kind = "act"
	KindResult   Kind = "result"
	KindStoker   Kind = "stoker"
	KindError    Kind = "error"
	// KindScreen is the text of the pane a console watches.
	KindScreen Kind = "screen"
	// KindHook is the first and only message of `matchblox hook`: one
	// hooks.Event, sent after the service hello, with no hello back.
	KindHook Kind = "hook"
	// KindNews is the message of a hook that asks for its match's news,
	// after the service hello; the service answers with one KindNews that
	// holds a NewsReply. A hook that printed the news sends one more, with
	// no body, to say it heard it.
	KindNews Kind = "news"
)

type Envelope struct {
	Kind Kind            `json:"kind"`
	ID   string          `json:"id,omitempty"`
	Body json.RawMessage `json:"body,omitempty"`
}

type Hello struct {
	Version int    `json:"version"`
	Binary  string `json:"binary"`
	Host    string `json:"host"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	// Client is what says hello to the service: ClientConsole, or
	// ClientCLI for a matchblox command. Empty: a command.
	Client string `json:"client,omitempty"`
	// Tmux is the service's tmux version, as tmux -V says it without
	// "tmux ", or "none". "": an older service, which does not say.
	Tmux string `json:"tmux,omitempty"`
}

// The kinds of client in a hello. Only a console is a person at the keys.
const (
	ClientConsole = "console"
	ClientCLI     = "cli"
)

// State is what the console draws. History is set only in the first
// snapshot to each console; later snapshots leave it nil and the console
// extends it from each sample.
type State struct {
	state.Doc
	History *history.Series `json:"history,omitempty"`
	// Found is set in the first snapshot to each console and when it
	// changes; nil: as the last one. A service with no scan never sets it.
	Found *Found `json:"found,omitempty"`
	// Used is where sessions started: set as Found is.
	Used *Used `json:"used,omitempty"`
	// Clones are the clones the service runs, and the ones that ended
	// in the last minute: in every snapshot while any is listed.
	Clones []Clone `json:"clones,omitempty"`
}

type Event struct {
	Type string `json:"type"`
	Text string `json:"text"`
	Pane string `json:"pane,omitempty"`
}

// Act asks the service to run an action it built itself. The console names
// the action; it never sends argv.
type Act struct {
	RecID   string `json:"rec_id"`
	Which   string `json:"which"`             // "primary" | "secondary"
	Confirm string `json:"confirm,omitempty"` // "y" for a destructive action
	// Text is what the person typed for an "answer:<pane>" act.
	Text string `json:"text,omitempty"`
	// From is the tmux pane the act was typed in ($TMUX_PANE of the CLI).
	// Where the service knows the caller's pid, it finds the pane itself
	// and this is ignored.
	From string `json:"from,omitempty"`
	// Seen is when the console last saw the screen it acts on.
	Seen time.Time `json:"seen,omitempty"`
	// PID is the agent the console saw in the pane a "keys:" act names.
	PID int `json:"pid,omitempty"`
	// Start is that agent's process start, which tells a reused pid apart.
	Start uint64 `json:"start,omitempty"`
	// Cols and Rows are, on a watch act, the size the console's chat gives
	// the agent's screen: the service sizes the agent's window to it. 0: a
	// chat that is not drawn, or a console before sizes.
	Cols int `json:"cols,omitempty"`
	Rows int `json:"rows,omitempty"`
	// Console: the act came from a console, a person at the keys. The
	// service sets it from the hello; it never comes over the wire.
	Console bool `json:"-"`
	// FromKnown: the service found From itself, by the caller's pid, so
	// the act cannot name another pane as its own. It never comes over
	// the wire.
	FromKnown bool `json:"-"`
	// FromPlaced: the service knows who sends the act, a match's pane or
	// the person: by the caller's pane or terminal, or a console's hello,
	// or because the service makes the act itself. A send that is not
	// placed is refused. It never comes over the wire.
	FromPlaced bool `json:"-"`
}

// WatchPrefix starts an act that names the pane a console watches; with no
// pane after it, the console watches none. An older console's Text "up:<n>"
// is ignored: the screen is always the live one.
const WatchPrefix = "watch:"

// KeysPrefix starts an act that sends one key to the pane after it. Text is
// a key name of KeyNames, or LitPrefix and the text to type.
const KeysPrefix = "keys:"

// CommandPrefix starts an act that types a command of the command table
// (keycmd) into the pane after it, at rest. Text is the command's id; PID
// is the agent the console showed.
const CommandPrefix = "cmd:"

// ChainPrefix starts an act from an agent's pane that asks to chain with the
// agent in the pane after it. After it, "-" leaves the chain, "off" refuses
// chains and "on" takes them again.
const ChainPrefix = "chain:"

// LitPrefix starts the text of a keys: act that types the text after it:
// at most MaxLit bytes, with no control character.
const (
	LitPrefix = "lit:"
	MaxLit    = 200
)

// KeyNames are the tmux keys a keys: act may press by name.
var KeyNames = append([]string{"Up", "Down", "Left", "Right", "Enter", "Tab", "BTab", "Escape", "C-c", "C-a", "BSpace", "PPage", "NPage"}, ctrlNames()...)

// CtrlKeys are the letters a chat sends with ctrl, as C-<letter>, beyond
// C-a and C-c. Left out: ctrl+z, which stops the agent; ctrl+s and ctrl+q,
// which stop and start the terminal; ctrl+h, i, j and m, which are
// Backspace, Tab and Enter; and ctrl+o, the console's own.
const CtrlKeys = "bdefgklnprtuvwxy"

func ctrlNames() []string {
	var out []string
	for _, l := range CtrlKeys {
		out = append(out, "C-"+string(l))
	}
	return out
}

// WheelUp and WheelDown are the text of a keys: act that turns the mouse
// wheel in the pane. An agent that reads the mouse scrolls itself; for one
// that does not, the service shows the console the pane's history.
const (
	WheelUp   = "WheelUp"
	WheelDown = "WheelDown"
)

// PickPrefix starts an act that answers the question tool in the pane
// after it; its text is the said.Answer.
const PickPrefix = "pick:"

// Screen is one capture of the watched pane. Gone: the pane is not there.
type Screen struct {
	Pane string    `json:"pane"`
	At   time.Time `json:"at"`
	Text string    `json:"text"`
	Gone bool      `json:"gone,omitempty"`
	// Err says the pane could not be read this time; the session is still
	// there, and Text is empty.
	Err string `json:"err,omitempty"`
	// Cols and Rows are the pane's size where the service sized its window
	// to a chat; 0 where it did not. A chat as wide draws the agent's lines
	// as they are.
	Cols int `json:"cols,omitempty"`
	Rows int `json:"rows,omitempty"`
	// Back is how many lines back in the pane's history the screen is, for
	// an agent that does not read the mouse; 0 is the live screen.
	Back int `json:"back,omitempty"`
}

type Result struct {
	ActID   string     `json:"act_id"`
	Ran     [][]string `json:"ran,omitempty"`
	Skipped []string   `json:"skipped,omitempty"`
	Err     string     `json:"err,omitempty"`
	// Preview is the read-only evidence shown before an agent switch.
	Preview string `json:"preview,omitempty"`
	// Scan is how a scan:yes's scan went.
	Scan *ScanSummary `json:"scan,omitempty"`
	// Clone is a clone:plan's plan.
	Clone *ClonePlan `json:"clone,omitempty"`
	// CloneID is the id of the clone a clone:start started.
	CloneID int64 `json:"clone_id,omitempty"`
	// Note is a line the builder reads, not a refusal: what a URL in
	// new:session came to, or that a clone:stop came after the clone was
	// done.
	Note string `json:"note,omitempty"`
	// Names answers the names act: what each number of the island is
	// called now.
	Names *Names `json:"names,omitempty"`
	// Refused: the service refused the act before it ran anything (a
	// check, a busy or gone account). An Err without it is a failure of
	// something that ran, or of a file that did not read. Only a
	// new:session and a switch:<pane> set it.
	Refused bool `json:"refused,omitempty"`
	// Recover is a recover act's answer: each entry of the recovery record,
	// and what recover found or did with it.
	Recover []Recovered `json:"recover,omitempty"`
}

// Recover is the Text of a "recover" act: reopen the conversations of the
// recovery record that no agent runs now (#215). All reopens each one that
// can; Names reopens those of these tmux sessions. Neither: a preview,
// which changes nothing. Dirs is the new folder of a session whose folder
// moved, by its name.
type Recover struct {
	All   bool              `json:"all,omitempty"`
	Names []string          `json:"names,omitempty"`
	Dirs  map[string]string `json:"dirs,omitempty"`
}

// The states of a Recovered.
const (
	RecoverLive     = "live"     // an agent runs the conversation now
	RecoverReady    = "ready"    // it can reopen
	RecoverReopened = "reopened" // it reopened, in As
	RecoverCannot   = "cannot"   // it cannot reopen: Why says why and the fix
)

// Recovered is one entry of the recovery record: its tmux session, agent,
// conversation and folder, and its state. Why is the cause and the fix of
// a cannot, or a note.
type Recovered struct {
	Name         string `json:"name"`
	Agent        string `json:"agent"`
	Conversation string `json:"conversation"`
	Dir          string `json:"dir"`
	State        string `json:"state"`
	Why          string `json:"why,omitempty"`
	As           string `json:"as,omitempty"`
}

// ClonePlan is where a clone of a URL goes, found offline. Target is
// <clone_root>/<repo>, or a free name when something else is there; Taken
// says what: "another repo", "not a repo", "a file" or "being cloned" ("" when
// free or the same repo). Have are the checkouts of the same remote, <clone_root>/<repo>
// first when it is one; then Fresh is a free name to clone anew into.
type ClonePlan struct {
	Repo   string   `json:"repo"` // owner/repo
	URL    string   `json:"url"`
	Target string   `json:"target"`
	Taken  string   `json:"taken,omitempty"`
	Have   []string `json:"have,omitempty"`
	Fresh  string   `json:"fresh,omitempty"`
}

// CloneStart is the Text of a "clone:start" act: clone URL into Target.
// Token is the console's own for this start: the clone's record carries
// it, so a console that lost the answer knows its clone from another's.
type CloneStart struct {
	URL    string `json:"url"`
	Target string `json:"target"`
	Token  string `json:"token,omitempty"`
}

// CloneStop is the Text of a "clone:stop" act: stop the clone ID into
// Target. A stop whose ID is not the clone's that runs there stops
// nothing. A bare Target, from an older console, stops the clone there.
type CloneStop struct {
	Target string `json:"target"`
	ID     int64  `json:"id,omitempty"`
}

// Clone is one clone the service runs or ran.
type Clone struct {
	// ID tells this clone from an earlier one into the same Target.
	ID     int64  `json:"id,omitempty"`
	Target string `json:"target"`
	Repo   string `json:"repo"`
	Pct    int    `json:"pct"` // -1: no percent yet
	Err    string `json:"err,omitempty"`
	Done   bool   `json:"done,omitempty"`
	Token  string `json:"token,omitempty"` // its clone:start's, if any
}

// Names are the session and account names by their island numbers.
type Names struct {
	Sessions map[int]string `json:"sessions"`
	Accounts map[int]string `json:"accounts"`
}

// The notes of a start on an account that ran: each is one skip of its
// Result, and the start ran all the same (StartNotes).
const (
	// NotLinked starts the note of a like-default account whose new shared
	// entries did not all link.
	NotLinked = "some shared entries did not link: "
	// StillShared starts the note of a link a copy left on the default's
	// file, and why.
	StillShared = "still shared with the default: "
	// NotCopied starts the note of an entry of the default a copy left
	// out, and why.
	NotCopied = "not copied: "
	// CopyGoesOn is the note of a copy that has not ended.
	CopyGoesOn = "the copy is not complete: it goes on at the next start"
)

// noteMax is how many entries Notes names before it counts the rest.
const noteMax = 5

// Notes is one note per item, each after prefix: the first five, then one
// that says how many more there are.
func Notes(prefix string, items []string) []string {
	var out []string
	for i, it := range items {
		if i == noteMax {
			return append(out, fmt.Sprintf("%sand %d more", prefix, len(items)-noteMax))
		}
		out = append(out, prefix+it)
	}
	return out
}

// StartNotes are the skips of r when r has no error and every skip is a
// note of a start on an account (NotLinked, StillShared, NotCopied,
// CopyGoesOn): the act ran all the same.
func StartNotes(r Result) ([]string, bool) {
	if r.Err != "" || len(r.Skipped) == 0 {
		return nil, false
	}
	for _, k := range r.Skipped {
		if k != CopyGoesOn && !strings.HasPrefix(k, NotLinked) && !strings.HasPrefix(k, StillShared) && !strings.HasPrefix(k, NotCopied) {
			return nil, false
		}
	}
	return slices.Clone(r.Skipped), true
}

// News asks for the news of the match in Pane, at the hook Event.
type News struct {
	Pane string `json:"pane"`
	// Socket is the tmux server of Pane ($TMUX's socket), as in a hook event.
	Socket    string `json:"socket,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Event     string `json:"event,omitempty"`
}

// NewsReply is the text the hook prints. Empty: nothing new.
type NewsReply struct {
	Text string `json:"text,omitempty"`
}

type Error struct {
	Text string `json:"text"`
}

// Encode writes one message and its newline in a single Write, so two
// goroutines that share a lock never interleave half lines.
func Encode(w io.Writer, kind Kind, id string, body any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	line, err := json.Marshal(Envelope{Kind: kind, ID: id, Body: raw})
	if err != nil {
		return err
	}
	_, err = w.Write(append(line, '\n'))
	return err
}

// Decode reads one message. A line over MaxLine is an error, not a stall.
func Decode(r *bufio.Reader) (Envelope, error) {
	var line []byte
	for {
		chunk, err := r.ReadSlice('\n')
		if len(line)+len(chunk) > MaxLine {
			return Envelope{}, ErrLineTooLong
		}
		line = append(line, chunk...)
		if err == nil {
			break
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			if errors.Is(err, io.EOF) && len(bytes.TrimSpace(line)) > 0 {
				return Envelope{}, io.ErrUnexpectedEOF
			}
			return Envelope{}, err
		}
	}
	var env Envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return Envelope{}, err
	}
	return env, nil
}

// StokerReadBack is how long the service reads back the result of a compact.
const StokerReadBack = stoker.ResumeWait

// The console draws git data without importing the scanner, the ships
// without importing their tracker, and the stoker's steps without
// importing the stoker.
type (
	GitReport   = gitscan.Report
	Worktree    = gitscan.Worktree
	PR          = gitscan.PR
	Found       = gitscan.Found
	Used        = state.Used
	Use         = state.Use
	ScanSummary = gitscan.Summary
	Checkout    = gitscan.Checkout
	Ships       = ships.Report
	Ship        = ships.Ship
	ShipStage   = ships.Stage
	ShipSession = ships.SessionRef
	StokerView  = stoker.View
	StokerMode  = stoker.Mode
	StokerEntry = stoker.Entry
	StokerCard  = stoker.Card
	StokerMiss  = stoker.Miss
	StokerLapse = stoker.Lapse
	StokerOrder = stoker.Order
	StokerPlan  = stoker.PlanStep
	StokerSaid  = stoker.Said
	// StokerTrigger is a `matchblox stoker when` that has not fired.
	StokerTrigger = stoker.Trigger
	// StokerUnguard is a plan's session no merge guard watches.
	StokerUnguard = stoker.Unguard
	Hold          = holds.Hold
	IdleReason    = idle.Reason
	// Focus is the session a tap on the island or a buzz opens.
	Focus = state.Focus
)

// RepoOf is gitscan.RepoOf: the console finds a session's checkout as the
// service does.
func RepoOf(r *GitReport, cwd string) (repoPath, worktreePath string, ok bool) {
	return gitscan.RepoOf(r, cwd)
}

// ShipsOf is ships.Of: the console finds a session's ships as the
// service, the CLI and the stoker do.
func ShipsOf(all []Ship, pane, id string) []Ship { return ships.Of(all, pane, id) }

// ShipDrives is ships.Drives.
func ShipDrives(sh Ship, pane, id string) bool { return ships.Drives(sh, pane, id) }

// NewSession is the Text of a "new:session" act: start an agent in a tmux
// session of its own on the builder's server. Branch, when set, first
// adds a worktree for it next to the repository in Where. Name is the
// tmux session's, its window's and the agent's; empty, it is the
// directory's. First is the agent's first message. Account is the
// account the agent starts on: empty or "default" is the sign-in the
// builder has. NewAccount makes the account first, set up as Setup says
// (like-default, copy or empty; empty is like-default), and the session
// starts without First: the builder signs in there. Cloned says the
// folder in Where is a clone made for this session: Cursor asks before it
// trusts it. It can only take trust away.
type NewSession struct {
	Where      string `json:"where"`
	Branch     string `json:"branch,omitempty"`
	Name       string `json:"name,omitempty"`
	Agent      string `json:"agent"`
	First      string `json:"first,omitempty"`
	Account    string `json:"account,omitempty"`
	NewAccount bool   `json:"new_account,omitempty"`
	Setup      string `json:"setup,omitempty"`
	Cloned     bool   `json:"cloned,omitempty"`
}

// SwitchAgent is the Text of a "switch:<pane>" act: start Agent in the
// pane in place of the one there, with Handoff as its first message.
// Clients fill Handoff, DefaultHandoff unless the builder changed it; empty
// is no first message. Anyway is the second confirm of a session that
// works or waits.
type SwitchAgent struct {
	Agent   string `json:"agent"`
	Handoff string `json:"handoff,omitempty"`
	Anyway  bool   `json:"anyway,omitempty"`
	// ExpectedPID/Start bind consent to the process shown by the client.
	// Older clients omit them and retain the existing service-side guard.
	ExpectedPID   int    `json:"expected_pid,omitempty"`
	ExpectedStart uint64 `json:"expected_start,omitempty"`
	// Account is the account the sender's confirm named for Agent ("default"
	// included). The service refuses the switch when it would pick another
	// now. Empty, from an older client, takes whichever it picks.
	Account string `json:"account,omitempty"`
}

// DefaultHandoff selects an evidence capture and the service's fixed switch
// prompt. Handoff adds the builder's extras to it. Any other handoff stays
// the builder's exact text; empty starts no turn.
const DefaultHandoff = "Continue from the saved handover: check the goal, latest user direction, progress, decisions, verification and next action against the current tree."

// handoffExtras joins extras to DefaultHandoff. A service that predates
// extras takes the whole line as a custom handoff, which still reads well.
const handoffExtras = " The builder adds: "

// Handoff is the handoff of a switch that continues from the handover, with
// the builder's extra instructions for it, if any.
func Handoff(extra string) string {
	if extra = strings.TrimSpace(extra); extra != "" {
		return DefaultHandoff + handoffExtras + extra
	}
	return DefaultHandoff
}

// HandoffExtra reports whether h continues from the handover, and its extras.
func HandoffExtra(h string) (extra string, ok bool) {
	if h == DefaultHandoff {
		return "", true
	}
	extra, ok = strings.CutPrefix(h, DefaultHandoff+handoffExtras)
	extra = strings.TrimSpace(extra)
	return extra, ok && extra != ""
}
