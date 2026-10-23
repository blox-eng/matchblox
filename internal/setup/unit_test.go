package setup

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/doors"
)

// unitEnv is env for goos with a fake runner: nothing here calls launchctl
// or systemctl, and every file is under a temp dir.
func unitEnv(t *testing.T, goos string) (Env, *[][]string) {
	t.Helper()
	e := env(t)
	e.GOOS, e.UID = goos, 501
	ran := new([][]string)
	e.Run = func(argv []string) error { *ran = append(*ran, argv); return nil }
	return e, ran
}

func TestTheKeepRunningDoorWritesAPlistOnMacOS(t *testing.T) {
	e, ran := unitEnv(t, "darwin")
	d := door(t, e, doors.KeepRunning)
	plist := filepath.Join(e.Home, "Library", "LaunchAgents", "com.blox.matchblox.plist")
	if !d.Open() || d.Title != "Keep matchblox running" || d.Why != "the phone buzzes with no console open" || d.Path != plist {
		t.Fatalf("door = %+v", d)
	}
	if _, err := Open(e, doors.KeepRunning, d.Sum, false); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(plist)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<string>com.blox.matchblox</string>",
		"<key>ProgramArguments</key>\n\t<array>\n\t\t<string>" + exe + "</string>\n\t\t<string>serve</string>\n\t</array>",
		"<key>RunAtLoad</key>\n\t<true/>",
		// Exit 0 is "the service already runs": only a nonzero exit (the binary
		// was replaced, or the socket removed) is started again.
		"<key>KeepAlive</key>\n\t<dict>\n\t\t<key>SuccessfulExit</key>\n\t\t<false/>\n\t</dict>",
		"<key>StandardErrorPath</key>\n\t<string>" + filepath.Join(e.StateDir, "serve.log") + "</string>",
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("plist lacks %q:\n%s", want, b)
		}
	}
	if want := [][]string{{"launchctl", "bootout", "gui/501/com.blox.matchblox"}, {"launchctl", "bootstrap", "gui/501", plist}}; !slices.EqualFunc(*ran, want, slices.Equal) {
		t.Fatalf("ran = %q, want %q", *ran, want)
	}
}

func TestTheKeepRunningDoorWritesAUnitOnLinux(t *testing.T) {
	e, ran := unitEnv(t, "linux")
	d := door(t, e, doors.KeepRunning)
	unit := filepath.Join(e.Home, ".config", "systemd", "user", "matchblox.service")
	if !d.Open() || d.Path != unit {
		t.Fatalf("door = %+v", d)
	}
	if !strings.Contains(d.Preview, "loginctl enable-linger") {
		t.Fatalf("no linger advice in the preview:\n%s", d.Preview)
	}
	if _, err := Open(e, doors.KeepRunning, d.Sum, false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(unit)
	for _, want := range []string{"ExecStart=" + exe + " serve\n", "Restart=on-failure\n", "RestartSec=5\n", "StartLimitIntervalSec=60\n", "StartLimitBurst=5\n", "WantedBy=default.target\n"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("unit lacks %q:\n%s", want, b)
		}
	}
	if want := [][]string{{"systemctl", "--user", "enable", "--now", "matchblox.service"}}; !slices.EqualFunc(*ran, want, slices.Equal) {
		t.Fatalf("ran = %q, want %q", *ran, want)
	}
}

func TestTheDoorIsDoneWhenTheUnitIsThere(t *testing.T) {
	for _, goos := range []string{"darwin", "linux"} {
		e, _ := unitEnv(t, goos)
		d := door(t, e, doors.KeepRunning)
		if _, err := Open(e, doors.KeepRunning, d.Sum, false); err != nil {
			t.Fatal(err)
		}
		if d := door(t, e, doors.KeepRunning); !d.Done {
			t.Fatalf("%s: door after = %+v", goos, d)
		}
		// A unit that holds other content is not done, and the preview shows the change.
		if err := os.WriteFile(door(t, e, doors.KeepRunning).Path, []byte("other\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if d := door(t, e, doors.KeepRunning); d.Done || !strings.Contains(d.Preview, "-other") {
			t.Fatalf("%s: door with other content = %+v", goos, d)
		}
	}
}

func TestTheDoorRefusesAFileThatChangedAfterThePreview(t *testing.T) {
	e, ran := unitEnv(t, "linux")
	d := door(t, e, doors.KeepRunning)
	if err := os.MkdirAll(filepath.Dir(d.Path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d.Path, []byte("mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(e, doors.KeepRunning, d.Sum, false); !errors.Is(err, ErrChanged) || len(*ran) != 0 {
		t.Fatalf("err = %v, ran = %q", err, *ran)
	}
}

func TestTheDoorShowsItsCommand(t *testing.T) {
	e, _ := unitEnv(t, "darwin")
	d := door(t, e, doors.KeepRunning)
	plist := d.Path
	if want := []string{"launchctl", "bootstrap", "gui/501", plist}; !slices.Equal(d.Term, want) || !strings.Contains(d.Preview, "launchctl bootstrap gui/501 "+plist) {
		t.Fatalf("darwin Term = %q, Preview:\n%s", d.Term, d.Preview)
	}
	e, _ = unitEnv(t, "linux")
	d = door(t, e, doors.KeepRunning)
	if want := []string{"systemctl", "--user", "enable", "--now", "matchblox.service"}; !slices.Equal(d.Term, want) || !strings.Contains(d.Preview, "systemctl --user enable --now matchblox.service") {
		t.Fatalf("linux Term = %q, Preview:\n%s", d.Term, d.Preview)
	}
}

func TestNoKeepRunningDoorOnOtherSystems(t *testing.T) {
	e, _ := unitEnv(t, "windows")
	for _, d := range Doors(e) {
		if d.ID == doors.KeepRunning {
			t.Fatalf("door on windows: %+v", d)
		}
	}
}

func TestAFailedStartLeavesTheDoorOpen(t *testing.T) {
	for _, goos := range []string{"darwin", "linux"} {
		e, _ := unitEnv(t, goos)
		e.Run = func(argv []string) error {
			if argv[1] == "bootout" {
				return errors.New("not loaded") // ignored
			}
			return errors.New("already loaded")
		}
		d := door(t, e, doors.KeepRunning)
		if _, err := Open(e, doors.KeepRunning, d.Sum, false); err == nil || !strings.Contains(err.Error(), "already loaded") {
			t.Fatalf("%s: err = %v", goos, err)
		}
		if d := door(t, e, doors.KeepRunning); d.Done || !d.Open() {
			t.Fatalf("%s: door after a failed start = %+v", goos, d)
		}
		// An older unit comes back as it was.
		mine := []byte("old unit\n")
		if err := os.MkdirAll(filepath.Dir(d.Path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(d.Path, mine, 0o600); err != nil {
			t.Fatal(err)
		}
		d = door(t, e, doors.KeepRunning)
		if _, err := Open(e, doors.KeepRunning, d.Sum, false); err == nil {
			t.Fatalf("%s: no error", goos)
		}
		if got, _ := os.ReadFile(d.Path); string(got) != string(mine) {
			t.Fatalf("%s: file = %q, want the old unit back", goos, got)
		}
	}
}

func TestAnUpdateReplacesTheLoadedAgent(t *testing.T) {
	e, ran := unitEnv(t, "darwin")
	e.Exe = "/old/matchblox"
	d := door(t, e, doors.KeepRunning)
	if _, err := Open(e, doors.KeepRunning, d.Sum, false); err != nil {
		t.Fatal(err)
	}
	*ran = nil
	e.Exe = exe
	d = door(t, e, doors.KeepRunning)
	if !d.Open() || !strings.Contains(d.Preview, "-\t\t<string>/old/matchblox</string>") {
		t.Fatalf("door = %+v", d)
	}
	if _, err := Open(e, doors.KeepRunning, d.Sum, false); err != nil {
		t.Fatal(err)
	}
	if len(*ran) != 2 || (*ran)[0][1] != "bootout" || (*ran)[1][1] != "bootstrap" {
		t.Fatalf("ran = %q", *ran)
	}
	if b, _ := os.ReadFile(d.Path); !strings.Contains(string(b), exe) || strings.Contains(string(b), "/old/") {
		t.Fatalf("plist:\n%s", b)
	}
}

func TestTheUnitCarriesThePATHOfTheProcessThatOpensIt(t *testing.T) {
	const path = "/opt/homebrew/bin:/usr/bin:/odd%dir"
	for goos, want := range map[string]string{
		"darwin": "<key>EnvironmentVariables</key>\n\t<dict>\n\t\t<key>PATH</key>\n\t\t<string>" + path + "</string>",
		"linux":  "Environment=\"PATH=/opt/homebrew/bin:/usr/bin:/odd%%dir\"\n",
	} {
		e, _ := unitEnv(t, goos)
		e.Getenv = func(k string) string {
			if k == "PATH" {
				return path
			}
			return ""
		}
		d := door(t, e, doors.KeepRunning)
		if _, err := Open(e, doors.KeepRunning, d.Sum, false); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(d.Path); !strings.Contains(string(b), want) {
			t.Errorf("%s lacks %q:\n%s", goos, want, b)
		}
	}
}

func TestAPercentInTheExeIsEscapedForSystemd(t *testing.T) {
	if got := service("/a%b/matchblox", ""); !strings.Contains(got, "ExecStart=/a%%b/matchblox serve\n") {
		t.Fatal(got)
	}
}

// The unit's serve is told it is the unit's: it waits for a socket another
// serve holds, instead of leaving with exit 0.
func TestTheUnitTellsServeItIsTheUnits(t *testing.T) {
	for goos, want := range map[string]string{
		"darwin": "<key>EnvironmentVariables</key>\n\t<dict>\n\t\t<key>MATCHBLOX_UNIT</key>\n\t\t<string>1</string>\n\t</dict>",
		"linux":  "Environment=MATCHBLOX_UNIT=1\n",
	} {
		e, _ := unitEnv(t, goos)
		d := door(t, e, doors.KeepRunning)
		if _, err := Open(e, doors.KeepRunning, d.Sum, false); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(d.Path); !strings.Contains(string(b), want) {
			t.Errorf("%s lacks %q:\n%s", goos, want, b)
		}
	}
}

// A unit written but not running (its serve left) leaves the door open: it
// is done when the unit runs, as systemctl or launchctl tell.
func TestTheDoorIsDoneOnlyWhenTheUnitRuns(t *testing.T) {
	for goos, check := range map[string][]string{
		"linux":  {"systemctl", "--user", "is-active", "--quiet", "matchblox.service"},
		"darwin": {"sh", "-c", `launchctl print "$1" | grep -q "state = running"`, "sh", "gui/501/com.blox.matchblox"},
	} {
		e, _ := unitEnv(t, goos)
		d := door(t, e, doors.KeepRunning)
		if _, err := Open(e, doors.KeepRunning, d.Sum, false); err != nil {
			t.Fatal(err)
		}
		var asked [][]string
		e.Run = func(argv []string) error {
			asked = append(asked, argv)
			return errors.New("inactive")
		}
		d = door(t, e, doors.KeepRunning)
		if d.Done || !d.Open() || !strings.Contains(d.Preview, "does not run") {
			t.Fatalf("%s: door of a unit that does not run = %+v", goos, d)
		}
		if len(asked) != 1 || !slices.Equal(asked[0], check) {
			t.Fatalf("%s: asked %q, want %q", goos, asked, check)
		}
	}
}

// The service reads the doors at every sample: the unit's check runs once
// in a while, not each time, and again at once after the door opens.
func TestTheUnitIsCheckedOnceInAWhile(t *testing.T) {
	e, _ := unitEnv(t, "linux")
	e.Running = &Running{}
	checks := 0
	e.Run = func(argv []string) error {
		if slices.Contains(argv, "is-active") {
			checks++
		}
		return nil
	}
	d := door(t, e, doors.KeepRunning)
	if _, err := Open(e, doors.KeepRunning, d.Sum, false); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if d = door(t, e, doors.KeepRunning); !d.Done {
			t.Fatalf("door %+v", d)
		}
	}
	if checks != 1 {
		t.Fatalf("checked %d times for 3 reads", checks)
	}
	e.Running.at = e.Running.at.Add(-time.Minute)
	door(t, e, doors.KeepRunning)
	if checks != 2 {
		t.Fatalf("checked %d times: a minute later it did not check again", checks)
	}
}

// The serve of the launchd job opens the door: a bootout from it would end
// it before the bootstrap. Both run in a shell of their own process group,
// which the bootout does not stop.
func TestTheJobDoesNotBootItselfOut(t *testing.T) {
	e, ran := unitEnv(t, "darwin")
	e.Getenv = func(k string) string {
		if k == "XPC_SERVICE_NAME" {
			return "com.blox.matchblox"
		}
		return ""
	}
	d := door(t, e, doors.KeepRunning)
	if _, err := Open(e, doors.KeepRunning, d.Sum, false); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"sh", "-c", `launchctl bootout "$1"; launchctl bootstrap "$2" "$3"`,
		"sh", "gui/501/com.blox.matchblox", "gui/501", d.Path}}
	if !slices.EqualFunc(*ran, want, slices.Equal) {
		t.Fatalf("ran = %q, want %q", *ran, want)
	}
}
