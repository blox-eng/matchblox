package limits

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeAuth(t *testing.T, path, token string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"accessToken":"` + token + `","refreshToken":"ref"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestDefaultCursorSourcesLinuxFilesOnly: on linux the default list reads
// ~/.config/cursor/auth.json and never calls Keychain.
func TestDefaultCursorSourcesLinuxFilesOnly(t *testing.T) {
	home := t.TempDir()
	writeAuth(t, filepath.Join(home, ".config", "cursor", "auth.json"), "acc")
	kc := TokenSource(func(context.Context) (string, error) {
		t.Fatal("keychain called on linux")
		return "", nil
	})
	got, err := TryTokenSources(context.Background(), DefaultCursorSources(home, "linux", kc))
	if err != nil || got != "acc" {
		t.Fatalf("token %q, %v, want acc", got, err)
	}
}

// TestDefaultCursorSourcesPrefersConfigOverDotCursor: config path wins when
// both auth files exist.
func TestDefaultCursorSourcesPrefersConfigOverDotCursor(t *testing.T) {
	home := t.TempDir()
	writeAuth(t, filepath.Join(home, ".config", "cursor", "auth.json"), "from-config")
	writeAuth(t, filepath.Join(home, ".cursor", "auth.json"), "from-dot")
	got, err := TryTokenSources(context.Background(), DefaultCursorSources(home, "linux", nil))
	if err != nil || got != "from-config" {
		t.Fatalf("token %q, %v, want from-config", got, err)
	}
}

// TestDefaultCursorSourcesDarwinFallsBackToKeychain: with no auth files,
// darwin uses Keychain.
func TestDefaultCursorSourcesDarwinFallsBackToKeychain(t *testing.T) {
	home := t.TempDir()
	kc := TokenSource(func(context.Context) (string, error) { return "from-kc", nil })
	got, err := TryTokenSources(context.Background(), DefaultCursorSources(home, "darwin", kc))
	if err != nil || got != "from-kc" {
		t.Fatalf("token %q, %v, want from-kc", got, err)
	}
}

// TestDefaultCursorSourcesFilesBeforeKeychain: a file wins over Keychain on
// darwin; Keychain is not called.
func TestDefaultCursorSourcesFilesBeforeKeychain(t *testing.T) {
	home := t.TempDir()
	writeAuth(t, filepath.Join(home, ".cursor", "auth.json"), "from-file")
	called := false
	kc := TokenSource(func(context.Context) (string, error) {
		called = true
		return "from-kc", nil
	})
	got, err := TryTokenSources(context.Background(), DefaultCursorSources(home, "darwin", kc))
	if err != nil || got != "from-file" {
		t.Fatalf("token %q, %v, want from-file", got, err)
	}
	if called {
		t.Fatal("keychain called when a file already supplied a token")
	}
}

// TestDefaultCursorSourcesDarwinNilKeychain: nil Keychain is not appended;
// missing files yield ErrNotExist.
func TestDefaultCursorSourcesDarwinNilKeychain(t *testing.T) {
	home := t.TempDir()
	_, err := TryTokenSources(context.Background(), DefaultCursorSources(home, "darwin", nil))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err %v, want os.ErrNotExist", err)
	}
}

// TestDefaultCursorSourcesEmptyAccessTokenIsMiss: empty accessToken is a miss.
func TestDefaultCursorSourcesEmptyAccessTokenIsMiss(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".config", "cursor", "auth.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"accessToken":""}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := TryTokenSources(context.Background(), DefaultCursorSources(home, "linux", nil))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err %v, want os.ErrNotExist", err)
	}
}

// TestDefaultCursorSourcesXDGConfigHome: XDG_CONFIG_HOME wins over ~/.config.
func TestDefaultCursorSourcesXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	writeAuth(t, filepath.Join(home, ".config", "cursor", "auth.json"), "from-home-config")
	writeAuth(t, filepath.Join(xdg, "cursor", "auth.json"), "from-xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	got, err := TryTokenSources(context.Background(), DefaultCursorSources(home, "linux", nil))
	if err != nil || got != "from-xdg" {
		t.Fatalf("token %q, %v, want from-xdg", got, err)
	}
}

// TestAccountCursorSourceOnlyUnderFolder: account sources never read the
// host ~/.config file.
func TestAccountCursorSourceOnlyUnderFolder(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	work := filepath.Join(root, "work")
	writeAuth(t, filepath.Join(home, ".config", "cursor", "auth.json"), "host-token")
	writeAuth(t, filepath.Join(work, "home", ".cursor", "auth.json"), "acct-token")
	got, err := TryTokenSources(context.Background(), AccountCursorSources(work, FileToken))
	if err != nil || got != "acct-token" {
		t.Fatalf("token %q, %v, want acct-token", got, err)
	}
}
