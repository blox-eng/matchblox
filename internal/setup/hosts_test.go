package setup

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/doors"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// withHosts is a machine whose ~/.ssh/config names hosts, with a config
// file from the first run.
func withHosts(t *testing.T, sshConfig string) Env {
	t.Helper()
	e := env(t)
	e.Config = filepath.Join(e.Home, ".config", "matchblox", "config.toml")
	writeFile(t, filepath.Join(e.Home, ".ssh", "config"), sshConfig)
	if _, err := WriteConfig(e.Config, 8, 16<<30); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestSSHHostsSkipsPatternsAndFollowsInclude(t *testing.T) {
	e := withHosts(t, `# my hosts
Include config.d/*
Host ws-1 ws-2
  HostName 10.0.0.2
host *.internal
Host !bad ws-3
Host *
  ServerAliveInterval 30
Match host ws-9
  User x
HOST ws-1
`)
	writeFile(t, filepath.Join(e.Home, ".ssh", "config.d", "work"), "Host build-1\n")
	got := SSHHosts(e.Home)
	want := []string{"build-1", "ws-1", "ws-2", "ws-3"}
	if !slices.Equal(got, want) {
		t.Fatalf("SSHHosts = %q, want %q", got, want)
	}
}

func TestSSHHostsIncludeCannotLoop(t *testing.T) {
	e := withHosts(t, "Include config\nHost ws-1\n")
	if got := SSHHosts(e.Home); !slices.Equal(got, []string{"ws-1"}) {
		t.Fatalf("SSHHosts = %q", got)
	}
}

func TestNoSSHHostsNoDoor(t *testing.T) {
	e := withHosts(t, "Host *\n  User me\n")
	for _, d := range Doors(e) {
		if d.ID == doors.Hosts {
			t.Fatalf("a hosts door with no host to pick: %+v", d)
		}
	}
}

func TestHostsDoorOffersEachHost(t *testing.T) {
	e := withHosts(t, "Host ws-1 ws-2\n")
	d := door(t, e, doors.Hosts)
	if !slices.Equal(d.Choices, []string{"ws-1", "ws-2"}) || d.Path != e.Config || d.Sum == "" || d.Done {
		t.Fatalf("door %+v", d)
	}
	if i := slices.Index(ids(Doors(e)), doors.Hosts); i != slices.Index(ids(Doors(e)), doors.WayBack)+1 {
		t.Fatalf("the hosts door comes after the way back: %q", ids(Doors(e)))
	}
}

func TestAddHostsAppendsAndKeepsTheBuildersEdits(t *testing.T) {
	e := withHosts(t, "Host ws-1 ws-2 ws-3\n")
	// The builder's own comment and value, which a TOML rewrite would lose.
	old, _ := os.ReadFile(e.Config)
	edited := strings.Replace(string(old), "load1_over = 8", "load1_over = 12 # my box runs hot", 1)
	writeFile(t, e.Config, edited)
	d := door(t, e, doors.Hosts)
	backup, err := Open(e, doors.Hosts, d.Sum, []string{"ws-3", "ws-1"})
	if err != nil {
		t.Fatal(err)
	}
	if backup == "" {
		t.Fatal("no backup")
	}
	got, _ := os.ReadFile(e.Config)
	if !strings.HasPrefix(string(got), edited) {
		t.Fatalf("the builder's lines changed:\n%s", got)
	}
	if want := doors.HostsBlock([]string{"ws-1", "ws-3"}); !strings.HasSuffix(string(got), want) {
		t.Fatalf("want the block %q at the end:\n%s", want, got)
	}
	cfg, err := config.Load(e.Config)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cfg.Remote.Hosts, []string{"ws-1", "ws-3"}) || cfg.Alerts.Load1Over != 12 {
		t.Fatalf("config %+v %+v", cfg.Remote, cfg.Alerts)
	}
	if d := door(t, e, doors.Hosts); !d.Done {
		t.Fatalf("the door must fold when hosts are set: %+v", d)
	}
}

func TestAddHostsRefusesAHostNotInTheList(t *testing.T) {
	e := withHosts(t, "Host ws-1\n")
	d := door(t, e, doors.Hosts)
	if _, err := Open(e, doors.Hosts, d.Sum, []string{"ws-1", "evil\"]\n[x"}); err == nil {
		t.Fatal("wrote a host that is not in ~/.ssh/config")
	}
	if _, err := Open(e, doors.Hosts, d.Sum, nil); err == nil {
		t.Fatal("wrote no host")
	}
}

func TestAddHostsRefusesAChangedFile(t *testing.T) {
	e := withHosts(t, "Host ws-1\n")
	d := door(t, e, doors.Hosts)
	writeFile(t, e.Config, "[alerts]\nload1_over = 3\n")
	if _, err := Open(e, doors.Hosts, d.Sum, []string{"ws-1"}); !errors.Is(err, ErrChanged) {
		t.Fatalf("err = %v, want ErrChanged", err)
	}
}

// A [remote] table the builder wrote without hosts cannot get a second
// one: the door says where to add them.
func TestHostsDoorWithARemoteTableSaysWhere(t *testing.T) {
	e := withHosts(t, "Host ws-1\n")
	writeFile(t, e.Config, "[remote]\n# later\n")
	d := door(t, e, doors.Hosts)
	if d.Problem == "" || d.Sum != "" || !strings.Contains(d.Problem, "[remote]") {
		t.Fatalf("door %+v", d)
	}
}

func TestHostsDoorDoneWhenHostsAreSet(t *testing.T) {
	e := withHosts(t, "Host ws-1\n")
	writeFile(t, e.Config, "[remote]\nhosts = [\"ws-1\"]\n")
	if d := door(t, e, doors.Hosts); !d.Done {
		t.Fatalf("door %+v", d)
	}
}

func TestHostsDoorOnABrokenConfigSaysFix(t *testing.T) {
	e := withHosts(t, "Host ws-1\n")
	writeFile(t, e.Config, "[alerts\n")
	if d := door(t, e, doors.Hosts); d.Problem == "" || d.Sum != "" {
		t.Fatalf("door %+v", d)
	}
}
