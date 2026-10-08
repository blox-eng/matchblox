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

// Ref is one key or command and where the docs name it.
type Ref struct {
	File string
	Line int
	Text string
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

// Keys returns each key in the first column of a Markdown table whose first
// header cell is "Key". A cell can name more than one key: `↑` or `k`.
func Keys(files []string) ([]Ref, error) {
	var refs []Ref
	err := eachLine(files, func(file string, n int, line string, _ bool, st *state) {
		cells := tableCells(line)
		switch {
		case cells == nil:
			st.keyTable = false
		case strings.EqualFold(strings.TrimSpace(cells[0]), "key"):
			st.keyTable = true
		case st.keyTable && !strings.HasPrefix(strings.TrimSpace(cells[0]), "-"):
			for _, m := range codeSpan.FindAllStringSubmatch(cells[0], -1) {
				refs = append(refs, Ref{file, n, m[1]})
			}
		}
	})
	return refs, err
}

// Commands returns each matchblox command line the docs show: a code span
// or a line of a fenced block that starts with `matchblox` (after a `$`
// prompt). The text starts at matchblox and ends at the span or the line.
func Commands(files []string) ([]Ref, error) {
	var refs []Ref
	err := eachLine(files, func(file string, n int, line string, fenced bool, _ *state) {
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
				refs = append(refs, Ref{file, n, c})
			}
		}
	})
	return refs, err
}

type state struct{ keyTable bool }

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
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				fenced = !fenced
				continue
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
