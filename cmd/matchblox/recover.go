package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/service"
)

// recoverAct is the act of matchblox recover: a preview with no names and
// no --all, else the reopen of those. --dir moves the folder of one name.
func recoverAct(words []string, all bool, dir, from string) (proto.Act, error) {
	if all && len(words) > 0 {
		return proto.Act{}, errors.New("recover: --all or names, not both")
	}
	r := proto.Recover{All: all, Names: words}
	if dir != "" {
		if len(words) != 1 {
			return proto.Act{}, errors.New("recover: --dir moves the folder of one session: matchblox recover <name> --dir <path>")
		}
		r.Dirs = map[string]string{words[0]: dir}
	}
	b, err := json.Marshal(r)
	if err != nil {
		return proto.Act{}, err
	}
	return proto.Act{RecID: service.RecoverID, Confirm: "y", Text: string(b), From: from}, nil
}

func cliRecover(words []string, all bool, dir string, asJSON bool, w io.Writer, path string) error {
	a, err := recoverAct(words, all, dir, os.Getenv("TMUX_PANE"))
	if err != nil {
		return err
	}
	res, err := doAct(path, a)
	if err != nil {
		return fmt.Errorf("recover: %w", err)
	}
	if res.Err != "" {
		return fmt.Errorf("recover: %s", res.Err)
	}
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(res.Recover)
	}
	return printRecover(w, res.Recover, all || len(words) > 0)
}

// printRecover writes one line per entry: its name, agent, state, folder
// and why. After a preview, it says what reopens them. After a reopen, an
// entry that could not reopen fails the command.
func printRecover(w io.Writer, rows []proto.Recovered, reopen bool) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "The recovery record is empty: matchblox records the agents on your tmux server while its service runs.")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	ready, failed := 0, 0
	for _, r := range rows {
		state := r.State
		switch r.State {
		case proto.RecoverReady:
			ready++
			state = "can reopen"
		case proto.RecoverReopened:
			if r.As != r.Name {
				state = "reopened as " + r.As
			}
		case proto.RecoverCannot:
			state = "cannot reopen"
			if reopen {
				failed++
			}
		}
		line := []string{r.Name, r.Agent, state, r.Dir}
		if r.Why != "" {
			line = append(line, r.Why)
		}
		if _, err := fmt.Fprintln(tw, strings.Join(line, "\t")); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if !reopen && ready > 0 {
		if _, err := fmt.Fprintf(w, "\nmatchblox recover --all reopens the %d that can reopen; matchblox recover <name> reopens one. Each waits for you: no prompt is sent.\n", ready); err != nil {
			return err
		}
	}
	if failed > 0 {
		return fmt.Errorf("recover: %d could not reopen: each line says why and the fix", failed)
	}
	return nil
}
