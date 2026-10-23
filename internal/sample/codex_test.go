package sample

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/limits"
)

// daemonStart is when the Codex process of the daemon tests started.
var daemonStart = fixtureNow.Add(-time.Hour)

// daemonRollout is a rollout of Codex 0.163: the session meta names the
// working directory and the start, then one reading of the plan's limits.
func daemonRollout(cwd string, started time.Time, used int) string {
	at := started.UTC().Format(time.RFC3339Nano)
	return `{"timestamp":"` + at + `","ordinal":0,"type":"session_meta","payload":{"id":"01a0-made-up","timestamp":"` + at + `","cwd":"` + cwd + `","cli_version":"0.163.0"}}` + "\n" +
		codexLimits(started.Add(time.Minute), `"primary":{"used_percent":`+strconv.Itoa(used)+`,"window_minutes":300,"resets_at":`+itoa(fixtureNow.Add(3*time.Hour).Unix())+`},"secondary":null`)
}

// daemonFile writes a rollout under the sessions folder of a Codex home, in
// the folder of the day the session started.
func daemonFile(t *testing.T, home, name, cwd string, started time.Time, used int) string {
	t.Helper()
	return daemonFileIn(t, time.Local, home, name, cwd, started, used)
}

// daemonFileIn is daemonFile in the day folder of a zone.
func daemonFileIn(t *testing.T, loc *time.Location, home, name, cwd string, started time.Time, used int) string {
	t.Helper()
	path := filepath.Join(home, "sessions", started.In(loc).Format("2006/01/02"), "rollout-"+name+".jsonl")
	writeFile(t, path, daemonRollout(cwd, started, used))
	return path
}

// daemonSession is a Codex process in /work/app that started an hour before
// fixtureNow and holds no rollout.
func daemonSession(t *testing.T) (*Sampler, string) {
	t.Helper()
	return daemonSessionAged(t, time.Hour)
}

// daemonSessionAged is daemonSession for a process that started age before
// the clock of the sampler.
func daemonSessionAged(t *testing.T, age time.Duration) (*Sampler, string) {
	t.Helper()
	root := copyFixture(t)
	addProc(t, root, 900, 1, "codex", []string{"codex"}, "%20")
	addPane(t, root, "%20\tapp:1.1\tw\t900\tcodex\t/work/app")
	setEnv(t, root, 900, "TMUX_PANE=%20")
	symlink(t, "/work/app", filepath.Join(root, "proc", "900", "cwd"))
	stat := filepath.Join(root, "proc", "900", "stat")
	b, err := os.ReadFile(stat)
	if err != nil {
		t.Fatal(err)
	}
	// The machine has been up long enough to hold a process of this age.
	uptime := age.Seconds() + 3600
	writeFile(t, filepath.Join(root, "proc", "uptime"), strconv.FormatFloat(uptime, 'f', 1, 64)+" 50000.0\n")
	ticks := strconv.Itoa(int((uptime - age.Seconds()) * 100))
	if err := os.WriteFile(stat, []byte(strings.Replace(string(b), " 1000 1 1", " "+ticks+" 1 1", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	smp := newFixtureSampler(root)
	smp.Agents = []string{"codex"}
	return smp, root
}

// codexUsed is the used percent of the first window of the measured Codex
// limits in the snapshot.
func codexUsed(snap Snapshot) (float64, bool) {
	for _, a := range snap.Limits {
		if a.Provider == "openai" && a.State == limits.Measured {
			return a.Uses(fixtureNow)[0].Pct, true
		}
	}
	return 0, false
}

// TestCodexRolloutBeforeTheProcessIsNotItsOwn: a rollout of the same
// directory that began before the process is an earlier session's.
func TestCodexRolloutBeforeTheProcessIsNotItsOwn(t *testing.T) {
	smp, root := daemonSession(t)
	daemonFile(t, filepath.Join(root, "home", ".codex"), "early", "/work/app", daemonStart.Add(-10*time.Minute), 42)
	if pct, ok := codexUsed(smp.Sample()); ok {
		t.Fatalf("measured %v from a rollout older than the process", pct)
	}
}

// TestCodexHeldRolloutStillWins: a process that holds a rollout open reads
// that one, even when a newer rollout of its directory exists.
func TestCodexHeldRolloutStillWins(t *testing.T) {
	smp, root := daemonSession(t)
	codexHome := filepath.Join(root, "home", ".codex")
	held := daemonFile(t, codexHome, "held", "/work/app", daemonStart.Add(time.Minute), 11)
	daemonFile(t, codexHome, "newer", "/work/app", daemonStart.Add(5*time.Minute), 99)
	symlink(t, held, filepath.Join(root, "proc", "900", "fd", "21"))
	if pct, ok := codexUsed(smp.Sample()); !ok || pct != 11 {
		t.Fatalf("used %v (measured %v), want 11 from the held rollout", pct, ok)
	}
}

// TestCodexAccountRolloutStaysInItsHome: a session of an account never
// reads a rollout of the default home, only those of its own.
func TestCodexAccountRolloutStaysInItsHome(t *testing.T) {
	smp, root := daemonSession(t)
	work := filepath.Join(root, "home", ".matchblox", "accounts", "codex", "work")
	setEnv(t, root, 900, "TMUX_PANE=%20", "CODEX_HOME="+work)
	daemonFile(t, filepath.Join(root, "home", ".codex"), "default", "/work/app", daemonStart.Add(time.Minute), 42)
	if pct, ok := codexUsed(smp.Sample()); ok {
		t.Fatalf("an account read the default's rollout: %v", pct)
	}
	daemonFile(t, work, "own", "/work/app", daemonStart.Add(2*time.Minute), 21)
	smp.procAt = time.Time{} // scan again
	if pct, ok := codexUsed(smp.Sample()); !ok || pct != 21 {
		t.Fatalf("used %v (measured %v), want 21 from its own home", pct, ok)
	}
}

// codexDaemonProc adds the app-server daemon of the default Codex home:
// pid 950, ppid 1, in cwd, started age before the clock of the sampler.
func codexDaemonProc(t *testing.T, root, cwd string, age time.Duration) {
	t.Helper()
	addProc(t, root, 950, 1, "codex", []string{"codex", "app-server", "--listen", "unix://"}, "")
	symlink(t, cwd, filepath.Join(root, "proc", "950", "cwd"))
	stat := filepath.Join(root, "proc", "950", "stat")
	b, err := os.ReadFile(stat)
	if err != nil {
		t.Fatal(err)
	}
	// The uptime of daemonSession: an hour more than the pane's process.
	ticks := strconv.Itoa(int((2*time.Hour.Seconds() - age.Seconds()) * 100))
	if err := os.WriteFile(stat, []byte(strings.Replace(string(b), " 1000 1 1", " "+ticks+" 1 1", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

// noSession fails when the snapshot has a session for pid.
func noSession(t *testing.T, snap Snapshot, pid int) {
	t.Helper()
	for _, s := range snap.Sessions {
		if s.PID == pid {
			t.Errorf("pid %d is a session (agent %s, pane %q)", pid, s.Agent, s.Pane)
		}
	}
}

// TestCodexAppServerHoldingTheRolloutIsNotASession: the app-server daemon
// (ppid 1, cwd /) holds the rollout open; it is not a session, so the pane's
// session reads that rollout.
func TestCodexAppServerHoldingTheRolloutIsNotASession(t *testing.T) {
	smp, root := daemonSession(t)
	p := daemonFile(t, filepath.Join(root, "home", ".codex"), "app", "/work/app", daemonStart.Add(3*time.Minute), 42)
	codexDaemonProc(t, root, "/", 2*time.Hour)
	symlink(t, p, filepath.Join(root, "proc", "950", "fd", "21"))
	snap := smp.Sample()
	noSession(t, snap, 950)
	if s := sessionOf(t, snap, 900); s.Context == "unmeasured" {
		t.Errorf("the pane's session is unmeasured: the daemon took its rollout")
	}
	if pct, ok := codexUsed(snap); !ok || pct != 42 {
		t.Errorf("used %v (measured %v), want 42 from the pane's rollout", pct, ok)
	}
}

// TestCodexAppServerHoldingNothingIsNotASession: on macOS no process shows
// its open files; the daemon has the cwd of the TUI that started it and
// starts just after it. It is not a session and takes no rollout.
func TestCodexAppServerHoldingNothingIsNotASession(t *testing.T) {
	smp, root := daemonSession(t)
	daemonFile(t, filepath.Join(root, "home", ".codex"), "app", "/work/app", daemonStart.Add(3*time.Minute), 42)
	codexDaemonProc(t, root, "/work/app", time.Hour-time.Second)
	snap := smp.Sample()
	noSession(t, snap, 950)
	if s := sessionOf(t, snap, 900); s.Context == "unmeasured" {
		t.Errorf("the pane's session is unmeasured: the daemon took its rollout")
	}
	if pct, ok := codexUsed(snap); !ok || pct != 42 {
		t.Errorf("used %v (measured %v), want 42 from the pane's rollout", pct, ok)
	}
}

// TestAnAgentUnderTheCodexAppServerIsNotASession: a `codex exec` the
// app-server daemon runs in a shell is the daemon's work, not a paneless
// session of its own.
func TestAnAgentUnderTheCodexAppServerIsNotASession(t *testing.T) {
	smp, root := daemonSession(t)
	codexDaemonProc(t, root, "/", 2*time.Hour)
	addProc(t, root, 960, 950, "bash", []string{"bash", "-c", "codex exec hi"}, "")
	addProc(t, root, 961, 960, "codex", []string{"codex", "exec", "hi"}, "")
	snap := smp.Sample()
	noSession(t, snap, 950)
	noSession(t, snap, 961)
}
