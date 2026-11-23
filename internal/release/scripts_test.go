package release

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
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
	for _, id := range []string{`id="t-script"`, `id="t-brew"`, `id="t-go"`, `id="note"`, `class="news"`} {
		if strings.Count(before, id) != 1 || strings.Count(after, id) != 1 {
			t.Errorf("%s: %d before, %d after the flip", id, strings.Count(before, id), strings.Count(after, id))
		}
	}
	if strings.Contains(after, "builds with Go") || !strings.Contains(after, "checksum and provenance checked") || !strings.Contains(after, `<span class="tag">New</span>`) {
		t.Error("the flipped site still says it builds with Go, or lost its news line")
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

// TestDocsDeployFromOneSourceOfTruth: a tag deploys its docs; main deploys
// only before any release while releases are off. Pages keeps the first
// deployment of a commit, so main must never deploy the commit a release
// tags next (the first release's tag deploy was dropped that way).
func TestDocsDeployFromOneSourceOfTruth(t *testing.T) {
	for _, c := range []struct {
		event, ref, releases, tag, want string
	}{
		{"push", "refs/tags/v0.1.1", "true", "v0.1.0", "true"},
		{"push", "refs/heads/main", "", "", "true"},      // no releases yet, none on: main is the truth
		{"push", "refs/heads/main", "true", "", "false"}, // releases on: the tag of this commit deploys
		{"push", "refs/heads/main", "false", "v0.1.0", "false"},
		{"pull_request", "refs/tags/v0.1.1", "true", "", "false"},
		{"push", "refs/heads/feature", "", "", "false"},
	} {
		out, err := script(t, "docs-deploy.sh", c.event, c.ref, c.releases, c.tag)
		if err != nil || strings.TrimSpace(out) != c.want {
			t.Errorf("%+v: %q %v", c, out, err)
		}
	}
}

const site = `<main>
      <a class="news" href="https://github.com/blox-eng/matchblox/blob/main/CHANGELOG.md"><span class="tag">New</span> <span>v0.1.0 is out: one line installs it, verified</span> <span class="go">Changelog →</span></a>
</main>
`

const released = `# Changelog

## [Unreleased]

## [0.4.0] - 2026-10-11

### Added

- **See which sessions your plan's limit stopped, and until when.** A
  session whose turn the limit refused says ` + "`out`" + `.
- **Every stoker card is kept.**

## [0.3.0] - 2026-10-11

- **Older <news> & more.**
`

func TestTheNewsLineNamesTheRelease(t *testing.T) {
	cl, page := file(t, released), file(t, site)
	if out, err := script(t, "announce-release.sh", "0.4.0", cl, page); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	want := `      <a class="news" href="https://github.com/blox-eng/matchblox/releases/tag/v0.4.0"><span class="tag">New</span> <span>v0.4.0 · See which sessions your plan&#39;s limit stopped, and until when</span> <span class="go">Release notes →</span></a>`
	if got := read(t, page); !strings.Contains(got, want+"\n") || !strings.HasPrefix(got, "<main>\n") || !strings.HasSuffix(got, "</main>\n") {
		t.Fatalf("got\n%s\nwant the line\n%s", got, want)
	}
	// The headline is text: markup in it is escaped.
	if out, err := script(t, "announce-release.sh", "0.3.0", cl, page); err != nil || !strings.Contains(read(t, page), "Older &lt;news&gt; &amp; more") {
		t.Fatalf("%v: %s\n%s", err, out, read(t, page))
	}
}

func TestTheNewsLineNeedsAHeadline(t *testing.T) {
	page := file(t, site)
	for _, c := range []struct{ version, changelog string }{
		{"0.5.0", released}, // no such section
		{"0.4.0", "## [0.4.0] - 2026-10-11\n\n- A plain line.\n"},
	} {
		if out, err := script(t, "announce-release.sh", c.version, file(t, c.changelog), page); err == nil || !strings.Contains(out, "announce-release:") {
			t.Fatalf("%s: no error (%v): %s", c.version, err, out)
		}
		if read(t, page) != site {
			t.Fatalf("%s: the page changed", c.version)
		}
	}
	if out, err := script(t, "announce-release.sh", "0.4.0", file(t, released), file(t, "<main></main>\n")); err == nil || !strings.Contains(out, "news") {
		t.Fatalf("a page without the news line: %v %s", err, out)
	}
}

// A headline is its first bold run, even when it wraps or has code in it;
// the code marks go, the words stay.
func TestTheNewsLineTakesTheWholeHeadline(t *testing.T) {
	for changelog, want := range map[string]string{
		"## [0.6.0] - 2026-10-13\n\n- **A night that\n  wraps.** More.\n":               "v0.6.0 · A night that wraps<",
		"## [0.6.0] - 2026-10-13\n\n- **Run `matchblox setup` once.** More **bold**.\n": "v0.6.0 · Run matchblox setup once<",
	} {
		page := file(t, site)
		if out, err := script(t, "announce-release.sh", "0.6.0", file(t, changelog), page); err != nil || !strings.Contains(read(t, page), want) {
			t.Fatalf("%q: %v %s\n%s", changelog, err, out, read(t, page))
		}
	}
}

func TestTheNewsLineRefusesWhatIsNoVersion(t *testing.T) {
	page := file(t, site)
	if out, err := script(t, "announce-release.sh", `0.4.0"><script>`, file(t, released), page); err == nil || read(t, page) != site {
		t.Fatalf("took a bad version: %v %s", err, out)
	}
}

// The news line on the page is what announce-release.sh writes for the
// release: a hand edit, a reworded headline or a missed announce fails.
func TestTheNewsGuardKeepsTheLineWithTheChangelog(t *testing.T) {
	cl, page := file(t, released), file(t, site)
	if out, err := script(t, "news-guard.sh", "0.4.0", cl, page); err == nil || !strings.Contains(out, "news-guard:") {
		t.Fatalf("a stale line passed: %v %s", err, out)
	}
	if read(t, page) != site {
		t.Fatal("the guard changed the page")
	}
	if out, err := script(t, "announce-release.sh", "0.4.0", cl, page); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if out, err := script(t, "news-guard.sh", "0.4.0", cl, page); err != nil {
		t.Fatalf("an announced line failed: %v %s", err, out)
	}
	if out, err := script(t, "news-guard.sh", "0.4.0", file(t, "## [0.4.0] - 2026-10-11\n\n- A plain line.\n"), page); err == nil {
		t.Fatalf("a release without a headline passed: %s", out)
	}
}

// The site names the newest release in CHANGELOG.md, as announce-release.sh
// writes it.
func TestTheSiteNamesTheNewestRelease(t *testing.T) {
	root := filepath.Join("..", "..")
	m := regexp.MustCompile(`(?m)^## \[(\d+\.\d+\.\d+[^\]]*)\] - `).FindStringSubmatch(read(t, filepath.Join(root, "CHANGELOG.md")))
	if m == nil {
		t.Skip("nothing released yet")
	}
	if out, err := script(t, "news-guard.sh", m[1]); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

// ghCall is one run of the fake gh: its arguments, and the --body-file it
// was handed, read at the time of the call.
type ghCall struct {
	args []string
	body string
}

func (c ghCall) is(verb ...string) bool {
	return len(c.args) >= len(verb) && slices.Equal(c.args[:len(verb)], verb)
}

func (c ghCall) flag(name string) string {
	if i := slices.Index(c.args, name); i >= 0 && i+1 < len(c.args) {
		return c.args[i+1]
	}
	return ""
}

func (c ghCall) writes() bool { return !c.is("issue", "list") }

// canaryRun is one run of canary-report.sh on a results folder, with a fake
// gh first on PATH. The fake gh answers `issue list` with list.
type canaryRun struct {
	out   string
	code  int
	calls []ghCall
	dir   string // the folder the script ran in
}

func (r canaryRun) writes() []ghCall {
	var w []ghCall
	for _, c := range r.calls {
		if c.writes() {
			w = append(w, c)
		}
	}
	return w
}

func (r canaryRun) count(verb ...string) int {
	n := 0
	for _, c := range r.calls {
		if c.is(verb...) {
			n++
		}
	}
	return n
}

const fakeGH = `#!/bin/sh
n=$(wc -l <"$GH_LOG" | tr -d ' ')
{ for a in "$@"; do printf '%s\037' "$a"; done; printf '\n'; } >>"$GH_LOG"
prev=
for a in "$@"; do
  if [ "$prev" = --body-file ]; then cat "$a" >"$GH_LOG.body.$n"; fi
  prev=$a
done
if [ "$1 $2" = "issue list" ]; then cat "$GH_LIST"; fi
if [ "$1 $2" = "issue create" ]; then echo https://github.com/blox-eng/matchblox/issues/900; fi
`

// canaryResults is a results folder holding files, where every agent the
// files do not name reads well, so a test sees only the agents it names.
func canaryResults(t *testing.T, files map[string]string) string {
	t.Helper()
	all := map[string]string{
		"claude.json":   result("Claude Code", "2.1.330", true, "", "fine"),
		"codex.json":    result("Codex", "0.98.0", true, "", "fine"),
		"opencode.json": result("OpenCode", "1.2.2", true, "", "fine"),
	}
	maps.Copy(all, files)
	return resultsDir(t, all)
}

func resultsDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, s := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func canaryReport(t *testing.T, list string, args ...string) canaryRun {
	t.Helper()
	sh, err := filepath.Abs(filepath.Join("..", "..", ".github", "scripts", "canary-report.sh"))
	if err != nil {
		t.Fatal(err)
	}
	bin, run := t.TempDir(), t.TempDir()
	if list == "" {
		list = "[]"
	}
	log, listFile := filepath.Join(bin, "gh.log"), filepath.Join(bin, "list.json")
	for p, s := range map[string]string{log: "", listFile: list} {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(fakeGH), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(sh, args...)
	cmd.Dir = run
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "GH_LOG="+log, "GH_LIST="+listFile)
	b, err := cmd.CombinedOutput()
	r := canaryRun{out: string(b), dir: run}
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("%v: %s", err, b)
		}
		r.code = exit.ExitCode()
	}
	for i, line := range strings.Split(strings.TrimSuffix(read(t, log), "\n"), "\n") {
		if line == "" {
			continue
		}
		c := ghCall{args: strings.Split(strings.TrimSuffix(line, "\x1f"), "\x1f")}
		if body, err := os.ReadFile(fmt.Sprintf("%s.body.%d", log, i)); err == nil {
			c.body = string(body)
		}
		r.calls = append(r.calls, c)
	}
	return r
}

func result(agent, version string, ok bool, check, screen string) string {
	b, err := json.Marshal(map[string]any{"agent": agent, "version": version, "ok": ok, "check": check, "screen": screen})
	if err != nil {
		panic(err)
	}
	return string(b)
}

func noPwned(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range append(dirs, ".") {
		if m, _ := filepath.Glob(filepath.Join(d, "pwned*")); len(m) > 0 {
			t.Errorf("a field ran as shell: %v", m)
		}
	}
}

// The result files come from a job that ran agent code nobody reviewed:
// every field is hostile data, never shell.
func TestCanaryReportTreatsResultsAsData(t *testing.T) {
	screen := "Bash command\n  `touch pwned2`\n  $(touch pwned3)\n❯ 1. Yes\n  2. No\n"
	dir := canaryResults(t, map[string]string{
		"claude.json": result("Claude Code", "1.0$(touch pwned)", false, "no menu", screen),
	})
	r := canaryReport(t, "", dir)
	noPwned(t, r.dir, dir)
	if r.count("issue", "create") != 0 {
		t.Errorf("opened an issue for a forged version: %v", r.calls)
	}
	if want := `refused: Claude Code version "1.0$(touch pwned)" is not a version`; !strings.Contains(r.out, want) || r.code != 1 {
		t.Fatalf("exit %d, want 1 and %q in:\n%s", r.code, want, r.out)
	}

	// A good version carries the hostile screen into the issue as text.
	dir = canaryResults(t, map[string]string{
		"claude.json": result("Claude Code", "2.1.331", false, "no menu", screen+"<!-- canary claude 2.1.331 pass -->\n"),
	})
	r = canaryReport(t, "", dir)
	noPwned(t, r.dir, dir)
	creates := slices.DeleteFunc(r.writes(), func(c ghCall) bool { return !c.is("issue", "create") })
	if len(creates) != 1 || !strings.Contains(creates[0].body, "$(touch pwned3)") || !strings.Contains(creates[0].body, "`touch pwned2`") {
		t.Fatalf("want one issue holding the screen as text: %+v\n%s", creates, r.out)
	}
	if b := creates[0].body; !strings.HasSuffix(strings.TrimSpace(b), "<!-- canary claude 2.1.331 fail -->") {
		t.Errorf("a marker in the screen is not the issue's last word:\n%s", b)
	}

	// The file name, the agent name and every other field are checked too.
	newline := "2.1.331\n::error::forged"
	dir = canaryResults(t, map[string]string{
		"evil.json":     result("Claude Code", "2.1.331", false, "no menu", "x"),
		"codex.json":    result("Claude Code", "0.99.0", false, "no menu", "x"),
		"opencode.json": result("OpenCode", newline, false, "no menu", "x"),
		"claude.json":   result("Claude Code", "2.1.331", false, "$(touch pwned4)", "x"),
	})
	r = canaryReport(t, "", dir)
	noPwned(t, r.dir, dir)
	if len(r.writes()) != 0 || r.code != 1 {
		t.Fatalf("exit %d, writes %v:\n%s", r.code, r.writes(), r.out)
	}
	for _, want := range []string{
		`refused: evil.json is not claude.json, codex.json or opencode.json`,
		`refused: codex.json names agent "Claude Code", not Codex`,
		`refused: OpenCode version "2.1.331?::error::forged" is not a version`,
		`refused: Claude Code check "$(touch pwned4)" is not a check`,
	} {
		if !strings.Contains(r.out, want) {
			t.Errorf("want %q in:\n%s", want, r.out)
		}
	}
	if strings.Contains(r.out, "\n::error::") {
		t.Errorf("a field reached the log as a workflow command:\n%s", r.out)
	}
}

func openIssue(n int, title, body string, comments ...string) string {
	cs := []map[string]string{}
	for _, c := range comments {
		cs = append(cs, map[string]string{"body": c})
	}
	b, err := json.Marshal([]map[string]any{{"number": n, "title": title, "body": body, "comments": cs}})
	if err != nil {
		panic(err)
	}
	return string(b)
}

const claudeBreak = "Claude Code 2.1.331 draws a screen matchblox cannot read"

func TestCanaryReportOpensOneIssue(t *testing.T) {
	screen := "Bash command\n  echo ```` hi\n Do you want to proceed?\n❯ 1. Yes\n  2. No\n"
	dir := canaryResults(t, map[string]string{
		"claude.json": result("Claude Code", "2.1.331", false, "wrong options", screen),
		"codex.json":  result("Codex", "0.99.0", true, "", "fine"),
	})
	r := canaryReport(t, "", dir)
	if r.code != 1 {
		t.Fatalf("exit %d, want 1:\n%s", r.code, r.out)
	}
	w := r.writes()
	if len(w) != 2 || !w[0].is("label", "create", "canary") || !w[1].is("issue", "create") {
		t.Fatalf("want the label, then one issue: %v\n%s", r.calls, r.out)
	}
	if w[0].flag("--color") != "FBCA04" || !slices.Contains(w[0].args, "--force") {
		t.Errorf("label: %v", w[0].args)
	}
	c := w[1]
	if c.flag("--label") != "canary" || c.flag("--title") != claudeBreak || c.flag("--body-file") == "" {
		t.Errorf("create: %q", c.args)
	}
	for _, want := range []string{"2.1.331", "wrong options", "testdata/menus/claude-bash-2.1.331.txt", "`````\n" + screen + "`````\n", "<!-- canary claude 2.1.331 fail -->"} {
		if !strings.Contains(c.body, want) {
			t.Errorf("the body lacks %q:\n%s", want, c.body)
		}
	}
	if !slices.ContainsFunc(r.calls, func(c ghCall) bool {
		return c.is("issue", "list") && c.flag("--search") == "Claude Code in:title" && c.flag("--state") == "open" && c.flag("--label") == "canary"
	}) {
		t.Errorf("did not look for the open issue: %v", r.calls)
	}
}

func TestCanaryReportCommentsOnANewerVersion(t *testing.T) {
	dir := canaryResults(t, map[string]string{
		"claude.json": result("Claude Code", "2.1.332", false, "no menu", "screen"),
	})
	r := canaryReport(t, openIssue(12, claudeBreak, "body\n<!-- canary claude 2.1.331 fail -->\n"), dir)
	w := r.writes()
	if r.code != 1 || len(w) != 1 || !w[0].is("issue", "comment", "12") {
		t.Fatalf("exit %d, want one comment on #12: %v\n%s", r.code, r.calls, r.out)
	}
	if !strings.Contains(w[0].body, "Claude Code 2.1.332") || !strings.Contains(w[0].body, "<!-- canary claude 2.1.332 fail -->") {
		t.Errorf("comment:\n%s", w[0].body)
	}
	// The kind counts too: a break after a could-not-run is news.
	r = canaryReport(t, openIssue(12, claudeBreak, "<!-- canary claude 2.1.332 norun -->"), dir)
	if w := r.writes(); len(w) != 1 || !w[0].is("issue", "comment", "12") {
		t.Fatalf("want one comment on #12: %v\n%s", r.calls, r.out)
	}
}

func TestCanaryReportStaysQuietOnTheSameVersion(t *testing.T) {
	dir := canaryResults(t, map[string]string{
		"claude.json": result("Claude Code", "2.1.331", false, "no menu", "screen"),
	})
	// The last marker counts: the body's, then each comment's, in order.
	list := openIssue(12, claudeBreak, "<!-- canary claude 2.1.330 fail -->", "<!-- canary claude 2.1.330 pass -->", "a note\n<!-- canary claude 2.1.331 fail -->")
	r := canaryReport(t, list, dir)
	if r.code != 1 || len(r.writes()) != 0 {
		t.Fatalf("exit %d, want 1 and no write: %v\n%s", r.code, r.writes(), r.out)
	}
}

func TestCanaryReportSaysItReadsAgain(t *testing.T) {
	dir := canaryResults(t, map[string]string{
		"claude.json": result("Claude Code", "2.1.332", true, "", "screen"),
	})
	r := canaryReport(t, openIssue(12, claudeBreak, "<!-- canary claude 2.1.331 fail -->"), dir)
	w := r.writes()
	if r.code != 0 || len(w) != 1 || !w[0].is("issue", "comment", "12") {
		t.Fatalf("exit %d, want 0 and one comment on #12: %v\n%s", r.code, r.calls, r.out)
	}
	if want := "matchblox reads Claude Code 2.1.332 again.\n\n<!-- canary claude 2.1.332 pass -->\n"; w[0].body != want {
		t.Errorf("comment %q, want %q", w[0].body, want)
	}
	r = canaryReport(t, openIssue(12, claudeBreak, "<!-- canary claude 2.1.331 fail -->", w[0].body), dir)
	if r.code != 0 || len(r.writes()) != 0 {
		t.Fatalf("a second run: exit %d, writes %v\n%s", r.code, r.writes(), r.out)
	}
	if r := canaryReport(t, "", dir); r.code != 0 || len(r.writes()) != 0 {
		t.Fatalf("no open issue: exit %d, writes %v\n%s", r.code, r.writes(), r.out)
	}
}

func TestCanaryReportCouldNotRun(t *testing.T) {
	dir := canaryResults(t, map[string]string{
		"codex.json": result("Codex", "", false, "could not run", "codex is not installed\nexec: \"codex\": not found"),
	})
	r := canaryReport(t, "", dir)
	w := r.writes()
	if r.code != 1 || len(w) != 2 || !w[1].is("issue", "create") {
		t.Fatalf("exit %d, want 1 and one issue: %v\n%s", r.code, r.calls, r.out)
	}
	if got, want := w[1].flag("--title"), "the canary could not run Codex: codex is not installed"; got != want {
		t.Errorf("title %q, want %q", got, want)
	}
	if !strings.Contains(w[1].body, `exec: "codex": not found`) || !strings.Contains(w[1].body, "<!-- canary codex none norun -->") {
		t.Errorf("body:\n%s", w[1].body)
	}

	// A reason is cut to one short line of plain characters before it is a title.
	reason := "the stand-in got no request @owner `x` <b>" + strings.Repeat("ö long", 30)
	dir = canaryResults(t, map[string]string{
		"opencode.json": result("OpenCode", "1.2.3", false, "could not run", reason+"\nsecond line"),
	})
	r = canaryReport(t, "", dir)
	w = r.writes()
	if len(w) != 2 {
		t.Fatalf("want the label and one issue: %v\n%s", r.calls, r.out)
	}
	title := w[1].flag("--title")
	cut := strings.TrimPrefix(title, "the canary could not run OpenCode: ")
	if cut == title || len(cut) > 80 || !regexp.MustCompile(`^[A-Za-z0-9 .,:'()_/-]*$`).MatchString(cut) || !strings.HasPrefix(cut, "the stand-in got no request owner x b") {
		t.Errorf("title %q", title)
	}
	if !strings.Contains(w[1].body, "<!-- canary opencode 1.2.3 norun -->") {
		t.Errorf("body:\n%s", w[1].body)
	}
}

// An agent that hung or never ran writes no file: that is a could-not-run,
// never a green run.
func TestCanaryReportMissingAgentIsNotGreen(t *testing.T) {
	dir := resultsDir(t, map[string]string{
		"claude.json": result("Claude Code", "2.1.330", true, "", "fine"),
	})
	r := canaryReport(t, "", dir)
	if r.code != 1 {
		t.Fatalf("exit %d, want 1:\n%s", r.code, r.out)
	}
	w := r.writes()
	if len(w) != 3 || !w[0].is("label", "create", "canary") || !w[1].is("issue", "create") || !w[2].is("issue", "create") {
		t.Fatalf("want the label, then an issue for Codex and one for OpenCode: %v\n%s", r.calls, r.out)
	}
	for i, a := range []struct{ key, name string }{{"codex", "Codex"}, {"opencode", "OpenCode"}} {
		c := w[i+1]
		if got, want := c.flag("--title"), "the canary could not run "+a.name+": the canary wrote no result for "+a.name; got != want {
			t.Errorf("title %q, want %q", got, want)
		}
		if c.flag("--label") != "canary" || !strings.Contains(c.body, "<!-- canary "+a.key+" none norun -->") {
			t.Errorf("issue for %s: %q\n%s", a.name, c.args, c.body)
		}
	}

	// The same rules as any could-not-run: an open issue with the same marker hears nothing.
	r = canaryReport(t, openIssue(14, "the canary could not run Codex: the canary wrote no result for Codex", "<!-- canary codex none norun -->"), dir)
	if r.code != 1 || slices.ContainsFunc(r.writes(), func(c ghCall) bool { return c.is("issue", "comment", "14") }) ||
		!strings.Contains(r.out, "Codex none: still #14, nothing new") {
		t.Errorf("exit %d, writes %v:\n%s", r.code, r.writes(), r.out)
	}
}

func TestCanaryReportNeedsResults(t *testing.T) {
	const title = "the canary tested nothing"
	for _, dir := range []string{t.TempDir(), filepath.Join(t.TempDir(), "missing")} {
		r := canaryReport(t, "", dir)
		w := r.writes()
		if r.code != 1 || !strings.Contains(r.out, "no canary results: the canary tested nothing") ||
			len(w) != 2 || !w[0].is("label", "create", "canary") || !w[1].is("issue", "create") {
			t.Fatalf("%s: exit %d, want 1, the label and one issue: %v:\n%s", dir, r.code, r.calls, r.out)
		}
		if w[1].flag("--title") != title || w[1].flag("--label") != "canary" || !strings.Contains(w[1].body, "<!-- canary run none norun -->") {
			t.Errorf("%s: issue %q\n%s", dir, w[1].args, w[1].body)
		}

		// No second issue while one is open.
		r = canaryReport(t, openIssue(15, title, "<!-- canary run none norun -->"), dir)
		if r.code != 1 || len(r.writes()) != 0 || !strings.Contains(r.out, "still #15, nothing new") {
			t.Errorf("%s: with #15 open: exit %d, writes %v:\n%s", dir, r.code, r.writes(), r.out)
		}
	}
	if r := canaryReport(t, ""); r.code != 2 {
		t.Errorf("no folder: exit %d, want 2:\n%s", r.code, r.out)
	}
}

type workflowStep struct {
	Name            string            `yaml:"name"`
	ID              string            `yaml:"id"`
	If              string            `yaml:"if"`
	Uses            string            `yaml:"uses"`
	Run             string            `yaml:"run"`
	With            map[string]any    `yaml:"with"`
	Env             map[string]string `yaml:"env"`
	ContinueOnError bool              `yaml:"continue-on-error"`
}

type workflowJob struct {
	Needs       any               `yaml:"needs"`
	If          string            `yaml:"if"`
	Permissions map[string]string `yaml:"permissions"`
	Env         map[string]string `yaml:"env"`
	Steps       []workflowStep    `yaml:"steps"`
}

// The canary job installs and runs each agent's latest release, code nobody
// reviewed, so it holds no write token and no secret. Only the report job,
// which runs matchblox's own script, may write issues.
func TestCanaryWorkflowKeepsTheTokenFromAgents(t *testing.T) {
	raw := read(t, filepath.Join("..", "..", ".github", "workflows", "canary.yml"))
	var wf struct {
		Permissions map[string]string      `yaml:"permissions"`
		Jobs        map[string]workflowJob `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(raw), &wf); err != nil {
		t.Fatal(err)
	}
	contentsRead := map[string]string{"contents": "read"}
	if !maps.Equal(wf.Permissions, contentsRead) {
		t.Errorf("top-level permissions %v, want %v", wf.Permissions, contentsRead)
	}

	pinned := regexp.MustCompile(`^[\w.-]+/[\w./-]+@[0-9a-f]{40}$`)
	for name, job := range wf.Jobs {
		for _, s := range job.Steps {
			if s.Uses != "" && !pinned.MatchString(s.Uses) {
				t.Errorf("job %s: %q is not pinned to a commit", name, s.Uses)
			}
			if strings.HasPrefix(s.Uses, "actions/checkout@") && s.With["persist-credentials"] != false {
				t.Errorf("job %s: checkout keeps the token: persist-credentials %v", name, s.With["persist-credentials"])
			}
		}
	}
	// A cache the canary saves holds what agent code wrote, and CI's other
	// jobs on main restore it. setup-go caches unless told not to; setup-node
	// only when told to.
	for _, s := range wf.Jobs["canary"].Steps {
		if strings.HasPrefix(s.Uses, "actions/setup-go@") && s.With["cache"] != false {
			t.Errorf("job canary: setup-go saves a cache that agent code can poison: cache %v", s.With["cache"])
		}
		if strings.HasPrefix(s.Uses, "actions/setup-node@") && s.With["cache"] != nil {
			t.Errorf("job canary: setup-node saves a cache that agent code can poison: cache %v", s.With["cache"])
		}
		if strings.HasPrefix(s.Uses, "actions/cache") {
			t.Errorf("job canary: %s saves a cache that agent code can poison", s.Uses)
		}
	}
	for _, line := range strings.Split(raw, "\n") {
		if strings.Contains(line, "uses:") && !regexp.MustCompile(`@[0-9a-f]{40} # v\d`).MatchString(line) {
			t.Errorf("%q names no version after its pin", strings.TrimSpace(line))
		}
	}

	canary, ok := wf.Jobs["canary"]
	if !ok {
		t.Fatal("no job canary")
	}
	if !maps.Equal(canary.Permissions, contentsRead) {
		t.Errorf("job canary permissions %v, want only %v", canary.Permissions, contentsRead)
	}
	b, err := yaml.Marshal(canary)
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"secrets.", "github.token", "GH_TOKEN", "GITHUB_TOKEN"} {
		if strings.Contains(string(b), word) {
			t.Errorf("job canary names %s", word)
		}
	}
	if n := len(canary.Steps); n == 0 || !strings.Contains(canary.Steps[n-1].If, "steps.run.outcome == 'failure'") ||
		!strings.Contains(canary.Steps[n-1].If, "github.event_name == 'pull_request'") {
		t.Error("job canary does not end with a step that fails a pull request when the canary fails")
	}
	run := slices.IndexFunc(canary.Steps, func(s workflowStep) bool { return s.ID == "run" })
	if run < 0 || !canary.Steps[run].ContinueOnError {
		t.Fatal("job canary has no step run with continue-on-error, so a break never reaches report")
	}
	// One failed install must not stop the others or the run: the agent it
	// missed writes "<bin> is not installed", and report files that.
	pkgs := []string{"@anthropic-ai/claude-code@latest", "@openai/codex@latest", "opencode-ai@latest"}
	for _, pkg := range pkgs {
		steps := slices.DeleteFunc(slices.Clone(canary.Steps), func(s workflowStep) bool { return !strings.Contains(s.Run, pkg) })
		if len(steps) != 1 {
			t.Errorf("job canary: %d steps install %s, want one", len(steps), pkg)
			continue
		}
		s := steps[0]
		alone := !slices.ContainsFunc(pkgs, func(p string) bool { return p != pkg && strings.Contains(s.Run, p) })
		if !s.ContinueOnError || !alone || slices.IndexFunc(canary.Steps, func(o workflowStep) bool { return o.Name == s.Name }) > run {
			t.Errorf("job canary: step %q installs %s, want it alone, before the run, with continue-on-error", s.Name, pkg)
		}
	}
	if !strings.Contains(canary.Steps[run].If, "always()") {
		t.Errorf("job canary: the run step runs if %q, want always(), so a failed install still runs it", canary.Steps[run].If)
	}
	for _, s := range canary.Steps {
		if strings.HasPrefix(s.Uses, "actions/upload-artifact@") && !strings.Contains(s.If, "always()") {
			t.Errorf("job canary: the upload runs if %q, want always()", s.If)
		}
	}

	report, ok := wf.Jobs["report"]
	if !ok {
		t.Fatal("no job report")
	}
	if want := map[string]string{"contents": "read", "issues": "write"}; !maps.Equal(report.Permissions, want) {
		t.Errorf("job report permissions %v, want %v", report.Permissions, want)
	}
	if report.Needs != "canary" {
		t.Errorf("job report needs %v, want canary", report.Needs)
	}
	if !strings.Contains(report.If, "always()") || !strings.Contains(report.If, "github.event_name != 'pull_request'") {
		t.Errorf("job report runs if %q, want always() and never on a pull request", report.If)
	}
	// No artifact is news too: the script opens "the canary tested nothing".
	if i := slices.IndexFunc(report.Steps, func(s workflowStep) bool { return strings.HasPrefix(s.Uses, "actions/download-artifact@") }); i < 0 || !report.Steps[i].ContinueOnError {
		t.Error("job report: the download has no continue-on-error, so a missing artifact stops the report before it files an issue")
	}
	agent := regexp.MustCompile(`\b(npm|npx|node|go|claude|codex|opencode)\b`)
	runs := 0
	for _, s := range report.Steps {
		if s.Run == "" {
			continue
		}
		runs++
		if run := strings.TrimSpace(s.Run); strings.Contains(run, "\n") || !strings.HasPrefix(run, ".github/scripts/canary-report.sh ") {
			t.Errorf("job report runs %q, want only canary-report.sh", run)
		}
		if agent.MatchString(s.Run) {
			t.Errorf("job report runs an agent or a toolchain: %q", s.Run)
		}
	}
	if runs != 1 {
		t.Errorf("job report has %d run steps, want one: canary-report.sh", runs)
	}
}
