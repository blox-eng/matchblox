package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/blox-eng/matchblox/internal/gitscan"
)

// Consent is the builder's answer to finding the repositories on the host.
// Kept apart from the cache: a lost cache never asks again.
type Consent struct {
	Answer string    `json:"answer"`          // "yes" or "no"; "": never asked
	Roots  []string  `json:"roots,omitempty"` // picked; empty: the defaults
	At     time.Time `json:"at"`
}

// ScanPath holds the Consent, next to state.json.
func ScanPath() string { return filepath.Join(filepath.Dir(Path()), "scan.json") }

// ReposPath holds the last scan's Found, next to state.json.
func ReposPath() string { return filepath.Join(filepath.Dir(Path()), "repos.json") }

// ReadConsent is the zero Consent, never asked, for a missing or broken file.
func ReadConsent(path string) Consent {
	var c Consent
	if readJSON(path, &c) != nil {
		return Consent{}
	}
	return c
}

func WriteConsent(path string, c Consent) error { return writeJSON(path, c) }

// ReadFound is empty for a missing or broken file.
func ReadFound(path string) gitscan.Found {
	var f gitscan.Found
	if readJSON(path, &f) != nil {
		return gitscan.Found{}
	}
	return f
}

func WriteFound(path string, f gitscan.Found) error { return writeJSON(path, f) }

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
