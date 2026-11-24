package app

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// The D review sheet: the session's diff floats over the agent; comments
// collect in reviews.json until one send pastes them to the agent.

// reviewSheet is the open review: the session it opened on, the diff, and
// the line the owner is on.
type reviewSheet struct {
	pane, name, host, key, act string
	against                    string
	note, err                  string
	files                      []proto.DiffFile
	file                       int  // index in files
	at                         int  // selected new (or old) line number
	typing                     bool // a comment is being typed
	draft                      string
	loading                    bool
}

// openReview opens the review sheet on s and asks the service for its diff.
func (m Model) openReview(s sample.Session) (Model, tea.Cmd) {
	key := reviewKey(s)
	m.review = &reviewSheet{
		pane: s.Pane, name: m.cardName(s), host: s.Host, key: key,
		loading: true,
	}
	m.over = "review"
	m.agent.keys = false
	id, cmd := m.sendToID(s.Host, proto.Act{RecID: proto.DiffPrefix + s.Pane, Console: true})
	m.review.act = id
	return m, cmd
}

// closeReview closes the sheet; unsent comments stay in reviews.json.
func (m Model) closeReview() Model {
	m.over = ""
	m.review = nil
	return m
}

// reviewResult takes a diff: answer for the open sheet.
func (m Model) reviewResult(r proto.Result) (Model, bool) {
	if m.review == nil || r.ActID == "" || r.ActID != m.review.act {
		return m, false
	}
	m.review.loading, m.review.act = false, ""
	if r.Err != "" {
		m.review.err = r.Err
		return m, true
	}
	if r.Diff == nil {
		m.review.err = m.review.name + " has no changes."
		return m, true
	}
	m.review.against = r.Diff.Against
	m.review.note = r.Diff.Note
	m.review.files = r.Diff.Files
	m.review.file = 0
	m.review.at = firstReviewLine(r.Diff.Files)
	return m, true
}

func firstReviewLine(files []proto.DiffFile) int {
	for _, f := range files {
		for _, h := range f.Hunks {
			for _, l := range h.Lines {
				return reviewLineNo(l)
			}
		}
	}
	return 0
}

func reviewLineNo(l proto.DiffLine) int {
	if l.New != 0 {
		return l.New
	}
	return l.Old
}

// reviewKey is a key while the review sheet is open.
func (m Model) reviewKey(k tea.KeyPressMsg) (Model, tea.Cmd) {
	r := m.review
	if r == nil {
		return m, nil
	}
	if r.typing {
		return m.reviewType(k)
	}
	switch {
	case bkey.Matches(k, m.keys.side.Leave, m.keys.main.Back):
		return m.closeReview(), nil
	case bkey.Matches(k, m.keys.main.Down):
		m.reviewMove(1)
	case bkey.Matches(k, m.keys.main.Up):
		m.reviewMove(-1)
	case bkey.Matches(k, m.keys.board.SpaceAfter) && len(r.files) > 0:
		r.file = (r.file + 1) % len(r.files)
		r.at = firstFileLine(r.files[r.file])
	case bkey.Matches(k, m.keys.board.SpaceBefore) && len(r.files) > 0:
		r.file = (r.file - 1 + len(r.files)) % len(r.files)
		r.at = firstFileLine(r.files[r.file])
	case bkey.Matches(k, m.keys.board.Copy): // c: comment on the line (the review has the keys)
		if len(r.files) == 0 || r.at == 0 && firstFileLine(r.files[r.file]) == 0 {
			return m, nil
		}
		file := r.files[r.file].Path
		r.typing = true
		r.draft = m.reviewDraftOf(r.key, file, r.at)
	case bkey.Matches(k, m.keys.main.Open):
		return m.sendReview()
	}
	return m, nil
}

// heldReview is a review that waits for a busy agent that takes no typed
// input while it works: the paste text, and enough to clear ✎ when it goes.
type heldReview struct {
	text, name, key string
	n               int
	comments        []reviewComment // restore if the later paste fails
}

// reviewPaste is a paste: of a review waiting for the service's answer.
// Comments stay in reviews.json until the paste succeeds.
type reviewPaste struct {
	key, name string
	n         int
	comments  []reviewComment
}

// sendReview pastes every unsent comment to the sheet's session as one
// message. The pane is the one the sheet opened on, not the list's choice.
func (m Model) sendReview() (Model, tea.Cmd) {
	r := m.review
	if r == nil {
		return m, nil
	}
	cs := m.reviews.Comments[r.key]
	if len(cs) == 0 {
		return m, nil
	}
	text := reviewPasteText(m.reviewMessage(cs))
	if len(text) > proto.MaxPaste {
		kb := (len(text) + 1023) / 1024
		return m, m.notice(fmt.Sprintf(
			"not sent: the review is too long (%d KB, %d KB at most). Send fewer comments at once.",
			kb, proto.MaxPaste/1024))
	}
	s, have := m.sessionAt(r.host, r.pane)
	name, key, n, pane, host := r.name, r.key, len(cs), r.pane, r.host
	if have {
		name = m.cardName(s)
	}
	m = m.closeReview()
	if have && m.lineWaits(s) {
		m.reviewHeld = maps.Clone(m.reviewHeld)
		if m.reviewHeld == nil {
			m.reviewHeld = map[paneKey]heldReview{}
		}
		m.reviewHeld[paneOf(s)] = heldReview{text: text, name: name, key: key, n: n, comments: slices.Clone(cs)}
		return m, m.notice("queued for " + name + " · sends at the end of its turn")
	}
	if why := m.cannotReach(host); why != "" {
		return m, m.notice(why)
	}
	cmd, went := m.trackReviewPaste(host, pane, text, reviewPaste{key: key, name: name, n: n, comments: slices.Clone(cs)})
	if !went {
		return m, m.notice("not sent: " + name + " is not reached")
	}
	if have && m.sideStateOf(s) == sideWork {
		return m, tea.Batch(cmd, m.notice("queued for "+name+" · sends at the end of its turn"))
	}
	return m, cmd
}

// trackReviewPaste sends one paste: and remembers the comments until the
// service answers. ok is false when no act went (no conn).
func (m *Model) trackReviewPaste(host, pane, text string, sent reviewPaste) (tea.Cmd, bool) {
	id, cmd := m.sendToID(host, proto.Act{
		RecID: proto.PastePrefix + pane, Text: text, Confirm: "y", Console: true,
	})
	if id == "" {
		return nil, false
	}
	m.reviewPastes = maps.Clone(m.reviewPastes)
	if m.reviewPastes == nil {
		m.reviewPastes = map[string]reviewPaste{}
	}
	m.reviewPastes[id] = sent
	return cmd, true
}

// reviewPasteResult is the service's answer to a review paste: success
// clears the comments and says they went; a refusal puts them back.
func (m Model) reviewPasteResult(r proto.Result) (Model, tea.Cmd, bool) {
	sent, ok := m.reviewPastes[r.ActID]
	if !ok {
		return m, nil, false
	}
	m.reviewPastes = maps.Clone(m.reviewPastes)
	delete(m.reviewPastes, r.ActID)
	why := r.Err
	if why == "" && len(r.Skipped) > 0 {
		why = r.Skipped[len(r.Skipped)-1]
	}
	if why != "" {
		m.setReviewComments(sent.key, sent.comments)
		return m, m.notice("not sent: "+why), true
	}
	m.setReviewComments(sent.key, nil)
	return m, m.notice(fmt.Sprintf("sent a review of %d comments to %s · now in WORKING", sent.n, sent.name)), true
}

// releaseHeldReviews sends each held review whose session no longer waits,
// as heldRelease does for a typed line. A session that is gone drops the
// hold and keeps the comments unsent.
func (m Model) releaseHeldReviews() (Model, tea.Cmd) {
	if len(m.reviewHeld) == 0 {
		return m, nil
	}
	var cmds []tea.Cmd
	for k, h := range m.reviewHeld {
		i := slices.IndexFunc(m.all.sessions, func(s sample.Session) bool { return paneOf(s) == k })
		if i >= 0 && m.lineWaits(m.all.sessions[i]) {
			continue
		}
		m.reviewHeld = maps.Clone(m.reviewHeld)
		delete(m.reviewHeld, k)
		if i < 0 {
			continue // comments stay; the agent is gone
		}
		s := m.all.sessions[i]
		if why := m.cannotReach(s.Host); why != "" {
			cmds = append(cmds, m.notice(why))
			continue
		}
		cmd, ok := m.trackReviewPaste(s.Host, s.Pane, h.text, reviewPaste{
			key: h.key, name: h.name, n: h.n, comments: slices.Clone(h.comments),
		})
		if !ok {
			cmds = append(cmds, m.notice("not sent: "+h.name+" is not reached"))
			continue
		}
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

// reviewMessage is the paste body: each comment with its file:line, the
// diff line it sits on, and the note.
func (m Model) reviewMessage(cs []reviewComment) string {
	var b strings.Builder
	for i, c := range cs {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s:%d\n", c.File, c.Line)
		if line := m.reviewLineText(c.File, c.Line); line != "" {
			b.WriteString(line)
			b.WriteByte('\n')
		}
		b.WriteString(c.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

func (m Model) reviewLineText(file string, line int) string {
	r := m.review
	if r == nil {
		return ""
	}
	for _, f := range r.files {
		if f.Path != file {
			continue
		}
		for _, h := range f.Hunks {
			for _, l := range h.Lines {
				if reviewLineNo(l) == line {
					return l.Text
				}
			}
		}
	}
	return ""
}

// reviewPasteText is pasteText's rule: drop every control but \n and \t so
// a hostile comment or diff line never reaches the agent's tools.
func reviewPasteText(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, strings.ToValidUTF8(text, ""))
}

func firstFileLine(f proto.DiffFile) int {
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			return reviewLineNo(l)
		}
	}
	return 0
}

func (m *Model) reviewMove(by int) {
	r := m.review
	if r == nil || len(r.files) == 0 {
		return
	}
	lines := reviewLineNos(r.files[r.file])
	if len(lines) == 0 {
		return
	}
	i := 0
	for j, n := range lines {
		if n == r.at {
			i = j
			break
		}
	}
	i = min(max(i+by, 0), len(lines)-1)
	r.at = lines[i]
}

func reviewLineNos(f proto.DiffFile) []int {
	var out []int
	seen := map[int]bool{}
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			n := reviewLineNo(l)
			if n == 0 || seen[n] {
				continue
			}
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

func (m Model) reviewType(k tea.KeyPressMsg) (Model, tea.Cmd) {
	r := m.review
	switch {
	case bkey.Matches(k, m.keys.side.Leave, m.keys.main.Back):
		file := ""
		if len(r.files) > 0 {
			file = r.files[r.file].Path
		}
		m.setReviewDraft(r.key, reviewDraft{File: file, Line: r.at, Text: r.draft})
		r.typing = false
	case bkey.Matches(k, m.keys.main.Open):
		text := strings.TrimSpace(r.draft)
		r.typing, r.draft = false, ""
		if text == "" || len(r.files) == 0 {
			return m, nil
		}
		file := r.files[r.file].Path
		cs := append([]reviewComment{}, m.reviews.Comments[r.key]...)
		cs = append(cs, reviewComment{File: file, Line: r.at, Text: text})
		m.setReviewComments(r.key, cs)
		m.setReviewDraft(r.key, reviewDraft{}) // clear the draft for this key
	case bkey.Matches(k, m.keys.search.Erase):
		if rs := []rune(r.draft); len(rs) > 0 {
			r.draft = string(rs[:len(rs)-1])
		}
	case printsText(k) && !strings.ContainsFunc(k.Text, unicode.IsControl):
		r.draft += k.Text
	}
	return m, nil
}

// reviewLines is the sheet's body for the open review.
func (m Model) reviewLines() []string {
	r := m.review
	if r == nil {
		return nil
	}
	st := m.st
	if r.loading {
		return []string{st.faint.Render("reading the diff of "+r.name+"…") + " " + m.pulse()}
	}
	if r.err != "" {
		return []string{st.warn.Render(r.err)}
	}
	added, removed := 0, 0
	for _, f := range r.files {
		added += f.Added
		removed += f.Removed
	}
	esc := m.keys.side.Leave.Help().Key
	head := st.text.Bold(true).Render(r.name) +
		st.muted.Render(" · diff against "+r.against+" · "+strconv.Itoa(len(r.files))+" files ") +
		st.muted.Render("+"+strconv.Itoa(added)) + " " +
		st.neg.Render("−"+strconv.Itoa(removed)) +
		st.muted.Render("   ") + st.text.Render(esc) + st.muted.Render(" chat")
	lines := []string{head, m.reviewFilesLine(), ""}
	if r.note != "" {
		lines = append(lines, st.warn.Render(r.note), "")
	}
	if len(r.files) == 0 {
		return lines
	}
	f := r.files[r.file]
	if len(f.Hunks) == 0 {
		lines = append(lines, st.faint.Render("no hunks in this answer · open one file alone when the diff is big"))
		return lines
	}
	for _, h := range f.Hunks {
		lines = append(lines, st.muted.Render(fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldCount, h.NewStart, h.NewCount)))
		for _, l := range h.Lines {
			lines = append(lines, m.reviewDiffLine(f.Path, l)...)
		}
	}
	return lines
}

func (m Model) reviewFilesLine() string {
	r, st := m.review, m.st
	if r == nil || len(r.files) == 0 {
		return ""
	}
	var parts []string
	for i, f := range r.files {
		counts := " +" + strconv.Itoa(f.Added)
		if f.Removed > 0 {
			counts += " −" + strconv.Itoa(f.Removed)
		}
		name := f.Path
		if i == r.file {
			parts = append(parts, st.accent.Render("▸ ")+st.accent.Render(name)+st.faint.Render(counts))
		} else {
			parts = append(parts, st.muted.Render(name)+st.faint.Render(counts))
		}
	}
	return strings.Join(parts, "   ")
}

func (m Model) reviewDiffLine(file string, l proto.DiffLine) []string {
	r, st := m.review, m.st
	no := reviewLineNo(l)
	on := no == r.at
	mark := " "
	numInk := st.faint
	if on {
		mark, numInk = st.accent.Render("▌"), st.accent
	}
	num := numInk.Render(fmt.Sprintf("%4d ", no))
	var glyph string
	var markInk, bodyInk lipgloss.Style
	switch l.Kind {
	case "+":
		glyph, markInk, bodyInk = "+", st.accent, st.text
	case "-":
		glyph, markInk, bodyInk = "−", st.neg, st.faint
	default:
		glyph, markInk, bodyInk = " ", st.faint, st.text
	}
	line := mark + num + markInk.Render(glyph) + " " + bodyInk.Render(showDiffText(l.Text))
	out := []string{line}
	for _, c := range m.reviews.Comments[r.key] {
		if c.File == file && c.Line == no {
			out = append(out, "        "+st.accent.Render("✎ ")+st.muted.Render(c.Text))
		}
	}
	if r.typing && on {
		out = append(out, "        "+st.accent.Render("✎ ")+st.text.Render(r.draft)+"▏")
	}
	return out
}

// reviewFoot is the sheet's last line.
func (m Model) reviewFoot(w int) string {
	r, st := m.review, m.st
	if r == nil {
		return ""
	}
	sep := st.faint.Render(" · ")
	enter, esc := m.keys.main.Open.Help().Key, m.keys.side.Leave.Help().Key
	if r.typing {
		line := st.accent.Render("comment on "+r.files[r.file].Path+":"+strconv.Itoa(r.at)) +
			"   " + st.text.Render(enter) + st.muted.Render(" keeps it") +
			sep + st.text.Render(esc) + st.muted.Render(" keeps the draft")
		return fit(line, w)
	}
	n := len(m.reviews.Comments[r.key])
	word := strconv.Itoa(n) + " comment"
	if n != 1 {
		word += "s"
	}
	send := st.muted.Render(" send review")
	if n > 0 {
		send = st.accent.Render(" send review to " + r.name)
	}
	// ] then [: next file, then last (the board's space keys, in that order)
	fileKeys := m.keys.board.SpaceAfter.Keys()[0] + " " + m.keys.board.SpaceBefore.Keys()[0]
	line := st.muted.Render(word) + sep +
		st.text.Render(m.keys.board.Copy.Help().Key) + st.muted.Render(" comment") + sep +
		st.text.Render(fileKeys) + st.muted.Render(" file") + sep +
		st.text.Render(enter) + send
	return fit(line, w)
}

// showDiffText maps C0 and C1 controls to their visible Control Pictures so
// a hostile escape in a file draws as text and never reaches the terminal.
func showDiffText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 0x20:
			b.WriteRune(0x2400 + r)
		case r == 0x7f:
			b.WriteRune(0x2421) // ␡
		case r >= 0x80 && r <= 0x9f:
			b.WriteRune(0x2400 + (r & 0x1f))
		case unicode.IsPrint(r):
			b.WriteRune(r)
		default:
			b.WriteRune('\uFFFD')
		}
	}
	return b.String()
}

// reviewTitle is the float's top-edge title.
func (m Model) reviewTitle() string {
	if m.review == nil {
		return "review"
	}
	return "review · " + m.review.name
}

// hasUnsentReview: the session has comments waiting to be sent.
func (m Model) hasUnsentReview(s sample.Session) bool {
	return len(m.reviewsOf(s)) > 0
}
