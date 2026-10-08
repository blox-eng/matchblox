// Package docscheck reads the keys and the commands that the README, the
// contributing guide and the docs name, so tests can hold them to the binary.
package docscheck

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Ref is one key or command and where the docs name it: the file, the
// line, the heading it is under, and if it is in a Key table.
type Ref struct {
	File    string
	Line    int
	Text    string
	Section string
	Table   bool
}

// Files lists the Markdown a builder reads: README.md, CONTRIBUTING.md and
// every page under docs/, relative to the repository root.
func Files(root string) ([]string, error) {
	files := []string{filepath.Join(root, "README.md"), filepath.Join(root, "CONTRIBUTING.md")}
	err := filepath.WalkDir(filepath.Join(root, "docs"), func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".md") {
			files = append(files, p)
		}
		return nil
	})
	return files, err
}

var codeSpan = regexp.MustCompile("`([^`]+)`")

// keyWords are the keys prose names by a word, not a character.
var keyWords = map[string]bool{
	"enter": true, "esc": true, "space": true, "tab": true, "shift+tab": true,
	"backspace": true, "ctrl+c": true, "↑": true, "↓": true,
}

// Keys returns each key in the first column of a Markdown table whose first
// header cell is "Key" (a cell can name more than one: `↑` or `k`), and
// each key in prose: a code span of one ASCII character or a key word.
func Keys(files []string) ([]Ref, error) {
	var refs []Ref
	err := eachLine(files, func(file string, n int, line string, fenced bool, st *state) {
		if fenced {
			return
		}
		cells := tableCells(line)
		prose := line
		switch {
		case cells == nil:
			st.keyTable = false
		case strings.EqualFold(strings.TrimSpace(cells[0]), "key"):
			st.keyTable = true
			return
		case st.keyTable && !strings.HasPrefix(strings.TrimSpace(cells[0]), "-"):
			for _, m := range codeSpan.FindAllStringSubmatch(cells[0], -1) {
				refs = append(refs, Ref{File: file, Line: n, Text: m[1], Section: st.section, Table: true})
			}
			prose = strings.Join(cells[1:], "|")
		}
		for _, m := range codeSpan.FindAllStringSubmatch(prose, -1) {
			if isKey(m[1]) {
				refs = append(refs, Ref{File: file, Line: n, Text: m[1], Section: st.section})
			}
		}
	})
	return refs, err
}

func isKey(s string) bool {
	return (len(s) == 1 && s[0] > ' ' && s[0] < 0x7f) || keyWords[strings.ToLower(s)]
}

// Commands returns each matchblox command line the docs show: a code span
// or a line of a fenced block that starts with `matchblox` (after a `$`
// prompt). The text starts at matchblox and ends at the span or the line.
func Commands(files []string) ([]Ref, error) {
	var refs []Ref
	err := eachLine(files, func(file string, n int, line string, fenced bool, st *state) {
		var cands []string
		if fenced {
			cands = []string{line}
		} else {
			for _, m := range codeSpan.FindAllStringSubmatch(line, -1) {
				cands = append(cands, m[1])
			}
		}
		for _, c := range cands {
			c = strings.TrimPrefix(strings.TrimSpace(c), "$ ")
			if c == "matchblox" || strings.HasPrefix(c, "matchblox ") {
				refs = append(refs, Ref{File: file, Line: n, Text: c, Section: st.section})
			}
		}
	})
	return refs, err
}

type state struct {
	keyTable bool
	section  string
}

func eachLine(files []string, f func(file string, n int, line string, fenced bool, st *state)) error {
	for _, file := range files {
		h, err := os.Open(file)
		if err != nil {
			return err
		}
		sc := bufio.NewScanner(h)
		fenced, n, st := false, 0, &state{}
		for sc.Scan() {
			n++
			line := sc.Text()
			if t := strings.TrimSpace(line); strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
				fenced = !fenced
				continue
			}
			if !fenced && strings.HasPrefix(line, "#") {
				st.section = strings.TrimSpace(strings.TrimLeft(line, "#"))
			}
			f(file, n, line, fenced, st)
		}
		_ = h.Close()
		if err := sc.Err(); err != nil {
			return err
		}
	}
	return nil
}

// tableCells splits a Markdown table row; nil for a line that is not one.
func tableCells(line string) []string {
	s := strings.TrimSpace(line)
	if !strings.HasPrefix(s, "|") {
		return nil
	}
	return strings.Split(strings.Trim(s, "|"), "|")
}
