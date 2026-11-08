package app

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/remote"
)

var errNotAttached = errors.New("this device is not attached: reconnect")

// consoleClient is a client of this console's tmux server: its tty, its
// last activity, and whether it shows the console's pane.
type consoleClient struct {
	tty    string
	active int64
	here   bool
}

// clientsOf reads tmux list-clients -F '#{client_activity} #{client_tty}
// #{pane_id}'. A client shows the console when its pane is the console's,
// whatever session it is attached to.
func clientsOf(out, consolePane string) []consoleClient {
	var cs []consoleClient
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) != 3 || !remote.ClientTTY(f[1]) {
			continue
		}
		at, err := strconv.ParseInt(f[0], 10, 64)
		if err != nil {
			continue
		}
		cs = append(cs, consoleClient{tty: f[1], active: at, here: consolePane != "" && f[2] == consolePane})
	}
	return cs
}

// deskOf is the client an unmarked key came from: the latest active one on
// the console that is no device; a device alone is its own desk.
func deskOf(cs []consoleClient, devices map[string]string) string {
	latest := func(ok func(consoleClient) bool) string {
		tty, at := "", int64(-1)
		for _, c := range cs {
			if c.here && ok(c) && c.active > at {
				tty, at = c.tty, c.active
			}
		}
		return tty
	}
	if desk := latest(func(c consoleClient) bool { _, device := devices[c.tty]; return !device }); desk != "" {
		return desk
	}
	return latest(func(consoleClient) bool { return true })
}

// othersHere tells if a client other than tty shows the console.
func othersHere(cs []consoleClient, tty string) bool {
	for _, c := range cs {
		if c.here && c.tty != tty {
			return true
		}
	}
	return false
}

// markLasts is how long a mark waits for the key it names.
const markLasts = 300 * time.Millisecond

// clientMark is the tty the tmux binding named, and when: the next input
// event is that client's.
type clientMark struct {
	tty string
	at  time.Time
}

func (c clientMark) spend(now time.Time) string {
	if c.tty == "" || now.Sub(c.at) > markLasts {
		return ""
	}
	return c.tty
}

// askingTTY is the client the event being handled came from: the marked
// one, else the desk. "" outside tmux or on a tmux error: the jump moves
// whichever client tmux picks, as before.
func (m Model) askingTTY() string {
	if m.askedBy != "" {
		return m.askedBy
	}
	if m.opt.OutsideTmux || m.opt.Pane == "" {
		return ""
	}
	out, err := m.clientList()()
	if err != nil {
		return ""
	}
	var devices map[string]string
	if h := m.opt.Pair; h != nil && h.DeviceTTYs != nil {
		devices = h.DeviceTTYs()
	}
	return deskOf(clientsOf(out, m.opt.Pane), devices)
}

// watchedBesides tells if a client other than tty shows the console pane.
// A tmux error says no.
func watchedBesides(list func() (string, error), consolePane, tty string) bool {
	out, err := list()
	return err == nil && othersHere(clientsOf(out, consolePane), tty)
}

func (m Model) clientList() func() (string, error) {
	if m.opt.ClientList != nil {
		return m.opt.ClientList
	}
	return tmuxClientList
}

func tmuxClientList() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", "list-clients", "-F", "#{client_activity} #{client_tty} #{pane_id}").Output()
	return string(out), err
}
