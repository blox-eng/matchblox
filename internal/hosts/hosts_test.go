package hosts

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestAddKeepsTheBuildersLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "matchblox", "hosts")
	if got := Read(path); got != nil {
		t.Fatalf("no file: %q", got)
	}
	if err := Add(path, "ws-1"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	own := string(b) + "# the build box\nbuild-1   \n"
	if err := os.WriteFile(path, []byte(own), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Add(path, "ws-2"); err != nil {
		t.Fatal(err)
	}
	if err := Add(path, "ws-1"); err != nil { // there already
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	if string(b) != own+"ws-2\n" {
		t.Fatalf("hosts:\n%s", b)
	}
	if got := Read(path); !slices.Equal(got, []string{"ws-1", "build-1", "ws-2"}) {
		t.Fatalf("Read = %q", got)
	}
}

func TestRemoveTakesOnlyItsLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(path, []byte("# mine\nws-1\nws-2 # the second\nws-12\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Remove(path, "ws-1"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "# mine\nws-2 # the second\nws-12\n" {
		t.Fatalf("hosts:\n%s", b)
	}
}

func TestReadSkipsWhatIsNotAHost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(path, []byte("ws-1\n-oProxyCommand=x\nws 1\n\n  # c\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Read(path); !slices.Equal(got, []string{"ws-1"}) {
		t.Fatalf("Read = %q", got)
	}
	if err := Add(path, "-oProxyCommand=x"); err == nil {
		t.Fatal("added an option as a host")
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSSHHostsSkipsPatternsAndFollowsInclude(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".ssh", "config"), `# my hosts
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
	writeFile(t, filepath.Join(home, ".ssh", "config.d", "work"), "Host build-1\n")
	if got, want := SSHHosts(home), []string{"build-1", "ws-1", "ws-2", "ws-3"}; !slices.Equal(got, want) {
		t.Fatalf("SSHHosts = %q, want %q", got, want)
	}
}

func TestSSHHostsIncludeCannotLoop(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".ssh", "config"), "Include config\nHost ws-1\n")
	if got := SSHHosts(home); !slices.Equal(got, []string{"ws-1"}) {
		t.Fatalf("SSHHosts = %q", got)
	}
}
