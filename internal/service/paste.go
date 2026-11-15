package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/proto"
)

// paste puts a text of many lines into the pane of a session that does not
// wait, as one bracketed paste, then presses Enter: the agent reads it as
// one message. It goes where a message goes, under the same guards. The
// text goes into a tmux buffer of its own through stdin, so two pastes at
// once never mix.
func (s *Service) paste(ctx context.Context, pane string, a proto.Act) proto.Result {
	if a.Confirm != "y" {
		return proto.Result{Err: "needs confirm"}
	}
	x, ok := s.sessionAt(pane)
	if !ok {
		return proto.Result{Err: "no agent session in " + pane + " any more"}
	}
	if err := s.messageable(pane, x.PID); err != nil {
		return proto.Result{Err: err.Error()}
	}
	text := pasteText(a.Text)
	switch {
	case strings.TrimSpace(text) == "":
		return proto.Result{Err: "the paste is empty"}
	case len(text) > proto.MaxPaste:
		return proto.Result{Err: fmt.Sprintf("the paste is %d bytes: the most is %d", len(text), proto.MaxPaste)}
	}
	m := s.mux()
	if m.NoBracketedPaste(pane) {
		return proto.Result{Err: x.Name + " does not take a paste · send it as one line"}
	}
	name, err := pasteBuffer()
	if err != nil {
		return proto.Result{Err: "no paste buffer: " + err.Error()}
	}
	release := s.holdPane(pane)
	defer release()
	if err := m.Load(pane, name, text); err != nil {
		return proto.Result{Err: "the paste did not load: " + err.Error()}
	}
	g := messageGuard(pane, x)
	res := s.Actions.Do(ctx, advice.Action{
		Label: "paste", Destructive: true,
		Steps:  [][]string{m.Paste(pane, name), m.Keys(pane, "Enter")},
		Guards: []advice.Guard{g, g},
	}, a.Confirm)
	if len(res.Ran) == 0 {
		_ = m.Drop(pane, name)
	}
	return res
}

// pasteText is text without a key a terminal reads: every C0 control
// character but \n and \t (ESC among them), DEL, the C1 characters, and
// bytes that are no UTF-8. An ESC sequence in it stays as plain text.
func pasteText(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, strings.ToValidUTF8(text, ""))
}

// pasteBuffer is a tmux buffer name no other paste uses.
func pasteBuffer() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "matchblox-paste-" + hex.EncodeToString(b), nil
}
