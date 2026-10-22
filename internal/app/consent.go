package app

import (
	"strings"
)

// consenting is a question the builder answers with a key: a pending step,
// or the stoker's take-over. A door's long diff keeps its confirm on the
// key line: the diff fills the body, and a box would hide what it asks
// about.
func (m Model) consenting() bool {
	if m.held != nil {
		return true
	}
	if m.pending == nil {
		return false
	}
	_, preview := m.previewDoor()
	return !preview
}

// consentBox is the question in a box: what runs, in words and commands,
// and the key that runs it. A destructive step says so in its title.
func (m Model) consentBox(bw int) []string {
	st := m.st
	iw := bw - 4
	var lines []string
	text := func(s string) {
		for _, l := range wrapPlain(s, iw) {
			lines = append(lines, st.text.Render(l))
		}
	}
	if m.held != nil {
		text("The stoker is on. Take over?")
		lines = append(lines, "")
		if m.flash != "" {
			for _, l := range wrapPlain(m.flash, iw-2) {
				lines = append(lines, st.warn.Render("! ")+st.text.Render(l))
			}
			lines = append(lines, "")
		}
		lines = append(lines, st.accent.Render("y")+st.muted.Render(" rests it while you work"), st.accent.Render("n")+st.muted.Render(" keeps it on"))
		box, _ := boxFrame(st.hair, st.warn.Bold(true).Render("Take over"), lines, []footPart{{st.accent.Render("y"), "", ""}, {st.accent.Render("n"), "", " ─ "}}, bw)
		return box
	}
	return stepBox(st, *m.pending, bw)
}

// stepBox is a pending step in a box: what it does in its title, how in
// its lines, and the key that runs it in its foot.
func stepBox(st styles, a action, bw int) []string {
	var lines []string
	head, body := a.consentWords()
	for _, b := range body {
		for _, l := range wrapPlain(b, bw-4) {
			lines = append(lines, st.text.Render(l))
		}
	}
	title, foot := st.accent.Bold(true).Render(head), []footPart{{st.accent.Render("⏎") + st.muted.Render(" run"), "", ""}, {st.accent.Render("esc") + st.muted.Render(" cancels"), "", " ─ "}}
	if a.destructive {
		title, foot = st.neg.Bold(true).Render(head), []footPart{{st.neg.Render("y") + st.muted.Render(" run"), "", ""}, {st.muted.Render("any other key cancels"), "", " ─ "}}
	}
	box, _ := boxFrame(st.hair, title, lines, foot, bw)
	return box
}

// consentWords are the box's title and lines: what the step does, then how.
// A say of "what: how" splits there; else the title is the step's label
// and the lines are its commands, one each.
func (a action) consentWords() (string, []string) {
	if what, how, ok := strings.Cut(a.say, ": "); ok {
		if strings.HasPrefix(what, a.label+" ") {
			what = capital(what) // a verb: kill spark-docs; never a name
		}
		return what, []string{how}
	}
	if a.say != "" {
		return capital(a.label), []string{a.say}
	}
	var lines []string
	for _, s := range a.steps {
		lines = append(lines, strings.Join(s, " "))
	}
	return capital(a.label), lines
}

func capital(s string) string {
	if s == "" {
		return "Run"
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// overlayConsent lays the question over the dimmed screen: on a phone it
// rises from the key line, under the thumb and above the keyboard; wider,
// it sits in the middle. A tap anywhere cancels it: a tap is never consent.
func (m Model) overlayConsent(sc *drawn, w int) {
	phone := m.layoutAt(w) == Narrow || m.upright()
	bw := min(w, 72)
	if !phone {
		bw = min(m.consentWidth()+4, 72, w-8)
	}
	box := m.consentBox(bw)
	press := make([]string, len(box))
	m.overlayBox(sc, w, bw, phone, box, press, nil)
}

// consentWidth is the columns the question's words take on one line each,
// 40 at least: a desk's box is no wider than what it says.
func (m Model) consentWidth() int {
	if m.held != nil {
		return 44
	}
	return m.pending.width()
}

func (a action) width() int {
	head, body := a.consentWords()
	w := max(40, len([]rune(head))+16)
	for _, b := range body {
		w = max(w, len([]rune(b)))
	}
	return w
}
