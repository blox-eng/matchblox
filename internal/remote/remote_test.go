package remote

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
)

// fakeSSH puts an ssh on PATH that drops its options and the host and runs
// the command here with sh, as the far side would. matchblox is the far
// side's binary; "" means it has none.
func fakeSSH(t *testing.T, matchblox string) {
	t.Helper()
	dir := t.TempDir()
	ssh := `#!/bin/sh
while [ $# -gt 0 ]; do
  case "$1" in
    -o) shift 2 ;;
    --) shift; break ;;
    -*) shift ;;
    *) break ;;
  esac
done
echo "$1" > "` + dir + `/host"
shift
exec sh -c "$*"
`
	write(t, filepath.Join(dir, "ssh"), ssh)
	far := filepath.Join(dir, "far")
	if err := os.Mkdir(far, 0o700); err != nil {
		t.Fatal(err)
	}
	if matchblox != "" {
		write(t, filepath.Join(far, "matchblox"), matchblox)
	}
	// No matchblox of this machine leaks in.
	t.Setenv("PATH", dir+":"+far+":/usr/bin:/bin")
	t.Setenv("HOME", t.TempDir())
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil { //nolint:gosec // a test script
		t.Fatal(err)
	}
}

// echoHello is a far-side matchblox that says hello as ws-1, then echoes.
const echoHello = `#!/bin/sh
[ "$1 $2" = "serve --stdio" ] || { echo "wrong args: $*" >&2; exit 2; }
printf '%s\n' '{"kind":"hello","body":{"version":1,"binary":"v0.1.0","host":"ws-1"}}'
cat
`

func TestRemoteHelloOverFakeSSH(t *testing.T) {
	fakeSSH(t, echoHello)
	c, err := Connect(context.Background(), "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	env, err := c.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if env.Kind != proto.KindHello || !strings.Contains(string(env.Body), `"host":"ws-1"`) {
		t.Fatalf("got %s %s", env.Kind, env.Body)
	}
	if err := c.Send(proto.KindHello, "", proto.Hello{Version: proto.Version}); err != nil {
		t.Fatal(err)
	}
	if env, err := c.Recv(); err != nil || env.Kind != proto.KindHello {
		t.Fatalf("the echo: %v %v", env, err)
	}
}

func TestMissingBinaryIsNotInstalled(t *testing.T) {
	fakeSSH(t, "")
	c, err := Connect(context.Background(), "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, err = c.Recv()
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("err = %v, want ErrNotInstalled", err)
	}
}

func TestSSHErrorNamesTheCause(t *testing.T) {
	fakeSSH(t, "#!/bin/sh\necho 'ssh: Could not resolve hostname ws-9: Name or service not known' >&2\nexit 255\n")
	c, err := Connect(context.Background(), "ws-9")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, err = c.Recv()
	if err == nil || errors.Is(err, ErrNotInstalled) || !strings.Contains(err.Error(), "Could not resolve hostname") {
		t.Fatalf("err = %v, want the last line of ssh's stderr", err)
	}
}

func TestCloseEndsSSH(t *testing.T) {
	fakeSSH(t, echoHello)
	c, err := Connect(context.Background(), "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Recv(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := c.Recv(); done <- err }()
	_ = c.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Recv after Close: no error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Recv still blocks after Close")
	}
}

func TestNeedsInstall(t *testing.T) {
	for _, c := range []struct {
		stderr string
		exit   int
		want   bool
	}{
		{"", 127, true},
		{"sh: 1: exec: matchblox: not found", 127, true},
		{"bash: line 1: matchblox: command not found", 1, true},
		{"ssh: connect to host ws-1 port 22: Connection refused", 255, false},
		{"", 0, false},
	} {
		if got := NeedsInstall(c.stderr, c.exit); got != c.want {
			t.Errorf("NeedsInstall(%q, %d) = %v", c.stderr, c.exit, got)
		}
	}
}

func TestArgvIsBatchAndEndsOptions(t *testing.T) {
	a := Argv("ws-1")
	if a[0] != "ssh" || !slices.Contains(a, "BatchMode=yes") || !slices.Contains(a, "-T") {
		t.Fatalf("argv %q: want ssh -T with BatchMode=yes", a)
	}
	i := slices.Index(a, "--")
	if i < 0 || a[i+1] != "ws-1" || a[i+2] != ServeCommand || len(a) != i+3 {
		t.Fatalf("argv %q: want -- ws-1 %q at the end", a, ServeCommand)
	}
}

func TestValidHost(t *testing.T) {
	for h, want := range map[string]bool{
		"ws-1": true, "build.example.com": true, "me@ws-1": true, "10.0.0.2": true,
		"": false, "-oProxyCommand=x": false, "ws 1": false, "ws;1": false, "ws-1/x": false,
	} {
		if got := ValidHost(h); got != want {
			t.Errorf("ValidHost(%q) = %v", h, got)
		}
	}
}

func TestInstallArgv(t *testing.T) {
	want := []string{"ssh", "-t", "--", "ws-1", "curl -fsSL https://matchblox.sh | MATCHBLOX_NO_START=1 sh"}
	if got := InstallArgv("ws-1"); !slices.Equal(got, want) {
		t.Fatalf("InstallArgv = %q, want %q", got, want)
	}
}

func TestTermArgvQuotesEachWord(t *testing.T) {
	got := TermArgv("ws-1", []string{"claude", "Help me; read it's docs"})
	want := []string{"ssh", "-t", "--", "ws-1", `claude 'Help me; read it'\''s docs'`}
	if !slices.Equal(got, want) {
		t.Fatalf("TermArgv = %q, want %q", got, want)
	}
}

func TestNavArgvAttachesOnTheHost(t *testing.T) {
	jump := NavArgv("ws-1", [][]string{{"tmux", "switch-client", "-t", "%12"}})
	want := []string{"ssh", "-t", "--", "ws-1",
		`tmux select-window -t %12 ';' select-pane -t %12 ';' attach-session -t %12`}
	if !slices.Equal(jump, want) {
		t.Fatalf("jump = %q\nwant %q", jump, want)
	}
	shell := NavArgv("ws-1", [][]string{{"tmux", "new-window", "-c", "/w/a"}})
	want = []string{"ssh", "-t", "--", "ws-1", `tmux new-window -c /w/a ';' attach-session`}
	if !slices.Equal(shell, want) {
		t.Fatalf("shell = %q\nwant %q", shell, want)
	}
}

// The line is what a person can paste: the remote command stays one word,
// so its pipe runs on the host.
func TestLineShowsTheRemoteCommandAsTyped(t *testing.T) {
	for _, c := range []struct {
		argv []string
		want string
	}{
		{TermArgv("ws-1", []string{"sudo", "sh", "-c", "apt-get update && apt-get install -y tmux"}), `ssh -t -- ws-1 "sudo sh -c 'apt-get update && apt-get install -y tmux'"`},
		{InstallArgv("ws-1"), `ssh -t -- ws-1 "curl -fsSL https://matchblox.sh | MATCHBLOX_NO_START=1 sh"`},
		{TermArgv("ws-1", []string{"echo", `a"b`}), `ssh -t -- ws-1 'echo '\''a"b'\'''`},
		{TermArgv("ws-1", []string{"claude"}), `ssh -t -- ws-1 claude`},
	} {
		if got := Line(c.argv); got != c.want {
			t.Errorf("Line = %s\nwant   %s", got, c.want)
		}
	}
}

// What the far side wrote before it spoke is not why it ended later.
func TestLossAfterHelloIgnoresStartupStderr(t *testing.T) {
	fakeSSH(t, `#!/bin/sh
echo "matchblox: wrote config.toml" >&2
printf '%s\n' '{"kind":"hello","body":{"version":1}}'
sleep 0.2
exit 1
`)
	c, err := Connect(context.Background(), "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Recv(); err != nil {
		t.Fatal(err)
	}
	_, err = c.Recv()
	if err == nil || strings.Contains(err.Error(), "wrote config") || !strings.Contains(err.Error(), "exited 1") {
		t.Fatalf("err = %v, want the exit, not the startup line", err)
	}
}
