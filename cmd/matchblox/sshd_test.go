package main

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/hosts"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/transport"
)

// realHost is a real sshd on 127.0.0.1 whose sessions run in a fake home,
// with this test binary as matchblox. ssh on PATH reads only its config.
type realHost struct{ dir, laptop, far, bin string }

func startSSHD(t *testing.T) realHost {
	t.Helper()
	sshd, err := exec.LookPath("sshd")
	if err != nil {
		sshd = "/usr/sbin/sshd"
	}
	if _, err := os.Stat(sshd); err != nil || testing.Short() {
		t.Skip("no sshd here")
	}
	dir, err := os.MkdirTemp("", "mbs") // short: the far socket path must fit 108 bytes
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	h := realHost{dir: dir, laptop: filepath.Join(dir, "laptop"), far: filepath.Join(dir, "far"), bin: filepath.Join(dir, "bin")}
	for _, d := range []string{h.laptop, filepath.Join(h.far, ".ssh"), filepath.Join(h.far, ".local", "bin"), h.bin, filepath.Join(dir, "run"), filepath.Join(dir, "lrun")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	keygen := func(path, comment string) {
		if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", comment, "-f", path).CombinedOutput(); err != nil { //nolint:gosec // test
			t.Skipf("ssh-keygen: %v %s", err, out)
		}
	}
	keygen(filepath.Join(dir, "hostkey"), "host")
	keygen(filepath.Join(dir, "builder"), "builder")
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := os.ReadFile(filepath.Join(dir, "builder.pub"))
	hostPub, _ := os.ReadFile(filepath.Join(dir, "hostkey.pub"))
	files := map[string]string{
		"sshd_config": fmt.Sprintf(`Port %d
ListenAddress 127.0.0.1
HostKey %[2]s/hostkey
PidFile %[2]s/sshd.pid
AuthorizedKeysFile %[2]s/far/.ssh/authorized_keys
PasswordAuthentication no
KbdInteractiveAuthentication no
UsePAM no
StrictModes no
PermitUserRC no
AllowUsers %[3]s
SetEnv HOME=%[2]s/far XDG_RUNTIME_DIR=%[2]s/run XDG_STATE_HOME=%[2]s/far/state XDG_CONFIG_HOME=%[2]s/far/config TMUX_TMPDIR=%[2]s/run
`, port, dir, u.Username),
		"ssh_config":               fmt.Sprintf("Host ws-1\n  HostName 127.0.0.1\n  Port %d\n  User %s\n  IdentityFile %s/builder\n  UserKnownHostsFile %s/known_hosts\n", port, u.Username, dir, dir),
		"known_hosts":              fmt.Sprintf("[127.0.0.1]:%d %s\n", port, strings.Join(strings.Fields(string(hostPub))[:2], " ")),
		"far/.ssh/authorized_keys": string(pub),
		"bin/ssh":                  "#!/bin/sh\nexec " + mustLook(t, "ssh") + " -F " + dir + "/ssh_config \"$@\"\n",
		// The far side has the test binary as matchblox, under the wrapper's
		// own name (the gate line names the binary by its argv[0]), and a
		// curl whose install does nothing: no network in a test. Both sit in
		// ~/.local/bin, which ssh's own PATH does not have.
		"far/.local/bin/curl":      "#!/bin/sh\necho true\n",
		"far/.local/bin/matchblox": "#!/bin/bash\nIFS=$'\\x1f'; MATCHBLOX_TEST_ARGS=\"$*\" exec -a \"$0\" " + os.Args[0] + "\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o700); err != nil { //nolint:gosec // test files
			t.Fatal(err)
		}
	}
	if out, err := exec.Command(sshd, "-f", filepath.Join(dir, "sshd_config"), "-E", filepath.Join(dir, "sshd.log")).CombinedOutput(); err != nil { //nolint:gosec // test
		t.Skipf("sshd does not start here: %v %s", err, out)
	}
	t.Cleanup(func() {
		// The service the far side started outlives its stream; stop it.
		if pid := transport.Owner(filepath.Join(dir, "run", "matchblox", "matchblox.sock")); pid > 0 {
			if p, err := os.FindProcess(pid); err == nil {
				_ = p.Kill()
			}
		}
		if b, err := os.ReadFile(filepath.Join(dir, "sshd.pid")); err == nil {
			var pid int
			if _, err := fmt.Sscan(string(b), &pid); err == nil && pid > 0 {
				if p, err := os.FindProcess(pid); err == nil {
					_ = p.Kill()
				}
			}
		}
	})
	t.Setenv("PATH", h.bin+":"+os.Getenv("PATH"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(dir, "lrun"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(h.laptop, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h.laptop, "config"))
	time.Sleep(200 * time.Millisecond)
	return h
}

func mustLook(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("no %s here", name)
	}
	return p
}

// TestConnectAndGateOnARealSSHD connects a host with a real sshd, then
// tries what a stolen matchblox key would try.
func TestConnectAndGateOnARealSSHD(t *testing.T) {
	h := startSSHD(t)
	run := func(argv []string) error {
		c := exec.Command(argv[0], argv[1:]...) //nolint:gosec // test
		out, err := c.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w: %s", err, out)
		}
		return nil
	}
	var out bytes.Buffer
	list := filepath.Join(h.laptop, "hosts")
	if err := connectHost(connectEnv{Home: h.laptop, Hosts: list, Run: run, Out: &out, Verify: verifyHost}, "ws-1"); err != nil {
		t.Fatalf("connect: %v\n%s", err, out.String())
	}
	if got := hosts.Read(list); len(got) != 1 || got[0] != "ws-1" {
		t.Fatalf("hosts %q", got)
	}
	key := remote.KeyPath(h.laptop)
	tg := remote.Target{Host: "ws-1", Key: key}
	for _, cmd := range []string{"", "id", "sh -c 'id'", "tmux attach-session", "tmux new-window -c / ';' attach-session",
		"claude", "matchblox serve --stdio; id"} {
		argv := tg.Argv()
		argv[len(argv)-1] = cmd
		c := exec.Command(argv[0], argv[1:]...) //nolint:gosec // test
		c.Stdin = strings.NewReader("")
		b, err := c.CombinedOutput()
		var ee *exec.ExitError
		if !strings.Contains(string(b), "refused") || !errors.As(err, &ee) || ee.ExitCode() != 126 {
			t.Errorf("%q with the matchblox key: %v\n%s", cmd, err, b)
		}
	}
}
