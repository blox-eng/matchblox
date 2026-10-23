package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/service"
)

// stat is os.Stat; a test swaps it.
var stat = os.Stat

// evalSymlinks is filepath.EvalSymlinks; a test swaps it.
var evalSymlinks = filepath.EvalSymlinks

// resolveHome is home with its links followed, as the service reports
// paths; home itself when it does not resolve, or not in within.
func resolveHome(home string, within time.Duration) string {
	if home == "" {
		return ""
	}
	ch := make(chan string, 1)
	ev := evalSymlinks // read here: the goroutine may outlive a test that swaps it back
	go func() {
		if p, err := ev(home); err == nil {
			ch <- p
		} else {
			ch <- home
		}
	}()
	t := time.NewTimer(within)
	defer t.Stop()
	select {
	case p := <-ch:
		return p
	case <-t.C:
		return home
	}
}

var errNoFolder = errors.New("no folder")

// statDir is nil when path is a folder. A folder that does not answer in
// within is an error: a dead network mount must not hang the CLI.
func statDir(path string, within time.Duration) error {
	type answer struct {
		fi  os.FileInfo
		err error
	}
	ch := make(chan answer, 1)
	st := stat // read here: the goroutine may outlive a test that swaps it back
	go func() {
		fi, err := st(path)
		ch <- answer{fi, err}
	}()
	t := time.NewTimer(within)
	defer t.Stop()
	select {
	case a := <-ch:
		if a.err != nil || a.fi == nil || !a.fi.IsDir() {
			return fmt.Errorf("scan: %w %s", errNoFolder, path)
		}
		return nil
	case <-t.C:
		return fmt.Errorf("scan: %s did not answer in %s", path, within)
	}
}

// scanAct is the act of `matchblox scan`: a yes, with the folders given
// (absolute, ~ the home), or off: a no.
func scanAct(words []string, here, home string) (proto.Act, error) {
	if len(words) == 1 && words[0] == "off" {
		return proto.Act{RecID: "scan:no"}, nil
	}
	var roots, missing []string
	for _, w := range words {
		switch {
		case w == "~":
			w = home
		case strings.HasPrefix(w, "~/"):
			w = filepath.Join(home, w[2:])
		case !filepath.IsAbs(w):
			w = filepath.Join(here, w)
		}
		switch err := statDir(w, 2*time.Second); {
		case errors.Is(err, errNoFolder):
			missing = append(missing, w)
		case err != nil:
			return proto.Act{}, err
		}
		roots = append(roots, w)
	}
	if len(missing) > 0 {
		return proto.Act{}, errors.New("scan: no folder " + strings.Join(missing, ", "))
	}
	return proto.Act{RecID: "scan:yes", Confirm: "y", Text: strings.Join(roots, "\n")}, nil
}

// scanBudget and scanMaxDirs bound one walk of the host.
const (
	scanBudget  = 30 * time.Second
	scanMaxDirs = 100_000
)

// ioSlack is what the wait adds for the service to answer.
const ioSlack = 10 * time.Second

// scanOpts is how the service walks the host.
func scanOpts(cfg config.Config, home string) gitscan.FindOpts {
	return gitscan.FindOpts{Depth: gitscan.FindDepth, Remote: cfg.Git.Remote, MaxDirs: scanMaxDirs, Budget: scanBudget, Home: home, Protected: gitscan.DefaultProtected(home)}
}

// scanWait outlasts a walk queued behind one already running.
func scanWait(o gitscan.FindOpts) time.Duration { return 2*o.Limit() + ioSlack }

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
	res, err := doActFor(path, a, scanWait(scanOpts(config.Config{}, home)))
	if err != nil {
		if errors.Is(err, errNoResult) {
			return errors.New("scan: not known whether it ended: the N form lists what it found")
		}
		return fmt.Errorf("scan: did not run: %w", err)
	}
	if err := reportResult(io.Discard, res); err != nil {
		if service.ScanRan(res.Err) { // it ran, but kept nothing for this yes
			return fmt.Errorf("scan: %w", err)
		}
		return fmt.Errorf("scan: refused: %w", err)
	}
	if a.RecID == "scan:no" {
		_, err := fmt.Fprintln(w, "the scan is off, and its list is gone")
		return err
	}
	sc := res.Scan
	if sc == nil {
		return errors.New("scan: it ran, but the service did not say what it found: the N form lists it")
	}
	noun := "repositories"
	if sc.Repos == 1 {
		noun = "repository"
	}
	if _, err := fmt.Fprintf(w, "found %d %s in %s\n", sc.Repos, noun, sc.Took.Round(100*time.Millisecond)); err != nil {
		return err
	}
	for _, e := range sc.Errors {
		if _, err := fmt.Fprintln(w, e); err != nil {
			return err
		}
	}
	real := resolveHome(home, 2*time.Second)
	for _, k := range sc.Skipped {
		p := k.Path
		for _, h := range []string{real, home} {
			if h != "" && (p == h || strings.HasPrefix(p, h+string(filepath.Separator))) {
				p = "~" + p[len(h):]
				break
			}
		}
		if _, err := fmt.Fprintf(w, "skipped %s: %s\n", p, k.Why); err != nil {
			return err
		}
	}
	if sc.SkippedMore > 0 {
		_, err := fmt.Fprintf(w, "and %d more\n", sc.SkippedMore)
		return err
	}
	return nil
}
