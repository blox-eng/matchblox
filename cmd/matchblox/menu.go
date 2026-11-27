package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/state"
)

// pocketCmd runs Pocket's two commands for its head: `menu <who> --json`
// reads the menu a session draws, and `choose <who> <n> --text <t> --sign
// <s>` picks option n of it, while it still reads as menu printed it. Both
// say hello as Pocket, a console for a menu only.
func pocketCmd(cmd string, args []string, w io.Writer, path string) error {
	fl := flag.NewFlagSet(cmd, flag.ContinueOnError)
	asJSON := fl.Bool("json", false, "menu: print JSON")
	text := fl.String("text", "", "choose: the option's whole text, as menu printed it")
	sign := fl.String("sign", "", "choose: the menu's sign, as menu printed it")
	words, err := positionals(fl, args)
	if err != nil {
		return err
	}
	if cmd == "menu" && !*asJSON {
		return errors.New("menu prints JSON only: matchblox menu <who> --json")
	}
	doc, err := readDoc(path, staleAfter)
	if err != nil {
		return err
	}
	a, err := pocketAct(doc, cmd, words, *text, *sign, os.Getenv("TMUX_PANE"))
	if err != nil {
		return err
	}
	res, err := doActAs(path, proto.ClientPocket, a, actWait, nil)
	if cmd == "choose" {
		return handOutcome(w, cmd, words[0], res, err)
	}
	if err == nil && res.Err != "" {
		err = errors.New(res.Err)
	}
	if err != nil {
		return fmt.Errorf("menu %s: %w", words[0], err)
	}
	return writeMenu(w, res.Menu)
}

// pocketAct is the act menu or choose asks the service for.
func pocketAct(doc state.Doc, cmd string, words []string, text, sign, from string) (proto.Act, error) {
	switch {
	case cmd == "menu" && len(words) != 1:
		return proto.Act{}, errors.New("menu reads one session: matchblox menu <who> --json")
	case cmd == "choose" && len(words) != 2:
		return proto.Act{}, errors.New("choose picks one option: matchblox choose <who> <n> --text <t> --sign <s>")
	}
	sess, err := resolve(doc, words[0])
	if err != nil {
		return proto.Act{}, err
	}
	if cmd == "menu" {
		return proto.Act{RecID: proto.MenuPrefix + sess.Pane, From: from}, nil
	}
	if n, err := strconv.Atoi(words[1]); err != nil || n < 1 || n > 9 {
		return proto.Act{}, fmt.Errorf("choose picks an option from 1 to 9, not %q", words[1])
	}
	if text == "" || sign == "" {
		return proto.Act{}, errors.New("choose needs the option's --text and the menu's --sign, as menu printed them")
	}
	return proto.Act{RecID: proto.ChoosePrefix + sess.Pane, Text: words[1] + "\x00" + text + "\x00" + sign, PID: sess.PID, Start: sess.Start, Confirm: "y", From: from}, nil
}

// writeMenu prints m as JSON; no menu is no options, a list all the same.
func writeMenu(w io.Writer, m *proto.Menu) error {
	if m == nil {
		m = &proto.Menu{}
	}
	if m.Options == nil {
		m.Options = []proto.Option{}
	}
	return writeJSON(w, m)
}
