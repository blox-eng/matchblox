package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// The screens below are live captures (tmux, 120x40) of codex-cli
// 0.163.0 and Claude Code 2.1.308 on a fresh home, up to the login
// choice; the theme list is cut after its fourth option. Codex marks the
// selected option with an ASCII >, Claude Code with ❯.

// codexAt is Codex's sign-in picker with option n highlighted (0: none).
func codexAt(n int) string {
	opts := []string{
		"1. Sign in with ChatGPT\n     Usage included with Plus, Pro, Business, and Enterprise plans",
		"2. Sign in with Device Code\n     Sign in from another device with a one-time code",
		"3. Provide your own API key\n     Pay for what you use",
	}
	out := "  Welcome to Codex, OpenAI's command-line coding agent\n" +
		"  Sign in with ChatGPT to use Codex as part of your paid plan\n" +
		"  or connect an API key for usage-based billing\n"
	for i, o := range opts {
		mark := "  "
		if i+1 == n {
			mark = "> "
		}
		out += mark + o + "\n"
	}
	return out + "  Press enter to continue\n"
}

// claudeThemeOn is Claude Code's theme screen with the line of label
// highlighted; Dark mode carries the check mark of the current choice.
func claudeThemeOn(label string) string {
	out := " Let's get started.\n Choose the text style that looks best with your terminal\n To change this later, run /theme\n"
	for _, o := range []string{"Auto (match terminal)", "✔ Dark mode", "Light mode", "Dark mode (colorblind-friendly)"} {
		mark := "   "
		if strings.TrimPrefix(o, "✔ ") == label {
			mark = " ❯ "
		}
		if !strings.HasPrefix(o, "✔ ") {
			o = "  " + o
		}
		out += mark + o + "\n"
	}
	return out
}

// claudeLoginOn is Claude Code's login method screen with option n
// highlighted.
func claudeLoginOn(n int) string {
	out := " Select login method:\n"
	for i, o := range []string{
		"1. Claude account with subscription · Pro, Max, Team, or Enterprise",
		"2. Anthropic Console account · API usage billing",
		"3. 3rd-party platform · Amazon Bedrock, Microsoft Foundry, Google Vertex AI",
	} {
		mark := "   "
		if i+1 == n {
			mark = " ❯ "
		}
		out += mark + o + "\n"
	}
	return out
}

var (
	claudeTheme      = claudeThemeOn("Dark mode")
	claudeThemeLight = claudeThemeOn("Light mode")
	claudeLogin      = claudeLoginOn(1)
	claudeLoginTwo   = claudeLoginOn(2)
)

func TestSignInScreensAreTheLiveCaptures(t *testing.T) {
	const theme = " Let's get started.\n Choose the text style that looks best with your terminal\n To change this later, run /theme\n" +
		"     Auto (match terminal)\n ❯ ✔ Dark mode\n     Light mode\n     Dark mode (colorblind-friendly)\n"
	if claudeTheme != theme {
		t.Fatalf("theme fixture\n%s\nwant\n%s", claudeTheme, theme)
	}
	const codex = "  Welcome to Codex, OpenAI's command-line coding agent\n" +
		"  Sign in with ChatGPT to use Codex as part of your paid plan\n" +
		"  or connect an API key for usage-based billing\n" +
		"> 1. Sign in with ChatGPT\n     Usage included with Plus, Pro, Business, and Enterprise plans\n" +
		"  2. Sign in with Device Code\n     Sign in from another device with a one-time code\n" +
		"  3. Provide your own API key\n     Pay for what you use\n" +
		"  Press enter to continue\n"
	if codexAt(1) != codex {
		t.Fatalf("codex fixture\n%s\nwant\n%s", codexAt(1), codex)
	}
}

func TestSignInStepOfEachScreen(t *testing.T) {
	for _, c := range []struct {
		name, agent, screen string
		step                int
		ready               bool
	}{
		{"claude theme", "claude", claudeTheme, 0, true},
		{"claude theme, light picked", "claude", claudeThemeLight, 0, false},
		{"claude theme, colorblind picked", "claude", claudeThemeOn("Dark mode (colorblind-friendly)"), 0, false},
		{"claude theme, auto picked", "claude", claudeThemeOn("Auto (match terminal)"), 0, false},
		{"claude theme, older check mark after", "claude", "Choose the text style\n ❯ Dark mode ✔\n   Light mode", 0, true},
		{"claude login", "claude", claudeLogin, 1, true},
		{"claude login, console picked", "claude", claudeLoginTwo, 1, false},
		{"claude login, third-party picked", "claude", claudeLoginOn(3), 1, false},
		{"codex on its first option", "codex", codexAt(1), 0, true},
		{"codex on device code", "codex", codexAt(2), 1, true},
		{"codex on the api key", "codex", codexAt(3), 0, false},
		{"codex, nothing marked", "codex", codexAt(0), 0, false},
		{"codex drawn with ›", "codex", "Sign in with ChatGPT\n› 1. Sign in with ChatGPT\n  2. Sign in with Device Code", 0, true},
		{"codex, > without a space", "codex", "Sign in with ChatGPT\n>1. Sign in with ChatGPT", 0, false},
		{"codex, another > line", "codex", codexAt(1) + "> quoted text", 0, false},
		{"cursor", "cursor-agent", "Press any key to log in", 0, true},
		{"other claude", "claude", "Welcome back", -1, false},
		{"codex on a claude screen", "codex", claudeLogin, -1, false},
		{"unknown agent", "opencode", "Press any key to log in", -1, false},
		{"styled", "codex", "  \x1b[1mSign in with \x1b[36mChatGPT\x1b[0m\n\x1b[36m> 1. Sign in with ChatGPT\x1b[0m\n  2. Sign in with Device Code", 0, true},
		{"claude, both screens", "claude", "Choose the text style\n     Dark mode\n\n" + claudeLogin, 1, true},
		{"in a frame", "claude", "│ Select login method │\n│ ❯ 1. Claude account with subscription │", 1, true},
		{"two marks", "codex", "Sign in with ChatGPT\n> 1. Sign in with ChatGPT\n> 2. Sign in with Device Code", 0, false},
		{"the title as an option", "codex", "> Sign in with ChatGPT", 0, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			step, ready := signInStep(c.agent, c.screen)
			if step != c.step || ready != c.ready {
				t.Fatalf("step %d ready %v, want %d %v", step, ready, c.step, c.ready)
			}
		})
	}
}

// signInTest is a life test whose sign-in runs fast, with the steps it
// ran recorded, the screen it reads set by the test, and the new
// account's agent sampled in %42.
func signInTest(t *testing.T, screen func() string) (*Service, *[][]string) {
	t.Helper()
	s, ran := lifeTest(t)
	s.signInWait, s.signInEvery = 50*time.Millisecond, time.Millisecond
	s.Capture = func([]string) map[string]string { return map[string]string{"%42": screen()} }
	snap := lifeSnap(false)
	snap.Sessions = append(snap.Sessions, sample.Session{Agent: "codex", PID: 142, Start: 9, Pane: "%42", Target: "fresh:1.1", Name: "fresh"})
	snap.Panes = append(snap.Panes, sample.Pane{ID: "%42", Target: "fresh:1.1", Window: "fresh", Command: "codex"})
	s.publish(snap, func(state.Doc) {})
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

// agentFake is an agent's screen that answers the keys sent to it: on a
// key it has a screen for, it moves there. The builder's keys are the
// screens the test sets by hand.
type agentFake struct {
	mu     sync.Mutex
	screen string
	on     map[string]map[string]string // screen → key → next screen
}

func (f *agentFake) show() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.screen
}

func (f *agentFake) set(screen string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.screen = screen
}

// fakeSignIn wires f to a sign-in test: keys sent move its screen.
func fakeSignIn(t *testing.T, f *agentFake) (*Service, *[][]string) {
	t.Helper()
	s, _ := signInTest(t, f.show)
	var mu sync.Mutex
	var ran [][]string
	s.Actions.Run = func(argv []string) error {
		mu.Lock()
		ran = append(ran, argv)
		mu.Unlock()
		f.mu.Lock()
		defer f.mu.Unlock()
		if next, ok := f.on[f.screen][argv[len(argv)-1]]; ok {
			f.screen = next
		}
		return nil
	}
	return s, &ran
}

func TestSignInSendsKeysOnce(t *testing.T) {
	f := &agentFake{screen: codexAt(1), on: map[string]map[string]string{codexAt(1): {"Down": codexAt(2)}, codexAt(2): {"Enter": "Open this link"}}}
	s, ran := fakeSignIn(t, f)
	s.signInWait = time.Second
	s.driveSignIn(t.Context(), "%42", "codex", "fresh")
	if got := sentKeys(ran); !slices.Equal(got, []string{"send-keys -t %42 Down", "send-keys -t %42 Enter"}) {
		t.Fatalf("sent %q", got)
	}
	if n := s.State().SignIns; n != nil {
		t.Fatalf("notes %v", n)
	}
}

func TestSignInDrivesClaudeThroughItsTwoScreens(t *testing.T) {
	f := &agentFake{screen: claudeTheme, on: map[string]map[string]string{claudeTheme: {"Enter": claudeLogin}, claudeLogin: {"Enter": "Browser didn't open? Use the url below"}}}
	s, ran := fakeSignIn(t, f)
	s.signInWait = time.Second
	s.driveSignIn(t.Context(), "%42", "claude", "fresh")
	if got := sentKeys(ran); !slices.Equal(got, []string{"send-keys -t %42 Enter", "send-keys -t %42 Enter"}) {
		t.Fatalf("sent %q", got)
	}
	if n := s.State().SignIns; n != nil {
		t.Fatalf("notes %v", n)
	}
}

// TestSignInTakesTheLaterScreen: a screen that still shows the theme
// picker above the login method is at the login method; the driver sends
// its keys once and stops.
func TestSignInTakesTheLaterScreen(t *testing.T) {
	s, ran := signInTest(t, func() string {
		return "\x1b[2mChoose the text style\x1b[0m\n     Dark mode\n\n\x1b[1mSelect login\x1b[0m method:\n ❯ 1. Claude account with subscription · Pro, Max, Team, or Enterprise"
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

// TestSignInDoesNotPressAfterTheBuilderMoved: a key goes only when the
// option it picks, or moves from, is the highlighted one. The builder who
// moved the highlight first gets no key at all; one who moved it after
// the driver's Down gets no Enter.
func TestSignInDoesNotPressAfterTheBuilderMoved(t *testing.T) {
	t.Run("before the first key", func(t *testing.T) {
		s, ran := signInTest(t, func() string { return codexAt(3) })
		s.driveSignIn(t.Context(), "%42", "codex", "fresh")
		if got := sentKeys(ran); got != nil {
			t.Fatalf("sent %q", got)
		}
		if got := s.State().SignIns["%42"]; got != SignInByHand {
			t.Fatalf("note %q", got)
		}
	})
	t.Run("after the driver's Down", func(t *testing.T) {
		// The builder pressed Down too: the highlight is past device code.
		f := &agentFake{screen: codexAt(1), on: map[string]map[string]string{codexAt(1): {"Down": codexAt(3)}}}
		s, ran := fakeSignIn(t, f)
		s.signInWait = time.Second
		s.driveSignIn(t.Context(), "%42", "codex", "fresh")
		if got := sentKeys(ran); !slices.Equal(got, []string{"send-keys -t %42 Down"}) {
			t.Fatalf("sent %q", got)
		}
		if got := s.State().SignIns["%42"]; got != SignInByHand {
			t.Fatalf("note %q", got)
		}
	})
	t.Run("claude's theme", func(t *testing.T) {
		s, ran := signInTest(t, func() string { return claudeThemeLight })
		s.driveSignIn(t.Context(), "%42", "claude", "fresh")
		if got := sentKeys(ran); got != nil {
			t.Fatalf("sent %q", got)
		}
	})
}

// TestSignInStopsWhenTheBuilderTypes: once the pane changes in a way the
// driver's own keys do not explain, it sends nothing more, even when the
// screen comes back to one it would answer, and notes the pane.
func TestSignInStopsWhenTheBuilderTypes(t *testing.T) {
	t.Run("between two screens", func(t *testing.T) {
		// After the theme, the builder pressed Down at the login method,
		// then Up again.
		f := &agentFake{screen: claudeTheme, on: map[string]map[string]string{claudeTheme: {"Enter": claudeLoginTwo}}}
		s, ran := fakeSignIn(t, f)
		s.signInWait = 200 * time.Millisecond
		done := make(chan struct{})
		go func() { s.driveSignIn(t.Context(), "%42", "claude", "fresh"); close(done) }()
		deadline := time.Now().Add(5 * time.Second)
		for f.show() != claudeLoginTwo && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		time.Sleep(20 * time.Millisecond)
		f.set(claudeLogin)
		<-done
		if got := sentKeys(ran); !slices.Equal(got, []string{"send-keys -t %42 Enter"}) {
			t.Fatalf("sent %q", got)
		}
		if got := s.State().SignIns["%42"]; got != SignInByHand {
			t.Fatalf("note %q", got)
		}
	})
	t.Run("right before a key", func(t *testing.T) {
		// The screen is still for two looks, then the builder types: the
		// look right before the key shows it, and no key goes.
		var mu sync.Mutex
		looks := 0
		s, ran := signInTest(t, func() string {
			mu.Lock()
			defer mu.Unlock()
			looks++
			if looks <= 2 {
				return codexAt(1)
			}
			return codexAt(1) + "hello"
		})
		s.driveSignIn(t.Context(), "%42", "codex", "fresh")
		if got := sentKeys(ran); got != nil {
			t.Fatalf("sent %q", got)
		}
		if got := s.State().SignIns["%42"]; got != SignInByHand {
			t.Fatalf("note %q", got)
		}
	})
	t.Run("before any key", func(t *testing.T) {
		// The driver waits for the agent to be sampled; the builder moves
		// the highlight meanwhile and back.
		var mu sync.Mutex
		looks := 0
		s, ran := signInTest(t, func() string {
			mu.Lock()
			defer mu.Unlock()
			looks++
			if looks > 5 && looks <= 10 {
				return codexAt(2)
			}
			return codexAt(1)
		})
		s.publish(lifeSnap(false), func(state.Doc) {}) // not sampled yet
		snap := lifeSnap(false)
		snap.Sessions = append(snap.Sessions, sample.Session{Agent: "codex", PID: 142, Start: 9, Pane: "%42"})
		snap.Panes = append(snap.Panes, sample.Pane{ID: "%42"})
		go func() {
			for {
				mu.Lock()
				n := looks
				mu.Unlock()
				if n > 12 {
					s.publish(snap, func(state.Doc) {})
					return
				}
				time.Sleep(time.Millisecond)
			}
		}()
		s.signInWait = time.Second
		s.driveSignIn(t.Context(), "%42", "codex", "fresh")
		if got := sentKeys(ran); got != nil {
			t.Fatalf("sent %q", got)
		}
	})
}

// TestSignInSendsCarryGuards: each key carries the pid and start of the
// agent first sampled in the pane, so it never reaches another agent.
func TestSignInSendsCarryGuards(t *testing.T) {
	f := &agentFake{screen: claudeTheme, on: map[string]map[string]string{claudeTheme: {"Enter": claudeLogin}, claudeLogin: {"Enter": "Open this link"}}}
	s, ran := fakeSignIn(t, f)
	s.signInWait = time.Second
	var mu sync.Mutex
	var guards []advice.Guard
	s.Actions.Check = func(g advice.Guard) error {
		mu.Lock()
		defer mu.Unlock()
		guards = append(guards, g)
		return nil
	}
	s.driveSignIn(t.Context(), "%42", "claude", "fresh")
	want := advice.Guard{PID: 142, StartTicks: 9}
	if len(guards) != 2 || guards[0] != want || guards[1] != want {
		t.Fatalf("guards %+v", guards)
	}
	if got := sentKeys(ran); len(got) != 2 {
		t.Fatalf("sent %q", got)
	}

	t.Run("a guard that fails", func(t *testing.T) {
		s, ran := signInTest(t, func() string { return claudeTheme })
		s.Actions.Check = func(advice.Guard) error { return errors.New("pid 142 is no longer the process that was flagged") }
		s.driveSignIn(t.Context(), "%42", "claude", "fresh")
		if got := sentKeys(ran); got != nil {
			t.Fatalf("sent %q", got)
		}
		if got := s.State().SignIns["%42"]; got != SignInByHand {
			t.Fatalf("note %q", got)
		}
	})
	t.Run("another agent in the pane", func(t *testing.T) {
		f := &agentFake{screen: claudeTheme, on: map[string]map[string]string{claudeTheme: {"Enter": claudeLogin}}}
		s, ran := fakeSignIn(t, f)
		s.signInWait = time.Second
		s.Actions.Check = func(g advice.Guard) error {
			// The first key went; the pane's agent is replaced before the
			// next screen is read.
			snap := lifeSnap(false)
			snap.Sessions = append(snap.Sessions, sample.Session{Agent: "claude", PID: 143, Start: 11, Pane: "%42"})
			snap.Panes = append(snap.Panes, sample.Pane{ID: "%42"})
			s.publish(snap, func(state.Doc) {})
			return nil
		}
		s.driveSignIn(t.Context(), "%42", "claude", "fresh")
		if got := sentKeys(ran); !slices.Equal(got, []string{"send-keys -t %42 Enter"}) {
			t.Fatalf("sent %q", got)
		}
		if got := s.State().SignIns["%42"]; got != SignInByHand {
			t.Fatalf("note %q", got)
		}
	})
	t.Run("no agent sampled", func(t *testing.T) {
		s, ran := signInTest(t, func() string { return claudeTheme })
		s.publish(lifeSnap(false), func(state.Doc) {})
		s.driveSignIn(t.Context(), "%42", "claude", "fresh")
		if got := sentKeys(ran); got != nil {
			t.Fatalf("sent %q", got)
		}
	})
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
	s, ran := signInTest(t, func() string { return codexAt(1) + "Your code: " + code })
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
	s.Capture = func([]string) map[string]string { return map[string]string{"%42": claudeTheme} }
	snap := lifeSnap(false)
	snap.Sessions = append(snap.Sessions, sample.Session{Agent: "claude", PID: 142, Start: 9, Pane: "%42"})
	snap.Panes = append(snap.Panes, sample.Pane{ID: "%42"})
	s.publish(snap, func(state.Doc) {})
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

// TestSignInWaitsForASlowRedraw: an agent that redraws a while after the
// driver's key, past a still look at its old screen, is not the builder
// typing: the driver goes on.
func TestSignInWaitsForASlowRedraw(t *testing.T) {
	f := &agentFake{screen: claudeTheme}
	s, _ := signInTest(t, f.show)
	var mu sync.Mutex
	var ran [][]string
	s.Actions.Run = func(argv []string) error {
		mu.Lock()
		ran = append(ran, argv)
		mu.Unlock()
		if argv[len(argv)-1] == "Enter" {
			from := f.show()
			time.AfterFunc(20*s.signInEvery, func() {
				switch from {
				case claudeTheme:
					f.set(claudeLogin)
				case claudeLogin:
					f.set("Browser didn't open? Use the url below")
				}
			})
		}
		return nil
	}
	s.signInWait = 5 * time.Second
	s.driveSignIn(t.Context(), "%42", "claude", "fresh")
	mu.Lock()
	defer mu.Unlock()
	if got := sentKeys(&ran); !slices.Equal(got, []string{"send-keys -t %42 Enter", "send-keys -t %42 Enter"}) {
		t.Fatalf("sent %q", got)
	}
	if n := s.State().SignIns; n != nil {
		t.Fatalf("notes %v", n)
	}
}
