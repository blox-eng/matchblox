// Package demo is the real console on a scripted machine: no service, no tmux,
// no agents. SPIKE: throwaway, to measure and to feel.
package demo

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/app"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

const askID = "toolu_demo"

var options = []string{"Rotate the token, keep the old one a minute", "Stop the old token at once"}

const invite = "\n● Done. The token rotates, and no one was logged out.\n\n" +
	"  That was a demo. Your own agents wait the same way:\n\n" +
	"    curl -fsSL https://matchblox.sh | sh\n\n" +
	"  Then choose \"Pair a device\" and scan its code.\n"

// Conn is the service of the demo machine.
type Conn struct {
	mu     sync.Mutex
	out    chan proto.Envelope
	stop   chan struct{}
	once   sync.Once
	queue  *queue.Queue
	start  time.Time
	cursor int
	picked string
	done   bool
	watch  string
}

// New starts the demo machine.
func New() *Conn {
	c := &Conn{out: make(chan proto.Envelope, 64), stop: make(chan struct{}), queue: queue.New(), start: time.Now()}
	go c.tick()
	return c
}

func (c *Conn) tick() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-t.C:
			c.mu.Lock()
			c.snapshot()
			c.screen()
			c.mu.Unlock()
		}
	}
}

func (c *Conn) post(kind proto.Kind, id string, body any) {
	raw, err := json.Marshal(body)
	if err != nil {
		return
	}
	select {
	case c.out <- proto.Envelope{Kind: kind, ID: id, Body: raw}:
	case <-c.stop:
	default:
	}
}

func (c *Conn) sessions() []sample.Session {
	now := time.Now()
	flint := sample.Session{
		PID: 101, Start: 1, Pane: "%1", Target: "flint-auth:0.0", Name: "flint-auth", Agent: "claude",
		Cwd: "/work/flint-auth", Model: "opus", Started: c.start.Add(-40 * time.Minute), ContextPct: 41, Tokens: 82000, Window: 200000,
	}
	flint.Status, flint.Idle, flint.LastLine = "idle", 40*time.Second, "Which way do we rotate the token?"
	switch {
	case c.picked == "":
		flint.Asks = true
		flint.Ask = &said.Ask{ID: askID, Questions: []said.Question{{Question: "Which way do we rotate the token?", Options: []said.Option{{Label: options[0]}, {Label: options[1]}}}}}
	case !c.done:
		flint.Busy, flint.Status, flint.LastLine = true, "busy", "Rotating…"
	default:
		flint.LastLine = "Done. The token rotates."
	}
	return []sample.Session{
		flint,
		{PID: 102, Start: 1, Pane: "%2", Target: "rate-docs:0.0", Name: "rate-docs", Agent: "claude", Cwd: "/work/flint-auth", Model: "sonnet", Busy: true, Status: "busy", Started: now.Add(-12 * time.Minute), ContextPct: 18, Tokens: 36000, Window: 200000},
		{PID: 103, Start: 1, Pane: "%3", Target: "ember-api:0.0", Name: "ember-api", Agent: "codex", Cwd: "/work/ember", Model: "gpt", Status: "busy", Busy: true, Started: now.Add(-3 * time.Hour), ContextPct: 77, Tokens: 154000, Window: 200000},
	}
}

func (c *Conn) snapshot() {
	now := time.Now()
	snap := sample.Snapshot{At: now, Sessions: c.sessions()}
	c.queue.Merge(snap.Sessions, now)
	c.post(proto.KindSnapshot, "", proto.State{Doc: state.Doc{Snapshot: snap, Queue: c.queue.Items()}})
}

func (c *Conn) text() string {
	head := "❯ Rotate the login token without logging anyone out.\n\n● I found the check in auth/token.go.\n"
	switch {
	case c.picked == "":
		var b strings.Builder
		b.WriteString(head + "────────────────────────\n ☐ Token\n\nWhich way do we rotate the token?\n\n")
		for i, o := range options {
			mark := "  "
			if i == c.cursor {
				mark = "❯ "
			}
			fmt.Fprintf(&b, "%s%d. %s\n", mark, i+1, o)
		}
		b.WriteString("\nEnter to select · ↑/↓ to navigate · Esc to cancel\n")
		return b.String()
	case !c.done:
		return head + "\n● " + c.picked + ".\n\n✻ Rotating…\n"
	}
	return head + "\n● " + c.picked + "." + invite
}

func (c *Conn) screen() {
	switch c.watch {
	case "":
	case "%1":
		c.post(proto.KindScreen, "", proto.Screen{Pane: "%1", At: time.Now(), Text: c.text()})
	case "%2":
		c.post(proto.KindScreen, "", proto.Screen{Pane: "%2", At: time.Now(), Text: "● Writing the rate limit docs.\n\n✻ Writing docs/limits.md…\n"})
	default:
		c.post(proto.KindScreen, "", proto.Screen{Pane: c.watch, At: time.Now(), Text: "● All tests pass. The migration is ready.\n"})
	}
}

// press plays one key of the chat on flint-auth's menu.
func (c *Conn) press(key string) {
	if c.picked != "" {
		return
	}
	pick := -1
	switch key {
	case "Up":
		c.cursor = max(c.cursor-1, 0)
	case "Down":
		c.cursor = min(c.cursor+1, len(options)-1)
	case "Enter":
		pick = c.cursor
	default:
		if n, err := strconv.Atoi(strings.TrimPrefix(key, proto.LitPrefix)); err == nil && n >= 1 && n <= len(options) {
			pick = n - 1
		}
	}
	if pick < 0 {
		return
	}
	c.picked = options[pick]
	time.AfterFunc(3*time.Second, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.done = true
		c.snapshot()
		c.screen()
	})
}

// Send is the console speaking to the machine.
func (c *Conn) Send(kind proto.Kind, id string, body any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch kind {
	case proto.KindHello:
		c.post(proto.KindHello, "", proto.Hello{Version: proto.Version, Binary: "demo", Host: "demo", OS: "ios", Arch: "arm64", Client: proto.ClientConsole, Tmux: "none"})
		c.snapshot()
	case proto.KindAct:
		a, ok := body.(proto.Act)
		if !ok {
			return nil
		}
		switch {
		case strings.HasPrefix(a.RecID, proto.WatchPrefix):
			c.watch = strings.TrimPrefix(a.RecID, proto.WatchPrefix)
			c.post(proto.KindResult, id, proto.Result{ActID: id})
		case strings.HasPrefix(a.RecID, proto.KeysPrefix):
			if strings.TrimPrefix(a.RecID, proto.KeysPrefix) == "%1" {
				c.press(a.Text)
			}
			c.post(proto.KindResult, id, proto.Result{ActID: id})
			c.snapshot()
		default:
			c.post(proto.KindResult, id, proto.Result{ActID: id, Err: "this is a demo: install matchblox to do that"})
			return nil
		}
		c.screen()
	}
	return nil
}

// Recv is what the machine says next.
func (c *Conn) Recv() (proto.Envelope, error) {
	select {
	case env := <-c.out:
		return env, nil
	case <-c.stop:
		return proto.Envelope{}, io.EOF
	}
}

// Close stops the machine.
func (c *Conn) Close() error {
	c.once.Do(func() { close(c.stop) })
	return nil
}

// Options is the console on the demo machine.
func Options(c *Conn) app.Options {
	return app.Options{Conn: c, Binary: "demo", Run: func([]string) error { return fmt.Errorf("this is a demo") }}
}

// Program is the console on the demo machine, on the streams of a terminal view.
func Program(in io.Reader, out io.Writer, cols, rows int) (*tea.Program, *Conn) {
	c := New()
	return tea.NewProgram(app.New(Options(c)), tea.WithInput(in), tea.WithOutput(out), tea.WithWindowSize(cols, rows),
		tea.WithFPS(15), tea.WithEnvironment([]string{"TERM=xterm-256color", "COLORTERM=truecolor"})), c
}
