package setup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/doors"
)

// opencodeEnv is a machine with OpenCode on the PATH.
func opencodeEnv(t *testing.T) Env {
	e := env(t)
	look := e.LookPath
	e.LookPath = func(name string) (string, error) {
		if name == "opencode" {
			return "/usr/bin/opencode", nil
		}
		return look(name)
	}
	return e
}

// TestSetupInstallsTheOpencodePluginOnce: the door writes the plugin into
// OpenCode's global plugin folder, as it is in this binary; a second run
// finds it done and writes nothing.
func TestSetupInstallsTheOpencodePluginOnce(t *testing.T) {
	e := opencodeEnv(t)
	path := filepath.Join(e.Home, ".config", "opencode", "plugins", "matchblox.ts")
	d := door(t, e, doors.OpencodePlugin)
	if !d.Open() || d.Path != path || !strings.Contains(d.Preview, "+export const MatchbloxPlugin") {
		t.Fatalf("door = %+v", d)
	}
	if backup, err := Open(e, doors.OpencodePlugin, d.Sum, false); err != nil || backup != "" {
		t.Fatalf("open: backup %q, %v", backup, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(opencodePlugin) {
		t.Fatalf("file = %q, %v", got, err)
	}
	again := door(t, e, doors.OpencodePlugin)
	if !again.Done || again.Preview != "" {
		t.Fatalf("second run: %+v, want done with no change", again)
	}
	if ents, _ := os.ReadDir(filepath.Dir(path)); len(ents) != 1 {
		t.Fatalf("the plugin folder has %d files, want 1", len(ents))
	}
	// A file changed after the preview is not overwritten.
	if err := os.WriteFile(path, []byte("// mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(e, doors.OpencodePlugin, d.Sum, false); !errors.Is(err, ErrChanged) {
		t.Fatalf("err = %v, want ErrChanged", err)
	}
}

// TestNoOpencodeNoPluginDoor: a machine without OpenCode gets no door for it.
func TestNoOpencodeNoPluginDoor(t *testing.T) {
	for _, d := range Doors(env(t)) {
		if d.ID == doors.OpencodePlugin {
			t.Fatalf("door %+v on a machine without OpenCode", d)
		}
	}
}
