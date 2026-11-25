package limits

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// TokenSource reads one place for a Cursor access token. A missing login is
// an error that is os.ErrNotExist, or an empty token.
type TokenSource func(context.Context) (string, error)

// DefaultCursorSources is the ordered list for the default Cursor login
// (dir ""). Files come first; on darwin, keychain is appended only when
// non-nil. Never appends a nil source.
func DefaultCursorSources(home, goos string, keychain TokenSource) []TokenSource {
	configDir := filepath.Join(home, ".config")
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		configDir = xdg
	}
	out := []TokenSource{
		fileSource(filepath.Join(configDir, "cursor", "auth.json")),
		fileSource(filepath.Join(home, ".cursor", "auth.json")),
	}
	if goos == "darwin" && keychain != nil {
		out = append(out, keychain)
	}
	return out
}

// AccountCursorSources is the ordered list for a Cursor account folder: only
// home/.cursor/auth.json under that folder. Never host ~/.config or Keychain.
func AccountCursorSources(folder string, file func(path string) (string, error)) []TokenSource {
	if file == nil {
		return nil
	}
	path := filepath.Join(folder, "home", ".cursor", "auth.json")
	return []TokenSource{func(context.Context) (string, error) { return file(path) }}
}

func fileSource(path string) TokenSource {
	return func(context.Context) (string, error) { return FileToken(path) }
}

// TryTokenSources tries each source in order. os.ErrNotExist or an empty
// token moves to the next. The first other error is returned. When every
// source misses, the error is os.ErrNotExist.
func TryTokenSources(ctx context.Context, sources []TokenSource) (string, error) {
	var last error
	for _, src := range sources {
		if src == nil {
			continue
		}
		tok, err := src(ctx)
		if err == nil && tok != "" {
			return tok, nil
		}
		if err == nil || errors.Is(err, os.ErrNotExist) {
			last = os.ErrNotExist
			continue
		}
		return "", err
	}
	if last == nil {
		last = os.ErrNotExist
	}
	return "", last
}
