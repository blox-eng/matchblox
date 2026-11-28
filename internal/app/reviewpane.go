package app

import (
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

const (
	// reviewSendMax is the most bytes of one review message: under the
	// service's paste cap (proto.MaxPaste), with room.
	reviewSendMax = 15000
	// reviewWait is how many quarter-seconds the pane waits for the list to
	// take its word, and reviewSendWait for the service's answer to the
	// paste, before it says so and takes the keys back.
	reviewWait     = 12
	reviewSendWait = 20
	reviewField    = 500 // the most runes of one comment
	reviewGutterW  = 10
)

// reviewPane is the state of `matchblox frame --kind=review`.
type reviewPane struct {
	spec      reviewSpec
	ready     bool   // the diff was read
	why       string // the sentence that says there is nothing to review
	diff      *reviewDiff
	at        int  // the file shown, or chosen in the file list
	cur       int  // the line the cursor is on
	filesView bool // a large diff: the file list shows
	comments  map[string]reviewComment
	field     *string // the comment being typed; nil when no field is open
	note      string
	done      bool // the list or the service has not answered yet
	ticks     int
	wait      int    // the ticks it waits for that answer
	sentID    string // the act id of the paste in flight; "" when none
	n         int    // the comments in that paste
}

// rkeys are the keys the review names in its words, each from its binding.
type rkeys struct{ esc, enter, add, move string }

func (m frameModel) rkeys() rkeys {
	return rkeys{
		esc:   m.keys.main.Back.Help().Key,
		enter: m.keys.side.Enter.Help().Key,
		add:   m.keys.board.Copy.Help().Key,
		move:  m.keys.main.Up.Help().Key,
	}
}

// leaves: esc, ctrl+q and ctrl+c all leave the review, as they leave the
// field.
func (m frameModel) leaves(k tea.KeyPressMsg) bool {
	return bkey.Matches(k, m.keys.main.Back, m.keys.side.Flip, m.keys.global.Quit)
}

// reviewHandFailed: tmux did not take the act.
type reviewHandFailed struct{}

// reviewFrameFromArgv builds the review pane from the flags after `frame`.
func reviewFrameFromArgv(m Model, args []string) (frameModel, error) {
	fs := flag.NewFlagSet("frame", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	kind := fs.String("kind", "", "")
	spec := reviewSpec{}
	fs.StringVar(&spec.name, "session", "", "")
	fs.StringVar(&spec.host, "host", "", "")
	fs.StringVar(&spec.pane, "pane", "", "")
	fs.StringVar(&spec.dir, "dir", "", "")
	fs.StringVar(&spec.base, "base", "", "")
	if err := fs.Parse(args); err != nil {
		return frameModel{}, err
	}
	if *kind != "review" || spec.name == "" || spec.pane == "" {
		return frameModel{}, fmt.Errorf("frame --kind=review needs --session and --pane")
	}
	f := newFrame("review", m)
	f.rev = &reviewPane{spec: spec, comments: map[string]reviewComment{}}
	return f, nil
}

// reviewLoad reads the diff off the update loop.
func (m frameModel) reviewLoad() tea.Cmd {
	spec := m.rev.spec
	return func() tea.Msg { return loadReview(spec) }
}

func (m frameModel) reviewMode(mode string) tea.Cmd {
	run, pane := m.tmuxRun(), m.opt.Pane
	if run == nil {
		return nil
	}
	return func() tea.Msg {
		writeMode(run, pane, mode)
		return nil
	}
}

// reviewTake is what the review pane does with its own messages.
func (m frameModel) reviewTake(msg tea.Msg) (tea.Cmd, bool) {
	r := m.rev
	switch msg := msg.(type) {
	case reviewLoaded:
		r.ready, r.why, r.diff = true, msg.why, msg.diff
		if msg.diff != nil {
			r.at, r.filesView = 0, msg.diff.big
			r.cur = r.firstLine()
		}
	case reviewFileLoaded:
		if r.diff == nil || msg.at >= len(r.diff.files) {
			return nil, true
		}
		if msg.why != "" {
			r.note = msg.why
			return nil, true
		}
		r.diff.files[msg.at] = msg.file
		if msg.at == r.at {
			r.cur = r.firstLine()
		}
	case fromConn:
		res, ok := msg.msg.(resultMsg)
		if !ok || msg.conn != m.conn || r.sentID == "" || proto.Result(res).ActID != r.sentID {
			return nil, false // the console's own messages: Model takes them
		}
		return m.reviewSentResult(proto.Result(res)), true
	case reviewHandFailed:
		r.done = false
		r.note = "the list was not told: tmux refused. " + m.rkeys().esc + " tries again"
	default:
		return nil, false
	}
	return nil, true
}

// reviewSentResult is the service's answer to the paste. A refusal stays
// in the pane, with its comments, and says why. Else the pane tells the list,
// which gives the keys and restores the agent view.
func (m frameModel) reviewSentResult(res proto.Result) tea.Cmd {
	r := m.rev
	r.sentID = ""
	if why := cmp.Or(res.Err, lastOf(res.Skipped)); why != "" {
		r.done = false
		r.note = "not sent: " + reviewClip(reviewVisible(why), 200)
		return nil
	}
	_, cmd := m.reviewHand(reviewAct{Act: "sent", Host: r.spec.host, Pane: r.spec.pane, N: r.n})
	return cmd
}

func lastOf(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[len(s)-1]
}

// reviewTick is one quarter-second of the pane: it counts the wait for the
// list's restore or the service's answer.
func (m frameModel) reviewTick() tea.Cmd {
	r := m.rev
	if r == nil || !r.done {
		return nil
	}
	if r.ticks++; r.ticks > r.wait {
		r.done, r.sentID = false, ""
		if r.n > 0 && r.wait == reviewSendWait {
			r.note = "the service did not answer: the review may not be sent. " + m.rkeys().esc + " leaves, " + m.rkeys().enter + " sends again"
			return nil
		}
		r.note = "the list did not answer. " + m.rkeys().esc + " tries again"
	}
	return nil
}

func (r *reviewPane) file() *reviewFile {
	if r.diff == nil || r.at < 0 || r.at >= len(r.diff.files) {
		return nil
	}
	return &r.diff.files[r.at]
}

// firstLine is the first line of source of the file shown: the cursor
// starts there.
func (r *reviewPane) firstLine() int {
	if f := r.file(); f != nil {
		if i := slices.IndexFunc(f.lines, reviewLine.commentable); i >= 0 {
			return i
		}
	}
	return 0
}

// commentKey names a line of a file: its number after the change, or
// "-" and its number before for a line the diff takes away.
func commentKey(path string, l reviewLine) string {
	if l.kind == '-' {
		return path + "\x00-" + strconv.Itoa(l.old)
	}
	return path + "\x00+" + strconv.Itoa(l.new)
}

// sorted are the comments in the order of the files, then of the lines.
func (r *reviewPane) sorted() []reviewComment {
	var out []reviewComment
	for _, f := range r.diff.files {
		var in []reviewComment
		for _, c := range r.comments {
			if c.path == f.path {
				in = append(in, c)
			}
		}
		slices.SortFunc(in, func(a, b reviewComment) int {
			if a.line != b.line {
				return a.line - b.line
			}
			if a.removed != b.removed {
				if a.removed {
					return -1
				}
				return 1
			}
			return 0
		})
		out = append(out, in...)
	}
	return out
}

func (m frameModel) reviewKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	r := m.rev
	if r == nil || r.done {
		return m, nil
	}
	if r.field != nil {
		return m.fieldKey(k)
	}
	r.note = ""
	if m.leaves(k) {
		return m.reviewHand(reviewAct{Act: "leave"})
	}
	f := r.file()
	if !r.ready || f == nil {
		return m, nil
	}
	mk, bk := m.keys.main, m.keys.board
	page := max(m.height-5, 1)
	switch {
	case bkey.Matches(k, mk.Up):
		return m, r.move(-1)
	case bkey.Matches(k, mk.Down):
		return m, r.move(1)
	case bkey.Matches(k, m.keys.desk.PageUp):
		return m, r.move(-page)
	case bkey.Matches(k, m.keys.desk.PageDown):
		return m, r.move(page)
	case bkey.Matches(k, bk.SpaceNext):
		return m, r.step(1, r.spec)
	case bkey.Matches(k, bk.SpacePrev):
		return m, r.step(-1, r.spec)
	case bkey.Matches(k, bk.Copy):
		return m.openField()
	case bkey.Matches(k, m.keys.side.Enter):
		if r.filesView {
			r.filesView = false
			return m, r.open(r.spec)
		}
		return m.reviewSend()
	}
	return m, nil
}

// move moves the cursor, or the choice in the file list.
func (r *reviewPane) move(d int) tea.Cmd {
	if r.filesView {
		r.at = min(max(r.at+d, 0), len(r.diff.files)-1)
		return nil
	}
	if f := r.file(); f != nil {
		r.cur = min(max(r.cur+d, 0), max(len(f.lines)-1, 0))
	}
	return nil
}

// step goes to the next or the previous file. A file of a large diff reads
// when it opens.
func (r *reviewPane) step(d int, spec reviewSpec) tea.Cmd {
	if r.filesView {
		return r.move(d)
	}
	at := r.at + d
	if at < 0 || at >= len(r.diff.files) {
		return nil
	}
	r.at = at
	return r.open(spec)
}

// open shows the chosen file, reading it first where the diff is large.
func (r *reviewPane) open(spec reviewSpec) tea.Cmd {
	f := r.file()
	r.cur = 0
	if f == nil {
		return nil
	}
	if f.loaded {
		r.cur = r.firstLine()
		return nil
	}
	at, mb, file := r.at, r.diff.mergeBase, *f
	return func() tea.Msg { return loadReviewFile(spec, mb, at, file) }
}

// openField opens the one-line field on the line of the cursor.
func (m frameModel) openField() (tea.Model, tea.Cmd) {
	r, k := m.rev, m.rkeys()
	f := r.file()
	if r.filesView || f == nil || r.cur >= len(f.lines) || !f.lines[r.cur].commentable() {
		r.note = k.add + " comments a line of code: " + k.move + " choose one"
		return m, nil
	}
	empty := ""
	r.field = &empty
	return m, m.reviewMode("COMMENT")
}

func (m frameModel) fieldKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	r := m.rev
	f := r.file()
	switch {
	case m.leaves(k):
		r.field = nil
		return m, m.reviewMode("REVIEW")
	case bkey.Matches(k, m.keys.side.Erase):
		if rs := []rune(*r.field); len(rs) > 0 {
			*r.field = string(rs[:len(rs)-1])
		}
		return m, nil
	case bkey.Matches(k, m.keys.side.Enter):
		text := strings.TrimSpace(*r.field)
		r.field = nil
		if f != nil && r.cur < len(f.lines) {
			l := f.lines[r.cur]
			key := commentKey(f.path, l)
			if text == "" {
				delete(r.comments, key) // an empty comment takes the line's comment away
			} else {
				n := l.new
				if l.kind == '-' {
					n = l.old
				}
				r.comments[key] = reviewComment{path: f.path, line: n, removed: l.kind == '-', src: l.text, text: text}
			}
		}
		return m, m.reviewMode("REVIEW")
	}
	if k.Text != "" && len([]rune(*r.field)) < reviewField {
		*r.field += reviewVisible(k.Text)
	}
	return m, nil
}

// reviewSend sends the one message itself: the paste: act on the pane's
// conn to the service, to the pane the review opened on. The text goes to
// the service and nowhere else: no tmux option, no log.
func (m frameModel) reviewSend() (tea.Model, tea.Cmd) {
	r, k := m.rev, m.rkeys()
	switch {
	case r.diff == nil || len(r.comments) == 0:
		r.note = "no comments yet · " + k.add + " comments a line"
		return m, nil
	case r.spec.host != "":
		r.note = "not sent: " + hostName(r.spec.host) + " is another machine: this pane reaches only this machine's service. " + k.esc + " leaves"
		return m, nil
	case m.conn == nil:
		r.note = "not sent: no service · " + k.esc + " leaves, " + k.enter + " tries again"
		return m, nil
	}
	cs := r.sorted()
	text := reviewText(r.spec.name, r.diff.against, cs)
	if len(text) > reviewSendMax {
		r.note = "the review is " + strconv.Itoa(len(text)) + " bytes: the most is " + strconv.Itoa(reviewSendMax) + " · " + k.add + " then " + k.enter + " with nothing typed takes a line's comment away"
		return m, nil
	}
	id, cmd := m.sendOn(m.conn, proto.Act{RecID: proto.PastePrefix + r.spec.pane, Text: text, Confirm: "y"})
	r.sentID, r.n, r.done, r.ticks, r.wait = id, len(cs), true, 0, reviewSendWait
	return m, cmd
}

// reviewHand writes the word to the window option the list looks at:
// leave, or sent and a count. Never review text. The list clears it as soon
// as it reads it.
func (m frameModel) reviewHand(a reviewAct) (tea.Model, tea.Cmd) {
	r, k := m.rev, m.rkeys()
	run, pane := m.tmuxRun(), m.opt.Pane
	raw, err := json.Marshal(a)
	if run == nil || err != nil {
		r.note = "the list was not told: no tmux here. " + k.esc + " tries again"
		return m, nil
	}
	r.done, r.ticks, r.wait = true, 0, reviewWait
	return m, func() tea.Msg {
		if _, err := run("set-option", "-w", "-t", pane, sample.ReviewOption, string(raw)); err != nil {
			return reviewHandFailed{}
		}
		return nil
	}
}

// brow is a row of the file view: a line of the diff, the comment under
// it, or the open field.
type brow struct {
	line int
	kind byte // 'l', 'c' or 'f'
}

func (m frameModel) reviewView() tea.View {
	w, h := max(m.width, 20), max(m.height, 6)
	rows := m.reviewRows(w, h)
	for i := range rows {
		rows[i] = fit(rows[i], w)
	}
	v := tea.NewView(strings.Join(rows, "\n"))
	v.AltScreen = true
	return v
}

func (m frameModel) reviewHead(w int, left string) string {
	return spread(" "+left, m.st.muted.Render(m.rkeys().esc+" chat")+" ", w)
}

func (m frameModel) reviewRows(w, h int) []string {
	r := m.rev
	name := reviewVisible(r.spec.name)
	var rows []string
	switch {
	case !r.ready:
		rows = []string{m.reviewHead(w, m.st.text.Render(name)+m.st.muted.Render(" · review")), "", " " + m.st.muted.Render("reading the diff…")}
	case r.diff == nil:
		rows = []string{
			m.reviewHead(w, m.st.text.Render(name)+m.st.muted.Render(" · review")), "",
			" " + m.st.text.Render(reviewVisible(r.why)),
			" " + m.st.muted.Render(m.rkeys().esc+" goes back to the chat"),
		}
	default:
		return m.reviewDiffRows(w, h, name)
	}
	return rows
}

func (m frameModel) reviewDiffRows(w, h int, name string) []string {
	r, d := m.rev, m.rev.diff
	add, del := d.totals()
	left := m.st.text.Render(name) + m.st.muted.Render(" · diff against ") + m.st.text.Render(reviewVisible(d.against)) +
		m.st.muted.Render(" · "+plural(len(d.files), "file")+" ") + m.st.accent.Render("+"+strconv.Itoa(add)) + " " + m.st.neg.Render("−"+strconv.Itoa(del))
	rows := []string{m.reviewHead(w, left)}
	f := r.file()
	for _, n := range m.reviewNotes() {
		for _, line := range wrapPlain(n, max(w-1, 4)) {
			rows = append(rows, " "+m.st.warn.Render(line))
		}
	}
	if r.filesView {
		rows = append(rows, " "+m.st.muted.Render("these files open one at a time: they are large"))
	} else if f != nil {
		pos := m.st.muted.Render(fmt.Sprintf("  %d/%d", r.at+1, len(d.files)))
		fresh := ""
		if f.untrack {
			fresh = m.st.muted.Render(" new")
		}
		rows = append(rows, " "+m.st.text.Bold(true).Render(reviewVisible(f.path))+fresh+"  "+m.st.accent.Render("+"+strconv.Itoa(f.add))+" "+m.st.neg.Render("−"+strconv.Itoa(f.del))+pos)
	}
	room := max(h-len(rows)-1, 1)
	var body []string
	switch {
	case r.filesView:
		body = m.reviewFileList(w, room)
	case f != nil && !f.loaded:
		body = []string{" " + m.st.muted.Render("reading this file…")}
	case f != nil:
		body = m.reviewLines(f, w, room)
	}
	rows = append(rows, body...)
	for len(rows) < h-1 {
		rows = append(rows, "")
	}
	return append(rows[:h-1], m.reviewFoot(w))
}

// reviewNotes are the lines under the head: the base that is gone, and a
// file that is cut.
func (m frameModel) reviewNotes() []string {
	r := m.rev
	var out []string
	if r.diff.note != "" {
		out = append(out, reviewVisible(r.diff.note))
	}
	if f := r.file(); f != nil && f.cut && !r.filesView && f.loaded && len(f.lines) > 1 {
		out = append(out, "showing the first "+strconv.Itoa(reviewMaxLines)+" lines of "+reviewClip(reviewVisible(f.path), 60))
	}
	return out
}

func (m frameModel) reviewFileList(w, room int) []string {
	r := m.rev
	top := min(max(r.at-room/2, 0), max(len(r.diff.files)-room, 0))
	var out []string
	for i := top; i < len(r.diff.files) && len(out) < room; i++ {
		f := r.diff.files[i]
		mark, style := " ", m.st.text
		if i == r.at {
			mark, style = "▸", m.st.selected
		}
		counts := "+" + strconv.Itoa(f.add) + " −" + strconv.Itoa(f.del)
		if f.binary {
			counts = "binary"
		}
		if f.untrack {
			counts = "new " + counts
		}
		path := ansi.Truncate(reviewVisible(f.path), max(w-len(counts)-6, 4), "…")
		line := " " + mark + " " + path
		line = line + strings.Repeat(" ", max(w-lipgloss.Width(line)-len(counts)-2, 1)) + counts + " "
		out = append(out, style.Render(fit(line, w)))
	}
	return out
}

// reviewLines draws the lines of f around the cursor, with the comments
// under their lines and the field under the cursor.
func (m frameModel) reviewLines(f *reviewFile, w, room int) []string {
	r := m.rev
	var rows []brow
	curRow := 0
	for i, l := range f.lines {
		rows = append(rows, brow{i, 'l'})
		if i == r.cur {
			curRow = len(rows) - 1
		}
		if _, ok := r.comments[commentKey(f.path, l)]; ok && l.commentable() {
			rows = append(rows, brow{i, 'c'})
		}
		if i == r.cur && r.field != nil {
			rows = append(rows, brow{i, 'f'})
			curRow = len(rows) - 1
		}
	}
	top := min(max(curRow-room/2, 0), max(len(rows)-room, 0))
	var out []string
	for _, b := range rows[top:min(top+room, len(rows))] {
		l := f.lines[b.line]
		switch b.kind {
		case 'c':
			c := r.comments[commentKey(f.path, l)]
			out = append(out, m.st.accent.Render(fit(strings.Repeat(" ", reviewGutterW-4)+"✎ "+reviewClip(reviewVisible(c.text), max(w-reviewGutterW, 4)), w)))
		case 'f':
			k := m.rkeys()
			text := reviewVisible(*r.field)
			if text != "" {
				text += "  "
			}
			out = append(out, m.st.text.Render(strings.Repeat(" ", reviewGutterW-4)+"›  "+text)+m.st.muted.Render(k.enter+" add · "+k.esc+" cancel"))
		default:
			out = append(out, m.reviewLine(l, b.line == r.cur, r.comments[commentKey(f.path, l)].text != "", w))
		}
	}
	return out
}

func (m frameModel) reviewLine(l reviewLine, cursor, commented bool, w int) string {
	if l.kind == '@' || l.kind == 'm' {
		style := m.st.faint
		if l.kind == 'm' {
			style = m.st.muted
		}
		text := strings.Repeat(" ", reviewGutterW-2) + l.text
		if cursor {
			text = " ▸" + text[2:]
		}
		return style.Render(fit(text, w))
	}
	mark, note, num := " ", " ", l.new
	if cursor {
		mark = "▸"
	}
	if commented {
		note = "✎"
	}
	if l.kind == '-' {
		num = l.old
	}
	plain := fmt.Sprintf("%s%s %4d %c ", mark, note, num, l.kind) + ansi.Truncate(l.text, max(w-reviewGutterW, 4), "…")
	switch {
	case cursor:
		return m.st.selected.Render(fit(plain, w))
	case l.kind == '+':
		return m.st.accent.Render(plain)
	case l.kind == '-':
		return m.st.neg.Render(plain)
	}
	return m.st.text.Render(plain)
}

func (m frameModel) reviewFoot(w int) string {
	r, k := m.rev, m.rkeys()
	next, prev := m.keys.board.SpaceNext.Help().Key, m.keys.board.SpacePrev.Help().Key
	switch {
	case r.note != "":
		return " " + m.st.warn.Render(r.note)
	case r.done:
		return " " + m.st.muted.Render("handing the review to the list…")
	case r.field != nil:
		return " " + m.st.muted.Render("a comment for this line · "+k.enter+" with nothing typed takes the line's comment away")
	case r.filesView:
		return " " + m.st.muted.Render(k.enter+" opens a file · "+k.move+" choose")
	case len(r.comments) == 0:
		return " " + m.st.muted.Render(k.add+" comments a line · "+next+" "+prev+" files · "+k.move+" lines")
	}
	return " " + m.st.muted.Render(plural(len(r.comments), "comment")+" · "+k.enter+" sends them to "+reviewClip(reviewVisible(r.spec.name), 40)+" · "+k.add+" comment · "+next+" "+prev+" files")
}
