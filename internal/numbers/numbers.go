// Package numbers gives each live session a number from 1 to 9 that stays
// with it while it lives.
package numbers

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/blox-eng/matchblox/internal/chains"
)

const most = 9

type entry struct {
	chains.Member
	Number int `json:"number"`
}

// Store keeps numbers.json in a directory. An empty dir keeps them in memory.
type Store struct {
	dir string
	mu  sync.Mutex
	got []entry
}

func (s *Store) path() string { return filepath.Join(s.dir, "numbers.json") }

// Open reads numbers.json. A missing file is no numbers; one that does not
// parse is moved aside.
func Open(dir string) (*Store, error) {
	s := &Store{dir: dir}
	if dir == "" {
		return s, nil
	}
	b, err := os.ReadFile(s.path()) //nolint:gosec // our own state dir
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.got); err != nil {
		s.got = nil
		bad := s.path() + ".bad-" + time.Now().UTC().Format("20060102T150405")
		if err := os.Rename(s.path(), bad); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// samePlace is the same process in the same pane: it also finds a cleared
// chat, which has a new session id.
func samePlace(a, b chains.Member) bool {
	return a.Pane != "" && a.PID > 0 && a.Pane == b.Pane && a.PID == b.PID && a.Start == b.Start
}

// keptLocked is the number each present member kept, 0 for none. A member
// finds it by its pane, pid and start first, and by its session id only
// after: two live sessions can report one id. Hold s.mu.
func (s *Store) keptLocked(present []chains.Member) (nums []int, used map[int]bool) {
	nums, used = make([]int, len(present)), map[int]bool{}
	for _, same := range []func(a, b chains.Member) bool{samePlace, chains.Member.Same} {
		for i, m := range present {
			if nums[i] != 0 {
				continue
			}
			j := slices.IndexFunc(s.got, func(e entry) bool { return !used[e.Number] && same(e.Member, m) })
			if j < 0 {
				continue
			}
			nums[i] = s.got[j].Number
			used[nums[i]] = true
		}
	}
	return nums, used
}

// Peek is the number each present member kept, 0 for none, and changes
// nothing: for a sample that missed.
func (s *Store) Peek(present []chains.Member) []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	nums, _ := s.keptLocked(present)
	return nums
}

// Assign returns one number per present member, in order: the one it kept,
// else the lowest free one, else 0 past 9. A member not present loses its
// number. A failed save still returns the numbers.
func (s *Store) Assign(present []chains.Member) ([]int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	nums, used := s.keptLocked(present)
	var next []entry
	free := 1
	for i, m := range present {
		if nums[i] != 0 {
			next = append(next, entry{Member: m, Number: nums[i]})
			continue
		}
		for used[free] {
			free++
		}
		if free > most {
			continue // a kept number further on still goes into next
		}
		nums[i] = free
		used[free] = true
		next = append(next, entry{Member: m, Number: free})
	}
	slices.SortFunc(next, func(a, b entry) int { return a.Number - b.Number })
	if slices.Equal(next, s.got) {
		return nums, nil
	}
	s.got = next
	return nums, s.save()
}

func (s *Store) save() error {
	if s.dir == "" {
		return nil
	}
	b, err := json.MarshalIndent(s.got, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, ".numbers-*.json")
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
	return os.Rename(tmp.Name(), s.path())
}
