package setup

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blox-eng/matchblox/internal/doors"
)

const (
	label       = "com.blox.matchblox"
	plistName   = label + ".plist"
	serviceName = "matchblox.service"
)

// unit is what keeps `matchblox serve` running on this machine: the file,
// its text, and the command that starts it.
type unit struct {
	path, text string
	argv       []string
	advice     string // a line for the preview, "" for none
	// before runs first and its error is ignored (an agent not loaded yet).
	before []string
	check  []string // exits 0 when the unit's serve runs
}

// unitFor is the user unit for e.GOOS: a launchd agent on macOS, a systemd
// user unit on Linux. ok is false on any other system.
func unitFor(e Env) (u unit, ok bool) {
	exe := e.Exe
	if exe == "" {
		exe = "matchblox"
		if e.LookPath != nil {
			if p, err := e.LookPath(exe); err == nil {
				exe = p
			}
		}
	}
	switch e.GOOS {
	case "darwin":
		path := filepath.Join(e.Home, "Library", "LaunchAgents", plistName)
		domain := "gui/" + strconv.Itoa(e.UID)
		job := domain + "/" + label
		u := unit{path: path, text: plist(exe, filepath.Join(e.StateDir, "serve.log"), e.Getenv("PATH")),
			before: []string{"launchctl", "bootout", job},
			argv:   []string{"launchctl", "bootstrap", domain, path},
			check:  []string{"sh", "-c", `launchctl print "$1" | grep -q "state = running"`, "sh", job}}
		if e.Getenv("XPC_SERVICE_NAME") == label {
			// This serve is the job: its own bootout would stop it before the
			// bootstrap. A shell runs both in a process group of its own, which
			// the bootout leaves running.
			u.before = nil
			u.argv = []string{"sh", "-c", `launchctl bootout "$1"; launchctl bootstrap "$2" "$3"`, "sh", job, domain, path}
		}
		return u, true
	case "linux":
		dir := e.Getenv("XDG_CONFIG_HOME")
		if dir == "" {
			dir = filepath.Join(e.Home, ".config")
		}
		return unit{path: filepath.Join(dir, "systemd", "user", serviceName), text: service(exe, e.Getenv("PATH")),
			argv:   []string{"systemctl", "--user", "enable", "--now", serviceName},
			check:  []string{"systemctl", "--user", "is-active", "--quiet", serviceName},
			advice: "# to keep it running after you log out: loginctl enable-linger"}, true
	}
	return unit{}, false
}

// The unit sets PATH to the one of the process that opens the door: launchd
// and systemd start with a minimal one, and serve runs tmux and claude.
// Both start serve again only after a nonzero exit (the binary was
// replaced, or the socket removed). MATCHBLOX_UNIT tells serve it is the
// unit's: when a console's serve holds the socket, it waits to take over
// instead of leaving, so the unit stays up.
func plist(exe, log, path string) string {
	esc := func(s string) string {
		var b bytes.Buffer
		_ = xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	env := "\t<key>EnvironmentVariables</key>\n\t<dict>\n"
	if path != "" {
		env += "\t\t<key>PATH</key>\n\t\t<string>" + esc(path) + "</string>\n"
	}
	env += "\t\t<key>MATCHBLOX_UNIT</key>\n\t\t<string>1</string>\n\t</dict>\n"
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.blox.matchblox</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + esc(exe) + `</string>
		<string>serve</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
` + env + `	<key>StandardErrorPath</key>
	<string>` + esc(log) + `</string>
</dict>
</plist>
`
}

func service(exe, path string) string {
	// systemd splits ExecStart on spaces: quote a path that has one.
	if strings.ContainsAny(exe, " \t") {
		exe = `"` + strings.ReplaceAll(exe, `"`, `\"`) + `"`
	}
	exe = strings.ReplaceAll(exe, "%", "%%") // systemd specifiers
	env := ""
	if path != "" {
		env = "Environment=\"PATH=" + strings.NewReplacer("%", "%%", `"`, `\"`).Replace(path) + "\"\n"
	}
	env += "Environment=MATCHBLOX_UNIT=1\n"
	// The start limit ends a loop of failed starts (the binary removed, the
	// unit kept): 5 within the minute and systemd stops trying.
	return `[Unit]
Description=matchblox serve: the phone buzzes with no console open
StartLimitIntervalSec=60
StartLimitBurst=5

[Service]
ExecStart=` + exe + ` serve
Restart=on-failure
RestartSec=5
` + env + `
[Install]
WantedBy=default.target
`
}

// keepRunningDoor writes the user unit and starts it, so the phone buzzes
// with no console open. It is done when the file holds this very text and
// the unit runs. The zero Door (no ID): this system has no user units.
func keepRunningDoor(e Env) doors.Door {
	u, ok := unitFor(e)
	if !ok {
		return doors.Door{}
	}
	d := doors.Door{ID: doors.KeepRunning, Title: "Keep matchblox running", Path: u.path, Term: u.argv,
		Why: "the phone buzzes with no console open"}
	_, old, _, err := read(u.path)
	if err != nil {
		d.Problem = err.Error()
		return d
	}
	d.Preview = diff(string(old), u.text)
	if string(old) == u.text {
		if e.unitRuns(u.check) {
			d.Done = true
			return d
		}
		d.Preview = "the unit is in place, but its serve does not run"
	}
	d.Sum = Sum(old)
	d.Preview += "\n\nthen runs: " + shellJoin(u.argv)
	if u.advice != "" {
		d.Preview += "\n" + u.advice
	}
	return d
}

// openKeepRunning writes the unit when the file is still what the person
// saw, then runs the command that starts it. A refused write runs nothing.
func openKeepRunning(e Env, seen string) error {
	defer e.Running.forget()
	u, ok := unitFor(e)
	if !ok {
		return fmt.Errorf("no user unit on %s", e.GOOS)
	}
	target, old, _, err := read(u.path)
	if err != nil {
		return err
	}
	if _, err := edit(u.path, seen, func([]byte) ([]byte, error) { return []byte(u.text), nil }); err != nil {
		return err
	}
	if u.before != nil {
		_ = e.run(u.before) // a stale agent leaves, so the new file is the one that loads
	}
	if err := e.run(u.argv); err != nil {
		// The service did not start: put the file back, so the door stays open.
		if old == nil {
			_ = os.Remove(target)
		} else {
			_ = replace(target, old, 0o600)
		}
		return err
	}
	return nil
}

// run runs a command of the unit with e.Run, else on this machine in a
// process group of its own: a bootout of the job that runs it stops the
// job's group only.
func (e Env) run(argv []string) error {
	if e.Run != nil {
		return e.Run(argv)
	}
	c := exec.Command(argv[0], argv[1:]...) //nolint:gosec // the unit's own commands, from unitFor
	ownGroup(c)
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", shellJoin(argv), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Running keeps the last check of the unit for a while: the service reads
// the doors at every sample, and systemctl or launchctl need not run as
// often. Opening the door forgets it.
type Running struct {
	mu   sync.Mutex
	at   time.Time
	runs bool
}

const runningFor = 30 * time.Second

// unitRuns says whether the unit's serve runs, from e.Running when it was
// checked within runningFor.
func (e Env) unitRuns(check []string) bool {
	r := e.Running
	if r == nil {
		return e.run(check) == nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if now := time.Now(); r.at.IsZero() || now.Sub(r.at) >= runningFor {
		r.at, r.runs = now, e.run(check) == nil
	}
	return r.runs
}

func (r *Running) forget() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.at = time.Time{}
	r.mu.Unlock()
}
