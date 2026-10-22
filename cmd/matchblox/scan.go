package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/state"
)

// scanAct is the act of `matchblox scan`: a yes, with the folders given
// (absolute, ~ the home), or off: a no.
func scanAct(words []string, here, home string) (proto.Act, error) {
	if len(words) == 1 && words[0] == "off" {
		return proto.Act{RecID: "scan:no"}, nil
	}
	var roots []string
	for _, w := range words {
		switch {
		case w == "~":
			w = home
		case strings.HasPrefix(w, "~/"):
			w = filepath.Join(home, w[2:])
		case !filepath.IsAbs(w):
			w = filepath.Join(here, w)
		}
		if fi, err := os.Stat(w); err != nil || !fi.IsDir() {
			return proto.Act{}, errors.New("scan: no folder " + w)
		}
		roots = append(roots, w)
	}
	return proto.Act{RecID: "scan:yes", Confirm: "y", Text: strings.Join(roots, "\n")}, nil
}

// cliScan sends the act, then says what the scan found in reposPath.
func cliScan(words []string, w io.Writer, path, reposPath string) error {
	here, err := os.Getwd()
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	a, err := scanAct(words, here, home)
	if err != nil {
		return err
	}
	res, err := doAct(path, a)
	if err != nil {
		if errors.Is(err, errNoResult) {
			return errors.New("scan: not known whether it ended: the N form lists what it found")
		}
		return fmt.Errorf("scan: did not run: %w", err)
	}
	if err := reportResult(io.Discard, res); err != nil {
		return fmt.Errorf("scan: refused: %w", err)
	}
	if a.RecID == "scan:no" {
		_, err := fmt.Fprintln(w, "the scan is off, and its list is gone")
		return err
	}
	f := state.ReadFound(reposPath)
	noun := "repositories"
	if len(f.Repos) == 1 {
		noun = "repository"
	}
	_, err = fmt.Fprintf(w, "found %d %s in %s\n", len(f.Repos), noun, f.Took.Round(100*time.Millisecond))
	return err
}
