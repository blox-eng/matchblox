package askpass

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func listen(t *testing.T) *Server {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ap") // short, not macOS $TMPDIR: the socket path must fit sun_path
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	s, err := Listen(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// envOf is the helper's environment as Env gives it to ssh.
func envOf(env []string) func(string) string {
	m := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	return func(k string) string { return m[k] }
}

type asked struct {
	out bytes.Buffer
	err error
}

func ask(env func(string) string, prompt string) <-chan *asked {
	done := make(chan *asked, 1)
	go func() {
		a := &asked{}
		a.err = Ask(env, prompt, &a.out)
		done <- a
	}()
	return done
}

func TestAskRoundTrip(t *testing.T) {
	s := listen(t)
	done := ask(envOf(s.Env("ws-1")), "Password: ")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, answer, err := s.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.Gone == nil {
		t.Fatal("the prompt has no Gone")
	}
	if p.Gone = nil; p != (Prompt{Host: "ws-1", Text: "Password: "}) {
		t.Fatalf("prompt %+v", p)
	}
	answer("s3cret", true)
	a := <-done
	if a.err != nil {
		t.Fatal(a.err)
	}
	if a.out.String() != "s3cret\n" {
		t.Fatalf("out %q", a.out.String())
	}
}

func TestAskCancelled(t *testing.T) {
	s := listen(t)
	done := ask(envOf(s.Env("ws-1")), "Password: ")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, answer, err := s.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	answer("ignored", false)
	if a := <-done; a.err == nil || a.out.Len() != 0 {
		t.Fatalf("a cancel must fail with no output: %v %q", a.err, a.out.String())
	}
}

func TestAskUnansweredCancelledWithContext(t *testing.T) {
	s := listen(t)
	done := ask(envOf(s.Env("ws-1")), "Password: ")
	ctx, cancel := context.WithCancel(context.Background())
	if _, _, err := s.Next(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case a := <-done:
		if a.err == nil {
			t.Fatal("an unanswered prompt must fail the helper")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the helper hangs")
	}
}

func TestAskWrongTokenRefused(t *testing.T) {
	s := listen(t)
	env := envOf(s.Env("ws-1"))
	wrong := func(k string) string {
		if k == EnvToken {
			return strings.Repeat("0", 64)
		}
		return env(k)
	}
	done := ask(wrong, "Password: ")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if p, _, err := s.Next(ctx); err == nil {
		t.Fatalf("Next yields %+v for a wrong token", p)
	}
	if a := <-done; a.err == nil || a.out.Len() != 0 {
		t.Fatalf("a wrong token must fail with no output: %v %q", a.err, a.out.String())
	}
}

func TestAskWithoutEnvFails(t *testing.T) {
	var out bytes.Buffer
	if err := Ask(func(string) string { return "" }, "Password: ", &out); err == nil || out.Len() != 0 {
		t.Fatalf("no socket must fail: %v %q", err, out.String())
	}
}

func TestOnePromptAtATime(t *testing.T) {
	s := listen(t)
	env := envOf(s.Env("ws-1"))
	first := ask(env, "one: ")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p1, answer1, err := s.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second := ask(env, "two: ")
	short, stop := context.WithTimeout(ctx, 300*time.Millisecond)
	defer stop()
	if p, _, err := s.Next(short); err == nil {
		t.Fatalf("a second prompt %+v before the first is answered", p)
	}
	answer1("a", true)
	p2, answer2, err := s.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	answer2("b", true)
	if p1.Text != "one: " || p2.Text != "two: " {
		t.Fatalf("prompts %q %q", p1.Text, p2.Text)
	}
	if a, b := <-first, <-second; a.out.String() != "a\n" || b.out.String() != "b\n" {
		t.Fatalf("answers %q %q", a.out.String(), b.out.String())
	}
}

func TestListenModes(t *testing.T) {
	s := listen(t)
	sock := envOf(s.Env("ws-1"))(EnvSocket)
	for path, want := range map[string]os.FileMode{filepath.Dir(sock): 0o700, sock: 0o600} {
		fi, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != want {
			t.Errorf("%s mode %o, want %o", path, fi.Mode().Perm(), want)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Dir(sock)); !os.IsNotExist(err) {
		t.Errorf("Close leaves %s: %v", filepath.Dir(sock), err)
	}
}

func TestListenTooLong(t *testing.T) {
	if _, err := Listen("/" + strings.Repeat("d", 120)); err == nil {
		t.Fatal("a socket path past sun_path must be refused")
	}
}

func TestEnv(t *testing.T) {
	s := listen(t)
	s.Program = "/bin/matchblox"
	env := envOf(s.Env("ws-1"))
	if env("SSH_ASKPASS") != "/bin/matchblox" || env("SSH_ASKPASS_REQUIRE") != "force" || env(EnvHost) != "ws-1" {
		t.Fatalf("env %q", s.Env("ws-1"))
	}
	if len(env(EnvToken)) != 64 || env(EnvSocket) == "" {
		t.Fatalf("token or socket missing: %q", s.Env("ws-1"))
	}
	// ssh before 8.4 has no SSH_ASKPASS_REQUIRE: it asks askpass only with
	// a DISPLAY, and no terminal (the console's ssh has none).
	t.Setenv("DISPLAY", "")
	if env := envOf(s.Env("ws-1")); env("DISPLAY") == "" {
		t.Fatalf("no DISPLAY: %q", s.Env("ws-1"))
	}
	t.Setenv("DISPLAY", ":7")
	if env := envOf(s.Env("ws-1")); env("DISPLAY") != ":7" {
		t.Fatalf("the builder's DISPLAY is not kept: %q", s.Env("ws-1"))
	}
}

func TestCleanPrompt(t *testing.T) {
	for in, want := range map[string]string{
		"\x1b]0;pwn\x07Password: \x1b[31m":         "Password: ",
		"\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\": "link",
		"a\u009b31mb\u009d0;t\u009cc\x00d\x7fe":    "abcde",
		"line one\nline two\r\t":                   "line one\nline two",
		"a\n\n\n\n\x1b[0m\nb":                      "a\n\nb",
		"\u202eevil":                               "evil",
		"bad \xff utf8":                            "bad  utf8",
	} {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
	if got := []rune(Clean(strings.Repeat("é", 1500))); len(got) != 1000 {
		t.Errorf("a 1500-rune prompt keeps %d runes, want 1000", len(got))
	}
}

// hostKeyPrompt and randomartPrompt are what OpenSSH 9.6 gives askpass
// for an unknown host key (recorded from the test sshd, with VisualHostKey
// no and yes; host renamed).
const (
	hostKeyPrompt = "The authenticity of host 'ws-1 (192.0.2.1)' can't be established.\n" +
		"ED25519 key fingerprint is SHA256:lViGGtJjGWBALGmVnR9Y7N8oBi25EwO/ufx8inKeOLw.\n" +
		"This key is not known by any other names.\n" +
		"Are you sure you want to continue connecting (yes/no/[fingerprint])? "
	randomartPrompt = "The authenticity of host 'ws-1 (192.0.2.1)' can't be established.\n" +
		"ED25519 key fingerprint is SHA256:lViGGtJjGWBALGmVnR9Y7N8oBi25EwO/ufx8inKeOLw.\n" +
		"+--[ED25519 256]--+\n| ++o=o*+ .o      |\n|o..+.+*o.+ .     |\n|..  oo=+o o      |\n|     B.+ .       |\n" +
		"|      O S o      |\n|     = o o .     |\n|  . . + .        |\n|   +.++  .       |\n|   E*+.+o        |\n" +
		"+----[SHA256]-----+\n" +
		"This key is not known by any other names.\n" +
		"Are you sure you want to continue connecting (yes/no/[fingerprint])? "
)

func TestConfirmPrompt(t *testing.T) {
	long := "The authenticity of host 'ws-1' can't be established.\n" + strings.Repeat("ws-1 known as ws-1.example.com\n", 60) +
		"Are you sure you want to continue connecting (yes/no/[fingerprint])? "
	s := listen(t)
	for name, text := range map[string]string{
		"short":     "Are you sure you want to continue connecting (yes/no/[fingerprint])? ",
		"host key":  hostKeyPrompt,
		"randomart": randomartPrompt,
		"past cap":  long,
	} {
		done := ask(envOf(s.Env("ws-1")), text)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		p, answer, err := s.Next(ctx)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		answer("yes", true)
		<-done
		if !p.Confirm {
			t.Errorf("%s: prompt %q is a yes/no prompt", name, p.Text)
		}
		if name != "past cap" && p.Text != text {
			t.Errorf("%s: text %q, want it whole", name, p.Text)
		}
	}
}

func TestListenDirIsNoPartOfTheToken(t *testing.T) {
	s := listen(t)
	env := envOf(s.Env("ws-1"))
	dir := filepath.Base(filepath.Dir(env(EnvSocket)))
	if !strings.HasPrefix(dir, "ask-") || strings.Contains(env(EnvToken), strings.TrimPrefix(dir, "ask-")) {
		t.Fatalf("dir %q shows part of the token", dir)
	}
}

func TestNothingLogged(t *testing.T) {
	var mu sync.Mutex
	var logged bytes.Buffer
	prevSlog, prevOut := slog.Default(), log.Writer()
	slog.SetDefault(slog.New(slog.NewTextHandler(lockedWriter{&mu, &logged}, &slog.HandlerOptions{Level: slog.LevelDebug})))
	log.SetOutput(lockedWriter{&mu, &logged})
	defer func() { slog.SetDefault(prevSlog); log.SetOutput(prevOut) }()

	const secret = "hunter2-answer"
	s := listen(t)
	done := ask(envOf(s.Env("ws-1")), "Password: ")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, answer, err := s.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	answer(secret, true)
	if a := <-done; a.err != nil {
		t.Fatal(a.err)
	}
	s.Close()
	mu.Lock()
	defer mu.Unlock()
	if logged.Len() != 0 {
		t.Fatalf("askpass logs: %q", logged.String())
	}
}

type lockedWriter struct {
	mu *sync.Mutex
	w  *bytes.Buffer
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// helperThatLeaves sends one prompt with the right token, then closes its
// end: ssh gave up, or was killed.
func helperThatLeaves(t *testing.T, s *Server, prompt string) net.Conn {
	t.Helper()
	env := envOf(s.Env("ws-1"))
	c, err := net.Dial("unix", env(EnvSocket))
	if err != nil {
		t.Fatal(err)
	}
	req, _ := json.Marshal(request{Token: env(EnvToken), Host: "ws-1", Text: prompt})
	if _, err := c.Write(append(req, '\n')); err != nil {
		t.Fatal(err)
	}
	return c
}

// TestGoneHelperClosesPrompt: a prompt whose helper left after Next gave
// it closes its Gone.
func TestGoneHelperClosesPrompt(t *testing.T) {
	s := listen(t)
	c := helperThatLeaves(t, s, "Password: ")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, answer, err := s.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer answer("", false)
	select {
	case <-p.Gone:
		t.Fatal("Gone closed while the helper waits")
	case <-time.After(50 * time.Millisecond):
	}
	c.Close()
	select {
	case <-p.Gone:
	case <-time.After(5 * time.Second):
		t.Fatal("Gone did not close when the helper left")
	}
}

// TestGoneHelperSkipped: a prompt whose helper left before Next took it
// is not given once the server saw it leave; one Next took in the moment
// before comes with its Gone closed. Either way the next prompt is given.
func TestGoneHelperSkipped(t *testing.T) {
	s := listen(t)
	helperThatLeaves(t, s, "stale: ").Close()
	done := ask(envOf(s.Env("ws-1")), "fresh: ")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		p, answer, err := s.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if p.Text == "fresh: " {
			answer("a", true)
			break
		}
		select {
		case <-p.Gone:
		case <-ctx.Done():
			t.Fatalf("Next gave %q, and its Gone never closed", p.Text)
		}
		answer("", false)
	}
	<-done
}

// TestListenSweepsDeadDirs: a console that was killed leaves its prompt
// directory; the next one removes it. A directory whose socket answers is
// another console's, and stays; so does anything that is not a prompt
// directory.
func TestListenSweepsDeadDirs(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "ap")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	live, err := Listen(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	dead := filepath.Join(dir, "ask-deadbeef")
	if err := os.MkdirAll(dead, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dead, "s"), nil, 0o600); err != nil { // a socket file nobody listens on
		t.Fatal(err)
	}
	other := filepath.Join(dir, "ssh")
	if err := os.Mkdir(other, 0o700); err != nil {
		t.Fatal(err)
	}
	s, err := Listen(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := os.Stat(dead); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the dead console's directory stays: %v", err)
	}
	for _, keep := range []string{live.dir, other} {
		if _, err := os.Stat(keep); err != nil {
			t.Errorf("%s was removed", keep)
		}
	}
}
