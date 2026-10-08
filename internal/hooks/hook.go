// Package hooks reads the events a coding agent sends through its hooks
// (`matchblox hook <event>`), and keeps them in a spool while no service
// listens.
package hooks

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/said"
)

// Notification kinds the queue reads. Other kinds pass through as they are.
const (
	KindPermission = "permission"
	KindIdlePrompt = "idle_prompt"
)

type Event struct {
	Name      string    `json:"name"` // Notification | Stop | UserPromptSubmit | SessionStart | SessionEnd
	Kind      string    `json:"kind,omitempty"`
	SessionID string    `json:"session_id"`
	Cwd       string    `json:"cwd,omitempty"`
	Message   string    `json:"message,omitempty"`
	Asks      bool      `json:"asks,omitempty"` // Stop: the reply ends on a question
	At        time.Time `json:"at"`
	Pane      string    `json:"pane,omitempty"` // $TMUX_PANE
}

// input is the Claude Code hook JSON, only the fields we read.
type input struct {
	SessionID        string `json:"session_id"`
	Cwd              string `json:"cwd"`
	HookEventName    string `json:"hook_event_name"`
	Message          string `json:"message"`
	NotificationType string `json:"notification_type"`
	LastAssistant    string `json:"last_assistant_message"`
}

var ErrNoSession = errors.New("hooks: the event has no session_id")

// Parse reads one hook event. name is the event the hook was set for; when
// it is empty, the event name comes from the input.
func Parse(stdin io.Reader, name string, env func(string) string) (Event, error) {
	var in input
	if err := json.NewDecoder(io.LimitReader(stdin, 1<<20)).Decode(&in); err != nil {
		return Event{}, err
	}
	if in.SessionID == "" {
		return Event{}, ErrNoSession
	}
	if name == "" {
		name = in.HookEventName
	}
	ev := Event{Name: name, SessionID: in.SessionID, Cwd: in.Cwd, At: time.Now().UTC(), Pane: env("TMUX_PANE")}
	switch name {
	case "Notification":
		ev.Message = cut(strings.ReplaceAll(said.Clean(in.Message), "\n", " "))
		ev.Kind = notificationKind(in.NotificationType, in.Message)
	case "Stop":
		line, asks := said.Reply(in.LastAssistant)
		ev.Message, ev.Asks = cut(line), asks
	}
	return ev, nil
}

// notificationKind reads notification_type, or the message on older agent
// versions that do not send it.
func notificationKind(typ, msg string) string {
	switch typ {
	case "permission_prompt":
		return KindPermission
	case "":
	default:
		return typ
	}
	switch m := strings.ToLower(msg); {
	case strings.Contains(m, "permission"):
		return KindPermission
	case strings.Contains(m, "waiting for your input"):
		return KindIdlePrompt
	}
	return ""
}

func cut(line string) string {
	if r := []rune(line); len(r) > 200 {
		line = string(r[:199]) + "…"
	}
	return line
}

// ProgressPrompt is what the SessionStart hook adds to a new session: the
// console reads the bar from the agent's last reply.
const ProgressPrompt = "matchblox shows your progress. At the end of each reply, write one line: " +
	"Progress [████░░░░] <the step you do now>. Fill the bar for the part of the task that is done."
