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
    -o|-i) shift 2 ;;
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
	c, err := Connect(context.Background(), target(t, "ws-1"))
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

// target is a host reached with a key that exists.
func target(t *testing.T, host string) Target {
	t.Helper()
	key := filepath.Join(t.TempDir(), "matchblox_ed25519")
	write(t, key, "key")
	return Target{Host: host, Key: key}
}

func TestMissingBinaryIsNotInstalled(t *testing.T) {
	fakeSSH(t, "")
	c, err := Connect(context.Background(), target(t, "ws-1"))
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
	c, err := Connect(context.Background(), target(t, "ws-9"))
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
	c, err := Connect(context.Background(), target(t, "ws-1"))
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

func TestNoKeyIsNotConnected(t *testing.T) {
	fakeSSH(t, echoHello)
	_, err := Connect(context.Background(), Target{Host: "ws-1", Key: filepath.Join(t.TempDir(), "none")})
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("err = %v, want ErrNotConnected", err)
	}
}

func TestClassify(t *testing.T) {
	for _, c := range []struct {
		stderr string
		exit   int
		want   error
	}{
		{"", 127, ErrNotInstalled},
		{"sh: 1: exec: matchblox: not found", 127, ErrNotInstalled},
		{"bash: line 1: /home/b/.local/bin/matchblox: No such file or directory", 127, ErrNotInstalled},
		{"builder@ws-1: Permission denied (publickey).", 255, ErrNotConnected},
		{"Host key verification failed.", 255, ErrHostKeyUnknown},
		{"@@@ WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED! @@@\nHost key verification failed.", 255, ErrHostKeyChanged},
		{"ssh: connect to host ws-1 port 22: Connection refused", 255, nil},
	} {
		got := Classify(c.stderr, c.exit)
		if c.want == nil {
			if got != nil {
				t.Errorf("Classify(%q) = %v, want nil", c.stderr, got)
			}
			continue
		}
		if !errors.Is(got, c.want) {
			t.Errorf("Classify(%q, %d) = %v, want %v", c.stderr, c.exit, got, c.want)
		}
	}
}

func TestArgvIsLockedDown(t *testing.T) {
	tg := Target{Host: "ws-1", Key: "/h/.ssh/matchblox_ed25519"}
	a := tg.Argv()
	for _, want := range []string{"-T", "-i", "/h/.ssh/matchblox_ed25519", "IdentitiesOnly=yes", "IdentityAgent=none", "BatchMode=yes",
		"StrictHostKeyChecking=yes", "ForwardAgent=no", "ClearAllForwardings=yes", "ConnectTimeout=10"} {
		if !slices.Contains(a, want) {
			t.Errorf("argv %q lacks %q", a, want)
		}
	}
	i := slices.Index(a, "--")
	if i < 0 || a[i+1] != "ws-1" || a[i+2] != ServeCommand || len(a) != i+3 {
		t.Fatalf("argv %q: want -- ws-1 %q at the end", a, ServeCommand)
	}
	term := tg.TermArgv([]string{"claude"})
	if term[1] != "-t" || !slices.Contains(term, "IdentitiesOnly=yes") || term[len(term)-1] != "claude" {
		t.Fatalf("term argv %q", term)
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

func TestNavArgvAttachesOnTheHost(t *testing.T) {
	tg := Target{Host: "ws-1", Key: "/k"}
	jump := tg.NavArgv([][]string{{"tmux", "switch-client", "-t", "%12"}})
	if got, want := jump[len(jump)-1], `tmux select-window -t %12 ';' select-pane -t %12 ';' attach-session -t %12`; got != want {
		t.Fatalf("jump %s\nwant %s", got, want)
	}
	shell := tg.NavArgv([][]string{{"tmux", "new-window", "-c", "/w/a"}})
	if got, want := shell[len(shell)-1], `tmux new-window -c /w/a ';' attach-session`; got != want {
		t.Fatalf("shell %s\nwant %s", got, want)
	}
}

// The gate reads back only what Quote writes: plain words and single
// quotes. Anything a shell would expand is refused, not guessed.
func TestSplitIsTheInverseOfQuote(t *testing.T) {
	for _, argv := range [][]string{
		{"tmux", "select-window", "-t", "%12", ";", "attach-session", "-t", "%12"},
		{"claude", "Help me; read it's docs"},
		{"sudo", "sh", "-c", "apt-get update && apt-get install -y tmux"},
		{"a", ""},
	} {
		got, err := Split(Quote(argv))
		if err != nil || !slices.Equal(got, argv) {
			t.Errorf("Split(Quote(%q)) = %q, %v", argv, got, err)
		}
	}
	for _, bad := range []string{`echo "x"`, "echo $HOME", "a\\ b", "x `id`", "a 'b", "a; b", "a | b", "a\nb", "$(id)", "a > b"} {
		if got, err := Split(bad); err == nil {
			t.Errorf("Split(%q) = %q, want an error", bad, got)
		}
	}
}

func TestNavAllowedIsExact(t *testing.T) {
	for _, c := range []struct {
		argv []string
		ok   bool
	}{
		{[]string{"tmux", "switch-client", "-t", "%1"}, true},
		{[]string{"tmux", "attach-session", "-t", "%1"}, true},
		{[]string{"tmux", "new-window", "-c", "/w/a"}, true},
		{[]string{"tmux", "new-window", "-c", "/w/#(id)"}, false},
		{[]string{"tmux", "new-window", "rm -rf /"}, false},
		{[]string{"tmux", "switch-client", "-t", "%1;x"}, false},
		{[]string{"sh", "-c", "x"}, false},
	} {
		if got := NavAllowed(c.argv); got != c.ok {
			t.Errorf("NavAllowed(%q) = %v", c.argv, got)
		}
	}
}

func TestBootstrap(t *testing.T) {
	const pub = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFakeFakeFakeFakeFakeFakeFakeFakeFakeFakeFake matchblox"
	s, err := Bootstrap(pub, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"command -v matchblox", "curl -fsSL https://matchblox.sh | MATCHBLOX_NO_START=1 sh", "matchblox authorize '" + pub + "'"} {
		if !strings.Contains(s, want) {
			t.Errorf("bootstrap lacks %q:\n%s", want, s)
		}
	}
	up, _ := Bootstrap(pub, true)
	if strings.Contains(up, "command -v matchblox") {
		t.Errorf("an update must always install:\n%s", up)
	}
	for _, bad := range []string{"ssh-rsa AAAA x", "ssh-ed25519 AAAA'; rm -rf ~; echo '", "ssh-ed25519 AAAA x\nssh-ed25519 BBBB y", ""} {
		if _, err := Bootstrap(bad, false); err == nil {
			t.Errorf("Bootstrap(%q) took a key that is not one ed25519 key", bad)
		}
	}
}

func TestLineShowsTheRemoteCommandAsTyped(t *testing.T) {
	for _, c := range []struct {
		argv []string
		want string
	}{
		{[]string{"ssh", "-t", "--", "ws-1", "sudo sh -c 'apt-get update && apt-get install -y tmux'"}, `ssh -t -- ws-1 "sudo sh -c 'apt-get update && apt-get install -y tmux'"`},
		{[]string{"ssh", "-t", "--", "ws-1", "echo 'a\"b'"}, `ssh -t -- ws-1 'echo '\''a"b'\'''`},
		{[]string{"ssh", "-t", "--", "ws-1", "claude"}, `ssh -t -- ws-1 claude`},
	} {
		if got := Line(c.argv); got != c.want {
			t.Errorf("Line = %s\nwant   %s", got, c.want)
		}
	}
}

func TestLineFoldsTheConsoleOptions(t *testing.T) {
	tg := Target{Host: "ws-1", Key: "/h/.ssh/matchblox_ed25519"}
	if got, want := Line(tg.TermArgv([]string{"claude"})), "ssh … ws-1 claude"; got != want {
		t.Fatalf("Line = %s\nwant   %s", got, want)
	}
}
