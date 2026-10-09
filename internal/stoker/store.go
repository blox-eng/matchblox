package stoker

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store keeps the mode in stoker.json and every step in stoker.jsonl, so
// both survive a restart of the console and of the service.
type Store struct {
	dir     string
	mu      sync.Mutex
	mode    Mode
	entries []Entry
}

// View is what the console draws: the mode and the steps of the last run.
type View struct {
	Mode
	Run []Entry `json:"run,omitempty"`
	// Unseen: the run has a step the builder has not acked.
	Unseen bool `json:"unseen,omitempty"`
}

// Dir holds stoker.json and stoker.jsonl.
func (s *Store) Dir() string { return s.dir }

func (s *Store) modePath() string { return filepath.Join(s.dir, "stoker.json") }
func (s *Store) logPath() string  { return filepath.Join(s.dir, "stoker.jsonl") }

// ReadMode reads stoker.json alone: cheap enough for every sample. A
// missing file is the stoker that never ran.
func ReadMode(dir string) (Mode, error) {
	var m Mode
	b, err := os.ReadFile(filepath.Join(dir, "stoker.json")) //nolint:gosec // our own state dir
	if errors.Is(err, fs.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(b, &m)
}

// Open reads the files in dir. Missing files are an empty store.
func Open(dir string) (*Store, error) {
	m, err := ReadMode(dir)
	if err != nil {
		return nil, err
	}
	s := &Store{dir: dir, mode: m}
	f, err := os.Open(s.logPath())
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.ID != "" {
			s.merge(e)
		}
	}
	s.prune()
	return s, sc.Err()
}

func (s *Store) Mode() Mode {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mode
}

// Set writes the mode atomically.
func (s *Store) Set(m Mode) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, ".stoker-*.json")
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Rename(tmp.Name(), s.modePath()); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	s.mode = m
	s.prune()
	return nil
}

// Append writes one line and adds it to the entries; a line with a known
// ID fills in the step it names.
func (s *Store) Append(e Entry) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(s.logPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	s.merge(e)
	return nil
}

// merge adds e or fills in the entry with its ID. Hold s.mu.
func (s *Store) merge(e Entry) {
	for i := range s.entries {
		if s.entries[i].ID != e.ID {
			continue
		}
		x := &s.entries[i]
		if e.After != 0 {
			x.After = e.After
		}
		if e.Before != 0 && e.Result == "" {
			x.Before = e.Before // the count the compact itself reports
		}
		if e.Resume != "" {
			x.Resume = e.Resume
		}
		return
	}
	if !e.At.IsZero() {
		s.entries = append(s.entries, e)
	}
}

// prune keeps the steps of the last run and of the last PerSession, which
// the plan needs. Hold s.mu.
func (s *Store) prune() {
	keep := s.mode.Since.Add(-PerSession)
	if s.mode.Since.IsZero() {
		keep = time.Now().Add(-PerSession)
	}
	out := s.entries[:0]
	for _, e := range s.entries {
		if !e.At.Before(keep) {
			out = append(out, e)
		}
	}
	s.entries = out
}

// Entries are the steps kept in memory, oldest first.
func (s *Store) Entries() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Entry(nil), s.entries...)
}

// Pending are the sent compacts whose result is still to be read back.
func (s *Store) Pending(now time.Time) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Entry
	for _, e := range s.entries {
		if e.Result == Sent && e.After == 0 && now.Sub(e.At) < ResumeWait {
			out = append(out, e)
		}
	}
	return out
}

func (s *Store) View() View {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := View{Mode: s.mode}
	if s.mode.Since.IsZero() {
		return v
	}
	for _, e := range s.entries {
		if e.At.Before(s.mode.Since) {
			continue
		}
		v.Run = append(v.Run, e)
		if e.At.After(s.mode.Acked) {
			v.Unseen = true
		}
	}
	return v
}
