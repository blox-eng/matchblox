package release

import (
	"os"
	"os/exec"
	"path/filepath"
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
