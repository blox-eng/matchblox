package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// seenFile keeps the person's last open of each session, in the state
// directory, for every console and across a restart of the service.
const seenFile = "seen.json"

// seenKeep is how long the open of a session that is gone is kept.
const seenKeep = 7 * 24 * time.Hour

// seenOnlyFromConsole refuses a seen: act that no person at a console sent.
const seenOnlyFromConsole = "only a person at a console opens a session"

// seenEntry is when the person last opened a session, and the newest write
// of seen.json that found the session alive.
type seenEntry struct {
	At   time.Time `json:"at"`
	Here time.Time `json:"here"`
}

// seenKey is a session's key in seen.json: its conversation id, else its
// tmux session's name, for a pane id changes after a recover. Empty: none.
func seenKey(x sample.Session) string {
	if x.SessionID != "" {
		return "sid:" + x.SessionID
	}
	if name := x.TmuxSession(); name != "" {
		return "tmux:" + name
	}
	return ""
}

func (s *Service) seenPath() string {
	if s.StatePath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(s.StatePath), seenFile)
}

// loadSeenLocked reads seen.json once. A file that does not read is set
// aside, and the opens start empty. Hold s.mu.
func (s *Service) loadSeenLocked() {
	if s.opened != nil {
		return
	}
	s.opened = map[string]seenEntry{}
	path := s.seenPath()
	if path == "" {
		return
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return
	}
	if err == nil {
		if err = json.Unmarshal(b, &s.opened); err == nil {
			return
		}
		s.opened = map[string]seenEntry{}
	}
	bad, aerr := setAside(path, s.now())
	if aerr != nil {
		fmt.Fprintf(os.Stderr, "matchblox: seen.json did not read, so opens start empty: %v; it stays at %s: %v\n", err, path, aerr)
		return
	}
	fmt.Fprintf(os.Stderr, "matchblox: seen.json did not read, so opens start empty; it is kept at %s: %v\n", bad, err)
}

// setAside moves path to path.bad, or to path.bad-<time> when an older
// one is there: no bad file is overwritten.
func setAside(path string, now time.Time) (string, error) {
	bad := path + ".bad"
	err := os.Link(path, bad)
	if errors.Is(err, fs.ErrExist) {
		bad = path + ".bad-" + now.UTC().Format("20060102T150405")
		err = os.Link(path, bad)
	}
	if err != nil {
		return "", err
	}
	return bad, os.Remove(path)
}

// stampSeen sets each session's SeenAt from seen.json. Hold s.mu.
func (s *Service) stampSeen() {
	s.loadSeenLocked()
	copied := false
	for i, x := range s.cur.Sessions {
		at := s.opened[seenKey(x)].At
		if x.SeenAt.Equal(at) {
			continue
		}
		if !copied { // cur.Sessions can share its array with the sample
			s.cur.Sessions, copied = append(s.cur.Sessions[:0:0], s.cur.Sessions...), true
		}
		s.cur.Sessions[i].SeenAt = at
	}
}

// markSeen keeps now as the person's last open of the session in pane, for
// every console, and drops the opens of sessions gone for seenKeep.
func (s *Service) markSeen(a proto.Act, pane string) proto.Result {
	if !s.aConsole(a.Console, a) {
		return proto.Result{Err: seenOnlyFromConsole}
	}
	s.mu.Lock()
	x, ok := s.sessionAtLocked(pane)
	key := seenKey(x)
	if !ok || key == "" {
		s.mu.Unlock()
		return refuseText(fmt.Sprintf("not kept: no session %q", pane))
	}
	s.loadSeenLocked()
	now := s.now()
	next := maps.Clone(s.opened)
	next[key] = seenEntry{At: now}
	alive := map[string]bool{}
	for _, x := range s.cur.Sessions {
		alive[seenKey(x)] = true
	}
	for k, e := range next {
		switch {
		case alive[k]:
			e.Here = now
			next[k] = e
		case now.Sub(e.Here) > seenKeep:
			delete(next, k)
		}
	}
	if err := keepSeen(s.seenPath(), next); err != nil {
		s.mu.Unlock()
		return proto.Result{Err: "not kept: " + err.Error()}
	}
	s.opened = next
	s.stampSeen()
	s.mu.Unlock()
	s.broadcast()
	return proto.Result{}
}

// keepSeen writes seen.json whole or not at all, 0600. Empty path: none kept.
func keepSeen(path string, by map[string]seenEntry) error {
	if path == "" {
		return nil
	}
	b, err := json.Marshal(by)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), seenFile+".*") // 0600
	if err != nil {
		return err
	}
	_, werr := tmp.Write(b)
	if cerr := tmp.Close(); werr != nil || cerr != nil {
		_ = os.Remove(tmp.Name())
		return errors.Join(werr, cerr)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}
