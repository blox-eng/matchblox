// Package push holds what the host sends the phone through the relay: the
// content state of the Live Activity, the buzz, and the phone's registration.
// A payload holds only numbers, states, times and percents; no session name,
// account name, provider, path or text. The testdata/push fixtures are read by
// the Swift tests too.
package push

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"
)

// ContentState is what the Live Activity shows. Host is the Host8 tag of the
// host, so the widget can show nothing for a host it does not follow.
type ContentState struct {
	Host   string    `json:"host"`
	Burn   int       `json:"burn"`
	Wait   int       `json:"wait"`
	WaitAt time.Time `json:"wait_at,omitzero"` // when the newest wait began; zero if none
	Top    *Top      `json:"top,omitempty"`
	// Needs are the sessions that wait for the person, at most MaxNeeds:
	// asks before a finished turn, the oldest first. Need counts them all.
	Needs    []Need       `json:"needs,omitempty"`
	Need     int          `json:"need,omitempty"`
	Accounts []AccountUse `json:"accounts"`
	Stoker   bool         `json:"stoker"`
	// Chain is the chain that needs the owner most, by number; nil when
	// the neediest session is in no chain, and the island shows it alone.
	Chain *TopChain `json:"chain,omitempty"`
}

// MaxNeeds is how many sessions that need the person the lock screen names.
const MaxNeeds = 3

// Need is a session that waits for the person, by number. Kind is command
// (a permission), question or done (its turn finished).
type Need struct {
	N    int    `json:"n"`
	Kind string `json:"kind"`
}

// Top is the session the island leads with, by number.
// TopChain is a chain on the island: its number (Numbers.Chain), what it
// needs (asks, waits, burns or quiet), its stoker's plan steps done of
// all, and the chain stokers that run on the host.
type TopChain struct {
	N       int    `json:"n"`
	State   string `json:"state"`
	Done    int    `json:"done"`
	Steps   int    `json:"steps"`
	Stokers int    `json:"stokers"`
}

type Top struct {
	N     int       `json:"n"`
	State string    `json:"state"` // burns, waits, done, out or idle
	Pct   int       `json:"pct"`   // 0..100, in steps of 5
	Out   time.Time `json:"out,omitzero"`
}

// AccountUse is one account's windows, by number.
type AccountUse struct {
	N       int         `json:"n"`
	Windows []WindowUse `json:"windows"`
	Lasts   bool        `json:"lasts"`
	RunsOut time.Time   `json:"runs_out,omitzero"`
	Idle    bool        `json:"idle,omitempty"` // no session works for it now: it does not lead the glance
}

// WindowUse is one usage window: Name is 5h, wk or mo.
type WindowUse struct {
	Name   string    `json:"name"`
	Pct    int       `json:"pct"`
	Resets time.Time `json:"resets,omitzero"`
	// Spent: at 100 % while a session of the account still works (extra or
	// on-demand usage), so it stops nothing and does not lead the glance.
	Spent bool `json:"spent,omitempty"`
}

// Buzz is the event that makes the phone buzz. Sealed is the HPKE enc followed
// by the ciphertext, so the relay cannot read it.
type Buzz struct {
	Host   string `json:"host"`
	N      int    `json:"n"`
	Sealed []byte `json:"sealed"`
}

// RegisterRequest is what the phone sends a host to be pushed to. Grants maps
// "<kind>:<target>" (device:<token>, start:<token>, channel:<id>) to the
// grant the relay checks, as Sender.send looks it up; NotifyKey is the X25519
// public key the buzz is sealed to.
type RegisterRequest struct {
	DeviceToken string            `json:"device_token"`
	Grants      map[string]string `json:"grants"`
	NotifyKey   []byte            `json:"notify_key"`
	Buzz        bool              `json:"buzz"`
	Island      *IslandReg        `json:"island,omitempty"`
}

// IslandReg is what the host needs to start and update the Live Activity.
type IslandReg struct {
	StartToken string `json:"start_token"`
	Channel    string `json:"channel"`
	// Ended: at this open the phone had no island of this host (swiped
	// away, or a new build installed). The host starts one at once.
	Ended bool `json:"ended,omitempty"`
}

// Registration is a request the host keeps, with the id of the phone's
// device line (setup.ValidPairID) and when it came. It lives here, not in
// setup: setup imports push, and push never imports setup.
type Registration struct {
	Device string `json:"device"`
	RegisterRequest
	At time.Time `json:"at"`
}

// RegisterReply answers a registration: the numbers the host keeps, each with
// the tag the island shows it by.
type RegisterReply struct {
	OK       bool           `json:"ok"`
	Error    string         `json:"error,omitempty"`
	Sessions map[int]string `json:"sessions"`
	Accounts map[int]string `json:"accounts"`
	Chains   map[int]string `json:"chains,omitempty"`
}

// Host8 is the first 8 hex characters of the SHA-256 in a fingerprint
// "SHA256:<base64>", lowercase; "" if it is not one.
func Host8(fingerprint string) string {
	b64, ok := strings.CutPrefix(fingerprint, "SHA256:")
	if !ok {
		return ""
	}
	sum, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(b64, "="))
	if err != nil || len(sum) < 4 {
		return ""
	}
	return hex.EncodeToString(sum[:4])
}
