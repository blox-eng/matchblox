package app

import (
	"fmt"
	"slices"
	"strings"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/remote"
)

// The layers of the console, as in k9s: Hosts above the console of one
// host (design/0005-remote-mode.md §1).
type layer int

const (
	layerHosts layer = iota
	layerConsole
)

// ShellOptions are what the layers need from the machine. The shell never
// reads files itself: main gives it these.
type ShellOptions struct {
	// Base holds what each console shares: the version, the clock, motion.
	Base Options
	// Hosts are the hosts the builder added (~/.config/matchblox/hosts).
	Hosts func() []string
	// SSHHosts are the hosts of ~/.ssh/config, to add one.
	SSHHosts func() []string
	// Open dials a host, "" for this machine, and returns its console's
	// options: the connection, the redial, the key.
	Open func(host string) Options
	// Remove takes a host out of the list.
	Remove func(host string) error
	// Exec gives this terminal to argv until it exits. Nil: tea.ExecProcess.
	Exec func(argv []string, done func(error) tea.Msg) tea.Cmd
	// Start is the host the shell opens first; "" decides by the list.
	Start string
	// FirstRun asks where the agents run before anything else; Answered
	// records that the builder answered.
	FirstRun bool
	Answered func()
	// TailscaleGate says that the gate does not hold on a host Tailscale
	// SSH answers (setup.TailscaleGateMessage, #185). The Hosts row of such
	// a host shows it when selected.
	TailscaleGate string
}

// Shell is the top of the console: the hosts layer, or one host's console.
type Shell struct {
	opt     ShellOptions
	layer   layer
	cur     Model
	curHost string
	hosts   []string
	sel     int
	adding  bool    // the rows are hosts to add
	input   *string // a host being typed
	pending *action // connect or remove, waiting for its y
	// connecting is the field of a connect that runs: its ssh asks there.
	connecting *Model
	flash      string
	width      int
	height     int
	dark       bool
	st         styles
	gen        int          // the generation of the last console opened
	offers     *pairWaits   // the pair offers of every console: Close waits for all
	keys       shellKeymaps // the Hosts list keys
	// tailscale: the hosts that are Tailscale SSH, as their consoles' last
	// check said.
	tailscale map[string]bool
}

// layerMsg asks the shell to go back to Hosts.
type layerMsg struct{}

// connectedHostMsg: `matchblox connect` gave the terminal back.
type connectedHostMsg struct {
	host string
	err  error
}

func NewShell(opt ShellOptions) Shell {
	s := Shell{opt: opt, width: 100, height: 30, dark: true, st: newStyles(true), offers: &pairWaits{}, keys: defaultShellKeys(), tailscale: map[string]bool{}}
	s.hosts = s.readHosts()
	switch {
	case opt.Start != "":
		s, _ = s.open(opt.Start, true)
	case len(s.hosts) == 0 && !opt.FirstRun:
		s, _ = s.open("", true)
	}
	return s
}

func (s Shell) readHosts() []string {
	if s.opt.Hosts == nil {
		return nil
	}
	return s.opt.Hosts()
}

// open starts the console of a host. The start screen plays only for the
// first console of a run.
func (s Shell) open(host string, first bool) (Shell, tea.Cmd) {
	s.cur.stopAsk()
	o := s.opt.Open(host)
	b := s.opt.Base
	o.Binary, o.CompactAt, o.CompactTokens, o.NoMotion, o.Now, o.Self, o.StateDir = b.Binary, b.CompactAt, b.CompactTokens, b.NoMotion, b.Now, b.Self, b.StateDir
	o.Clip = b.Clip
	// A jump starts from this machine's tmux, whatever the host.
	o.OutsideTmux, o.Prefix, o.Home, o.LocalTmux, o.JumpID = b.OutsideTmux, b.Prefix, b.Home, b.LocalTmux, b.JumpID
	// The builder's own ssh login goes from this machine, whatever the host.
	o.Check, o.MasterBase, o.SSH = b.Check, b.MasterBase, b.SSH
	o.Ask, o.NoAsk = b.Ask, b.NoAsk // the prompts of this machine's ssh
	o.Keys = b.Keys                 // the builder's keys are on this machine
	o.Graphics = b.Graphics         // the logos draw in this terminal, whatever the host
	if host == "" {
		o.Pair = b.Pair // the key goes in this machine's authorized_keys
	}
	if o.Run == nil {
		o.Run = b.Run
	}
	if o.Exec == nil {
		o.Exec = s.opt.Exec
	}
	s.gen++
	o.Layered, o.Gen = true, s.gen
	m := New(o)
	m.width, m.height, m.dark, m.st = s.width, s.height, s.dark, s.st
	m.splashDone = !first
	m.pairOffers = s.offers
	m.editLogin(host, func(l *hostLogin) { l.tailscale = s.tailscale[host] }) // until its check says
	s.cur, s.curHost, s.layer = m, host, layerConsole
	return s, m.Init()
}

func (s Shell) Init() tea.Cmd {
	if s.layer == layerConsole {
		return s.cur.Init()
	}
	return tea.RequestBackgroundColor
}

func (s Shell) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if s.connecting != nil {
		if next, cmd, ok := s.connectUpdate(msg); ok {
			return next, cmd
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width, s.height = msg.Width, msg.Height
		if s.connecting != nil {
			s.connecting.width, s.connecting.height = s.width, s.height
		}
	case tea.BackgroundColorMsg:
		s.dark = msg.IsDark()
		s.st = newStyles(s.dark)
		if s.connecting != nil {
			s.connecting.dark, s.connecting.st = s.dark, s.st
		}
	case layerMsg:
		return s.back(), nil
	case connectedHostMsg:
		if msg.err != nil {
			s.flash = "not connected: " + msg.err.Error()
			return s, nil
		}
		s.hosts, s.adding = s.readHosts(), false
		return s.open(msg.host, false)
	}
	if s.layer == layerConsole {
		next, cmd := s.cur.Update(msg)
		s.cur = next.(Model)
		return s, cmd
	}
	switch msg := msg.(type) {
	case connMsg:
		_ = msg.conn.Close() // a closed console's late connection
	case tea.KeyPressMsg:
		return s.key(msg)
	case tea.MouseClickMsg:
		return s.tap(msg.Mouse())
	}
	return s, nil
}

// back closes the console and shows Hosts with its host selected.
func (s Shell) back() Shell {
	if s.cur.conn != nil {
		_ = s.cur.conn.Close()
	}
	s.cur = s.cur.closePairPage("")
	s.cur.stopAsk()       // an open prompt is cancelled
	s.cur.quitting = true // its late messages find nothing to do
	if s.curHost != "" {
		s.tailscale[s.curHost] = s.cur.loginOf(s.curHost).tailscale
	}
	s.layer, s.adding, s.input, s.pending, s.flash = layerHosts, false, nil, nil, ""
	s.hosts = s.readHosts()
	s.sel = 0
	if i := slices.Index(s.hosts, s.curHost); i >= 0 {
		s.sel = i + 1
	}
	return s
}

// row is one line of the hosts layer: a host to open, or what to do.
type row struct {
	label, note string
	host        string // "" with open: this machine
	open        bool   // Enter opens host
	add         bool   // Enter lists the hosts to add
	connect     bool   // Enter connects host
	typed       bool   // Enter types a host
	removable   bool
	tailscale   bool // Tailscale SSH answers host: the gate does not hold there
}

func (s Shell) firstRun() bool { return s.opt.FirstRun && len(s.hosts) == 0 }

func (s Shell) rows() []row {
	if s.adding {
		var rs []row
		if s.opt.SSHHosts != nil {
			for _, h := range s.opt.SSHHosts() {
				if !slices.Contains(s.hosts, h) {
					rs = append(rs, row{label: h, note: "from ~/.ssh/config", host: h, connect: true})
				}
			}
		}
		return append(rs, row{label: "type a host…", note: "a name or user@address that ssh reaches", typed: true})
	}
	rs := []row{{label: "this machine", note: "the agents here", open: true}}
	if s.firstRun() {
		return append(rs, row{label: "another machine", note: "a host you reach with ssh", add: true})
	}
	for _, h := range s.hosts {
		r := row{label: h, note: "over ssh", host: h, open: true, removable: true, tailscale: s.tailscale[h]}
		if r.tailscale {
			r.note = "not gated, Tailscale SSH: tailscale set --ssh=false there"
		}
		rs = append(rs, r)
	}
	return append(rs, row{label: "+ add a host", note: "from ~/.ssh/config, or type one", add: true})
}

func (s Shell) key(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	sk := s.keys.shell
	// ctrl+c quits from Hosts, its name line and its questions too.
	if bkey.Matches(k, sk.Interrupt) {
		return s, tea.Quit
	}
	if s.input != nil {
		return s.typing(k)
	}
	if s.pending != nil {
		a := *s.pending
		s.pending = nil
		if bkey.Matches(k, sk.Yes) {
			return s.confirm(a)
		}
		s.flash = "cancelled: " + a.String()
		return s, nil
	}
	s.flash = ""
	rs := s.rows()
	switch {
	case bkey.Matches(k, sk.Quit):
		return s, tea.Quit
	case bkey.Matches(k, sk.Up):
		s.sel = max(s.sel-1, 0)
	case bkey.Matches(k, sk.Down):
		s.sel = min(s.sel+1, len(rs)-1)
	case bkey.Matches(k, sk.Back):
		if s.adding {
			s.adding, s.sel = false, len(s.hosts)+1
		}
	case bkey.Matches(k, sk.Remove):
		if r := rs[min(s.sel, len(rs)-1)]; r.removable {
			s.pending = &action{label: "remove", say: "remove " + r.host + " from the list (matchblox stays on " + r.host + ")", door: r.host, destructive: true}
		}
	case bkey.Matches(k, sk.Open):
		return s.enter(rs[min(s.sel, len(rs)-1)])
	}
	return s, nil
}

func (s Shell) enter(r row) (tea.Model, tea.Cmd) {
	if s.firstRun() && !s.adding && (r.open || r.add) {
		s.opt.FirstRun = false
		if s.opt.Answered != nil {
			s.opt.Answered()
		}
	}
	switch {
	case r.open:
		return s.open(r.host, false)
	case r.add:
		s.adding, s.sel = true, 0
	case r.typed:
		empty := ""
		s.input = &empty
	case r.connect:
		s.pending = s.connect(r.host)
	}
	return s, nil
}

// connect is `matchblox connect <host>` in this terminal: the ssh login is
// the builder's own (design/0005-remote-mode.md §3.2).
func (s Shell) connect(host string) *action {
	self := s.opt.Base.Self
	if self == "" {
		self = "matchblox"
	}
	argv := []string{self, "connect", host}
	return &action{label: "connect", say: shellLine(argv), steps: [][]string{argv}, destructive: true, connect: true, door: host}
}

func (s Shell) confirm(a action) (tea.Model, tea.Cmd) {
	host := a.door
	if a.connect && s.opt.Base.Ask != nil {
		s.gen++
		f, cmd := s.connectField(host, s.gen).connect(a.steps[0])
		s.connecting, s.flash = &f, "connecting "+host+"…"
		return s, cmd
	}
	if a.connect { // no prompt socket: ssh asks in this terminal
		s.flash = "running: " + a.String()
		done := func(err error) tea.Msg { return connectedHostMsg{host, err} }
		if s.opt.Exec != nil {
			return s, s.opt.Exec(a.steps[0], done)
		}
		return s, Model{}.exec(a.steps[0], done)
	}
	if s.opt.Remove == nil {
		return s, nil
	}
	if err := s.opt.Remove(host); err != nil {
		s.flash = "not removed: " + err.Error()
		return s, nil
	}
	s.hosts = s.readHosts()
	s.sel = min(s.sel, len(s.rows())-1)
	s.flash = "removed " + host
	return s, nil
}

// typing edits a host name; Enter shows its connect.
func (s Shell) typing(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	text := *s.input
	tk := s.keys.typing
	switch {
	case bkey.Matches(k, tk.Cancel):
		s.input = nil
		return s, nil
	case bkey.Matches(k, tk.Connect):
		s.input = nil
		if !remote.ValidHost(text) {
			s.flash = fmt.Sprintf("%q is not a host name: a name or user@address that ssh reaches", text)
			return s, nil
		}
		s.pending = s.connect(text)
		return s, nil
	case bkey.Matches(k, tk.Erase):
		text = trimLast(text)
	default:
		if k.Text != "" && len(text) < 200 {
			text += k.Text
		}
	}
	s.input = &text
	return s, nil
}

// tap selects a row; a tap on the selected row does what Enter does.
func (s Shell) tap(ms tea.Mouse) (tea.Model, tea.Cmd) {
	if ms.Button != tea.MouseLeft || s.pending != nil || s.input != nil {
		return s, nil
	}
	rs := s.rows()
	y := ms.Y - hostsTop
	i := y + s.scrollTop(len(rs))
	if y < 0 || y >= s.rowRoom() || i >= len(rs) {
		return s, nil
	}
	if i != s.sel {
		s.sel = i
		return s, nil
	}
	return s.enter(rs[i])
}

// rowRoom is how many rows fit under the label, with the hint and, at the
// bottom, a hairline and the action line.
func (s Shell) rowRoom() int { return max(3, s.height-hostsTop-4) }

// scrollTop is the first row on the screen: the list follows the
// selection, so every row is reachable.
func (s Shell) scrollTop(n int) int {
	room := s.rowRoom()
	if n <= room || s.sel < room {
		return 0
	}
	return min(s.sel-room+1, n-room)
}

// hostsTop is the first row's line: the header, a hairline and the region
// label.
const hostsTop = 3

func (s Shell) View() tea.View {
	if s.layer == layerConsole {
		return s.cur.View()
	}
	v := tea.NewView(s.render())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (s Shell) render() string {
	if s.layer == layerConsole {
		return s.cur.render()
	}
	st, w := s.st, max(s.width, minWidth)
	out := []string{
		fit(" "+st.accent.Render("▰")+" "+st.text.Render("matchblox")+st.faint.Render(" · ")+st.muted.Render("hosts"), w),
		st.hair.Render(strings.Repeat("─", w)),
	}
	label := fmt.Sprintf(" HOSTS · %d", len(s.hosts)+1)
	switch {
	case s.adding:
		label = " ADD A HOST"
	case s.firstRun():
		label = " WHERE DO YOUR AGENTS RUN?"
	}
	out = append(out, st.label.Render(fit(label, w)))
	var b body
	rs := s.rows()
	top := s.scrollTop(len(rs))
	for i, r := range rs[top:min(len(rs), top+s.rowRoom())] {
		i += top
		mark := "  "
		if i == s.sel {
			mark = "▌ "
		}
		line := " " + st.accent.Render(mark) + st.text.Render(pad(r.label, 24)) + st.muted.Render(r.note)
		if layout(w, s.height) == Narrow {
			line = " " + st.accent.Render(mark) + st.text.Render(r.label)
		}
		b.addRow(i, i == s.sel, w, st, line)
	}
	out = append(out, b.lines...)
	if s.sel < len(rs) && rs[s.sel].tailscale && s.opt.TailscaleGate != "" {
		out = append(out, "")
		why := rs[s.sel].host + ": " + s.opt.TailscaleGate + " See docs/remote.md#tailscale-ssh."
		for l := range strings.SplitSeq(ansi.Wrap(why, max(w-4, 8), ""), "\n") {
			out = append(out, " "+st.warn.Render("! ")+st.text.Render(l))
		}
	}
	if s.firstRun() && !s.adding {
		out = append(out, "", fit(" "+st.faint.Render("You can add a host later: "+defaultKeys().main.Back.Help().Key+" in a console comes back here."), w))
	}
	// The action line is at the bottom, as in a console (design/0006 §11.1);
	// a step to confirm is a box over the dimmed list (#158).
	for len(out) < s.height-2 {
		out = append(out, "")
	}
	out = append(out, st.hair.Render(strings.Repeat("─", w)), s.actionLine(w))
	if s.pending != nil {
		phone := layout(w, s.height) == Narrow || portrait(w, s.height)
		bw := min(w, 72)
		if !phone {
			bw = min(s.pending.width()+4, 72, w-8)
		}
		layBox(st, out, w, bw, phone, stepBox(st, *s.pending, bw, s.keys.shell.Yes, s.keys.shell.Open, s.keys.shell.Back))
	}
	if f := s.connecting; f != nil && f.asking() {
		phone := layout(w, s.height) == Narrow || portrait(w, s.height)
		bw := min(w, 72)
		if !phone {
			bw = min(72, w-8)
		}
		layBox(st, out, w, bw, phone, f.fieldBox(bw, len(out)-3))
	}
	return strings.Join(out, "\n")
}

func (s Shell) actionLine(w int) string {
	st, sk := s.st, s.keys.shell
	switch {
	case s.input != nil:
		return fit(" "+st.label.Render("HOST ")+st.text.Render(*s.input)+st.accent.Render("▏")+"  "+st.faint.Render(plainHints(hintFor(s.keys.typing.Connect, ""), hintFor(s.keys.typing.Cancel, ""))), w)
	case s.pending != nil:
		return "" // the box says it all
	case s.flash != "":
		return fit(" "+st.text.Render(s.flash), w)
	case s.adding:
		return fit(" "+st.muted.Render(plainHints(hintFor(sk.Up, ""), hintFor(sk.Open, "connect"), hintFor(sk.Back, ""), hintFor(sk.Quit, ""))), w)
	}
	keys := plainHints(hintFor(sk.Up, ""), hintFor(sk.Open, ""), hintFor(sk.Quit, ""))
	if rs := s.rows(); s.sel < len(rs) && rs[s.sel].removable {
		keys = plainHints(hintFor(sk.Up, ""), hintFor(sk.Open, ""), hintFor(sk.Remove, ""), hintFor(sk.Quit, ""))
	}
	return fit(" "+st.muted.Render(keys), w)
}

// Close ends the open console's connection and its pair code when the
// program ends.
func (s Shell) Close() {
	s.cur.ClosePair()
	s.cur.stopAsk()
	if s.connecting != nil {
		s.connecting.stopAsk()
	}
	if s.layer == layerConsole && s.cur.conn != nil {
		_ = s.cur.conn.Close()
	}
}
