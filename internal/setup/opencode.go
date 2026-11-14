package setup

import (
	"bytes"
	_ "embed"
	"os"
	"path/filepath"

	"github.com/blox-eng/matchblox/internal/doors"
)

// opencodePlugin is the plugin by which OpenCode tells matchblox its state:
// it writes <state dir>/matchblox/opencode/<pid>.json, which the sampler
// reads.
//
//go:embed matchblox.ts
var opencodePlugin []byte

// opencodeConfigDir is OpenCode's global config folder:
// $XDG_CONFIG_HOME/opencode, else ~/.config/opencode.
func opencodeConfigDir(e Env) string {
	dir := e.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(e.Home, ".config")
	}
	return filepath.Join(dir, "opencode")
}

// opencodePluginPath is in the folder whose plugins every OpenCode loads.
func opencodePluginPath(e Env) string {
	return filepath.Join(opencodeConfigDir(e), "plugins", "matchblox.ts")
}

func usesOpencode(e Env) bool {
	if _, err := e.LookPath("opencode"); err == nil {
		return true
	}
	_, err := os.Stat(opencodeConfigDir(e))
	return err == nil
}

// opencodeDoor offers the plugin. It is done while the file is the plugin
// of this matchblox; an older one shows as a diff.
func opencodeDoor(e Env) doors.Door {
	path := opencodePluginPath(e)
	d := doors.Door{ID: doors.OpencodePlugin, Title: "Add the OpenCode plugin", Path: path,
		Why: "OpenCode tells matchblox when an agent works or needs you. Without the plugin the board reads its pane. Delete the file to stop it."}
	_, old, _, err := read(path)
	if err != nil {
		d.Problem = err.Error()
		return d
	}
	if bytes.Equal(old, opencodePlugin) {
		d.Done = true
		return d
	}
	d.Sum, d.Preview = Sum(old), diff(string(old), string(opencodePlugin))
	return d
}

// AddOpencodePlugin writes the plugin, over an older one after a backup.
// A file changed after the preview is not overwritten: the sums differ.
func AddOpencodePlugin(path, seen string) (backup string, err error) {
	return edit(path, seen, func([]byte) ([]byte, error) { return opencodePlugin, nil })
}
