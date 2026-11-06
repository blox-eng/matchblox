package transport

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
)

// ErrInUse means a live service already listens on the socket.
var ErrInUse = errors.New("transport: a service already listens on this socket")

// SocketPath is $XDG_RUNTIME_DIR/matchblox/matchblox.sock, else a per-user
// directory in the system temp directory.
func SocketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "matchblox", "matchblox.sock")
	}
	return filepath.Join(os.TempDir(), "matchblox-"+uid(), "matchblox.sock")
}

func uid() string { return strconv.Itoa(os.Getuid()) }

// Listen creates the socket in a 0700 directory. It removes a socket file
// that is left over, but never one that a live service still answers on.
func Listen(path string) (net.Listener, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	// MkdirAll keeps the mode of a directory that exists; other users must
	// not reach the socket, so set it each time.
	if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // a directory needs x to be entered; 0700 is owner only
		return nil, err
	}
	if err := CheckDir(dir); err != nil {
		return nil, err
	}
	// Only the holder of the lock may remove a stale socket; without it two
	// services that start together can each remove the other's socket.
	lk, err := lock(path + ".lock")
	if err != nil {
		return nil, err
	}
	release := func() error {
		if lk == nil {
			return nil
		}
		return lk.Close()
	}
	if _, err := os.Lstat(path); err == nil {
		if c, err := net.DialTimeout("unix", path, time.Second); err == nil {
			c.Close()
			return nil, errors.Join(ErrInUse, release())
		}
		if err := os.Remove(path); err != nil {
			return nil, errors.Join(fmt.Errorf("remove stale socket: %w", err), release())
		}
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, errors.Join(err, release())
	}
	if lk != nil {
		// The pid lets a console name the service when it does not answer.
		_ = lk.Truncate(0)
		_, _ = lk.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	}
	return &locked{Listener: l, release: release}, nil
}

// Owner is the pid of the service that holds the socket, or 0.
func Owner(path string) int {
	b, err := os.ReadFile(path + ".lock")
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return pid
}

// locked holds the lock for as long as the listener lives.
type locked struct {
	net.Listener
	release func() error
}

func (l *locked) Close() error {
	return errors.Join(l.Listener.Close(), l.release())
}

// Dial connects only through a directory that CheckDir accepts.
func Dial(path string, timeout time.Duration) (Conn, error) {
	if err := CheckDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	c, err := net.DialTimeout("unix", path, timeout)
	if err != nil {
		return nil, err
	}
	return NewConn(c), nil
}

// Notify sends one message to the service and leaves, all within timeout:
// it reads the service hello first, so the service has written it before
// the message arrives. It never waits longer, so a hook never holds the
// agent that runs it.
func Notify(path string, timeout time.Duration, kind proto.Kind, body any) error {
	nc, _, err := greet(path, timeout)
	if err != nil {
		return err
	}
	defer nc.Close()
	return proto.Encode(nc, kind, "", body)
}

// Ask sends one message to the service and reads its one reply of the same
// kind into reply, all within timeout, as Notify does.
func Ask(path string, timeout time.Duration, kind proto.Kind, body, reply any) error {
	return AskHeard(path, timeout, kind, body, reply, nil)
}

// AskHeard is Ask for a reply the service wants word of: heard runs once
// the reply is read, and when it returns true AskHeard sends one more
// message of kind, still within timeout, to say the reply was taken.
func AskHeard(path string, timeout time.Duration, kind proto.Kind, body, reply any, heard func() bool) error {
	nc, r, err := greet(path, timeout)
	if err != nil {
		return err
	}
	defer nc.Close()
	if err := proto.Encode(nc, kind, "", body); err != nil {
		return err
	}
	env, err := proto.Decode(r)
	if err != nil {
		return err
	}
	if env.Kind != kind {
		return fmt.Errorf("transport: the service answered %s with %s", kind, env.Kind)
	}
	if err := json.Unmarshal(env.Body, reply); err != nil {
		return err
	}
	if heard != nil && heard() {
		return proto.Encode(nc, kind, "", struct{}{})
	}
	return nil
}

// greet dials path and reads the service hello, with the whole connection
// bound to end within timeout.
func greet(path string, timeout time.Duration) (net.Conn, *bufio.Reader, error) {
	if timeout <= 0 {
		return nil, nil, errors.New("transport: no time left to reach the service")
	}
	deadline := time.Now().Add(timeout)
	if err := CheckDir(filepath.Dir(path)); err != nil {
		return nil, nil, err
	}
	nc, err := net.DialTimeout("unix", path, timeout)
	if err != nil {
		return nil, nil, err
	}
	if err := nc.SetDeadline(deadline); err != nil {
		nc.Close()
		return nil, nil, err
	}
	r := bufio.NewReader(nc)
	env, err := proto.Decode(r)
	var h proto.Hello
	if err == nil && (env.Kind != proto.KindHello || json.Unmarshal(env.Body, &h) != nil || h.Version != proto.Version) {
		err = errors.New("transport: the service speaks another protocol")
	}
	if err != nil {
		nc.Close()
		return nil, nil, err
	}
	return nc, r, nil
}
