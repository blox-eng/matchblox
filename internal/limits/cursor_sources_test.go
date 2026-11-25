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

// TestTryTokenSourcesHardErrorStopsList: a non-ErrNotExist from an earlier
// source is returned at once; later sources are not tried.
func TestTryTokenSourcesHardErrorStopsList(t *testing.T) {
	called := false
	_, err := TryTokenSources(context.Background(), []TokenSource{
		func(context.Context) (string, error) { return "", errors.New("permission denied") },
		func(context.Context) (string, error) {
			called = true
			return "from-next", nil
		},
	})
	if err == nil || err.Error() != "permission denied" {
		t.Fatalf("err %v, want permission denied", err)
	}
	if called {
		t.Fatal("later source called after a hard error")
	}
}

// TestDefaultCursorSourcesEmptyHomeNoRelativeFiles: with no home and no
// absolute XDG_CONFIG_HOME, no file sources are added (CWD is never searched).
func TestDefaultCursorSourcesEmptyHomeNoRelativeFiles(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	srcs := DefaultCursorSources("", "linux", nil)
	if len(srcs) != 0 {
		t.Fatalf("sources %d, want 0", len(srcs))
	}
	kc := TokenSource(func(context.Context) (string, error) { return "from-kc", nil })
	srcs = DefaultCursorSources("", "darwin", kc)
	if len(srcs) != 1 {
		t.Fatalf("darwin sources %d, want 1 (keychain only)", len(srcs))
	}
	got, err := TryTokenSources(context.Background(), srcs)
	if err != nil || got != "from-kc" {
		t.Fatalf("token %q, %v, want from-kc", got, err)
	}
}

// TestDefaultCursorSourcesRelativeXDGIgnored: a relative XDG_CONFIG_HOME is
// ignored so auth.json under CWD is never opened.
func TestDefaultCursorSourcesRelativeXDGIgnored(t *testing.T) {
	home := t.TempDir()
	writeAuth(t, filepath.Join(home, ".config", "cursor", "auth.json"), "from-home")
	t.Setenv("XDG_CONFIG_HOME", "relative-xdg")
	got, err := TryTokenSources(context.Background(), DefaultCursorSources(home, "linux", nil))
	if err != nil || got != "from-home" {
		t.Fatalf("token %q, %v, want from-home", got, err)
	}
}

// TestDefaultCursorSourcesEmptyAccessTokenContinues: empty accessToken in the
// first file is a miss; the next source supplies the token.
func TestDefaultCursorSourcesEmptyAccessTokenContinues(t *testing.T) {
	home := t.TempDir()
	empty := filepath.Join(home, ".config", "cursor", "auth.json")
	if err := os.MkdirAll(filepath.Dir(empty), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(empty, []byte(`{"accessToken":""}`), 0o600); err != nil {
		t.Fatal(err)
	}
	writeAuth(t, filepath.Join(home, ".cursor", "auth.json"), "from-dot")
	got, err := TryTokenSources(context.Background(), DefaultCursorSources(home, "linux", nil))
	if err != nil || got != "from-dot" {
		t.Fatalf("token %q, %v, want from-dot", got, err)
	}
}
