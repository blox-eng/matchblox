package app

import (
	"context"
	"maps"

	"github.com/blox-eng/matchblox/internal/remote"
)

// hostLogin is the builder's own ssh login to one host (R13): its check,
// its master's open and its key ladder.
type hostLogin struct {
	login     remote.Login // as the check found it
	checking  bool         // the login check runs
	checked   bool         // the login check started once
	jumpLater *action      // the jump that waits for the login check
	opening   bool         // the master of a jump opens, its prompts in the console
	notNow    bool         // the key ladder offers nothing more: not now, or the key is on the host
	// checkStop ends the login check that runs; checkCancelled: the builder
	// ended it at its Tailscale link. tailscale: the host is Tailscale SSH,
	// as its last check with the server's version line said. loginURL is
	// Tailscale's check link in the field, while the ssh that printed it
	// waits; "" none. Only remote.VisitURL gives it.
	checkStop      context.CancelFunc
	checkCancelled bool
	tailscale      bool
	loginURL       string
	// note is what the ladder said before the jump it held logs in: the
	// login covers the flash, and the jump that goes shows it again.
	note string
	// openStop ends the open-first that runs; askCancelled: the builder
	// cancelled its last prompt.
	openStop     context.CancelFunc
	askCancelled bool
}

// loginOf is host's login: the zero value before its check.
func (m Model) loginOf(host string) hostLogin { return m.logins[host] }

// setLogin keeps l as host's login. The map is copied: a Model is a value,
// and an older copy keeps its own.
func (m *Model) setLogin(host string, l hostLogin) {
	logins := maps.Clone(m.logins)
	if logins == nil {
		logins = map[string]hostLogin{}
	}
	logins[host] = l
	m.logins = logins
}

// editLogin changes host's login with f.
func (m *Model) editLogin(host string, f func(*hostLogin)) {
	l := m.loginOf(host)
	f(&l)
	m.setLogin(host, l)
}
