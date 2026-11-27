package demo

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/stoker"
	"github.com/blox-eng/matchblox/internal/transport"
)

// notDemo answers any line but Pocket's three commands.
const notDemo = "not a demo command"

// askWait bounds one command's act, as the service may wait on a pane.
const askWait = 10 * time.Second

// Run answers one of Pocket's commands as matchblox prints it on a host:
// `menu <who> --json`, `choose <who> <n> --text <t> --sign <s>` and
// `sessions --json`, each word as a shell reads it. ok is false where
// matchblox would fail, and out then says why.
func (d *Demo) Run(line string) (out string, ok bool) {
	w, err := words(line)
	switch {
	case err != nil:
	case slices.Equal(w, []string{"sessions", "--json"}):
		return d.sessions()
	case len(w) == 3 && w[0] == "menu" && w[2] == "--json":
		return d.menu(w[1])
	case len(w) == 7 && w[0] == "choose" && w[3] == "--text" && w[5] == "--sign":
		return d.choose(w[1], w[2], w[4], w[6])
	}
	return notDemo, false
}

// sessionRow is a row of `sessions --json`, the part Pocket reads.
type sessionRow struct {
	N      int    `json:"n,omitempty"`
	Name   string `json:"name"`
	Pane   string `json:"pane"`
	Status string `json:"status"`
	Word   string `json:"word"`
}

// sessions lists the demo's sessions, each with the number `console --open
// <n>` focuses; the first list gives each its number, as a phone's register
// does on a host.
func (d *Demo) sessions() (string, bool) {
	rows := []sessionRow{}
	for _, s := range d.svc.State().Sessions {
		r := sessionRow{Name: s.Name, Pane: s.Pane, Status: s.Status, Word: stoker.Word(s)}
		if s.SessionID != "" {
			r.N = d.svc.PushNumbers.Session(s.SessionID)
		}
		rows = append(rows, r)
	}
	return printed(rows)
}

func (d *Demo) menu(who string) (string, bool) {
	s, err := d.resolve(who)
	if err != nil {
		return "menu " + who + ": " + err.Error(), false
	}
	res, err := d.ask(proto.Act{RecID: proto.MenuPrefix + s.Pane})
	if err == nil && res.Err != "" {
		err = errors.New(res.Err)
	}
	if err != nil {
		return "menu " + who + ": " + err.Error(), false
	}
	m := proto.Menu{Options: []proto.Option{}}
	if res.Menu != nil {
		m.Chosen, m.Sign = res.Menu.Chosen, res.Menu.Sign
		m.Options = append(m.Options, res.Menu.Options...)
	}
	return printed(m)
}

func (d *Demo) choose(who, digit, text, sign string) (string, bool) {
	name := "choose " + who
	s, err := d.resolve(who)
	if err != nil {
		return name + ": " + err.Error(), false
	}
	if n, err := strconv.Atoi(digit); err != nil || n < 1 || n > 9 || strconv.Itoa(n) != digit {
		return fmt.Sprintf("choose picks an option from 1 to 9, not %q", digit), false
	}
	if text == "" || sign == "" {
		return "choose needs the option's --text and the menu's --sign, as menu printed them", false
	}
	res, err := d.ask(proto.Act{RecID: proto.ChoosePrefix + s.Pane, Text: digit + "\x00" + text + "\x00" + sign,
		PID: s.PID, Start: s.Start, Confirm: "y"})
	switch {
	case err != nil:
		return name + ": did not run: " + err.Error(), false
	case res.Err != "":
		return name + ": refused: " + res.Err, false
	case len(res.Skipped) > 0:
		return name + ": refused: " + strings.Join(res.Skipped, "; "), false
	}
	var b strings.Builder
	for _, argv := range res.Ran {
		b.WriteString(strings.Join(argv, " ") + "\n")
	}
	return b.String(), true
}

// resolve finds the session who names by pane, target or name, as matchblox
// does on a host.
func (d *Demo) resolve(who string) (sample.Session, error) {
	var hit []sample.Session
	for _, s := range d.svc.State().Sessions {
		if s.Pane == who || s.Target == who {
			return s, nil
		}
		if s.Name == who {
			hit = append(hit, s)
		}
	}
	switch len(hit) {
	case 0:
		return sample.Session{}, errors.New("no session named " + who)
	case 1:
		return hit[0], nil
	}
	return sample.Session{}, fmt.Errorf("%s is %d sessions; name the pane", who, len(hit))
}

// ask sends a to the service as Pocket's commands do, saying hello as
// Pocket, and waits for its result.
func (d *Demo) ask(a proto.Act) (proto.Result, error) {
	mine, theirs := net.Pipe()
	go func() { _ = d.svc.Handle(d.ctx, transport.NewConn(theirs)) }()
	c := transport.NewConn(mine)
	defer c.Close()
	timer := time.AfterFunc(askWait, func() { c.Close() })
	defer timer.Stop()
	// A pipe holds nothing: the service's hello is read before ours goes.
	if _, err := c.Recv(); err != nil {
		return proto.Result{}, err
	}
	if err := c.Send(proto.KindHello, "", proto.Hello{Version: proto.Version, Binary: binary, Client: proto.ClientPocket}); err != nil {
		return proto.Result{}, err
	}
	const id = "pocket"
	if err := c.Send(proto.KindAct, id, a); err != nil {
		return proto.Result{}, err
	}
	for {
		env, err := c.Recv()
		if err != nil {
			return proto.Result{}, errors.New("the service took the act and did not answer")
		}
		var res proto.Result
		if env.Kind == proto.KindResult && env.ID == id && json.Unmarshal(env.Body, &res) == nil {
			return res, nil
		}
	}
}

func printed(v any) (string, bool) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err.Error(), false
	}
	return string(b) + "\n", true
}

// words splits line as a shell reads Pocket's quoting: blanks between
// words, single quotes around a word's text, and a backslash before a
// character outside quotes, which is how a quoted word holds a quote.
func words(line string) ([]string, error) {
	var out []string
	var w strings.Builder
	begun, quoted := false, false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quoted && c == '\'':
			quoted = false
		case quoted:
			w.WriteByte(c)
		case c == '\'':
			quoted, begun = true, true
		case c == '\\' && i+1 < len(line):
			i++
			w.WriteByte(line[i])
			begun = true
		case c == ' ':
			if begun {
				out = append(out, w.String())
				w.Reset()
				begun = false
			}
		default:
			w.WriteByte(c)
			begun = true
		}
	}
	if quoted {
		return nil, errors.New("a quote is not closed")
	}
	if begun {
		out = append(out, w.String())
	}
	return out, nil
}
