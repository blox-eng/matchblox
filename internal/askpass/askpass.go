// Package askpass sends the prompts of the builder's own ssh to the console
// (design §3.4). ssh runs `matchblox "<prompt>"` with SSH_ASKPASS; that
// helper (Ask) gives the prompt to the console over a unix socket and prints
// the answer for ssh. The socket is per console run, in a 0700 directory,
// mode 0600, and the helper must present a random token.
//
// The protocol is one JSON line each way on one connection:
// {"token","host","text"} from the helper, {"answer","ok"} from the server.
package askpass

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/blox-eng/matchblox/internal/transport"
)

// The helper's environment. EnvSocket also tells `matchblox` to run as the
// helper.
const (
	EnvSocket = "MATCHBLOX_ASKPASS"
	EnvToken  = "MATCHBLOX_ASKPASS_TOKEN" //nolint:gosec // the name of a variable, not a credential
	EnvHost   = "MATCHBLOX_ASKPASS_HOST"
)

const (
	// sunPath is the smallest sun_path (darwin), with its NUL.
	sunPath = 104
	// maxLine bounds one JSON line each way: maxSent with every byte
	// escaped as \u00XX fits.
	maxLine = 64 << 10
	// maxSent bounds the prompt the helper sends. A host-key prompt with
	// its randomart is far below it.
	maxSent = 8 << 10
	// readRequest is the time a helper has to send its prompt.
	readRequest = 10 * time.Second
	// waitAnswer is the longest a helper waits for the console.
	waitAnswer = 10 * time.Minute
	// maxRunes is the longest prompt the console shows.
	maxRunes = 1000
)

// Prompt is one ssh prompt for the console. Text is already cleaned.
// Confirm is a yes/no prompt (a new host key). Notify is a notice that
// takes no answer: touch a security key; ssh ends the helper after the
// touch. Gone closes when the helper left (ssh gave up or was killed, or
// the touch came): nobody waits for the answer then.
type Prompt struct {
	Host, Text string
	Confirm    bool
	Notify     bool
	Gone       <-chan struct{}
}

type request struct {
	Token string `json:"token"`
	Host  string `json:"host"`
	Text  string `json:"text"`
	Kind  string `json:"kind,omitempty"` // ssh's SSH_ASKPASS_PROMPT: "none", "confirm" or ""
}

type reply struct {
	Answer string `json:"answer"`
	OK     bool   `json:"ok"`
}

type pending struct {
	conn   net.Conn
	prompt Prompt
	gone   chan struct{}
}

// Server is the console's end of the socket.
type Server struct {
	// Program is SSH_ASKPASS: this binary, from Listen.
	Program string

	dir, sock string
	token     string
	ln        net.Listener
	prompts   chan pending
	turn      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

// Listen makes a fresh 0700 directory under dir (which the caller has
// checked) with the socket in it, and starts to accept helpers.
func Listen(dir string) (*Server, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	var b [32 + 4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, err
	}
	sweep(dir)
	token := hex.EncodeToString(b[:32])
	sub := filepath.Join(dir, "ask-"+hex.EncodeToString(b[32:]))
	sock := filepath.Join(sub, "s")
	if len(sock)+1 > sunPath {
		return nil, fmt.Errorf("%s is too long for a socket", sock)
	}
	if err := os.Mkdir(sub, 0o700); err != nil {
		return nil, err
	}
	if err := transport.CheckDir(sub); err != nil {
		_ = os.RemoveAll(sub)
		return nil, err
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		_ = os.RemoveAll(sub)
		return nil, err
	}
	if err := os.Chmod(sock, 0o600); err != nil {
		ln.Close()
		_ = os.RemoveAll(sub)
		return nil, err
	}
	s := &Server{
		Program: self,
		dir:     sub, sock: sock, token: token, ln: ln,
		prompts: make(chan pending),
		turn:    make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
	go s.accept()
	return s, nil
}

// sweep removes the prompt directories under dir that a console left
// when it was killed: this user's, no link, and their socket answers no
// dial. A socket that answers is another console's, and stays.
func sweep(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		sub := filepath.Join(dir, e.Name())
		if !strings.HasPrefix(e.Name(), "ask-") || !e.IsDir() || transport.CheckDir(sub) != nil {
			continue
		}
		if c, err := net.DialTimeout("unix", filepath.Join(sub, "s"), time.Second); err == nil {
			c.Close()
			continue
		}
		_ = os.RemoveAll(sub)
	}
}

// Env is the environment that sends the prompts of an ssh to host here.
// The token is the helper's key to this socket, not a secret of the
// builder. The ssh has no terminal to ask on (it runs in a session of its
// own), so it asks here on any version.
func (s *Server) Env(host string) []string {
	display := os.Getenv("DISPLAY")
	if display == "" {
		display = "matchblox:0" // ssh before 8.4 asks askpass only with one; X11 stays off (remote.NoForward)
	}
	return []string{
		"DISPLAY=" + display,
		"SSH_ASKPASS=" + s.Program,
		"SSH_ASKPASS_REQUIRE=force",
		EnvSocket + "=" + s.sock,
		EnvToken + "=" + s.token,
		EnvHost + "=" + host,
	}
}

func (s *Server) accept() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.serve(c)
	}
}

// serve reads the helper's request and queues it for Next. A wrong token
// or a bad line closes the connection, and the helper fails.
func (s *Server) serve(c net.Conn) {
	_ = c.SetReadDeadline(time.Now().Add(readRequest))
	line, err := bufio.NewReader(io.LimitReader(c, maxLine)).ReadBytes('\n')
	if err != nil {
		c.Close()
		return
	}
	var req request
	if json.Unmarshal(line, &req) != nil || subtle.ConstantTimeCompare([]byte(req.Token), []byte(s.token)) != 1 {
		c.Close()
		return
	}
	_ = c.SetReadDeadline(time.Time{})
	text := clean(req.Text)
	gone := make(chan struct{})
	notify := req.Kind == "none"
	p := pending{conn: c, gone: gone, prompt: Prompt{Host: Clean(req.Host), Text: cut(text), Gone: gone,
		Confirm: !notify && (req.Kind == "confirm" || strings.Contains(text, "(yes/no")), Notify: notify}}
	// The helper sends nothing more: a read ends only when it leaves, or
	// when the answer closes the connection.
	go func() {
		_, _ = c.Read(make([]byte, 1))
		close(gone)
	}()
	select {
	case s.prompts <- p:
	case <-s.done:
		c.Close()
	}
}

// Next waits for the next prompt, one at a time: a second helper waits
// until the first is answered. answer gives the reply once; ok=false makes
// the helper fail, and ssh cancels. When ctx ends first, the prompt is
// cancelled. A prompt whose helper already left is skipped.
func (s *Server) Next(ctx context.Context) (Prompt, func(answer string, ok bool), error) {
	select {
	case s.turn <- struct{}{}:
	case <-ctx.Done():
		return Prompt{}, nil, ctx.Err()
	case <-s.done:
		return Prompt{}, nil, net.ErrClosed
	}
	var p pending
	for p.conn == nil {
		select {
		case p = <-s.prompts:
		case <-ctx.Done():
			<-s.turn
			return Prompt{}, nil, ctx.Err()
		case <-s.done:
			<-s.turn
			return Prompt{}, nil, net.ErrClosed
		}
		select {
		case <-p.gone:
			p.conn.Close()
			p = pending{}
		default:
		}
	}
	var once sync.Once
	answered := make(chan struct{})
	answer := func(a string, ok bool) {
		once.Do(func() {
			close(answered)
			send(p.conn, a, ok)
			<-s.turn
		})
	}
	go func() {
		select {
		case <-answered:
		case <-ctx.Done():
			answer("", false)
		case <-s.done:
			answer("", false)
		}
	}()
	return p.prompt, answer, nil
}

// send writes the reply once and zeroes the reply line. The answer string
// and the copies json makes cannot be zeroed.
func send(c net.Conn, answer string, ok bool) {
	defer c.Close()
	if !ok {
		answer = ""
	}
	b, err := json.Marshal(reply{Answer: answer, OK: ok})
	if err != nil {
		return
	}
	b = append(b, '\n')
	_ = c.SetWriteDeadline(time.Now().Add(readRequest))
	_, _ = c.Write(b)
	clear(b)
}

// Close stops the server, cancels the prompts that wait, and removes the
// socket and its directory.
func (s *Server) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.done)
		err = s.ln.Close()
		if rerr := os.RemoveAll(s.dir); err == nil {
			err = rerr
		}
	})
	return err
}

// Ask is the helper: it sends prompt to the console named by env and
// writes the answer and a newline to out. Any doubt is an error, and ssh
// then gets no answer.
func Ask(env func(string) string, prompt string, out io.Writer) error {
	sock, token := env(EnvSocket), env(EnvToken)
	if sock == "" || token == "" {
		return errors.New("no console to ask")
	}
	if len(prompt) > maxSent {
		prompt = prompt[:maxSent]
	}
	c, err := net.DialTimeout("unix", sock, readRequest)
	if err != nil {
		return fmt.Errorf("the console does not answer: %w", err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(waitAnswer))
	req, err := json.Marshal(request{Token: token, Host: env(EnvHost), Text: prompt, Kind: env("SSH_ASKPASS_PROMPT")})
	if err != nil {
		return err
	}
	if _, err := c.Write(append(req, '\n')); err != nil {
		return fmt.Errorf("the console does not answer: %w", err)
	}
	line, err := bufio.NewReader(io.LimitReader(c, maxLine)).ReadBytes('\n')
	defer clear(line)
	if err != nil {
		return errors.New("the console gave no answer")
	}
	var r reply
	if err := json.Unmarshal(line, &r); err != nil {
		return errors.New("the console's answer is not valid")
	}
	if !r.OK {
		return errors.New("cancelled")
	}
	// line and b are zeroed; r.Answer and the copies of bufio and json
	// cannot be.
	b := append([]byte(r.Answer), '\n')
	defer clear(b)
	if bytes.ContainsAny(b[:len(b)-1], "\r\n\x00") {
		return errors.New("the answer has a line break")
	}
	if _, err := out.Write(b); err != nil {
		return err
	}
	return nil
}

// Clean makes a prompt safe to draw: it drops escape sequences, C0 and
// C1 controls but the line break, and what is not printable or not UTF-8.
// It keeps at most two line breaks in a row and 1000 runes. A
// keyboard-interactive prompt comes from the server.
func Clean(s string) string { return cut(clean(s)) }

func cut(s string) string {
	n := 0
	for i := range s {
		if n == maxRunes {
			return s[:i]
		}
		n++
	}
	return s
}

func clean(s string) string {
	var b strings.Builder
	breaks := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == 0x1b && i+1 < len(s):
			i += escape(s[i+1:]) + 1
			continue
		case r == 0x9b: // C1 CSI
			i += size + csi(s[i+size:])
			continue
		case r == 0x90 || r == 0x9d || r == 0x98 || r == 0x9e || r == 0x9f: // C1 strings
			i += size + str(s[i+size:])
			continue
		case r == '\n':
			if breaks < 2 {
				b.WriteByte('\n')
			}
			breaks++
		case r == utf8.RuneError && size <= 1:
		case unicode.IsPrint(r):
			b.WriteRune(r)
			breaks = 0
		}
		i += size
	}
	return b.String()
}

// escape is the length of the sequence after an ESC.
func escape(s string) int {
	switch s[0] {
	case '[':
		return 1 + csi(s[1:])
	case ']', 'P', 'X', '^', '_':
		return 1 + str(s[1:])
	}
	// ESC with intermediates (0x20-0x2f), then one final byte.
	i := 0
	for i < len(s) && s[i] >= 0x20 && s[i] <= 0x2f {
		i++
	}
	if i < len(s) {
		i++
	}
	return i
}

// csi is the length of a control sequence's parameters and final byte.
func csi(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x40 && s[i] <= 0x7e {
			return i + 1
		}
		if s[i] < 0x20 || s[i] > 0x3f {
			return i // not a sequence: the rest is text
		}
	}
	return len(s)
}

// str is the length of a control string up to its end: BEL, ESC \ or ST.
func str(s string) int {
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == 0x07:
			return i + 1
		case s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\':
			return i + 2
		case strings.HasPrefix(s[i:], "\u009c"):
			return i + 2
		}
	}
	return len(s)
}
