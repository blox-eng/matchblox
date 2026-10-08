// Package remote reaches the service of another host over SSH: the console
// there speaks the same wire as a local one, on the stdin and stdout of
// `matchblox serve --stdio`.
package remote

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/transport"
)

// ErrNotInstalled: the host has no matchblox on its PATH.
var ErrNotInstalled = errors.New("matchblox is not installed there")

// ServeCommand is what ssh runs on the host. ssh starts a non-login shell,
// which does not have the install script's ~/.local/bin on its PATH.
const ServeCommand = `PATH="$HOME/.local/bin:$PATH" exec matchblox serve --stdio`

// install is the one install command, with no data in it. MATCHBLOX_NO_START
// keeps the script from starting a console inside this one.
const install = "curl -fsSL https://matchblox.sh | MATCHBLOX_NO_START=1 sh"

var hostRE = regexp.MustCompile(`^[A-Za-z0-9_.@:%\[\]-]+$`)

// ValidHost is a name ssh reads as a host and never as an option.
func ValidHost(h string) bool { return hostRE.MatchString(h) && !strings.HasPrefix(h, "-") }

// Argv is the ssh command that joins this console to the host's service.
// BatchMode: no password prompt inside the console. ServerAlive: a dropped
// network ends the connection in 15 s instead of never, and a host that is
// down fails in 10 s.
func Argv(host string) []string {
	return []string{"ssh", "-T", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=5", "-o", "ServerAliveCountMax=3",
		"--", host, ServeCommand}
}

// InstallArgv installs matchblox on the host, in this terminal.
func InstallArgv(host string) []string { return []string{"ssh", "-t", "--", host, install} }

// TermArgv runs a command on the host in this terminal. ssh hands the far
// side one shell string, so each word is quoted.
func TermArgv(host string, argv []string) []string {
	return []string{"ssh", "-t", "--", host, Quote(argv)}
}

// NavArgv turns the console's tmux moves into one command that attaches to
// the host's tmux: there is no tmux client of the host to switch.
func NavArgv(host string, steps [][]string) []string {
	cmd := []string{"tmux"}
	attach := []string{"attach-session"}
	for _, s := range steps {
		if len(s) < 2 || s[0] != "tmux" {
			continue
		}
		switch args := s[1:]; args[0] {
		case "switch-client":
			cmd = append(cmd, "select-window", "-t", args[2], ";", "select-pane", "-t", args[2], ";")
			attach = []string{"attach-session", "-t", args[2]}
		case "attach-session":
			attach = args
		default:
			cmd = append(append(cmd, args...), ";")
		}
	}
	return TermArgv(host, append(cmd, attach...))
}

// Quote is argv as a shell reads it: a word with a space, a quote or a
// shell character gets single quotes.
func Quote(argv []string) string {
	q := make([]string, len(argv))
	for i, a := range argv {
		q[i] = a
		if a == "" || strings.ContainsAny(a, " \t\n'\"\\$`;&|<>()*?#~{}[]!=") {
			q[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
	}
	return strings.Join(q, " ")
}

// Line is argv as the person would type it. ssh joins the words after the
// host into one shell line, so that line shows as it is.
func Line(argv []string) string {
	if len(argv) > 1 && argv[0] == "ssh" {
		return Quote(argv[:len(argv)-1]) + " " + argv[len(argv)-1]
	}
	return Quote(argv)
}

// NeedsInstall reads ssh's exit: 127 is the far shell's "not found".
func NeedsInstall(stderr string, exit int) bool {
	return exit == 127 || strings.Contains(stderr, "matchblox: command not found") ||
		strings.Contains(stderr, "matchblox: not found")
}

// Connect starts ssh and speaks on its stdin and stdout. It returns once ssh
// runs; a host that cannot be reached shows on the first Recv, with the
// cause ssh gave.
func Connect(ctx context.Context, host string) (transport.Conn, error) {
	if !ValidHost(host) {
		return nil, fmt.Errorf("%q is not a host name", host)
	}
	argv := Argv(host)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // argv, the host checked by ValidHost
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	c := &conn{cmd: cmd}
	cmd.Stderr = &c.stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c.Conn = transport.Stdio(out, in)
	return c, nil
}

type conn struct {
	transport.Conn
	cmd    *exec.Cmd
	stderr lockedBuffer
	once   sync.Once
	err    error
	spoke  bool
}

// Recv names why ssh ended instead of a bare EOF.
func (c *conn) Recv() (proto.Envelope, error) {
	env, err := c.Conn.Recv()
	if err != nil {
		return env, c.exit(err)
	}
	if !c.spoke {
		// What the far side wrote before it spoke (a first run's note) is
		// not why it ends later.
		c.spoke = true
		c.stderr.Reset()
	}
	return env, nil
}

func (c *conn) Close() error {
	_ = c.Conn.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.exit(nil)
	return nil
}

// exit waits for ssh once and keeps why it ended.
func (c *conn) exit(readErr error) error {
	c.once.Do(func() {
		werr := c.cmd.Wait()
		code := 0
		var ee *exec.ExitError
		if errors.As(werr, &ee) {
			code = ee.ExitCode()
		}
		stderr := strings.TrimSpace(c.stderr.String())
		switch {
		case NeedsInstall(stderr, code):
			c.err = ErrNotInstalled
		case stderr != "":
			c.err = errors.New(lastLine(stderr))
		case code > 0:
			c.err = fmt.Errorf("ssh exited %d", code)
		case werr != nil:
			c.err = werr
		case readErr != nil:
			c.err = readErr
		default:
			c.err = errors.New("ssh ended")
		}
	})
	return c.err
}

func lastLine(s string) string {
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return s
}

// lockedBuffer is ssh's stderr: exec writes it while Recv may read it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.b.Len() > 64<<10 {
		l.b.Reset() // only the last lines matter
	}
	return l.b.Write(p)
}

func (l *lockedBuffer) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.b.Reset()
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}
