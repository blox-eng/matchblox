package release

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var targets = []string{"darwin-arm64", "darwin-amd64", "linux-arm64", "linux-amd64"}

// sums is the directory of .sha256 files that publish.yml attaches: one
// distinct sum per target.
func sums(t *testing.T, skip string) string {
	t.Helper()
	dir := t.TempDir()
	for i, target := range targets {
		if target == skip {
			continue
		}
		s := fmt.Sprintf("%064x", i+1)
		if err := os.WriteFile(filepath.Join(dir, "matchblox-"+target+".sha256"), []byte(s+"  matchblox-"+target+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func sumOf(target string) string {
	for i, t := range targets {
		if t == target {
			return fmt.Sprintf("%064x", i+1)
		}
	}
	return ""
}

func TestPackagesInstallTheReleaseBinaries(t *testing.T) {
	out := t.TempDir()
	if o, err := script(t, "packages.sh", "v0.17.0", sums(t, ""), out); err != nil {
		t.Fatalf("%v: %s", err, o)
	}
	base := "https://github.com/blox-eng/matchblox/releases/download/v0.17.0/"
	formula := read(t, filepath.Join(out, "Formula", "matchblox.rb"))
	for _, target := range targets {
		// Each url has its own sum on the next line.
		if want := `url "` + base + "matchblox-" + target + "\"\n      sha256 \"" + sumOf(target) + `"`; !strings.Contains(formula, want) {
			t.Errorf("the formula lacks\n%s\nin\n%s", want, formula)
		}
	}
	for _, want := range []string{`license "MIT"`, `depends_on "tmux"`, `=> "matchblox"`, `assert_equal "v#{version}"`} {
		if !strings.Contains(formula, want) {
			t.Errorf("the formula lacks %s", want)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "aur")); !os.IsNotExist(err) {
		t.Errorf("packages.sh wrote an aur/ directory: %v", err)
	}
}

func TestPackagesRefuseAPreRelease(t *testing.T) {
	out := t.TempDir()
	o, err := script(t, "packages.sh", "v0.17.0-rc.1", sums(t, ""), out)
	if err == nil || !strings.Contains(o, "a pre-release gets no package") {
		t.Fatalf("%v: %s", err, o)
	}
	if left, _ := os.ReadDir(out); len(left) != 0 {
		t.Fatalf("wrote %v", left)
	}
}

func TestPackagesRefuseAMissingSum(t *testing.T) {
	out := t.TempDir()
	o, err := script(t, "packages.sh", "v0.17.0", sums(t, "linux-arm64"), out)
	if err == nil || !strings.Contains(o, "matchblox-linux-arm64.sha256") {
		t.Fatalf("%v: %s", err, o)
	}
	if left, _ := os.ReadDir(out); len(left) != 0 {
		t.Fatalf("wrote %v", left)
	}
}

func TestPackagesRefuseABadSum(t *testing.T) {
	dir := sums(t, "")
	if err := os.WriteFile(filepath.Join(dir, "matchblox-darwin-amd64.sha256"), []byte("Not Found\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	o, err := script(t, "packages.sh", "v0.17.0", dir, t.TempDir())
	if err == nil || !strings.Contains(o, "holds no sha256") {
		t.Fatalf("%v: %s", err, o)
	}
}
