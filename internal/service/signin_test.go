package service

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/state"
)

func TestSignInKeysOfEachScreen(t *testing.T) {
	for _, c := range []struct {
		name, agent, screen string
		keys                []string
		ok                  bool
	}{
		{"claude theme", "claude", "Choose the text style\n 1. Dark", []string{"Enter"}, true},
		{"claude login", "claude", "Select login method\n 1. Claude account", []string{"Enter"}, true},
		{"codex", "codex", "  Sign in with ChatGPT\n", []string{"Down", "Enter"}, true},
		{"cursor", "cursor-agent", "Press any key to log in", []string{"Enter"}, true},
		{"other claude", "claude", "Welcome back", nil, false},
		{"codex on a claude screen", "codex", "Select login method", nil, false},
		{"unknown agent", "opencode", "Press any key to log in", nil, false},
		{"styled phrase", "codex", "  \x1b[1mSign in with \x1b[36mChatGPT\x1b[0m\n", []string{"Down", "Enter"}, true},
		{"claude, both screens", "claude", "Choose the text style\n 1. Dark\n\nSelect login method\n 1. Claude account", []string{"Enter"}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			keys, ok := signInKeys(c.agent, c.screen)
			if ok != c.ok || !slices.Equal(keys, c.keys) {
				t.Fatalf("keys %q ok %v", keys, ok)
			}
		})
	}
}

func TestSignInStepPrefersTheLaterScreen(t *testing.T) {
	if got := signInStep("claude", "Choose the text style\n 1. Dark\nSelect login method"); got != 1 {
		t.Fatalf("step %d, want 1: the login method", got)
	}
}

// signInTest is a life test whose sign-in runs fast, with the steps it
// ran recorded and the screen it reads set by the test.
func signInTest(t *testing.T, screen func() string) (*Service, *[][]string) {
	t.Helper()
	s, ran := lifeTest(t)
	s.signInWait, s.signInEvery = 50*time.Millisecond, time.Millisecond
	s.Capture = func([]string) map[string]string { return map[string]string{"%42": screen()} }
	return s, ran
}

func sentKeys(ran *[][]string) []string {
	var out []string
	for _, a := range *ran {
		if i := slices.Index(a, "send-keys"); i >= 0 {
			out = append(out, strings.Join(a[i:], " "))
		}
	}
	return out
}

func TestSignInSendsKeysOnce(t *testing.T) {
	s, ran := signInTest(t, func() string { return "Sign in with ChatGPT" })
	s.signInWait = time.Second
	s.driveSignIn(t.Context(), "%42", "codex", "fresh")
	var got []string
	for _, a := range *ran {
		if i := slices.Index(a, "send-keys"); i >= 0 {
			got = append(got, a[len(a)-1])
		}
	}
	if !slices.Equal(got, []string{"Down", "Enter"}) {
		t.Fatalf("sent %q from %q", got, *ran)
	}
	if n := s.State().SignIns; n != nil {
		t.Fatalf("notes %v", n)
	}
}

func TestSignInDrivesClaudeThroughItsTwoScreens(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	s, ran := signInTest(t, func() string {
		mu.Lock()
		defer mu.Unlock()
		calls++
		switch {
		case calls < 3:
			return "Choose the text style"
		default:
			return "Select login method"
		}
	})
	s.signInWait = time.Second
	s.driveSignIn(t.Context(), "%42", "claude", "fresh")
	if got := sentKeys(ran); !slices.Equal(got, []string{"send-keys -t %42 Enter", "send-keys -t %42 Enter"}) {
		t.Fatalf("sent %q", got)
	}
}

// TestSignInTakesTheLaterScreen: a screen that still shows the theme
// picker above the login method is at the login method; the driver sends
// its keys once and stops.
func TestSignInTakesTheLaterScreen(t *testing.T) {
	s, ran := signInTest(t, func() string {
		return "\x1b[2mChoose the text style\x1b[0m\n 1. Dark\n\n\x1b[1mSelect login\x1b[0m method\n 1. Claude account"
	})
	s.signInWait = time.Second
	s.driveSignIn(t.Context(), "%42", "claude", "fresh")
	if got := sentKeys(ran); !slices.Equal(got, []string{"send-keys -t %42 Enter"}) {
		t.Fatalf("sent %q", got)
	}
	if n := s.State().SignIns; n != nil {
		t.Fatalf("notes %v", n)
	}
}

func TestSignInGivesUpAndSaysSo(t *testing.T) {
	s, ran := signInTest(t, func() string { return "Welcome to something else" })
	s.driveSignIn(t.Context(), "%42", "claude", "fresh")
	if got := s.State().SignIns["%42"]; got != SignInByHand {
		t.Fatalf("note %q", got)
	}
	for _, a := range *ran {
		if slices.Contains(a, "send-keys") || slices.Contains(a, "kill-pane") {
			t.Fatalf("ran %q", a)
		}
	}
}

func TestSignInNoteGoesWithItsPane(t *testing.T) {
	s, _ := signInTest(t, func() string { return "" })
	s.signIns.set("%42", SignInByHand)
	s.publish(lifeSnap(false), func(state.Doc) {})
	if n := s.State().SignIns; n != nil {
		t.Fatalf("notes %v", n)
	}
}

func TestSignInStopsWithTheContext(t *testing.T) {
	s, _ := signInTest(t, func() string { return "nothing" })
	s.signInWait = time.Hour
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { s.driveSignIn(ctx, "%42", "claude", "fresh"); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the driver ran on")
	}
	if n := s.State().SignIns; n != nil {
		t.Fatalf("notes %v", n)
	}
}

func TestSignInNeverReadsTheCode(t *testing.T) {
	const code = "ABCD-12345"
	s, ran := signInTest(t, func() string { return "Sign in with ChatGPT\nYour code: " + code })
	s.signInWait = time.Second
	var logs strings.Builder
	s.driveSignIn(t.Context(), "%42", "codex", "fresh")
	logs.WriteString(strings.Join(flat(*ran), " "))
	for k, v := range s.State().SignIns {
		logs.WriteString(k + v)
	}
	if strings.Contains(logs.String(), code) {
		t.Fatalf("the code is in %q", logs.String())
	}
}

func flat(rows [][]string) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r...)
	}
	return out
}

func TestNewAccountDrivesItsSignIn(t *testing.T) {
	s, _, _ := accountTest(t)
	s.signInWait, s.signInEvery = time.Second, time.Millisecond
	var mu sync.Mutex
	var ran [][]string
	s.Actions.Run = func(argv []string) error {
		mu.Lock()
		defer mu.Unlock()
		ran = append(ran, argv)
		return nil
	}
	s.Capture = func([]string) map[string]string { return map[string]string{"%42": "Choose the text style"} }
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "fresh", Account: "work2", NewAccount: true}))
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := sentKeys(&ran)
		mu.Unlock()
		if len(got) > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no keys sent")
}
