package app

import (
	"strconv"
	"strings"
	"time"
)

const (
	pairFromDeviceLine   = "Pair from the host's own terminal."
	removeFromDeviceLine = "Remove devices from the host's own terminal."
)

// deviceAttach is how long a device's console may take from its note to its
// tmux client. A client made later on that tty is not the device's: the
// system gave the tty again.
const deviceAttach = 30 * time.Second

// lastClient is the tty and the creation time of the client that was
// active last, "" on any doubt.
func (m Model) lastClient() (string, time.Time) {
	last := m.opt.LastClient
	if last == nil {
		session := m.consoleSession()
		last = func() (string, time.Time, error) { return m.multiplexer().LastClient(session) }
	}
	tty, created, err := last()
	if err != nil {
		return "", time.Time{}
	}
	return tty, created
}

// fromDevice: the client that was active last is a device. The phone and
// the builder share one console, so this is the way to tell who pressed.
// The client is the device's only when the device's note came just before
// the client: a note from an earlier session names a tty that the system
// can give to the builder now. On any doubt the key is the builder's.
func (m Model) fromDevice() bool {
	h := m.opt.Pair
	if h == nil || h.DeviceTTYs == nil || h.DeviceIn == nil {
		return false
	}
	tty, created := m.lastClient()
	return h.deviceOn(tty, created)
}

// deviceOn: the tmux client on tty, made at created, is a device's. Its
// note names tty and came just before the client; a later client on that
// tty is the builder's. On any doubt it is no device.
func (h *PairHost) deviceOn(tty string, created time.Time) bool {
	if h == nil || h.DeviceTTYs == nil || h.DeviceIn == nil || tty == "" || created.IsZero() {
		return false
	}
	id, ok := h.DeviceTTYs()[tty]
	if !ok {
		return false
	}
	at, _, ok := h.DeviceIn(id)
	// tmux gives whole seconds: the note may be up to 1 s after it.
	return ok && !at.After(created.Add(time.Second)) && created.Sub(at) <= deviceAttach
}

// clientMadeOf is when tty's client was made, in the output of tmux
// list-clients -F '#{client_created} #{client_tty}'; zero when none is.
func clientMadeOf(out, tty string) time.Time {
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) != 2 || f[1] != tty {
			continue
		}
		if made, err := strconv.ParseInt(f[0], 10, 64); err == nil {
			return time.Unix(made, 0)
		}
	}
	return time.Time{}
}
