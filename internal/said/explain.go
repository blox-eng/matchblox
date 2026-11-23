package said

import "strings"

// The rules Explain names, in the order Screen and Menu try them.
const (
	RuleWorking  = "working line"
	RulePermits  = "approval prompt"
	RuleQuestion = "question"
	RuleMenuHint = "menu key hint"
	RuleFallback = "fallback"
)

// Hit is the line a rule matched: Line is 1-based in Trace.Lines, 0 when
// the rule matched no line or was not tried.
type Hit struct {
	Rule string
	Line int
	Text string
}

// Row is a numbered row of a menu: the digit that picks it, and its words.
type Row struct {
	N     int
	Label string
}

// Trace is how the rules read a screen: the verdict Screen and Menu give,
// each rule's hit, the menu's rows when it permits, and the lines the
// rules read, so nothing above them shows.
type Trace struct {
	Kind     Kind
	Line     string
	Hits     []Hit
	MenuRead bool
	Options  []Row
	Lines    []string
}

// Explain reads a screen as Screen and Menu do, and says which rule decided.
func Explain(screen string) Trace {
	ls := lines(screen)
	hit := map[string]int{} // the index in ls each rule matched
	from := max(len(ls)-screenWithin, 0)
	for i := len(ls) - 1; i >= from; i-- {
		if workingRe.MatchString(ls[i].text) {
			hit[RuleWorking] = i
			from = i + 1
			break
		}
	}
	t := Trace{}
	start := from
	if i, ok := hit[RuleWorking]; ok {
		start = i
	}
	if i, ok := permitsAt(ls, from); ok {
		hit[RulePermits] = i
		t.Kind, t.Line = Permits, ls[i].text
		for j := len(ls) - 1; j >= from; j-- {
			if strings.HasSuffix(ls[j].text, "?") && !strings.HasPrefix(ls[j].text, "?") {
				t.Line = ls[j].text
				break
			}
		}
		if last := len(ls) - 1; cancelRe.MatchString(ls[last].text) {
			hit[RuleMenuHint] = last
			t.MenuRead = true
		}
	} else if i, ok := questionAt(ls[from:]); ok {
		hit[RuleQuestion] = from + i
		t.Kind, t.Line = Asks, ls[from+i].text
	} else {
		for i := len(ls) - 1; i >= 0; i-- {
			if !ls[i].boxed && !ls[i].person && !hintRe.MatchString(ls[i].text) {
				hit[RuleFallback] = i
				t.Line = ls[i].text
				start = min(start, i)
				break
			}
		}
	}
	below := -1 // the working line's row: the rows under it are live
	if i, ok := hit[RuleWorking]; ok {
		below = ls[i].row
	}
	place := map[int]int{} // a screen row's 1-based place in t.Lines
	if start < len(ls) {
		for row, raw := range strings.Split(Clean(screen), "\n") {
			text := strings.TrimFunc(raw, frame)
			if row < ls[start].row || text == "" {
				continue
			}
			t.Lines = append(t.Lines, text)
			place[row] = len(t.Lines)
			if t.Kind == Permits && row > below {
				if n, label, ok := option(text); ok {
					t.Options = append(t.Options, Row{N: n, Label: label})
				}
			}
		}
	}
	for _, rule := range []string{RuleWorking, RulePermits, RuleQuestion, RuleMenuHint, RuleFallback} {
		h := Hit{Rule: rule}
		if i, ok := hit[rule]; ok {
			h.Line, h.Text = place[ls[i].row], ls[i].text
		}
		t.Hits = append(t.Hits, h)
	}
	return t
}

func permitsAt(ls []line, from int) (int, bool) {
	for i := from; i < len(ls); i++ {
		if permitsRe.MatchString(ls[i].text) {
			return i, true
		}
	}
	return 0, false
}
