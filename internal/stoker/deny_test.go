package stoker

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestDenyNamesWhy: F0b, the session reads why the guard stopped it, not
// always "the order did not give the go".
func TestDenyNamesWhy(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stoker.json"), []byte(`{"on":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	merge := Guards("gh pr merge 4")
	if got := Deny(dir, "", "%1", merge); got != DenyLost {
		t.Fatalf("a run without its driven file: %q", got)
	}
	d := Driven{Run: "r", Panes: map[string][]string{"%1": nil, "%2": {"merge"}}, StokerSocket: stokerSock}
	if err := WriteDriven(dir, d); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		pane, cmd, want string
	}{
		{"%1", "gh pr merge 4", DenyNoGo},
		{"%9@matchblox", "gh pr merge 4", DenyStokerPane},
		{"%2", `g"h pr merge 4`, DenyUnread},
		{"%2", "gh pr merge 4", ""},
		{"%7", "gh pr merge 4", ""},
	} {
		if got := Deny(dir, hookAt(c.pane), bare(c.pane), Guards(c.cmd)); got != c.want {
			t.Errorf("%s %q: %q, want %q", c.pane, c.cmd, got, c.want)
		}
	}
	if DenyNoGo != "the stoker's order did not give the go for this. Give it on the stoker's card (y) or stop the stoker." {
		t.Fatal("the spec's deny reason changed")
	}
	reasons := []string{DenyNoGo, DenyStokerPane, DenyUnread, DenyLost}
	for i, r := range reasons {
		if r == "" || slices.Index(reasons, r) != i {
			t.Fatalf("reason %d is empty or repeats: %q", i, r)
		}
	}
}

// TestADataHereDocIsNotACommand: F0c, the body of a here-document fed to
// a command that is not a shell is text; one fed to a shell, directly or
// down a pipe, is read as commands.
func TestADataHereDocIsNotACommand(t *testing.T) {
	merge1 := []Guard{{What: "merge", N: 1}}
	for cmd, want := range map[string][]Guard{
		"cat > notes.md <<'EOF'\nrun gh pr merge 1 once CI is green\ngh pr merge 1\nEOF": nil,
		"tee notes.md <<EOF >/dev/null\ngh pr merge 1\nEOF":                              nil,
		"cat <<-EOF > a\n\tgh pr merge 1\n\tEOF\ngo test ./...":                          nil,
		"cat > a <<'EOF'\ngh pr merge 1\nEOF\ngh pr merge 1":                             merge1,
		"bash <<'EOF'\ngh pr merge 1\nEOF":                                               merge1,
		"sh -s <<EOF\ngh pr merge 1\nEOF":                                                merge1,
		"cat <<'EOF' | bash\ngh pr merge 1\nEOF":                                         merge1,
		"zsh <<'EOF'\necho \"$(g''h pr merge 1)\"\nEOF":                                  merge1,
		"python3 - <<'EOF'\nimport os\nos.system(\"gh pr merge 1\")\nEOF":                merge1,
		"x=$(cat <<'EOF'\ngh pr merge 1\nEOF\n)":                                         nil,
		"gh pr create --body-file - <<'EOF'\nOnce green: gh pr merge 1 --squash.\nEOF":   nil,
		`cat <<< "gh pr merge 1"`:                                                        nil,
		`bash <<< "gh pr merge 1"`:                                                       merge1,
		// Ruling AW (M1): a << that is not a here-document, whose end word
		// never comes, swallows nothing.
		"(( n = 1 << 2 ))\ngh pr merge 1": merge1,
		"let x=1<<2\ngh pr merge 1":       merge1,
		"cat <<EOF\ngh pr merge 1":        merge1,
		// Ruling AW (M5): the stoker writes its plan this way.
		"matchblox stoker plan <<'EOF'\n1. work: gh pr merge 1 on green\nasks: merge:@work\nEOF": nil,
	} {
		if got := Guards(cmd); !slices.Equal(got, want) {
			t.Errorf("Guards(%q) = %+v, want %+v", cmd, got, want)
		}
	}
}

// TestASessionsMergeGoCoversItsOwnPullRequests: F2, merge:@<session> lets
// that session's pane merge the pull requests it drives, or its branch's
// own with no number, and nothing else.
func TestASessionsMergeGoCoversItsOwnPullRequests(t *testing.T) {
	for g, ok := range map[string]bool{"merge:@Zheleznik": true, "merge:@app:1.2": true, "merge:@": false, "merge:@a b": false} {
		if err := CheckGrant(g); (err == nil) != ok {
			t.Errorf("CheckGrant(%q) = %v", g, err)
		}
	}
	dir := t.TempDir()
	at := []string{"merge:@Zheleznik"}
	d := Driven{Run: "r", Panes: map[string][]string{"%1": at, "%2": at},
		Own: map[string]Own{"%1": {Names: []string{"Zheleznik", "%1", "zh:1.1"}, PRs: []int{4569}}, "%2": {Names: []string{"Notifications"}, PRs: []int{4570}}}}
	if err := WriteDriven(dir, d); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		pane, cmd string
		denied    bool
	}{
		{"%1", "gh pr merge 4569 --squash", false},
		{"%1", "gh pr merge --squash", false},
		{"%1", "gh pr merge 4570", true},
		{"%2", "gh pr merge 4570", true},
		{"%2", "gh pr merge", true},
		{"%1", "gh release create v1", true},
		// Ruling AU: a target the guard cannot read is not a bare merge.
		{"%1", "gh pr merge feat/other-branch", true},
		{"%1", "gh pr merge \"$PR\" --squash", true},
		{"%1", "gh api graphql -f query='mutation { mergePullRequest(input: {pullRequestId: \"x\"}) { clientMutationId } }'", true},
		{"%1", "gh pr merge --squash --subject \"feat: two words\" --delete-branch", false},
		{"%1", "gh pr merge -t 'feat: two words'", false},
		{"%1", "gh pr merge; gh pr merge feat/other-branch", true},
	} {
		if got := Denies(dir, "", c.pane, Guards(c.cmd)); got != c.denied {
			t.Errorf("%s %q: denied %v, want %v", c.pane, c.cmd, got, c.denied)
		}
	}
}

// TestAPullRequestNotKnownYetIsNamed: Ruling AW (M4), a session's merge go
// denies a number the ships poll has not tied to it yet with a reason that
// names the pull request, not with "the order did not give the go".
func TestAPullRequestNotKnownYetIsNamed(t *testing.T) {
	dir := t.TempDir()
	at := []string{"merge:@Zheleznik"}
	d := Driven{Run: "r", Panes: map[string][]string{"%1": at, "%2": nil},
		Own: map[string]Own{"%1": {Names: []string{"Zheleznik", "%1"}, PRs: []int{4569}}, "%2": {Names: []string{"Notifications"}, PRs: []int{4570}}}}
	if err := WriteDriven(dir, d); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		pane, cmd, want string
	}{
		{"%1", "gh pr merge 4571", DenyNotKnown("Zheleznik", 4571)},
		{"%1", "gh pr merge 4570", DenyNoGo}, // another session's
		{"%1", "gh pr merge feat/x", DenyNoGo},
		{"%2", "gh pr merge 4571", DenyNoGo},
		{"%1", "gh pr merge 4569", ""},
	} {
		if got := Deny(dir, "", c.pane, Guards(c.cmd)); got != c.want {
			t.Errorf("%s %q: %q, want %q", c.pane, c.cmd, got, c.want)
		}
	}
	if got := DenyNotKnown("Zheleznik", 4571); got != "the order gave merge:@Zheleznik, and #4571 is not known yet as Zheleznik's pull request: matchblox learns of a new one within about a minute. Try again then." {
		t.Fatalf("reason %q", got)
	}
}

// stokerSock is the socket of the stoker's tmux server in these tests.
const stokerSock = "/tmp/tmux-1000/matchblox"

// hookAt is the socket a hook in pane runs on: the stoker's for a pane
// of its server (%9@matchblox), the builder's otherwise.
func hookAt(pane string) string {
	if strings.Contains(pane, "@") {
		return stokerSock
	}
	return "/tmp/tmux-1000/default"
}

// bare is the pane id as the hook's $TMUX_PANE names it.
func bare(pane string) string {
	id, _, _ := strings.Cut(pane, "@")
	return id
}

// familyUnit makes the chain stoker id in state, running, and writes its
// driven.json when d is not nil.
func familyUnit(t *testing.T, state, id string, d *Driven) {
	t.Helper()
	dir := FamilyDir(state, id)
	if err := WriteLink(dir, Link{ID: id, Chain: "c-" + id, Giver: "person"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stoker.json"), []byte(`{"on":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if d != nil {
		if err := WriteDriven(dir, *d); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDenyFamilyChainUnitAllowsItsGo(t *testing.T) {
	st := t.TempDir()
	familyUnit(t, st, "s1", &Driven{Run: "r", Panes: map[string][]string{"%7": {"merge"}}})
	if got := DenyFamily(st, "", "%7", Guards("gh pr merge 4")); got != "" {
		t.Fatalf("a chain stoker's go: %q", got)
	}
}

func TestDenyFamilyOtherUnitsGoDoesNotLeak(t *testing.T) {
	st := t.TempDir()
	familyUnit(t, st, "s1", &Driven{Run: "r", Panes: map[string][]string{"%7": {"merge"}}})
	familyUnit(t, st, "s2", &Driven{Run: "r", Panes: map[string][]string{"%8": nil}})
	merge := Guards("gh pr merge 4")
	if got := DenyFamily(st, "", "%8", merge); got != DenyNoGo {
		t.Fatalf("s2's pane with s1's go: %q", got)
	}
	if got := DenyFamily(st, "", "%7", merge); got != "" {
		t.Fatalf("s1's pane: %q", got)
	}
}

func TestDenyFamilyLostDrivenFailsClosed(t *testing.T) {
	st := t.TempDir()
	familyUnit(t, st, "s1", nil)
	for _, pane := range []string{"%1", "%7"} {
		if got := DenyFamily(st, "", pane, Guards("gh pr merge 4")); got != DenyLost {
			t.Fatalf("%s: %q", pane, got)
		}
	}
}

func TestDenyFamilyStokerPaneOfAnyMember(t *testing.T) {
	st := t.TempDir()
	familyUnit(t, st, "s1", &Driven{Run: "r", Panes: map[string][]string{"%7": {"merge"}}, StokerSocket: stokerSock})
	if got := DenyFamily(st, stokerSock, "%9", Guards("gh pr merge 4")); got != DenyStokerPane {
		t.Fatalf("%q", got)
	}
}

func TestDenyFamilyBrokenFamilyDirFailsClosed(t *testing.T) {
	st := t.TempDir()
	if err := os.WriteFile(filepath.Join(st, "stokers"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	merge := Guards("gh pr merge 4")
	if got := DenyFamily(st, "", "%1", merge); got != DenyLost {
		t.Fatalf("a host that allows: %q", got)
	}
	if err := WriteDriven(st, Driven{Run: "r", Panes: map[string][]string{"%1": nil}}); err != nil {
		t.Fatal(err)
	}
	if got := DenyFamily(st, "", "%1", merge); got != DenyNoGo {
		t.Fatalf("the host's own answer stands: %q", got)
	}
}

func TestDenyFamilyNoFamilyIsToday(t *testing.T) {
	for _, state := range []string{"off", "lost", "driven"} {
		dir := t.TempDir()
		if state != "off" {
			if err := os.WriteFile(filepath.Join(dir, "stoker.json"), []byte(`{"on":true}`), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if state == "driven" {
			d := Driven{Run: "r", Panes: map[string][]string{"%1": nil, "%2": {"merge"}}, StokerSocket: stokerSock}
			if err := WriteDriven(dir, d); err != nil {
				t.Fatal(err)
			}
		}
		for _, pane := range []string{"%1", "%2", "%7", "%9@matchblox"} {
			for _, cmd := range []string{"gh pr merge 4", `g"h pr merge 4`, "gh release create v1", "ls"} {
				gs := Guards(cmd)
				if got, want := DenyFamily(dir, hookAt(pane), bare(pane), gs), Deny(dir, hookAt(pane), bare(pane), gs); got != want {
					t.Errorf("%s %s %q: %q, want %q", state, pane, cmd, got, want)
				}
			}
		}
	}
}

func TestDenyFamilyEveryUnitThatListsAPaneMustAllow(t *testing.T) {
	st := t.TempDir()
	familyUnit(t, st, "s1", &Driven{Run: "r", Panes: map[string][]string{"%7": {"merge"}}})
	familyUnit(t, st, "s2", &Driven{Run: "r", Panes: map[string][]string{"%7": nil}})
	merge := Guards("gh pr merge 4")
	if got := DenyFamily(st, "", "%7", merge); got != DenyNoGo {
		t.Fatalf("one unit refuses: %q", got)
	}
	familyUnit(t, st, "s2", &Driven{Run: "r", Panes: map[string][]string{"%7": {"merge"}}})
	if got := DenyFamily(st, "", "%7", merge); got != "" {
		t.Fatalf("both give the go: %q", got)
	}
}

func TestDenyFamilyALostUnitDeniesEveryPane(t *testing.T) {
	merge := Guards("gh pr merge 4")
	st := t.TempDir()
	familyUnit(t, st, "s1", &Driven{Run: "r", Panes: map[string][]string{"%7": {"merge"}}})
	familyUnit(t, st, "s2", nil)
	if got := DenyFamily(st, "", "%7", merge); got != DenyLost {
		t.Fatalf("a pane s1 lists: %q", got)
	}
	st = t.TempDir()
	if err := WriteDriven(st, Driven{Run: "r", Panes: map[string][]string{"%1": {"merge"}}}); err != nil {
		t.Fatal(err)
	}
	familyUnit(t, st, "s1", nil)
	if got := DenyFamily(st, "", "%1", merge); got != DenyLost {
		t.Fatalf("a pane the host lists: %q", got)
	}
}

func TestDenyFamilyAChainUnitWithBrokenModeAndNoDrivenIsLost(t *testing.T) {
	st := t.TempDir()
	familyUnit(t, st, "s1", nil)
	if err := os.WriteFile(filepath.Join(FamilyDir(st, "s1"), "stoker.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := DenyFamily(st, "", "%1", Guards("gh pr merge 4")); got != DenyLost {
		t.Fatalf("%q", got)
	}
	// The host keeps today's way: a broken mode file with no driven.json allows.
	hostOnly := t.TempDir()
	if err := os.WriteFile(filepath.Join(hostOnly, "stoker.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, want := DenyFamily(hostOnly, "", "%1", Guards("gh pr merge 4")), Deny(hostOnly, "", "%1", Guards("gh pr merge 4")); got != want {
		t.Fatalf("%q, want %q", got, want)
	}
}
