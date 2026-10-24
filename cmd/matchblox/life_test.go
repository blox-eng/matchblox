package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/accounts"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/transport"
)

func TestKillAndRenameNameThePane(t *testing.T) {
	a, err := lifeAct(cliDoc(), "kill", []string{"api"}, false, "%9")
	if err != nil || a.RecID != "kill:%3" || a.Text != "" || a.Confirm != "y" || a.From != "%9" {
		t.Fatalf("%+v %v", a, err)
	}
	if a, err = lifeAct(cliDoc(), "kill", []string{"%4"}, true, ""); err != nil || a.Text != "anyway" {
		t.Fatalf("%+v %v", a, err)
	}
	if a, err = lifeAct(cliDoc(), "rename", []string{"api", "ships-flake"}, false, ""); err != nil || a.RecID != "rename:%3" || a.Text != "ships-flake" {
		t.Fatalf("%+v %v", a, err)
	}
}

func TestKillAndRenameRefusals(t *testing.T) {
	for name, c := range map[string]struct {
		cmd   string
		words []string
		want  string
	}{
		"kill nobody":   {"kill", nil, "kill needs a session: matchblox kill <who>"},
		"kill two":      {"kill", []string{"api", "web"}, "kill takes one session"},
		"rename bare":   {"rename", []string{"api"}, "rename needs a name: matchblox rename <who> <name>"},
		"rename spaces": {"rename", []string{"api", "a", "b"}, "a name is one word"},
		"rename bad":    {"rename", []string{"api", "a.b"}, "a name is letters, digits"},
		"unknown":       {"kill", []string{"nope"}, "no session named nope"},
	} {
		if _, err := lifeAct(cliDoc(), c.cmd, c.words, false, ""); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}

func TestNewTakesWhereAgentAndFirstMessage(t *testing.T) {
	a, err := newSessionAct([]string{"work", "on", "#88"}, newOpts{where: "/src/api", agent: "codex", branch: "feat/88", name: "n88"}, "%9", "/home/u")
	if err != nil || a.RecID != "new:session" || a.From != "%9" {
		t.Fatalf("%+v %v", a, err)
	}
	var n proto.NewSession
	if err := json.Unmarshal([]byte(a.Text), &n); err != nil {
		t.Fatal(err)
	}
	if n != (proto.NewSession{Where: "/src/api", Agent: "codex", Branch: "feat/88", Name: "n88", First: "work on #88"}) {
		t.Fatalf("%+v", n)
	}
	// Bare, it starts Claude Code here.
	if a, err = newSessionAct(nil, newOpts{}, "", "/home/u/src"); err != nil {
		t.Fatal(err)
	}
	var bare proto.NewSession
	if err := json.Unmarshal([]byte(a.Text), &bare); err != nil || bare != (proto.NewSession{Where: "/home/u/src", Agent: "claude"}) {
		t.Fatalf("%+v %v", bare, err)
	}
	// A relative --where is from here.
	if a, err = newSessionAct(nil, newOpts{where: "api"}, "", "/home/u/src"); err != nil || !strings.Contains(a.Text, `"where":"/home/u/src/api"`) {
		t.Fatalf("%s %v", a.Text, err)
	}
}

func TestSwitchNamesThePaneAndTheAgent(t *testing.T) {
	decode := func(a proto.Act) proto.SwitchAgent {
		t.Helper()
		var sw proto.SwitchAgent
		if err := json.Unmarshal([]byte(a.Text), &sw); err != nil {
			t.Fatal(err)
		}
		return sw
	}
	a, err := switchAct(cliDoc(), []string{"api"}, switchOpts{agent: "codex"}, "%9")
	if err != nil || a.RecID != "switch:%3" || a.Confirm != "y" || a.From != "%9" {
		t.Fatalf("%+v %v", a, err)
	}
	if sw := decode(a); sw != (proto.SwitchAgent{Agent: "codex", Handoff: proto.DefaultHandoff}) {
		t.Fatalf("%+v", sw)
	}
	doc := cliDoc()
	doc.Accounts = map[string][]string{"codex": {"default", "team"}}
	for last, want := range map[string]string{"team": "team", "": "default", "gone": "default"} {
		doc.LastAccount = map[string]string{"codex": last}
		if a, err = switchAct(doc, []string{"api"}, switchOpts{agent: "codex"}, ""); err != nil || decode(a).Account != want {
			t.Fatalf("last %q: %+v %v, want account %q", last, a, err, want)
		}
	}
	none := ""
	if a, err = switchAct(cliDoc(), []string{"api"}, switchOpts{agent: "codex", handoff: &none}, ""); err != nil || decode(a).Handoff != "" {
		t.Fatalf("%+v %v", a, err)
	}
	if a, err = switchAct(cliDoc(), []string{"api"}, switchOpts{agent: "codex", anyway: true}, ""); err != nil || !decode(a).Anyway {
		t.Fatalf("%+v %v", a, err)
	}
}

func TestSwitchRefusals(t *testing.T) {
	doc := cliDoc()
	doc.Sessions = append(doc.Sessions, sample.Session{Name: "ghost", SessionID: "s6"})
	for name, c := range map[string]struct {
		words []string
		o     switchOpts
		want  string
	}{
		"nobody":   {nil, switchOpts{agent: "codex"}, "switch needs a session: matchblox switch <who> --agent <name>"},
		"no agent": {[]string{"api"}, switchOpts{}, "switch needs an agent: --agent claude, codex, opencode or cursor-agent"},
		"two":      {[]string{"api", "web"}, switchOpts{agent: "codex"}, "switch takes one session: matchblox switch <who> --agent <name>"},
		"unknown":  {[]string{"nope"}, switchOpts{agent: "codex"}, "no session named nope"},
		"no pane":  {[]string{"ghost"}, switchOpts{agent: "codex"}, "ghost is outside tmux: nothing to switch"},
	} {
		if _, err := switchAct(doc, c.words, c.o, ""); err == nil || err.Error() != c.want {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}

// switchService serves cliDoc and answers every act with res.
func switchService(t *testing.T, res proto.Result) string {
	t.Helper()
	sock := isolate(t)
	l, err := transport.Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			nc, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				c := transport.NewConn(nc)
				defer c.Close()
				_ = c.Send(proto.KindHello, "", proto.Hello{Version: proto.Version})
				_ = c.Send(proto.KindSnapshot, "", proto.State{Doc: cliDoc()})
				for {
					env, err := c.Recv()
					if err != nil {
						return
					}
					if env.Kind == proto.KindAct {
						r := res
						r.ActID = env.ID
						_ = c.Send(proto.KindResult, env.ID, r)
					}
				}
			}()
		}
	}()
	return sock
}

func TestSwitchJumpsOnlyAfterItRan(t *testing.T) {
	var jumped [][]string
	jump := func(argv []string) error { jumped = append(jumped, argv); return nil }
	o := switchOpts{agent: "codex"}

	var buf bytes.Buffer
	sock := switchService(t, proto.Result{Ran: [][]string{{"tmux", "respawn-pane"}}})
	if err := cliSwitch([]string{"api"}, o, &buf, sock, jump); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "switched api to codex\n" || len(jumped) != 1 || !slices.Equal(jumped[0], []string{"tmux", "switch-client", "-t", "%3"}) {
		t.Fatalf("%q %v", buf.String(), jumped)
	}

	buf.Reset()
	if err := cliSwitch([]string{"api"}, o, &buf, sock, nil); err != nil || buf.String() != "switched api to codex\n" {
		t.Fatalf("%q %v", buf.String(), err)
	}

	jumped = nil
	sock = switchService(t, proto.Result{Err: "api works: y again switches it mid-turn", Refused: true})
	err := cliSwitch([]string{"api"}, o, &buf, sock, jump)
	if err == nil || err.Error() != "switch api: refused: api works: y again switches it mid-turn" || jumped != nil {
		t.Fatalf("%v %v", err, jumped)
	}

	buf.Reset()
	sock = switchService(t, proto.Result{Ran: [][]string{{"tmux"}}})
	failing := func([]string) error { return errors.New("no client") }
	if err := cliSwitch([]string{"api"}, o, &buf, sock, failing); err != nil || !strings.Contains(buf.String(), "switched, but not jumped: no client") {
		t.Fatalf("%q %v", buf.String(), err)
	}
}

func TestNewTakesAnAccount(t *testing.T) {
	a, err := newSessionAct(nil, newOpts{where: "/work/app", agent: "codex", account: "work"}, "", "/home/u")
	if err != nil {
		t.Fatal(err)
	}
	var n proto.NewSession
	if err := json.Unmarshal([]byte(a.Text), &n); err != nil {
		t.Fatal(err)
	}
	if n != (proto.NewSession{Where: "/work/app", Agent: "codex", Account: "work"}) {
		t.Fatalf("%+v", n)
	}
	// A new account is set up like the default unless --setup says otherwise.
	a, err = newSessionAct(nil, newOpts{where: "/work/app", account: "work", newAccount: true}, "", "/home/u")
	if err != nil || !strings.Contains(a.Text, `"new_account":true`) || !strings.Contains(a.Text, `"setup":"like-default"`) {
		t.Fatalf("%s %v", a.Text, err)
	}
	a, err = newSessionAct(nil, newOpts{account: "work", newAccount: true, setup: "copy"}, "", "/home/u")
	if err != nil || !strings.Contains(a.Text, `"setup":"copy"`) {
		t.Fatalf("%s %v", a.Text, err)
	}
}

func TestNewAccountNeedsAName(t *testing.T) {
	_, err := newSessionAct(nil, newOpts{newAccount: true}, "", "/home/u")
	if err == nil || !strings.Contains(err.Error(), "--new-account needs --account") {
		t.Fatalf("%v", err)
	}
}

func TestSetupNeedsNewAccount(t *testing.T) {
	_, err := newSessionAct(nil, newOpts{account: "work", setup: "copy"}, "", "/home/u")
	if err == nil || !strings.Contains(err.Error(), "--setup needs --new-account") {
		t.Fatalf("%v", err)
	}
	_, err = newSessionAct(nil, newOpts{account: "work", newAccount: true, setup: "bogus"}, "", "/home/u")
	if err == nil || !strings.Contains(err.Error(), "like-default, copy or empty") {
		t.Fatalf("%v", err)
	}
}

func TestNewAccountTakesNoFirstMessage(t *testing.T) {
	_, err := newSessionAct([]string{"fix", "it"}, newOpts{account: "work", newAccount: true}, "", "/home/u")
	if err == nil || !strings.Contains(err.Error(), "without a first message") {
		t.Fatalf("%v", err)
	}
}

// TestNewThatRanThoughLinksFailedSucceeds: a start whose shared entries did
// not all link ran: the command succeeds and prints the note.
func TestNewThatRanThoughLinksFailedSucceeds(t *testing.T) {
	var buf bytes.Buffer
	note := proto.NotLinked + "open /home/u/.claude: permission denied"
	if err := newOutcome(&buf, proto.Result{Ran: [][]string{{"tmux", "new-session"}}, Skipped: []string{note}}, false); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "tmux new-session\n"+note+"\n" {
		t.Fatalf("%q", buf.String())
	}
	if err := newOutcome(&buf, proto.Result{Skipped: []string{"tmux: a guard stopped it"}}, false); err == nil {
		t.Fatal("any other skip is a refusal")
	}
}

// TestSwitchThatRanThoughLinksFailedSaysSo: a switch whose account's new
// shared entries did not all link ran: it says so after it switched.
func TestSwitchThatRanThoughLinksFailedSaysSo(t *testing.T) {
	var buf bytes.Buffer
	note := proto.NotLinked + "open /home/u/.codex: permission denied"
	sock := switchService(t, proto.Result{Ran: [][]string{{"tmux", "respawn-pane"}}, Skipped: []string{note}})
	if err := cliSwitch([]string{"api"}, switchOpts{agent: "codex"}, &buf, sock, nil); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "switched api to codex\n"+note+"\n" {
		t.Fatalf("%q", buf.String())
	}
}

// newStart is the result of a start of session w2 on a new account.
func newStart(skipped ...string) proto.Result {
	return proto.Result{Ran: [][]string{{"tmux", "new-session", "-d", "-s", "w2", "-n", "w2", "-c", "/work/app", "env", "codex"}}, Skipped: skipped}
}

// TestCLINewAccountFailureIsNotRefused: a start that ran and failed reads
// failed; only the service's refusal reads refused.
func TestCLINewAccountFailureIsNotRefused(t *testing.T) {
	o := newOpts{agent: "codex", account: "w2", newAccount: true}
	for _, c := range []struct {
		res  proto.Result
		want string
	}{
		{proto.Result{Err: "open /home/u/.codex/config.toml: permission denied"}, "new: failed: open /home/u/.codex/config.toml: permission denied"},
		{proto.Result{Err: "tmux new-session: exit status 1", Ran: [][]string{{"git"}}}, "new: failed: tmux new-session: exit status 1"},
		{proto.Result{Err: "the codex account w2 is busy with another start: start it when that ends", Refused: true}, "new: refused: the codex account w2 is busy with another start: start it when that ends"},
		{proto.Result{Skipped: []string{"a guard stopped it"}}, "new: refused: a guard stopped it"},
	} {
		var buf bytes.Buffer
		if err := cliNew(nil, o, &buf, switchService(t, c.res)); err == nil || err.Error() != c.want {
			t.Errorf("%+v: %v, want %q", c.res, err, c.want)
		}
	}
}

// TestCLINewAccountSaysWhereToSignIn: a started new account names its
// session to sign in in, after the notes of what it shared and skipped.
func TestCLINewAccountSaysWhereToSignIn(t *testing.T) {
	shared := proto.StillShared + "/home/u/.matchblox/accounts/codex/w2/rules -> /etc/rules: outside the default"
	var buf bytes.Buffer
	if err := cliNew(nil, newOpts{agent: "codex", account: "w2", newAccount: true}, &buf, switchService(t, newStart(shared, proto.CopyGoesOn))); err != nil {
		t.Fatal(err)
	}
	want := "tmux new-session -d -s w2 -n w2 -c /work/app env codex\n" + shared + "\n" + proto.CopyGoesOn + "\nsign in in w2\n"
	if buf.String() != want {
		t.Fatalf("%q, want %q", buf.String(), want)
	}
	// A start on an account that is there signs in already.
	buf.Reset()
	if err := cliNew(nil, newOpts{agent: "codex", account: "w2"}, &buf, switchService(t, newStart())); err != nil || strings.Contains(buf.String(), "sign in") {
		t.Fatalf("%q %v", buf.String(), err)
	}
}

// TestNewWaitDerivesFromCopyLimit: a start that may copy waits the copy's
// time limit on top of an act's; any other start an act's.
func TestNewWaitDerivesFromCopyLimit(t *testing.T) {
	home := t.TempDir()
	long := accounts.CopyLimit.Time + actWait
	copyAcct := accounts.Folder(home, "codex", "cp")
	if err := os.MkdirAll(copyAcct, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(copyAcct, accounts.SetupFile), []byte("copy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.Prepare(home, "codex", accounts.Folder(home, "codex", "ld"), accounts.LikeDefault); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		o    newOpts
		want time.Duration
	}{
		{newOpts{agent: "codex", account: "nw", newAccount: true}, long},
		{newOpts{account: "nw", newAccount: true}, long},
		{newOpts{agent: "codex", account: "cp"}, long},
		{newOpts{agent: "codex", account: "ld"}, actWait},
		{newOpts{agent: "codex"}, actWait},
		{newOpts{agent: "codex", account: "~/.codex-old"}, actWait},
	} {
		if got := newWait(c.o, home); got != c.want {
			t.Errorf("%+v: %v, want %v", c.o, got, c.want)
		}
	}
}

// TestCLISwitchFailureIsNotRefused: a switch that ran and failed reads
// failed; only the service's refusal reads refused.
func TestCLISwitchFailureIsNotRefused(t *testing.T) {
	for _, c := range []struct {
		res  proto.Result
		want string
	}{
		{proto.Result{Err: "tmux respawn-pane: exit status 1", Ran: [][]string{{"tmux", "rename-window"}}}, "switch api: failed: tmux respawn-pane: exit status 1"},
		{proto.Result{Err: "api runs codex already", Refused: true}, "switch api: refused: api runs codex already"},
	} {
		var buf bytes.Buffer
		if err := cliSwitch([]string{"api"}, switchOpts{agent: "codex"}, &buf, switchService(t, c.res), nil); err == nil || err.Error() != c.want {
			t.Errorf("%+v: %v, want %q", c.res, err, c.want)
		}
	}
}

// TestSwitchWaitDerivesFromCopyLimit: a switch to an agent whose last
// account is a copy account may go on with its copy, and waits for it.
func TestSwitchWaitDerivesFromCopyLimit(t *testing.T) {
	home := t.TempDir()
	cp := accounts.Folder(home, "codex", "cp")
	if err := os.MkdirAll(cp, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cp, accounts.SetupFile), []byte("copy\nincomplete\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := cliDoc()
	d.LastAccount = map[string]string{"codex": "cp", "claude": "default"}
	for agent, want := range map[string]time.Duration{"codex": accounts.CopyLimit.Time + actWait, "claude": actWait, "opencode": actWait} {
		if got := switchWait(d, agent, home); got != want {
			t.Errorf("%s: %v, want %v", agent, got, want)
		}
	}
}

// TestCLINewStoppedAfterAStepSaysSo: a start a guard stopped after a step
// ran is neither refused nor failed: it says what ran.
func TestCLINewStoppedAfterAStepSaysSo(t *testing.T) {
	res := proto.Result{Ran: [][]string{{"git", "worktree", "add"}}, Skipped: []string{"tmux: a guard stopped it"}}
	var buf bytes.Buffer
	err := cliNew(nil, newOpts{agent: "codex"}, &buf, switchService(t, res))
	if want := "new: stopped after git worktree add: tmux: a guard stopped it"; err == nil || err.Error() != want {
		t.Fatalf("%v, want %q", err, want)
	}
}
