package stoker

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/blox-eng/matchblox/internal/sample"
)

// HostID is the id of the host stoker, the one that has always run.
const HostID = ""

const (
	familyDirName = "stokers"
	linkName      = "link.json"
	chainPrefix   = sample.StokerSession + "-"
)

// FamilyDir is the store of a stoker: the state dir for the host stoker,
// stokers/<id> in it for a chain stoker.
func FamilyDir(state, id string) string {
	if id == HostID {
		return state
	}
	return filepath.Join(state, familyDirName, id)
}

// FamilyIDs are the ids of the chain stokers, each a stokers/ dir that
// holds a link.json, sorted by number. A missing stokers/ is no ids.
func FamilyIDs(state string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(state, familyDirName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(state, familyDirName, e.Name(), linkName)); err == nil {
			ids = append(ids, e.Name())
		}
	}
	slices.SortFunc(ids, func(a, b string) int {
		if d := idNumber(a) - idNumber(b); d != 0 {
			return d
		}
		return strings.Compare(a, b)
	})
	return ids, nil
}

// NextID is the id after the highest of ids: s1 for none.
func NextID(ids []string) string {
	top := 0
	for _, id := range ids {
		top = max(top, idNumber(id))
	}
	return "s" + strconv.Itoa(top+1)
}

// idNumber is the number of an id like s12; 0 for any other.
func idNumber(id string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(id, "s"))
	if err != nil || !strings.HasPrefix(id, "s") || n < 0 {
		return 0
	}
	return n
}

// SessionName is the tmux session of a stoker.
func SessionName(id string) string {
	if id == HostID {
		return sample.StokerSession
	}
	return chainPrefix + id
}

// IDOfSession is the id of the stoker that runs the tmux session name. It
// is false for any other session.
func IDOfSession(name string) (id string, ok bool) {
	if name == sample.StokerSession {
		return HostID, true
	}
	rest, found := strings.CutPrefix(name, chainPrefix)
	if !found || idNumber(rest) == 0 {
		return "", false
	}
	return rest, true
}

// Link ties a chain stoker to its chain: the stoker's id, the chain's name,
// who gave the stoker its goes ("person" or "host") and the goes it holds.
// Ended: its chain ended or joined another, and the stoker stopped for
// good; a new chain of the same id is not its.
type Link struct {
	ID     string   `json:"id"`
	Chain  string   `json:"chain"`
	Giver  string   `json:"giver"`
	Grants []string `json:"grants"`
	Ended  bool     `json:"ended,omitempty"`
}

// ReadLink reads link.json. A missing file is fs.ErrNotExist.
func ReadLink(dir string) (Link, error) {
	var l Link
	b, err := os.ReadFile(filepath.Join(dir, linkName)) //nolint:gosec // our own state dir
	if err != nil {
		return l, err
	}
	return l, json.Unmarshal(b, &l)
}

// WriteLink replaces link.json atomically.
func WriteLink(dir string, l Link) error {
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, linkName+".*")
	if err != nil {
		return err
	}
	_, werr := f.Write(b)
	if err := errors.Join(werr, f.Close()); err != nil {
		_ = os.Remove(f.Name())
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, linkName))
}
