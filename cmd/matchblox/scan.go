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

// scanBudget bounds one walk of the host.
const scanBudget = 30 * time.Second

// scanWait outlasts a walk queued behind one already running.
const scanWait = 2*scanBudget + 15*time.Second

// cliScan sends the act, then says what the service found.
func cliScan(words []string, w io.Writer, path string) error {
	here, err := os.Getwd()
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	a, err := scanAct(words, here, home)
	if err != nil {
		return err
	}
	res, err := doActFor(path, a, scanWait)
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
	st, ok := serviceState(path, staleAfter)
	if !ok || st.Found == nil {
		return errors.New("scan: it ran, but the service did not say what it found: the N form lists it")
	}
	f := st.Found
	noun := "repositories"
	if len(f.Repos) == 1 {
		noun = "repository"
	}
	if _, err := fmt.Fprintf(w, "found %d %s in %s\n", len(f.Repos), noun, f.Took.Round(100*time.Millisecond)); err != nil {
		return err
	}
	for _, e := range f.Errors {
		if _, err := fmt.Fprintln(w, e); err != nil {
			return err
		}
	}
	return nil
}
