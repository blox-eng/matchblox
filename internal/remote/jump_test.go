package remote

import (
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/sample"
)

func TestJumpRemoteToPane(t *testing.T) {
	j := Jump{Host: "ws-1", Pane: "%12", Prefix: "C-b", ID: "0a1b2c3d", Outer: true}
	const g = "'=matchblox-jump-0a1b2c3d:'"
	common := "tmux bind-key -T matchblox-jump-C-b C-b switch-client -T matchblox-jump-C-b-prefix ';'" +
		" bind-key -T matchblox-jump-C-b-prefix d detach-client ';'" +
		" bind-key -T matchblox-jump-C-b-prefix m detach-client ';'" +
		" new-session -d -t %12 -s matchblox-jump-0a1b2c3d ';'"
	tail := " set-option -t " + g + " @matchblox-jump 1 ';'" +
		" set-option -t " + g + " prefix None ';'" +
		" set-option -t " + g + " prefix2 None ';'" +
		" set-option -t " + g + " key-table matchblox-jump-C-b ';'" +
		" set-option -t " + g + " mouse off ';'" +
		" run-shell -C -t %12 'select-window -t =matchblox-jump-0a1b2c3d:#{window_id}' ';' select-pane -t %12 ';'" +
		" set-option -t " + g + " destroy-unattached on ';'" +
		" attach-session -t '=matchblox-jump-0a1b2c3d'"

	if got, want := Line(j.Remote()), common+" set-option -t "+g+" status off ';'"+tail; got != want {
		t.Errorf("outer:\n got %s\nwant %s", got, want)
	}
	j.Outer = false
	want := common + " set-option -t " + g + " status on ';'" +
		" set-option -t " + g + " status-left '◂ console: C-b m ' ';'" + tail
	if got := Line(j.Remote()); got != want {
		t.Errorf("not outer:\n got %s\nwant %s", got, want)
	}
}

func TestJumpRemoteToDir(t *testing.T) {
	j := Jump{Host: "ws-1", Dir: "/w/a", Prefix: "C-b", ID: "0a1b2c3d", Outer: true}
	line := Line(j.Remote())
	if !strings.Contains(line, "new-session -d -s matchblox-jump-0a1b2c3d -c /w/a ';'") {
		t.Errorf("no private session in the directory: %s", line)
	}
	for _, bad := range []string{" -t %", "select-window", "select-pane"} {
		if strings.Contains(line, bad) {
			t.Errorf("%q in a directory jump: %s", bad, line)
		}
	}
	if !strings.HasSuffix(line, "attach-session -t '=matchblox-jump-0a1b2c3d'") {
		t.Errorf("no attach: %s", line)
	}
}

func TestJumpLocal(t *testing.T) {
	const s = "'=matchblox-ws-1-0a1b2c3d:'"
	wrap := `/bin/sh -c 't=$1; h=$2; shift 2; trap '\''tmux unbind-key -a -T "$t" 2>/dev/null; exit 0'\'' HUP TERM; "$@"; s=$?; [ $s = 0 ] || { stty sane 2>/dev/null; printf "\nssh exited %s. Press Enter to go back.\n" "$s"; until read _; do sleep 0.2; done; }; for c in $(tmux list-clients -t "$TMUX_PANE" -F '\''#{client_tty}'\'' 2>/dev/null); do tmux switch-client -c "$c" -t "=$h" 2>/dev/null; done; tmux unbind-key -a -T "$t" 2>/dev/null; exit 0' sh matchblox-host-0a1b2c3d matchblox ssh x`
	back := "'unbind-key -a -T matchblox-host-0a1b2c3d ; switch-client -t =matchblox'"
	want := "tmux new-session -d -s matchblox-ws-1-0a1b2c3d -- " + wrap + " ';'" +
		" set-option -t " + s + " @matchblox-jump 1 ';'" +
		" set-option -t " + s + " detach-on-destroy off ';'" +
		" set-option -t " + s + " key-table matchblox-host-0a1b2c3d ';'" +
		" set-option -t " + s + " mouse on ';'" +
		" set-option -t " + s + " status-left '#[range=user|matchblox] ◂ console #[norange]' ';'" +
		" bind-key -T matchblox-host-0a1b2c3d User900 if -F 1 " + back + " '' ';'" +
		" bind-key -T matchblox-host-0a1b2c3d MouseDown1Status if -F '#{==:#{mouse_status_range},matchblox}' " + back + " '' ';'" +
		" switch-client -t '=matchblox-ws-1-0a1b2c3d' ';'" +
		" set-option -t " + s + " destroy-unattached on"
	j := Jump{Host: "ws-1", Pane: "%12", Prefix: "C-b", ID: "0a1b2c3d"}
	if got := Line(j.Local([]string{"ssh", "x"})); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	j.Home = "work"
	if got := Line(j.Local([]string{"ssh", "x"})); strings.Count(got, "; switch-client -t =work'") != 2 ||
		strings.Contains(got, "=matchblox ;") {
		t.Errorf("the way back ignores Home: %s", got)
	}
}

func TestHostSessionName(t *testing.T) {
	for host, want := range map[string]string{
		"ws-1":                 "matchblox-ws-1-0a1b2c3d",
		"me@build.example.com": "matchblox-me@build_example_com-0a1b2c3d",
		"[fe80::1]":            "matchblox-[fe80__1]-0a1b2c3d",
	} {
		got := HostSessionName(host, "0a1b2c3d")
		if got != want {
			t.Errorf("HostSessionName(%q) = %q, want %q", host, got, want)
		}
		if j := (Jump{Host: host, Pane: "%12", Prefix: "C-b", ID: "0a1b2c3d"}); !JumpAllowed(j) {
			t.Errorf("JumpAllowed refuses host %q", host)
		}
	}
}

func TestNewJumpID(t *testing.T) {
	a, b := NewJumpID(), NewJumpID()
	if !idRE.MatchString(a) || a == b {
		t.Errorf("ids %q and %q", a, b)
	}
}

func TestJumpAllowed(t *testing.T) {
	ok := Jump{Host: "ws-1", Pane: "%12", Prefix: "C-b", ID: "0a1b2c3d"}
	with := func(f func(*Jump)) Jump { j := ok; f(&j); return j }
	for name, c := range map[string]struct {
		j    Jump
		want bool
	}{
		"pane":            {ok, true},
		"dir":             {with(func(j *Jump) { j.Pane, j.Dir = "", "/w/a" }), true},
		"home":            {with(func(j *Jump) { j.Home = "work" }), true},
		"bad home":        {with(func(j *Jump) { j.Home = "a b" }), false},
		"home with dot":   {with(func(j *Jump) { j.Home = "a.b" }), false},
		"pane no percent": {with(func(j *Jump) { j.Pane = "12" }), false},
		"relative dir":    {with(func(j *Jump) { j.Pane, j.Dir = "", "w/a" }), false},
		"dir hash":        {with(func(j *Jump) { j.Pane, j.Dir = "", "/w/#(x)" }), false},
		"dir semicolon":   {with(func(j *Jump) { j.Pane, j.Dir = "", "/w;x" }), false},
		"both":            {with(func(j *Jump) { j.Dir = "/w/a" }), false},
		"neither":         {with(func(j *Jump) { j.Pane = "" }), false},
		"bad id":          {with(func(j *Jump) { j.ID = "XYZ" }), false},
		"bad host":        {with(func(j *Jump) { j.Host = "-oProxyCommand=x" }), false},
		"prefix C-a":      {with(func(j *Jump) { j.Prefix = "C-a" }), true},
		"prefix M-a":      {with(func(j *Jump) { j.Prefix = "M-a" }), true},
		"prefix ;":        {with(func(j *Jump) { j.Prefix = ";" }), false},
		"prefix format":   {with(func(j *Jump) { j.Prefix = "#{x}" }), false},
		"prefix empty":    {with(func(j *Jump) { j.Prefix = "" }), false},
	} {
		if got := JumpAllowed(c.j); got != c.want {
			t.Errorf("%s: JumpAllowed = %v, want %v", name, got, c.want)
		}
	}
}

func TestPrefixKey(t *testing.T) {
	for s, want := range map[string]bool{
		"C-b": true, "C-a": true, "M-a": true, "C-M-a": true, "`": false, "]": true, "C-]": true,
		"None": false, "C-bb": false, "": false, "C- ": false, "'": false,
	} {
		if got := PrefixKey(s); got != want {
			t.Errorf("PrefixKey(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestJumpSession(t *testing.T) {
	for name, want := range map[string]bool{
		"matchblox-jump-0a1b2c3d": true,  // a host's, made by a console without the mark
		"matchblox-ws-1-0a1b2c3d": false, // a builder may name a session so: the mark says a jump's
		"matchblox-api-deadbeef":  false,
		"matchblox":               false,
		"matchblox-notes":         false,
		"work":                    false,
	} {
		if got := sample.JumpSession(name); got != want {
			t.Errorf("JumpSession(%q) = %v, want %v", name, got, want)
		}
	}
}
