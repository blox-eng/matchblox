package app

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/askpass"
)

// oneAsk is a prompt socket that asks prompt once, if set, then waits.
type oneAsk struct {
	prompt  *askpass.Prompt
	answers chan string
}

func (a *oneAsk) Next(ctx context.Context) (askpass.Prompt, func(string, bool), error) {
	if p := a.prompt; p != nil {
		a.prompt = nil
		return *p, func(s string, ok bool) { a.answers <- s }, nil
	}
	<-ctx.Done()
	return askpass.Prompt{}, nil, ctx.Err()
}

func (a *oneAsk) Env(host string) []string { return fakeAsk{}.Env(host) }

// connectRig is Hosts with a prompt socket: the connect of ws-2 runs ssh,
// which waits for the prompt's answer when there is one, then ends with
// stderr and exit.
func connectRig(t *testing.T, prompt *askpass.Prompt, stderr string, exit int) (*shellRig, *oneAsk, *[]sshCall, tea.Cmd) {
	t.Helper()
	r := newShell(t, []string{"ws-1"}, []string{"ws-1", "ws-2"}, "", false)
	ask := &oneAsk{prompt: prompt, answers: make(chan string, 1)}
	calls := &[]sshCall{}
	r.sh.opt.Base.Ask = ask
	r.sh.opt.Base.SSH = func(_ context.Context, argv, env []string, _ func(string)) (string, int) {
		*calls = append(*calls, sshCall{argv, env})
		if prompt != nil {
			select {
			case <-ask.answers:
			case <-time.After(5 * time.Second):
				return "no answer", 255
			}
		}
		if exit == 0 {
			r.added = append(r.added, argv[2]) // what matchblox connect writes
		}
		return stderr, exit
	}
	r.key(t, "down")
	r.key(t, "down")
	r.key(t, "enter") // + add a host
	r.key(t, "enter") // ws-2
	next, cmd := key(r.sh, "y")
	r.sh = next.(Shell)
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("the connect starts %T, want the prompts' wait and the ssh", cmd())
	}
	if prompt != nil {
		next, _ = r.sh.Update(batch[0]())
		r.sh = next.(Shell)
	}
	return r, ask, calls, batch[1]
}

// finish feeds the connect's end to the shell, then what it asks for.
func (r *shellRig) finish(t *testing.T, run tea.Cmd) {
	t.Helper()
	done := make(chan tea.Msg, 1)
	go func() { done <- run() }()
	select {
	case msg := <-done:
		next, cmd := r.sh.Update(msg)
		r.sh = next.(Shell)
		r.drain(cmd)
	case <-time.After(10 * time.Second):
		t.Fatal("the connect did not end")
	}
}

// TestConnectAsksInTheConsole: a connect from Hosts runs in the
// background, and its ssh's host key question shows in the field over
// Hosts; the typed yes goes to ssh, then the host opens (#242).
func TestConnectAsksInTheConsole(t *testing.T) {
	p := askpass.Prompt{Host: "ws-2", Confirm: true, Text: "The authenticity of host 'ws-2' can't be established.\nED25519 key fingerprint is SHA256:abc.\nAre you sure you want to continue connecting (yes/no/[fingerprint])?"}
	r, _, calls, run := connectRig(t, &p, "", 0)
	if len(r.ran) != 0 {
		t.Fatalf("the connect took the terminal: %q", r.ran)
	}
	out := r.view()
	if !strings.Contains(out, "ssh ws-2 asks") || !strings.Contains(out, "SHA256:abc") {
		t.Fatalf("no field with the host key over Hosts:\n%s", out)
	}
	m := tea.Model(r.sh)
	m = typeIn(m, "yes")
	m, _ = key(m, "enter")
	r.sh = m.(Shell)
	r.finish(t, run)
	if len(*calls) != 1 || strings.Join((*calls)[0].argv, " ") != "/bin/matchblox connect ws-2" ||
		!slices.Equal((*calls)[0].env, fakeAsk{}.Env("ws-2")) {
		t.Fatalf("ssh calls %q", *calls)
	}
	if r.sh.connecting != nil || r.sh.layer != layerConsole || r.opened[len(r.opened)-1] != "ws-2" {
		t.Fatalf("after the connect: field %v, layer %v, opened %q", r.sh.connecting != nil, r.sh.layer, r.opened)
	}
}

// TestConnectByKeyAsksNothing: a login by key, agent or Tailscale SSH has
// no prompt: no field opens, and the host opens when the connect ends.
func TestConnectByKeyAsksNothing(t *testing.T) {
	r, _, _, run := connectRig(t, nil, "", 0)
	if r.sh.connecting.ask != nil {
		t.Fatal("a field opened with no prompt")
	}
	r.finish(t, run)
	if r.sh.layer != layerConsole || r.opened[len(r.opened)-1] != "ws-2" {
		t.Fatalf("after the connect: layer %v, opened %q", r.sh.layer, r.opened)
	}
}

// TestConnectFailureSaysItsFix: a connect that fails stays on Hosts, and
// says the last line matchblox connect printed.
func TestConnectFailureSaysItsFix(t *testing.T) {
	r, _, _, run := connectRig(t, nil, "debug1: Reading configuration data\nmatchblox: ssh ws-2: exit status 255\n", 1)
	r.finish(t, run)
	if r.sh.layer != layerHosts || r.sh.connecting != nil {
		t.Fatalf("a failed connect left Hosts: layer %v", r.sh.layer)
	}
	if want := "not connected: ssh ws-2: exit status 255"; r.sh.flash != want {
		t.Fatalf("flash %q\nwant %q", r.sh.flash, want)
	}
}

// TestConnectFailureIsPlainText: the host can write the connect's last
// line; no escape of it reaches the screen.
func TestConnectFailureIsPlainText(t *testing.T) {
	r, _, _, run := connectRig(t, nil, "matchblox: ssh ws-2: \x1b]52;c;cHduZWQ=\x07bad\u202e host\n\ndebug1: Exit status 1\n", 1)
	r.finish(t, run)
	if want := "not connected: ssh ws-2: ]52;c;cHduZWQ=bad host"; r.sh.flash != want {
		t.Fatalf("flash %q\nwant %q", r.sh.flash, want)
	}
}
