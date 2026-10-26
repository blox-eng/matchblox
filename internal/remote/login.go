package remote

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"unicode"

	"github.com/blox-eng/matchblox/internal/transport"
)

// Login is what the builder's own ssh login to a host needs before a jump.
type Login int

const (
	LoginOK Login = iota
	// LoginPassword: the host asks a password (or keyboard-interactive).
	LoginPassword
	// LoginKeyOnly: the host takes keys only, and none of the builder's.
	LoginKeyOnly
	// LoginTailscaleCheck: Tailscale SSH waits for a check in a browser.
	LoginTailscaleCheck
	// LoginTailscaleDenied: the tailnet's SSH rules refuse this login.
	LoginTailscaleDenied
	LoginHostKeyUnknown
	LoginHostKeyChanged
	// LoginUnreachable: no route, a refused port, or a timeout.
	LoginUnreachable
)

func (l Login) String() string {
	switch l {
	case LoginOK:
		return "ok"
	case LoginPassword:
		return "password"
	case LoginKeyOnly:
		return "key only"
	case LoginTailscaleCheck:
		return "tailscale check"
	case LoginTailscaleDenied:
		return "tailscale denied"
	case LoginHostKeyUnknown:
		return "host key unknown"
	case LoginHostKeyChanged:
		return "host key changed"
	case LoginUnreachable:
		return "unreachable"
	}
	return fmt.Sprintf("Login(%d)", int(l))
}

// MasterOpts share one connection per host for the builder's own ssh
// (design §3.1). dir "" is no master. ServerAlive: a master on a dead
// network (the laptop slept) ends in 15 s, so the next jump opens a new
// one instead of hanging on it.
func MasterOpts(dir string) []string {
	if dir == "" {
		return nil
	}
	return []string{"-o", "ControlMaster=auto", "-o", "ControlPath=" + filepath.Join(dir, "%C"), "-o", "ControlPersist=10m",
		"-o", "ServerAliveInterval=5", "-o", "ServerAliveCountMax=3"}
}

// NoForward keeps what ~/.ssh/config forwards out of the builder's own ssh:
// a master would hold a forward's port for 10 minutes, and ssh refuses a
// command when the config has a RemoteCommand.
func NoForward() []string {
	return []string{"-o", "ForwardAgent=no", "-o", "ClearAllForwardings=yes", "-o", "ForwardX11=no", "-o", "RemoteCommand=none"}
}

// ErrMasterTooLong: a master's socket path would not fit sun_path.
var ErrMasterTooLong = errors.New("too long for an ssh socket")

// MasterBase is the base of the console's sockets (the masters, the
// prompts): <runtimeDir>/matchblox when the system gives a private runtime
// directory, else <tmp>/matchblox-<uid>. When that name is not this user's
// (a link, or another user made it first), it is a fresh directory of this
// console's own, never theirs.
func MasterBase(runtimeDir, tmp string, uid int) string {
	if filepath.IsAbs(runtimeDir) && transport.CheckDir(runtimeDir) == nil {
		if base := filepath.Join(runtimeDir, "matchblox"); masterFits(filepath.Join(base, "ssh"), runtime.GOOS) {
			return base
		}
	}
	base := filepath.Join(tmp, "matchblox-"+strconv.Itoa(uid))
	if transport.Mine(base) {
		return base
	}
	if own, err := os.MkdirTemp(tmp, filepath.Base(base)+"-"); err == nil {
		return own
	}
	return base // EnsureMasterDir refuses it, and says why
}

// EnsureMasterDir is <base>/ssh, the directory of the master sockets: it
// makes base and it 0700, and refuses either when others can enter it,
// another user owns it, or it is a link. A socket there logs in as the
// builder. base is /tmp/matchblox-<uid>: short, so the socket path fits.
func EnsureMasterDir(base string) (string, error) {
	dir := filepath.Join(base, "ssh")
	if !masterFits(dir, runtime.GOOS) {
		return "", fmt.Errorf("%s is %w", dir, ErrMasterTooLong)
	}
	for _, d := range []string{base, dir} {
		if err := os.Mkdir(d, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return "", err
		}
		if err := transport.CheckDir(d); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// masterFits tells if a socket in dir fits sun_path (with its NUL: 104
// bytes on darwin, 108 elsewhere). ssh names it %C (40 characters), and
// first listens on that name plus "." and 16 random characters.
func masterFits(dir, goos string) bool {
	limit := 108
	if goos == "darwin" {
		limit = 104
	}
	return len(dir)+1+40+17 < limit
}

// CheckArgv is the login check: the builder's own ssh, never a prompt (it
// runs in the background), through the master. -v gives the server's
// version line and the reason a login fails.
func CheckArgv(host, dir string) []string {
	argv := append(append([]string{"ssh", "-v", "-o", "BatchMode=yes"}, MasterOpts(dir)...), NoForward()...)
	return append(argv, "-o", "ConnectTimeout=10", "--", host, "true")
}

// KeyCheckArgv checks a login with the builder's keys alone, after a key
// install: no master, as the install's password left one up.
func KeyCheckArgv(host string) []string {
	argv := append([]string{"ssh", "-v", "-o", "BatchMode=yes", "-o", "ControlMaster=no", "-o", "ControlPath=none"}, NoForward()...)
	return append(argv, "-o", "ConnectTimeout=10", "--", host, "true")
}

// MasterUpArgv asks if the master of host in dir runs: exit 0 when it
// does. It starts no connection.
func MasterUpArgv(host, dir string) []string {
	return append(append([]string{"ssh"}, MasterOpts(dir)...), "-O", "check", "--", host)
}

// OpenArgv opens the master of host in dir before a jump (design §3.4):
// without BatchMode, so ssh asks what it needs through askpass, in the
// console. It runs only true on the host.
func OpenArgv(host, dir string) []string {
	argv := append(append([]string{"ssh"}, MasterOpts(dir)...), NoForward()...)
	return append(argv, "-o", "ConnectTimeout=10", "--", host, "true")
}

var (
	tailscaleRE    = regexp.MustCompile(`(?m)^debug1: Remote protocol version 2\.0, remote software version Tailscale\s*$`)
	versionRE      = regexp.MustCompile(`(?m)^debug1: Remote protocol version [0-9.]+, remote software version `)
	tailscaleURLRE = regexp.MustCompile(`^https://login\.tailscale\.com/[A-Za-z0-9/]+$`)
)

// Tailscale tells if the server of a check's stderr is Tailscale SSH, from
// the version line of ssh -v.
func Tailscale(stderr string) bool { return tailscaleRE.MatchString(stderr) }

// ServerSeen tells if a check's stderr has the server's version line: a
// check through a master that runs has none, and tells no kind.
func ServerSeen(stderr string) bool { return versionRE.MatchString(stderr) }

// VisitURL reads the link line of Tailscale's check mode: ok when line is
// one, and its URL when it is a plain login.tailscale.com link, else "".
func VisitURL(line string) (url string, ok bool) {
	u, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "# To authenticate, visit: ")
	if !ok {
		return "", false
	}
	if u = strings.TrimSpace(u); !tailscaleURLRE.MatchString(u) {
		return "", true
	}
	return u, true
}

// ClassifyLogin names the result of the login check (ssh -v) from its
// stderr and exit, with the host's kind from the version line.
func ClassifyLogin(stderr string, exit int) (Login, string) {
	return ClassifyAs(stderr, exit, Tailscale(stderr))
}

// ClassifyAs names the result of a login from its stderr and exit. The
// string is the Tailscale check URL, the reason of a refusal, or why the
// host is not reached; "" when there is none. Tailscale's lines name a
// result only when tailscale says the host is Tailscale SSH. A URL that is
// not a plain login.tailscale.com link is never given.
func ClassifyAs(stderr string, exit int, tailscale bool) (Login, string) {
	switch {
	case exit == 0:
		return LoginOK, "" // an approved check too: its link is old
	case strings.Contains(stderr, "REMOTE HOST IDENTIFICATION HAS CHANGED"):
		return LoginHostKeyChanged, ""
	case strings.Contains(stderr, "Host key verification failed"):
		return LoginHostKeyUnknown, ""
	}
	var url, denied string
	var check bool
	for line := range strings.SplitSeq(stderr, "\n") {
		if u, ok := VisitURL(line); ok {
			url, check = u, true
		}
		if r, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "tailscale: "); ok {
			denied = r
		}
	}
	switch {
	case tailscale && check:
		return LoginTailscaleCheck, url
	case tailscale && denied != "":
		return LoginTailscaleDenied, printable(denied)
	case strings.Contains(stderr, "Permission denied"):
		return refused(stderr), ""
	}
	detail := printable(lastSaid(stderr))
	if exit < 0 || detail == "" {
		detail = "timed out" // ssh was ended, or said nothing
	}
	return LoginUnreachable, detail
}

var methodsRE = regexp.MustCompile(`Permission denied \(([a-z0-9@.,-]+)\)`)

// refused names a refused login from the methods ssh says the host takes:
// a password or keyboard-interactive method can install a key, a host that
// takes keys only cannot. A refusal without the methods (a wrong password)
// is a password login.
func refused(stderr string) Login {
	all := methodsRE.FindAllStringSubmatch(stderr, -1)
	if len(all) == 0 {
		return LoginPassword
	}
	for m := range strings.SplitSeq(all[len(all)-1][1], ",") {
		if m == "password" || m == "keyboard-interactive" {
			return LoginPassword
		}
	}
	return LoginKeyOnly
}

// lastSaid is the last line of stderr that is not ssh -v's debug output.
func lastSaid(stderr string) string {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" && !strings.HasPrefix(l, "debug") {
			return l
		}
	}
	return ""
}

// printable keeps the printable characters and spaces: the text comes from
// the network and goes on the screen, so no control, bidi or zero-width
// character reaches it.
func printable(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, s))
}

// tmuxVersion is a tmux version a host can say: "none", or a plain one
// such as 3.4, 3.3a or next-3.5.
var tmuxVersion = regexp.MustCompile(`^(none|[0-9A-Za-z.-]{1,20})$`)

// HostTmux is the tmux version of a host's hello, or "" (not known) when
// it is not a plain one: the host sets it, and the console shows it.
func HostTmux(v string) string {
	if !tmuxVersion.MatchString(v) {
		return ""
	}
	return v
}

// TmuxTooOld is true when tmux version v (as tmux -V says it, without
// "tmux ") is older than floor. A version it cannot read, such as a
// development build or "", is not too old.
func TmuxTooOld(v, floor string) bool {
	var ma, mi, fma, fmi int
	if _, err := fmt.Sscanf(strings.TrimPrefix(v, "next-"), "%d.%d", &ma, &mi); err != nil {
		return false
	}
	if _, err := fmt.Sscanf(floor, "%d.%d", &fma, &fmi); err != nil {
		return false
	}
	return ma < fma || ma == fma && mi < fmi
}
