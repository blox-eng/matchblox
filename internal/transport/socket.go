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
	if err := checkDir(dir); err != nil {
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

// Dial connects only through a directory that checkDir accepts.
func Dial(path string, timeout time.Duration) (Conn, error) {
	if err := checkDir(filepath.Dir(path)); err != nil {
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
	deadline := time.Now().Add(timeout)
	if err := checkDir(filepath.Dir(path)); err != nil {
		return err
	}
	nc, err := net.DialTimeout("unix", path, timeout)
	if err != nil {
		return err
	}
	defer nc.Close()
	if err := nc.SetDeadline(deadline); err != nil {
		return err
	}
	env, err := proto.Decode(bufio.NewReader(nc))
	if err != nil {
		return err
	}
	var h proto.Hello
	if env.Kind != proto.KindHello || json.Unmarshal(env.Body, &h) != nil || h.Version != proto.Version {
		return errors.New("transport: the service speaks another protocol")
	}
	return proto.Encode(nc, kind, "", body)
}
