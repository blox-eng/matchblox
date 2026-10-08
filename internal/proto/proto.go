// Package proto is the wire between the service and a console: one JSON
// envelope on each line, hello first from each side.
package proto

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/history"
	"github.com/blox-eng/matchblox/internal/state"
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
	// KindHook is the first and only message of `matchblox hook`: one
	// hooks.Event, sent after the service hello, with no hello back.
	KindHook Kind = "hook"
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
}

// State is what the console draws. History is set only in the first
// snapshot to each console; later snapshots leave it nil and the console
// extends it from each sample.
type State struct {
	state.Doc
	History *history.Series `json:"history,omitempty"`
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
	// Picks are the choices of a door the person picked (its hosts).
	Picks []string `json:"picks,omitempty"`
}

type Result struct {
	ActID   string     `json:"act_id"`
	Ran     [][]string `json:"ran,omitempty"`
	Skipped []string   `json:"skipped,omitempty"`
	Err     string     `json:"err,omitempty"`
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

// The console draws git data without importing the scanner.
type (
	GitReport = gitscan.Report
	Worktree  = gitscan.Worktree
)
