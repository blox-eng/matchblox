package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

func TestVersionNamesTheBuild(t *testing.T) {
	module := func(v string) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) { return &debug.BuildInfo{Main: debug.Module{Version: v}}, true }
	}
	none := func() (*debug.BuildInfo, bool) { return nil, false }
	for _, c := range []struct {
		name, stamp string
		read        func() (*debug.BuildInfo, bool)
		want        string
	}{
		{"a release build keeps its stamp", "v0.1.0", module("(devel)"), "v0.1.0"},
		{"go install names its module version", "dev", module("v0.1.0"), "v0.1.0"},
		{"go install of a commit names its pseudo-version", "dev", module("v0.1.1-0.20261010120000-88d178b94d00"), "v0.1.1-0.20261010120000-88d178b94d00"},
		{"a build without its module version stays dev", "dev", module("(devel)"), "dev"},
		{"no build info stays dev", "dev", none, "dev"},
		{"an empty module version stays dev", "dev", module(""), "dev"},
	} {
		if got := buildVersion(c.stamp, c.read); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// The release build stamps the version with -X, and nothing overrides it.
func TestTheReleaseStampIsTheVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	bin := filepath.Join(t.TempDir(), "matchblox")
	build := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags", "-X main.version=v0.1.0", "-o", bin, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("%v:\n%s", err, out)
	}
	out, err := exec.Command(bin, "version").Output()
	if err != nil || strings.TrimSpace(string(out)) != "v0.1.0" {
		t.Fatalf("matchblox version = %q, %v", out, err)
	}
}
