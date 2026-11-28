package service

import (
	"maps"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
)

// keysAtOnlyFromConsole refuses a keys: record that is not from a person
// at a console: KeysAt is the owner's last give of the keys, not an agent's.
const keysAtOnlyFromConsole = "only a person at a console gives the agent the keys"

// recordKeysAt notes that the owner gave the session the keys. Text empty
// on keys:<session-id> is this record, not a keystroke.
func (s *Service) recordKeysAt(a proto.Act, id string) proto.Result {
	if !s.aConsole(a.Console, a) {
		return proto.Result{Err: keysAtOnlyFromConsole}
	}
	if id == "" {
		return proto.Result{Err: "no session to give the keys"}
	}
	s.mu.Lock()
	next := maps.Clone(s.cur.KeysAt)
	if next == nil {
		next = map[string]time.Time{}
	}
	next[id] = s.now()
	s.cur.KeysAt = next
	s.mu.Unlock()
	s.broadcast()
	return proto.Result{}
}
