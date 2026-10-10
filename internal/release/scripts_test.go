package release

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func script(t *testing.T, name string, args ...string) (string, error) {
	t.Helper()
	out, err := exec.Command(filepath.Join("..", "..", ".github", "scripts", name), args...).CombinedOutput()
	return string(out), err
}

func file(t *testing.T, s string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f.md")
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const unreleased = `# Changelog

Intro.

## [Unreleased]

### Added

- The stoker.

### Fixed

- A fix.

## [0.1.0] - 2026-10-12

- First.

[Unreleased]: https://github.com/blox-eng/matchblox/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/blox-eng/matchblox/releases/tag/v0.1.0
`

func TestReleaseNotesAreTheUnreleasedSection(t *testing.T) {
	out, err := script(t, "release-notes.sh", file(t, unreleased))
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if want := "### Added\n\n- The stoker.\n\n### Fixed\n\n- A fix.\n"; out != want {
		t.Fatalf("notes =\n%q\nwant\n%q", out, want)
	}
}

func TestReleaseNotesRefuseAnEmptySection(t *testing.T) {
	out, err := script(t, "release-notes.sh", file(t, "# Changelog\n\n## [Unreleased]\n\n## [0.1.0] - 2026-10-12\n\n- First.\n"))
	if err == nil || !strings.Contains(out, "nothing under [Unreleased]") {
		t.Fatalf("%v: %s", err, out)
	}
}

func TestPromoteTheFirstRelease(t *testing.T) {
	p := file(t, "# Changelog\n\n## [Unreleased]\n\n### Added\n\n- All of it.\n")
	if out, err := script(t, "promote-changelog.sh", "0.1.0", "2026-10-12", p); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	want := "# Changelog\n\n## [Unreleased]\n\n## [0.1.0] - 2026-10-12\n\n### Added\n\n- All of it.\n\n" +
		"[Unreleased]: https://github.com/blox-eng/matchblox/compare/v0.1.0...HEAD\n" +
		"[0.1.0]: https://github.com/blox-eng/matchblox/releases/tag/v0.1.0\n"
	if got := read(t, p); got != want {
		t.Fatalf("CHANGELOG =\n%s\nwant\n%s", got, want)
	}
}

func TestPromoteANextRelease(t *testing.T) {
	p := file(t, unreleased)
	if out, err := script(t, "promote-changelog.sh", "0.2.0", "2026-11-15", p); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	got := read(t, p)
	for _, want := range []string{
		"## [Unreleased]\n\n## [0.2.0] - 2026-11-15\n\n### Added\n",
		"[Unreleased]: https://github.com/blox-eng/matchblox/compare/v0.2.0...HEAD\n[0.2.0]: https://github.com/blox-eng/matchblox/compare/v0.1.0...v0.2.0\n[0.1.0]:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("no %q in:\n%s", want, got)
		}
	}
	if out, err := script(t, "promote-changelog.sh", "0.2.0", "2026-11-15", p); err != nil || read(t, p) != got {
		t.Fatalf("a second run changed it (%v: %s):\n%s", err, out, read(t, p))
	}
}

func TestPromoteRefusesWhatIsNoVersion(t *testing.T) {
	p := file(t, unreleased)
	for _, args := range [][]string{{"v0.2.0", "2026-11-15"}, {"0.2.0; rm -rf /", "2026-11-15"}, {"0.2.0", "15.11.2026"}} {
		if out, err := script(t, "promote-changelog.sh", append(args, p)...); err == nil {
			t.Fatalf("%q accepted: %s", args, out)
		}
	}
	if read(t, p) != unreleased {
		t.Fatal("a refused run changed the file")
	}
}

const flippable = `# matchblox

Intro.

<!-- until-release -->
> No release yet: the script builds from source.
<!-- /until-release -->
<!-- after-release
Install the verified release:
after-release -->

curl -fsSL https://matchblox.sh | sh
`

func TestFlipShowsTheReleaseText(t *testing.T) {
	p := file(t, flippable)
	if out, err := script(t, "flip-release.sh", p); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	want := "# matchblox\n\nIntro.\n\nInstall the verified release:\n\ncurl -fsSL https://matchblox.sh | sh\n"
	if got := read(t, p); got != want {
		t.Fatalf("flipped =\n%s\nwant\n%s", got, want)
	}
	if out, err := script(t, "flip-release.sh", p); err != nil || read(t, p) != want {
		t.Fatalf("a second flip changed it (%v: %s)", err, out)
	}
}

func TestFlipRefusesAnOpenMarker(t *testing.T) {
	for _, s := range []string{
		"a\n<!-- until-release -->\nb\n",
		"a\n<!-- after-release\nb\n",
		"a\n<!-- /until-release -->\n",
		"<!-- until-release -->\n<!-- after-release\nx\nafter-release -->\n<!-- /until-release -->\n",
	} {
		p := file(t, s)
		if out, err := script(t, "flip-release.sh", p); err == nil || read(t, p) != s {
			t.Fatalf("%q: flipped (%s):\n%s", s, out, read(t, p))
		}
	}
}

// The README, docs and site flip cleanly at the release, and the site has
// each install element once, before and after.
func TestTheRepositoryFlips(t *testing.T) {
	t.Setenv("VERSION", "v0.1.0")
	root := filepath.Join("..", "..")
	files, _ := filepath.Glob(filepath.Join(root, "docs", "*.md"))
	files = append(files, filepath.Join(root, "README.md"), filepath.Join(root, "www", "index.html"))
	dir := t.TempDir()
	var copies []string
	for i, f := range files {
		p := filepath.Join(dir, fmt.Sprintf("%d-%s", i, filepath.Base(f)))
		if err := os.WriteFile(p, []byte(read(t, f)), 0o644); err != nil {
			t.Fatal(err)
		}
		copies = append(copies, p)
	}
	if out, err := script(t, "flip-release.sh", copies...); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	for _, p := range copies {
		if s := read(t, p); strings.Contains(s, "until-release") || strings.Contains(s, "after-release") {
			t.Fatalf("%s keeps a marker", p)
		}
	}
	site := func(s string) string { return regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllString(s, "") }
	before, after := site(read(t, files[len(files)-1])), site(read(t, copies[len(copies)-1]))
	for _, id := range []string{`id="t-script"`, `id="note"`, `class="news"`} {
		if strings.Count(before, id) != 1 || strings.Count(after, id) != 1 {
			t.Errorf("%s: %d before, %d after the flip", id, strings.Count(before, id), strings.Count(after, id))
		}
	}
	if strings.Contains(after, "builds with Go") || !strings.Contains(after, "checksum and provenance checked") || !strings.Contains(after, "v0.1.0 is out") {
		t.Error("the flipped site still says it builds with Go")
	}
}

func TestFlipNamesTheVersion(t *testing.T) {
	p := file(t, "<!-- after-release\n@VERSION@ is out\nafter-release -->\n")
	cmd := exec.Command(filepath.Join("..", "..", ".github", "scripts", "flip-release.sh"), p)
	cmd.Env = append(os.Environ(), "VERSION=v0.1.0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if got := read(t, p); got != "v0.1.0 is out\n" {
		t.Fatalf("flipped = %q", got)
	}
}

func TestFlipWithoutAVersionLeavesTheFileAlone(t *testing.T) {
	s := "<!-- after-release\n@VERSION@ is out\nafter-release -->\n"
	p := file(t, s)
	cmd := exec.Command(filepath.Join("..", "..", ".github", "scripts", "flip-release.sh"), p)
	cmd.Env = append(os.Environ(), "VERSION=")
	if out, err := cmd.CombinedOutput(); err == nil || read(t, p) != s || !strings.Contains(string(out), "VERSION") {
		t.Fatalf("%v: %s\n%s", err, out, read(t, p))
	}
}
