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
