package stoker

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/blox-eng/matchblox/internal/hooks"
)

func TestGuardedCommands(t *testing.T) {
	type want struct {
		what string
		n    int
		ok   bool
	}
	for cmd, w := range map[string]want{
		"gh pr merge 12 --squash":                          {"merge", 12, true},
		"cd x && gh pr merge":                              {"merge", 0, true},
		"gh pr view 12":                                    {"", 0, false},
		"gh release create v1":                             {"release", 0, true},
		"gh workflow run Release":                          {"deploy", 0, true},
		"gh release upload v1 a.tgz":                       {"release", 0, true},
		"gh release edit v1 --draft=false":                 {"release", 0, true},
		"go test ./... ; gh pr merge #4556":                {"merge", 4556, true},
		"git push || gh pr merge 7":                        {"merge", 7, true},
		"gh pr merge --squash --subject x 9":               {"merge", 9, true},
		"gh pr merge https://github.com/o/r/pull/31":       {"merge", 31, true},
		"/usr/bin/gh pr merge 3":                           {"merge", 3, true},
		"GH_REPO=o/r gh pr merge 5":                        {"merge", 5, true},
		"bash -c \"gh pr merge 6\"":                        {"merge", 6, true},
		"echo $(gh pr merge 8)":                            {"merge", 8, true},
		"gh pr merge \\\n  10":                             {"merge", 10, true},
		"gh pr merge-queue":                                {"", 0, false},
		"gh pr merge feat/x":                               {"merge", 0, true},
		"gh pr view 1 && gh workflow run deploy.yml":       {"deploy", 0, true},
		"cd /tmp && GH_X=1 gh release create v2 --notes x": {"release", 0, true},
		// Ruling T: other shell flags and a timeout prefix.
		"bash -lc 'gh pr merge 11'":                           {"merge", 11, true},
		"sh -ec \"gh release create v3\"":                     {"release", 0, true},
		"zsh -ic 'gh workflow run x'":                         {"deploy", 0, true},
		"timeout 30 gh pr merge 12":                           {"merge", 12, true},
		"timeout -s KILL 5m gh pr merge 13":                   {"merge", 13, true},
		"timeout --signal=TERM 10s bash -lc 'gh pr merge 14'": {"merge", 14, true},
		// gh api routes around gh pr merge, gh workflow run and gh release.
		"gh api -X PUT repos/o/r/pulls/15/merge":                                  {"merge", 15, true},
		"gh api repos/{owner}/{repo}/pulls/16/merge --method PUT":                 {"merge", 16, true},
		"gh api -X POST repos/o/r/actions/workflows/r.yml/dispatches -f ref=main": {"deploy", 0, true},
		"gh api repos/o/r/dispatches -f event_type=deploy":                        {"deploy", 0, true},
		"gh api -X POST repos/o/r/releases -f tag_name=v1":                        {"release", 0, true},
		"gh api repos/o/r/releases -f tag_name=v1":                                {"release", 0, true},
		"gh api --method=PATCH repos/o/r/releases/9":                              {"release", 0, true},
		"gh api -XPATCH repos/o/r/releases/9":                                     {"release", 0, true},
		"{ gh pr merge 18; }":                                                     {"merge", 18, true},
		// Fix round 2: attached field flags, shell options before -c,
		// graphql mutations and shell keywords (Ruling U).
		"gh api repos/o/r/releases --field=tag_name=v1":     {"release", 0, true},
		"gh api repos/o/r/releases -ftag_name=v1":           {"release", 0, true},
		"gh api repos/o/r/releases -Ftag_name=v1":           {"release", 0, true},
		"gh api repos/o/r/releases --raw-field=tag_name=v1": {"release", 0, true},
		"gh api repos/o/r/releases --input=body.json":       {"release", 0, true},
		"bash -l -c 'gh pr merge 5'":                        {"merge", 5, true},
		"bash --login -c 'gh pr merge 5'":                   {"merge", 5, true},
		"bash -o pipefail -c 'gh pr merge 5'":               {"merge", 5, true},
		`gh api graphql -f query='mutation { mergePullRequest(input: {pullRequestId: "x"}) { clientMutationId } }'`: {"merge", 0, true},
		`gh api graphql -f query='mutation { createRelease(input: {tagName: "v1"}) { release { id } } }'`:           {"release", 0, true},
		"if true; then gh pr merge 3; fi":    {"merge", 3, true},
		"while true; do gh pr merge 4; done": {"merge", 4, true},
		"! gh pr merge 6":                    {"merge", 6, true},
		"echo 7 | xargs -n1 gh pr merge":     {"merge", 0, true},
		"watch -n 5 gh workflow run x":       {"deploy", 0, true},
		"bash -l script.sh":                  {"", 0, false},
		// Ruling AM, the word rule: in a guarded pane gh with a guarded
		// word needs a go even where the reading finds a read, and a line
		// given to another command is read as one.
		"gh release view v1":                                {"release", 0, true},
		"gh workflow list":                                  {"deploy", 0, true},
		`git commit -m "gh pr merge later"`:                 {"merge", 0, true},
		"rg -n 'gh pr merge' .":                             {"merge", 0, true},
		"gh api repos/o/r/releases/latest":                  {"merge", 0, true},
		"gh api -X GET repos/o/r/releases -f per_page=5":    {"merge", 0, true},
		"gh api repos/o/r/releases --jq .[0].tag_name":      {"merge", 0, true},
		"gh api repos/o/r/actions/runs -X GET -f status=ok": {"merge", 0, true},
		"gh api repos/o/r/pulls/17":                         {"merge", 0, true},
		`gh api graphql -f query='query { repository(owner: "o", name: "r") { pullRequest(number: 1) { title } } }'`: {"merge", 0, true},
	} {
		what, n, ok := Guarded(cmd)
		if what != w.what || n != w.n || ok != w.ok {
			t.Errorf("Guarded(%q) = %q %d %v, want %q %d %v", cmd, what, n, ok, w.what, w.n, w.ok)
		}
	}
	// Every guarded command of the line counts, not only the first.
	if gs := Guards("gh pr merge 1 && gh pr merge 2"); len(gs) != 2 || gs[1] != (Guard{What: "merge", N: 2}) {
		t.Fatalf("two merges: %+v", gs)
	}
}

func TestAllowed(t *testing.T) {
	for _, c := range []struct {
		grants []string
		what   string
		n      int
		ok     bool
	}{
		{nil, "merge", 12, false},
		{[]string{"merge"}, "merge", 12, true},
		{[]string{"merge"}, "merge", 0, true},
		{[]string{"merge:#12"}, "merge", 12, true},
		{[]string{"merge:#12"}, "merge", 13, false},
		{[]string{"merge:#12"}, "merge", 0, false},
		{[]string{"release"}, "release", 0, true},
		{[]string{"release"}, "deploy", 0, false},
		{[]string{"merge"}, "release", 0, false},
		{[]string{"deploy"}, "deploy", 0, true},
	} {
		if got := Allowed(c.grants, c.what, c.n); got != c.ok {
			t.Errorf("Allowed(%q, %s, %d) = %v", c.grants, c.what, c.n, got)
		}
	}
}

func TestDrivenRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadDriven(dir); !os.IsNotExist(err) {
		t.Fatalf("missing file: %v", err)
	}
	d := Driven{Run: "run-1", Panes: map[string][]string{"%1": nil, "%2": {"merge:#4"}}}
	if err := WriteDriven(dir, d); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDriven(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, in := got.Panes["%1"]; !in || got.Run != "run-1" || len(got.Panes["%2"]) != 1 {
		t.Fatalf("read back %+v", got)
	}
	if err := RemoveDriven(dir); err != nil {
		t.Fatal(err)
	}
	if err := RemoveDriven(dir); err != nil {
		t.Fatalf("a second remove: %v", err)
	}
}

func TestDenies(t *testing.T) {
	dir := t.TempDir()
	merge := Guards("gh pr merge 4")
	// No file and no run: open everywhere.
	if Denies(dir, "", "%1", merge) {
		t.Fatal("no run, yet denied")
	}
	if err := os.WriteFile(filepath.Join(dir, "stoker.json"), []byte(`{"on":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// A run is on and the mark is gone: closed everywhere.
	if !Denies(dir, "", "%7", merge) {
		t.Fatal("a run without its driven file let a merge through")
	}
	if err := WriteDriven(dir, Driven{Run: "r", Panes: map[string][]string{"%1": {"merge:#4"}, "%2": nil}}); err != nil {
		t.Fatal(err)
	}
	switch {
	case Denies(dir, "", "%7", merge):
		t.Fatal("a pane the stoker never drove was denied")
	case Denies(dir, "", "%1", merge):
		t.Fatal("the granted merge was denied")
	case !Denies(dir, "", "%1", Guards("gh pr merge 4 && gh pr merge 5")):
		t.Fatal("a second, ungranted merge on the same line went through")
	case !Denies(dir, "", "%2", merge):
		t.Fatal("a driven pane without a grant merged")
	}
	// A step's go counts beside the order's.
	if err := WriteDriven(dir, Driven{Run: "r", Panes: map[string][]string{"%2": nil}, Steps: map[string][]string{"%2": {"release"}}}); err != nil {
		t.Fatal(err)
	}
	if Denies(dir, "", "%2", Guards("gh release create v1")) {
		t.Fatal("the step's release was denied")
	}
}

func TestDeniesTheStokersOwnPanesWhateverTheGrants(t *testing.T) {
	dir := t.TempDir()
	// The builder's %9 has every go: the stoker's %9 none.
	d := Driven{Run: "r", Panes: map[string][]string{"%9": {"merge", "release", "deploy"}},
		Steps: map[string][]string{"%9": {"merge:#4"}}, StokerSocket: stokerSock}
	if err := WriteDriven(dir, d); err != nil {
		t.Fatal(err)
	}
	for _, pane := range []string{"%9", "%10"} {
		for _, cmd := range []string{"gh pr merge 4", "gh release create v1", "gh workflow run Release"} {
			if !Denies(dir, stokerSock, pane, Guards(cmd)) {
				t.Fatalf("%s ran %q in the stoker's own pane", pane, cmd)
			}
		}
		if Denies(dir, stokerSock, pane, Guards("gh pr view 4")) {
			t.Fatalf("%s: a read was denied", pane)
		}
	}
}

// TestTheGuardReadsTheRawLine: N1, a value in quotes hides no command
// from the guard, whether the scrub sees it or not.
func TestTheGuardReadsTheRawLine(t *testing.T) {
	for _, cmd := range []string{
		`echo "set A='1" && gh pr merge 5 && echo 'ok'`,
		`echo " Z='" ; gh pr merge 5 ; echo "'"`,
		`FOO="x y" gh pr merge 5`,
	} {
		if gs := Guards(cmd); len(gs) != 1 || gs[0] != (Guard{What: "merge", N: 5}) {
			t.Errorf("%q: guards %+v", cmd, gs)
		}
	}
}

// TestTheGuardReadsQuotesAsTheShellDoes: Ruling AJ, a quote, a backslash
// or `sh -c` that joins a word the shell runs as gh still reads as gh.
func TestTheGuardReadsQuotesAsTheShellDoes(t *testing.T) {
	merge := []Guard{{What: "merge", N: 1}}
	for cmd, want := range map[string][]Guard{
		`g"h" pr merge 1`:                         merge,
		`'gh' pr merge 1`:                         merge,
		`$'gh' pr merge 1`:                        merge,
		`$'\x67h' pr merge 1`:                     merge,
		`$'\147h' pr merge 1`:                     merge,
		`\gh pr merge 1`:                          merge,
		`gh p''r merge 1`:                         merge,
		`gh pr "mer"ge 1`:                         merge,
		`bash -c "gh pr merge 1"`:                 merge,
		`sh -c 'g"h" pr merge 1'`:                 merge,
		`sh -c "bash -c 'g\"h\" pr merge 1'"`:     merge,
		`gh api repos/o/r/pulls/1/merg''e -X PUT`: merge,
		`x=$(g''h pr merge 1)`:                    merge,
		"echo `g''h pr merge 1`":                  merge,
		`echo "$(g''h pr merge 1)"`:               merge,
		`$"gh" pr merge 1`:                        merge,
		`gh pr view 1`:                            nil,
		`echo "gh pr merge is denied"`:            {{What: "merge"}}, // Ruling AM: a line in a word is read as one
		`git status`:                              nil,
		`echo "gh pr merge"`:                      {{What: "merge", Bare: true}},
		`git log --grep 'gh pr merge'`:            {{What: "merge", Bare: true}},
		`git commit -m "it's \"g\"h done"`:        nil,
		`echo hi # it's 'gh pr merge 1`:           nil,
		"gh pr create --body \"$(cat <<'EOF'\nIt's \"done\"\nEOF\n)\"": nil,
		"cat <<EOF\ndon't\nEOF\ngh pr merge 1":                         merge,
	} {
		if got := Guards(cmd); !slices.Equal(got, want) {
			t.Errorf("Guards(%q) = %+v, want %+v", cmd, got, want)
		}
	}
	// Four shells deep still reads; each line is denied in a driven pane
	// and in the stoker's own.
	nested := "g''h pr merge 1"
	for range maxDepth {
		nested = "sh -c " + strconv.Quote(nested)
	}
	if got := Guards(nested); !slices.Equal(got, merge) {
		t.Errorf("Guards(%q) = %+v, want %+v", nested, got, merge)
	}
	dir := t.TempDir()
	if err := WriteDriven(dir, Driven{Run: "r", Panes: map[string][]string{"%1": nil}, StokerSocket: stokerSock}); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{`g"h" pr merge 1`, `$'gh' pr merge 1`, `\gh pr merge 1`, `gh p''r merge 1`, `sh -c 'g"h" pr merge 1'`, nested} {
		for _, pane := range []string{"%1", "%9@matchblox"} {
			if !Denies(dir, hookAt(pane), bare(pane), Guards(cmd)) {
				t.Errorf("%s ran %q", pane, cmd)
			}
		}
	}
}

// TestALineTheGuardCannotReadIsGuarded: an unterminated quote, or shells
// nested deeper than the guard reads, that name gh count as guarded; in a
// guarded pane they are denied, elsewhere they run.
func TestALineTheGuardCannotReadIsGuarded(t *testing.T) {
	deep := "gh pr merge 1"
	for range 6 {
		deep = "sh -c " + strconv.Quote(deep)
	}
	for _, cmd := range []string{`g"h pr merge 1`, `echo 'x ; gh pr merge 1`, `gh pr view 1 "`, deep} {
		if gs := Guards(cmd); !slices.Contains(gs, Guard{What: Unread}) {
			t.Errorf("Guards(%q) = %+v, want it unread", cmd, gs)
		}
	}
	for _, cmd := range []string{`echo 'x`, `git commit -m "wip`} {
		if gs := Guards(cmd); len(gs) != 0 {
			t.Errorf("Guards(%q) = %+v: no gh, nothing to guard", cmd, gs)
		}
	}
	dir := t.TempDir()
	if err := WriteDriven(dir, Driven{Run: "r", Panes: map[string][]string{"%1": {"merge", "release", "deploy"}}}); err != nil {
		t.Fatal(err)
	}
	if !Denies(dir, "", "%1", Guards(`g"h pr merge 1`)) {
		t.Fatal("an unread line ran in a driven pane, whatever the grants")
	}
	if Denies(dir, "", "%7", Guards(`g"h pr merge 1`)) {
		t.Fatal("an unread line was denied in a pane the stoker never drove")
	}
}

// TestThePanesOfTheStokersServerAreGuardedByTheirSocket: pane ids
// repeat across tmux servers. A hook on the stoker's own server is the
// stoker's in any pane, one no sample listed yet too; the builder's pane
// of the same id is not, and keeps its goes.
func TestThePanesOfTheStokersServerAreGuardedByTheirSocket(t *testing.T) {
	dir := t.TempDir()
	const builder, stokers = "/tmp/tmux-1000/default", "/tmp/tmux-1000/matchblox"
	if err := WriteDriven(dir, Driven{Run: "r", Panes: map[string][]string{"%0": {"merge"}}, StokerSocket: stokers}); err != nil {
		t.Fatal(err)
	}
	merge := Guards("gh pr merge 4")
	switch {
	case Deny(dir, stokers, "%0", merge) != DenyStokerPane:
		t.Fatal("the stoker's %0 merged with the go of the builder's %0")
	case Deny(dir, stokers, "%12", merge) != DenyStokerPane:
		t.Fatal("a new pane of the stoker's server merged")
	case Denies(dir, builder, "%0", merge):
		t.Fatal("the builder's driven %0 lost the order's merge")
	case Denies(dir, builder, "%12", merge):
		t.Fatal("a pane the stoker never drove was denied")
	case Denies(dir, stokers, "%0", Guards("gh pr view 4")):
		t.Fatal("a read was denied")
	}
}

// A scrubbed line keeps every command the guard reads, and a value in
// quotes no longer hides the command after it.
func TestScrubbedLinesKeepTheirGuards(t *testing.T) {
	for _, cmd := range []string{
		"GH_TOKEN=abc gh pr merge 5",
		`FOO="x y" gh pr merge 5`,
		`X="$(gh pr merge 5)"`,
		"X=$(gh pr merge 5)",
		"A='q' && gh pr merge 5",
	} {
		if gs := Guards(hooks.Scrub(cmd)); len(gs) != 1 || gs[0] != (Guard{What: "merge", N: 5}) {
			t.Errorf("%q scrubbed to %q: guards %+v", cmd, hooks.Scrub(cmd), gs)
		}
	}
}

// TestTheWordRuleDeniesWhatAnHonestAgentWrites: Ruling AM, in a guarded
// pane a line whose words name gh with merge, release, workflow, api,
// graphql or mergePullRequest needs a go, however gh is reached: its own
// flags before the verb (B1), a path, a runner or a shell fed the line
// (B2). The plain reads and a pull request's body stay allowed.
func TestTheWordRuleDeniesWhatAnHonestAgentWrites(t *testing.T) {
	mutation := `-f query='mutation{mergePullRequest(input:{pullRequestId:"X"}){clientMutationId}}'`
	merge5, merge1, merge0 := []Guard{{What: "merge", N: 5}}, []Guard{{What: "merge", N: 1}}, []Guard{{What: "merge"}}
	release, deploy := []Guard{{What: "release"}}, []Guard{{What: "deploy"}}
	for cmd, want := range map[string][]Guard{
		// B1: gh's own syntax.
		"gh -R o/r pr merge 5":                                             merge5,
		"gh --repo o/r pr merge 5":                                         merge5,
		"gh pr -R o/r merge 5":                                             merge5,
		"gh pr --repo=o/r merge 5":                                         merge5,
		"gh -R o/r release create v1":                                      release,
		"gh release -R o/r create v1":                                      release,
		"gh workflow -R o/r run deploy.yml":                                deploy,
		"gh -R o/r workflow run deploy.yml":                                deploy,
		"gh release new v1":                                                release,
		"gh api -H 'Accept: application/json' graphql " + mutation:         merge0,
		"gh api --hostname github.com graphql " + mutation:                 merge0,
		"gh api graphql -f query='mutation{merge''PullRequest(input:{})}'": merge0,
		// B2: what leads gh, and what runs a line it is given.
		"/bin/bash -c 'gh pr merge 5'":         merge5,
		"/usr/bin/env bash -c 'gh pr merge 5'": merge5,
		"dash -c 'gh pr merge 5'":              merge5,
		"env -u X gh pr merge 5":               merge5,
		"env -- gh pr merge 5":                 merge5,
		"time -p gh pr merge 5":                merge5,
		"command -p gh pr merge 5":             merge5,
		"exec -a x gh pr merge 5":              merge5,
		"nice gh pr merge 5":                   merge5,
		"nice -n 5 gh pr merge 5":              merge5,
		"setsid gh pr merge 5":                 merge5,
		"stdbuf -o0 gh pr merge 5":             merge5,
		"coproc gh pr merge 5":                 merge5,
		"noglob gh pr merge 5":                 merge5,
		"=gh pr merge 5":                       merge5,
		"xargs -I X gh pr merge X":             merge0,
		`find . -exec gh pr merge 1 \;`:        merge1,
		`script -c "gh pr merge 1"`:            merge1,
		`script -qc 'gh pr merge 1' /dev/null`: merge1,
		`bash <<< "gh pr merge 1"`:             merge1,
		"echo gh pr merge 1 | bash":            merge0,
		"echo 'gh pr merge 1' | bash":          merge1,
		// B3: a case inside $(…) inside "…" no longer hides the merge.
		`echo "$(case a in a) g''h pr merge 5;; esac)"`: merge5,
		// The words alone: gh with a guarded word, whatever the verb.
		"gh release view v1":        release,
		"gh workflow list":          deploy,
		"gh api repos/o/r/pulls/17": merge0,
		// Positive controls: reads and a body stay allowed.
		"gh pr view 5": nil,
		"gh pr list":   nil,
		"gh pr create --title 'Merge the queue' --body \"$(cat <<'EOF'\nWe merge the release workflow with gh api.\nEOF\n)\"": nil,
		`gh pr create --body "lets you merge faster"`: nil,
		"git status":        nil,
		"go test ./...":     nil,
		"gh pr merge-queue": nil,
	} {
		if got := Guards(cmd); !slices.Equal(got, want) {
			t.Errorf("Guards(%q) = %+v, want %+v", cmd, got, want)
		}
	}
	dir := t.TempDir()
	if err := WriteDriven(dir, Driven{Run: "r", Panes: map[string][]string{"%1": nil, "%2": {"merge:#5"}}}); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"gh -R o/r pr merge 5", "/bin/bash -c 'gh pr merge 5'", "echo gh pr merge 1 | bash", "gh release new v1"} {
		if !Denies(dir, "", "%1", Guards(cmd)) {
			t.Errorf("%q ran in a driven pane without a go", cmd)
		}
	}
	// The go for #5 covers #5 however gh is reached.
	for _, cmd := range []string{"gh -R o/r pr merge 5", "env -u X gh pr merge 5", "/bin/bash -c 'gh pr merge 5'"} {
		if Denies(dir, "", "%2", Guards(cmd)) {
			t.Errorf("%q was denied with the go for #5", cmd)
		}
	}
}

// TestOneStepGoLetsOneGuardedCommandThrough: a go given with y covers one
// guarded command, so a line with two needs a go for each, whatever the
// kind. The order's goes cover any number.
func TestOneStepGoLetsOneGuardedCommandThrough(t *testing.T) {
	dir := t.TempDir()
	d := Driven{Run: "r", Panes: map[string][]string{"%3": nil, "%4": {"release"}},
		Steps: map[string][]string{"%3": {"merge:@%3", "release"}},
		Own:   map[string]Own{"%3": {Names: []string{"%3"}, PRs: []int{5, 6}}}}
	if err := WriteDriven(dir, d); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"gh pr merge 5 && gh pr merge 6", "gh release create a && gh release create b"} {
		if !Denies(dir, "", "%3", Guards(line)) {
			t.Fatalf("one y let %q through", line)
		}
	}
	for _, line := range []string{"gh pr merge 5", "gh release create a", "gh pr merge 5 && gh release create a"} {
		if Denies(dir, "", "%3", Guards(line)) {
			t.Fatalf("%q was denied with a go for each", line)
		}
	}
	if Denies(dir, "", "%4", Guards("gh release create a && gh release create b")) {
		t.Fatal("the order's release did not cover two")
	}
	d.Steps["%3"] = []string{"release", "release"}
	if err := WriteDriven(dir, d); err != nil {
		t.Fatal(err)
	}
	if Denies(dir, "", "%3", Guards("gh release create a && gh release create b")) {
		t.Fatal("two gos did not let two releases through")
	}
}
