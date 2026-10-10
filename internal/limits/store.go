package limits

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Keep is how long the store keeps a sample.
const Keep = 35 * 24 * time.Hour

// Sample is one line of the store: an account's reading.
type Sample struct {
	Provider string `json:"provider"`
	Account  string `json:"account"`
	Reading
}

// Store keeps the readings of every account in a local file, one line for
// each new reading, for the usage patterns to read. It prunes once, at the
// first append of a service.
type Store struct {
	Path string

	pruned bool
	last   map[string]time.Time // account -> its newest stored reading
}

func key(provider, account string) string { return provider + "\x00" + account }

// Append stores the measured readings that are newer than the stored ones.
func (s *Store) Append(accts []Account, now time.Time) error {
	if !s.pruned {
		if err := s.prune(now); err != nil {
			return err
		}
		s.pruned = true
	}
	var buf bytes.Buffer
	for _, a := range accts {
		if a.State == Unmeasured || a.At.IsZero() {
			continue
		}
		k := key(a.Provider, a.Account)
		if !a.At.After(s.last[k]) {
			continue
		}
		b, err := json.Marshal(Sample{a.Provider, a.Account, Reading{At: a.At, Windows: a.Windows}})
		if err != nil {
			return err
		}
		buf.Write(append(b, '\n'))
		s.last[k] = a.At
	}
	if buf.Len() == 0 {
		return nil
	}
	f, err := os.OpenFile(s.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		return errors.Join(err, f.Close())
	}
	return f.Close()
}

// prune drops the lines older than Keep and learns the newest reading of
// each account.
func (s *Store) prune(now time.Time) error {
	s.last = map[string]time.Time{}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var keep bytes.Buffer
	dropped := false
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var x Sample
		if json.Unmarshal(sc.Bytes(), &x) != nil || now.Sub(x.At) > Keep {
			dropped = true
			continue
		}
		keep.Write(append(sc.Bytes(), '\n'))
		if k := key(x.Provider, x.Account); x.At.After(s.last[k]) {
			s.last[k] = x.At
		}
	}
	if !dropped {
		return nil
	}
	return writeFile(s.Path, keep.Bytes())
}

// writeFile replaces path atomically, readable by its owner only.
func writeFile(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".limits-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// tapPath is the reading of one Claude Code config directory.
func tapPath(stateDir, claudeDir string) string {
	h := sha256.Sum256([]byte(filepath.Clean(claudeDir)))
	return filepath.Join(stateDir, "limits", "claude-"+hex.EncodeToString(h[:6])+".json")
}

type statusInput struct {
	RateLimits *struct {
		FiveHour *claudeWindow `json:"five_hour"`
		SevenDay *claudeWindow `json:"seven_day"`
	} `json:"rate_limits"`
}

type claudeWindow struct {
	Used     float64 `json:"used_percentage"`
	ResetsAt int64   `json:"resets_at"`
}

type tapFile struct {
	Dir string `json:"dir"`
	Reading
}

// Tap stores the limits Claude Code gave its status line, for the config
// directory the status line runs under. Input without limits (an API key)
// leaves the last reading.
func Tap(input []byte, claudeDir, stateDir string, now time.Time) error {
	var in statusInput
	if err := json.Unmarshal(input, &in); err != nil {
		return err
	}
	if in.RateLimits == nil {
		return nil
	}
	r := Reading{At: now}
	for _, w := range []struct {
		cw  *claudeWindow
		min int
	}{{in.RateLimits.FiveHour, BoxMinutes}, {in.RateLimits.SevenDay, WeekMinutes}} {
		if w.cw != nil {
			r.Windows = append(r.Windows, Window{UsedPct: w.cw.Used, Minutes: w.min, ResetsAt: time.Unix(w.cw.ResetsAt, 0)})
		}
	}
	if len(r.Windows) == 0 {
		return nil
	}
	b, err := json.Marshal(tapFile{filepath.Clean(claudeDir), r})
	if err != nil {
		return err
	}
	path := tapPath(stateDir, claudeDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeFile(path, b)
}

// ReadTap reads the last reading the tap stored for a config directory.
func ReadTap(stateDir, claudeDir string) (*Reading, bool) {
	if claudeDir == "" {
		return nil, false
	}
	b, err := os.ReadFile(tapPath(stateDir, claudeDir))
	if err != nil {
		return nil, false
	}
	var f tapFile
	if json.Unmarshal(b, &f) != nil || f.Dir != filepath.Clean(claudeDir) {
		return nil, false
	}
	return &f.Reading, true
}
