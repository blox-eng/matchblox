package askpass

import (
	"bytes"
	"context"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func listen(t *testing.T) *Server {
	t.Helper()
	dir, err := os.MkdirTemp("", "ap") // short: the socket path must fit sun_path
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
	if p != (Prompt{Host: "ws-1", Text: "Password: "}) {
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
}

func TestCleanPrompt(t *testing.T) {
	for in, want := range map[string]string{
		"\x1b]0;pwn\x07Password: \x1b[31m":         "Password: ",
		"\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\": "link",
		"a\u009b31mb\u009d0;t\u009cc\x00d\x7fe":    "abcde",
		"line one\nline two\r\t":                   "line one line two  ",
		"\u202eevil":                               "evil",
		"bad \xff utf8":                            "bad  utf8",
	} {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
	if got := []rune(Clean(strings.Repeat("é", 500))); len(got) != 200 {
		t.Errorf("a 500-rune prompt keeps %d runes, want 200", len(got))
	}
}

func TestConfirmPrompt(t *testing.T) {
	s := listen(t)
	done := ask(envOf(s.Env("ws-1")), "Are you sure you want to continue connecting (yes/no/[fingerprint])? ")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, answer, err := s.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	answer("yes", true)
	<-done
	if !p.Confirm {
		t.Fatalf("prompt %+v is a yes/no prompt", p)
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
