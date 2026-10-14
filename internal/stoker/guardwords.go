package stoker

import (
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Unread is the guard of a line that names gh and that the guard cannot
// read as the shell does: a quote left open, or `sh -c` nested deeper
// than maxDepth. No go gives it, so a guarded pane never runs such a line.
const Unread = "unread"

// maxDepth is how deep the guard reads a script inside a script: `sh -c`,
// $(…) and `…`.
const maxDepth = 4

// ghWord is gh as a word of a line, once quotes and backslashes are out.
var ghWord = regexp.MustCompile(`(^|[^A-Za-z0-9_.-])gh($|[^A-Za-z0-9_.-])`)

func namesGH(line string) bool {
	return ghWord.MatchString(strings.NewReplacer(`"`, "", `'`, "", `\`, "").Replace(line))
}

// wordGuards reads a line as the shell splits it into words: quotes and
// backslashes come out, $'…' is decoded, the pieces of a word join
// (g"h" is gh), and the script of `sh -c`, $(…) and `…` is read again.
// It does not expand variables, aliases or eval. ok is false when the
// line cannot be read to its end.
//
// On top of that reading it keeps the word rule (Ruling AM): a command
// the reading finds no guard in, whose words name gh and after it merge,
// release, workflow, api, graphql or mergePullRequest, needs that go. A
// command other than gh may run a line it is given (bash <<< "…", script
// -c, ssh, | bash), so each of its words that holds a line is read as
// one. gh's own words are not: the body of `gh pr create` is text.
//
// A here-document or a here-string is data when what reads it runs no
// code (cat, tee, gh pr create --body-file -): the line reading skips it,
// and data gets the byte spans of such bodies, so the raw readings skip
// them too. One fed to a shell or an interpreter, directly or down a
// pipe, is read as a script.
func wordGuards(line string, depth int, data *[][2]int) (gs []Guard, ok bool) {
	if depth > maxDepth {
		return nil, false
	}
	l := lexer{s: line, data: data}
	ok = l.scan(false)
	for _, sub := range append(l.subs, l.scripts...) {
		g, subOK := wordGuards(sub, depth+1, nil)
		gs, ok = append(gs, g...), ok && subOK
	}
	for ci, words := range l.cmds {
		rest, script := lead(words)
		if script {
			if len(rest) > 0 {
				g, subOK := wordGuards(rest[0], depth+1, nil)
				gs, ok = append(gs, g...), ok && subOK
			}
			continue
		}
		if g, found := guardOf(rest); found {
			gs = append(gs, g)
			continue
		}
		gs = append(gs, wordRule(words)...)
		if len(rest) > 0 && isGH(rest[0]) {
			continue
		}
		runs := l.runsInput(ci)
		for _, w := range rest {
			if !runs && slices.Contains(l.hereWords[ci], w) {
				continue // a here-string a command reads as text
			}
			if strings.ContainsAny(w, " \t\n;&|") {
				gs = append(gs, lineInWord(w, depth+1)...)
			}
		}
	}
	return gs, ok
}

// lineInWord reads a word that may be a line another command runs. A
// word that does not read as a line, or nests too deep, is prose: the word
// rule reads it with quotes as spaces.
func lineInWord(w string, depth int) []Guard {
	gs, ok := wordGuards(w, depth, nil)
	if ok {
		return gs
	}
	return union(gs, wordRule(strings.Fields(strings.NewReplacer(`"`, " ", `'`, " ").Replace(w))))
}

// ruleWords are the words that, after gh, make a command need a go, and
// the go each needs. A gh api call the guard reads as a read still needs
// the merge go: api is how a merge goes around gh pr merge.
var ruleWords = map[string]string{"merge": "merge", "api": "merge", "graphql": "merge", "release": "release", "workflow": "deploy"}

// wordRule is the go a command's words need: gh, by its base name, and a
// rule word after it.
func wordRule(words []string) []Guard {
	i := slices.IndexFunc(words, isGH)
	if i < 0 {
		return nil
	}
	var gs []Guard
	for _, w := range words[i+1:] {
		what, ok := ruleWords[w]
		if !ok && strings.Contains(w, "mergePullRequest") {
			what, ok = "merge", true
		}
		if g := (Guard{What: what}); ok && !slices.Contains(gs, g) {
			gs = append(gs, g)
		}
	}
	return gs
}

// lexer splits a shell line into commands of words. subs are the scripts
// of $(…) and `…`, read on their own.
type lexer struct {
	s        string
	i        int
	cmds     [][]string
	piped    []bool // cmds[i] pipes its output to cmds[i+1]
	subs     []string
	words    []string
	word     strings.Builder
	inWord   bool
	heredocs []heredoc
	// scripts are the here-document bodies a shell or an interpreter
	// reads; data, when set, gets the spans of the others.
	scripts []string
	data    *[][2]int
	// hereWords are the here-strings of each command, by its index; here:
	// the next word is one.
	hereWords map[int][]string
	here      bool
}

// heredoc is a here-document whose body starts on the next line.
type heredoc struct {
	end  string
	tabs bool // <<- strips leading tabs
	cmd  int  // the command it feeds, by its index in cmds
}

// inputRunners run code they read on their input, besides the shells.
var inputRunners = map[string]bool{"python": true, "python2": true, "python3": true, "perl": true, "ruby": true,
	"node": true, "deno": true, "bun": true, "php": true, "lua": true, "tclsh": true, "ssh": true,
	"source": true, ".": true, "eval": true, "osascript": true}

// runsInput tells if the input of cmds[i] reaches a command that runs it
// as code: a shell or an interpreter, there or further down its pipe, or
// xargs, which makes it the arguments of a command.
func (l *lexer) runsInput(i int) bool {
	for j := i; j < len(l.cmds); j++ {
		if runsCode(l.cmds[j]) {
			return true
		}
		if !l.piped[j] {
			break
		}
	}
	return false
}

func runsCode(words []string) bool {
	if slices.ContainsFunc(words, func(w string) bool { return filepath.Base(w) == "xargs" }) {
		return true
	}
	rest, script := lead(words)
	if script {
		return true
	}
	if len(rest) == 0 {
		return false
	}
	b := filepath.Base(rest[0])
	return shells[b] || inputRunners[b] || strings.HasPrefix(b, "python")
}

func (l *lexer) add(s string) { l.word.WriteString(s); l.inWord = true }

func (l *lexer) endWord() {
	if l.inWord {
		l.words = append(l.words, l.word.String())
		if l.here {
			if l.hereWords == nil {
				l.hereWords = map[int][]string{}
			}
			l.hereWords[len(l.cmds)] = append(l.hereWords[len(l.cmds)], l.word.String())
			l.here = false
		}
	}
	l.word.Reset()
	l.inWord = false
}

func (l *lexer) endCommand() {
	l.endWord()
	if len(l.words) > 0 {
		l.cmds = append(l.cmds, l.words)
		l.piped = append(l.piped, false)
	}
	l.words = nil
}

func (l *lexer) at(off int) byte {
	if l.i+off < len(l.s) {
		return l.s[l.i+off]
	}
	return 0
}

// scan reads to the end of the line or, with closing, to the ) that
// closes a $(. False: a quote, a $( or a ` is left open.
func (l *lexer) scan(closing bool) bool {
	parens := 0
	for l.i < len(l.s) {
		c := l.s[l.i]
		switch {
		case c == '\'':
			j := strings.IndexByte(l.s[l.i+1:], '\'')
			if j < 0 {
				return false
			}
			l.add(l.s[l.i+1 : l.i+1+j])
			l.i += j + 2
		case c == '$' && l.at(1) == '\'':
			l.i += 2
			if !l.ansiC() {
				return false
			}
		case c == '$' && l.at(1) == '"':
			l.i++
		case c == '"':
			l.i++
			if !l.double() {
				return false
			}
		case c == '$' && l.at(1) == '(':
			l.i += 2
			if !l.sub() {
				return false
			}
		case c == '`':
			if !l.backquote() {
				return false
			}
		case c == '\\':
			l.escaped()
		case c == '#' && !l.inWord:
			for l.i < len(l.s) && l.s[l.i] != '\n' {
				l.i++
			}
		case c == '<' && l.at(1) == '<' && l.at(2) == '<':
			l.endWord()
			l.i += 3
			l.here = true
		case c == '<' && l.at(1) == '<':
			l.endWord()
			l.i += 2
			l.heredoc()
		case c == '\n':
			l.endCommand()
			l.i++
			l.bodies()
		case c == ' ' || c == '\t' || c == '<' || c == '>':
			l.endWord()
			l.i++
		case c == '|' && l.at(1) == '|':
			l.endCommand()
			l.i += 2
		case c == '|':
			l.endCommand()
			if n := len(l.piped); n > 0 {
				l.piped[n-1] = true
			}
			l.i++
			if l.at(0) == '&' { // |& pipes stderr too
				l.i++
			}
		case c == ';' || c == '&' || c == '(':
			l.endCommand()
			l.i++
			if c == '(' {
				parens++
			}
		case c == ')':
			l.endCommand()
			l.i++
			if closing {
				if parens == 0 {
					return true
				}
				parens--
			}
		default:
			l.add(string(c))
			l.i++
		}
	}
	l.endCommand()
	return !closing
}

// escaped reads a backslash outside quotes: the next character stands
// for itself, and a backslash before a newline joins the lines.
func (l *lexer) escaped() {
	if l.i+1 >= len(l.s) {
		l.i++
		return
	}
	if l.s[l.i+1] == '\n' {
		l.i += 2
		return
	}
	_, n := utf8.DecodeRuneInString(l.s[l.i+1:])
	l.add(l.s[l.i+1 : l.i+1+n])
	l.i += 1 + n
}

// double reads "…" from past its opening quote.
func (l *lexer) double() bool {
	l.inWord = true
	for l.i < len(l.s) {
		c := l.s[l.i]
		switch {
		case c == '"':
			l.i++
			return true
		case c == '\\' && strings.IndexByte("\"\\$`\n", l.at(1)) >= 0 && l.at(1) != 0:
			if l.at(1) != '\n' {
				l.add(string(l.at(1)))
			}
			l.i += 2
		case c == '$' && l.at(1) == '(':
			l.i += 2
			if !l.sub() {
				return false
			}
		case c == '`':
			if !l.backquote() {
				return false
			}
		default:
			l.add(string(c))
			l.i++
		}
	}
	return false
}

// sub reads $(…) from past its $(, as its own script.
func (l *lexer) sub() bool {
	inner := lexer{s: l.s, i: l.i, data: l.data}
	if !inner.scan(true) {
		return false
	}
	l.subs = append(l.subs, l.s[l.i:inner.i-1])
	l.i = inner.i
	l.inWord = true
	return true
}

// backquote reads `…` as its own script.
func (l *lexer) backquote() bool {
	var script strings.Builder
	for j := l.i + 1; j < len(l.s); j++ {
		switch c := l.s[j]; {
		case c == '`':
			l.subs = append(l.subs, script.String())
			l.i = j + 1
			l.inWord = true
			return true
		case c == '\\' && j+1 < len(l.s) && strings.IndexByte("`\\$", l.s[j+1]) >= 0:
			script.WriteByte(l.s[j+1])
			j++
		default:
			script.WriteByte(c)
		}
	}
	return false
}

// ansiC reads $'…' from past its opening quote, decoding its escapes.
func (l *lexer) ansiC() bool {
	l.inWord = true
	for l.i < len(l.s) {
		c := l.s[l.i]
		if c == '\'' {
			l.i++
			return true
		}
		if c != '\\' || l.i+1 >= len(l.s) {
			l.add(string(c))
			l.i++
			continue
		}
		e := l.s[l.i+1]
		l.i += 2
		switch e {
		case 'n':
			l.add("\n")
		case 't':
			l.add("\t")
		case '\\', '\'', '"':
			l.add(string(e))
		case 'x':
			l.add(string([]byte{byte(l.digits(2, 16, 8))}))
		case 'u':
			l.add(string(rune(l.digits(4, 16, 21))))
		case 'U':
			l.add(string(rune(l.digits(8, 16, 21))))
		case '0', '1', '2', '3', '4', '5', '6', '7':
			l.i--
			l.add(string([]byte{byte(l.digits(3, 8, 8))}))
		default:
			l.add(`\` + string(e))
		}
	}
	return false
}

// digits reads up to n digits of base as a number.
// digits reads up to n digits; a value past bits saturates, as the
// shell keeps an escape inside a byte or a code point.
func (l *lexer) digits(n, base, bits int) uint64 {
	j := l.i
	for j < len(l.s) && j-l.i < n && isDigit(l.s[j], base) {
		j++
	}
	v, _ := strconv.ParseUint(l.s[l.i:j], base, bits)
	l.i = j
	return v
}

func isDigit(c byte, base int) bool {
	if base == 8 {
		return '0' <= c && c <= '7'
	}
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

// heredoc reads the word that ends a here-document; its body starts on
// the next line.
func (l *lexer) heredoc() {
	h := heredoc{cmd: len(l.cmds)}
	if l.at(0) == '-' {
		h.tabs = true
		l.i++
	}
	for l.at(0) == ' ' || l.at(0) == '\t' {
		l.i++
	}
	start := l.i
	for l.i < len(l.s) && strings.IndexByte(" \t\n;&|()<>", l.s[l.i]) < 0 {
		l.i++
	}
	h.end = strings.NewReplacer(`'`, "", `"`, "", `\`, "").Replace(l.s[start:l.i])
	if h.end != "" {
		l.heredocs = append(l.heredocs, h)
	}
}

// bodies skips the bodies of the here-documents of the line that ended:
// the shell reads them as text, not as commands. A body whose end word
// never comes is no data span: a << that is no here-document (an
// arithmetic shift) would otherwise hide the rest of the line from the
// raw readings.
func (l *lexer) bodies() {
	for _, h := range l.heredocs {
		start, end, ended := l.i, len(l.s), false
		for l.i < len(l.s) {
			at := l.i
			line, rest, _ := strings.Cut(l.s[l.i:], "\n")
			l.i = len(l.s) - len(rest)
			if h.tabs {
				line = strings.TrimLeft(line, "\t")
			}
			if line == h.end {
				end, ended = at, true
				break
			}
		}
		switch {
		case l.runsInput(h.cmd):
			l.scripts = append(l.scripts, l.s[start:end])
		case l.data != nil && ended:
			*l.data = append(*l.data, [2]int{start, end})
		}
	}
	l.heredocs = nil
}
