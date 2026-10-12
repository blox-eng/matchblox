package ships

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

var at = time.Date(2026, 10, 12, 9, 49, 19, 0, time.UTC)

// toolUse is an assistant line of Claude Code calling its Bash tool, as
// the transcript records it.
func toolUse(id, cwd, branch, command string) string {
	b, _ := json.Marshal(map[string]any{
		"parentUuid": "e0a59b62", "isSidechain": false, "type": "assistant",
		"cwd": cwd, "gitBranch": branch, "sessionId": "fc0ff0d6", "timestamp": at.Format(time.RFC3339Nano),
		"message": map[string]any{
			"model": "claude-opus-5", "type": "message", "role": "assistant",
			"content": []any{map[string]any{"type": "tool_use", "id": id, "name": "Bash",
				"input": map[string]any{"command": command, "description": "run it"}}},
		},
	})
	return string(b)
}

// toolResult is the user line with a call's result; content is a string
// or text blocks, and git is Claude Code's gitOperation record or nil.
func toolResult(id, cwd string, content any, git map[string]any) string {
	tur := map[string]any{"stdout": "", "stderr": "", "interrupted": false}
	if git != nil {
		tur["gitOperation"] = git
	}
	b, _ := json.Marshal(map[string]any{
		"parentUuid": "eff01baa", "isSidechain": false, "type": "user", "cwd": cwd,
		"timestamp": at.Format(time.RFC3339Nano),
		"message": map[string]any{"role": "user", "content": []any{map[string]any{
			"tool_use_id": id, "type": "tool_result", "content": content, "is_error": false}}},
		"toolUseResult": tur,
	})
	return string(b)
}

func assistantText(text string) string {
	b, _ := json.Marshal(map[string]any{
		"type": "assistant", "cwd": "/w", "timestamp": at.Format(time.RFC3339Nano),
		"message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": text}}},
	})
	return string(b)
}

func TestEvidence(t *testing.T) {
	home, _ := os.UserHomeDir()
	const url = "https://github.com/blox-eng/blox/pull/3479"
	cases := []struct {
		name  string
		lines []string
		want  []found
	}{
		{"gh pr create, the URL in its result",
			[]string{toolUse("t1", "/w", "fix/x", `gh pr create --base main --title "fix: x" --body "$(cat <<'EOF'
Fixes #3473, see https://github.com/blox-eng/blox/pull/12 and run gh pr merge 12
EOF
)"`), toolResult("t1", "/w", url+"\n", nil)},
			[]found{{Kind: Created, Repo: "blox-eng/blox", Number: 3479, At: at}}},
		{"the result as text blocks",
			[]string{toolUse("t1", "/w", "", "gh pr create -R blox-eng/blox --fill"),
				toolResult("t1", "/w", []any{map[string]any{"type": "text", "text": "Creating pull request\n" + url}}, nil)},
			[]found{{Kind: Created, Repo: "blox-eng/blox", Number: 3479, At: at}}},
		{"the result's gitOperation names the PR it created",
			[]string{toolUse("t1", "/w", "", "git push -u origin m6/stoker && gh pr create --fill"),
				toolResult("t1", "/w", "branch 'm6/stoker' set up to track", map[string]any{
					"push": map[string]any{"branch": "m6/stoker"},
					"pr":   map[string]any{"number": 60, "url": "https://github.com/blox-eng/matchblox/pull/60", "action": "created"}})},
			[]found{{Kind: Pushed, Dir: "/w", Branch: "m6/stoker", At: at},
				{Kind: Created, Repo: "blox-eng/matchblox", Number: 60, At: at},
				{Kind: Pushed, Dir: "/w", Branch: "m6/stoker", At: at}}},
		{"a create whose result has no URL is no evidence",
			[]string{toolUse("t1", "/w", "", "gh pr create --fill"), toolResult("t1", "/w", "a pull request for branch already exists", nil)},
			nil},
		{"git push names its branch",
			[]string{toolUse("t1", "/w", "main", "git push -u origin feat/4533-base-notices")},
			[]found{{Kind: Pushed, Dir: "/w", Branch: "feat/4533-base-notices", At: at}}},
		{"a bare push is the session's branch",
			[]string{toolUse("t1", "/w", "fix/pw", "git push")},
			[]found{{Kind: Pushed, Dir: "/w", Branch: "fix/pw", At: at}}},
		{"a push after cd waits for its result",
			[]string{toolUse("t1", "/w", "main", "cd ../wt/a && git push --force-with-lease"),
				toolResult("t1", "/w", "To github.com:blox-eng/blox.git", map[string]any{"push": map[string]any{"branch": "fix/a"}})},
			[]found{{Kind: Pushed, Dir: "/wt/a", Branch: "fix/a", At: at}}},
		{"cd ~ is the home directory",
			[]string{toolUse("t1", "/w", "main", "cd ~/dev/app && gh pr checks 57")},
			[]found{{Kind: Acted, Dir: filepath.Join(home, "dev/app"), Number: 57, At: at}}},
		{"a directory only the shell knows names no repository",
			[]string{toolUse("t1", "/w", "main", "cd \"$WT\" && gh pr view 58")},
			[]found{{Kind: Acted, Number: 58, At: at}}},
		{"git -C and a refspec",
			[]string{toolUse("t1", "/w", "main", "git -C /home/m/wt/b push origin +HEAD:refs/heads/feat/b")},
			[]found{{Kind: Pushed, Dir: "/home/m/wt/b", Branch: "feat/b", At: at}}},
		{"deleting a branch is no push",
			[]string{toolUse("t1", "/w", "main", "git push origin --delete feat/old; git push origin :feat/older")},
			nil},
		{"gh pr merge with -R",
			[]string{toolUse("t1", "/w", "main", "gh pr merge 4551 --squash -R blox-eng/blox")},
			[]found{{Kind: Acted, Repo: "blox-eng/blox", Dir: "/w", Number: 4551, At: at}}},
		{"a flag's value is not the selector",
			[]string{toolUse("t1", "/w", "main", "gh pr checks --interval 10 4557 --watch")},
			[]found{{Kind: Acted, Dir: "/w", Number: 4557, At: at}}},
		{"#n and a wrapper",
			[]string{toolUse("t1", "/w", "main", "ctx-wire run gh pr view '#77' --json state")},
			[]found{{Kind: Acted, Dir: "/w", Number: 77, At: at}}},
		{"a PR URL as the selector names its repository",
			[]string{toolUse("t1", "/w", "main", "gh pr view https://github.com/o/r/pull/7 --comments")},
			[]found{{Kind: Acted, Repo: "o/r", Number: 7, At: at}}},
		{"a PR URL in any command",
			[]string{toolUse("t1", "/w", "main", "curl -s https://github.com/o/r/pull/9.diff | head")},
			[]found{{Kind: Acted, Repo: "o/r", Number: 9, At: at}}},
		{"a branch selector is not guessed",
			[]string{toolUse("t1", "/w", "main", "gh pr view feat/x")},
			nil},
		{"gh pr list acts on none",
			[]string{toolUse("t1", "/w", "main", "gh pr list --state open --limit 5")},
			nil},
		{"a URL in a message is a mention",
			[]string{toolUse("t1", "/w", "main", `git commit -m "follows https://github.com/o/r/pull/3"`)},
			nil},
		{"a heredoc's body is not a command",
			[]string{toolUse("t1", "/w", "main", "gh pr create --body-file - <<'EOF'\nthen git push origin evil\nand gh pr merge 5\nEOF\necho done"),
				toolResult("t1", "/w", "could not create", nil)},
			nil},
		{"prose never counts",
			[]string{assistantText("I'll run `gh pr merge 4551` and `git push origin main`; see " + url)},
			nil},
		{"a result for a call we did not wait for",
			[]string{toolResult("t9", "/w", url, nil)},
			nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pend := map[string]pending{}
			var got []found
			for _, l := range c.lines {
				if mayHold([]byte(l), pend) {
					got = append(got, parseLine([]byte(l), pend)...)
				}
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got  %+v\nwant %+v", got, c.want)
			}
			if len(pend) != 0 {
				t.Fatalf("calls still wait: %v", pend)
			}
		})
	}
}

func TestCommands(t *testing.T) {
	cases := map[string][][]string{
		`cd a && git push || echo "no; way"`:       {{"cd", "a"}, {"git", "push"}, {"echo", "no; way"}},
		"a \\\n b # c\nd 'e f'":                    {{"a", "b"}, {"d", "e f"}},
		`X=$(gh pr view 1 --json url) ; y "$X"`:    {{"X=$(gh pr view 1 --json url)"}, {"y", "$X"}},
		"cat <<-EOF > f\n\tgit push x y\n\tEOF\nz": {{"cat", ">", "f"}, {"z"}},
		`(cd w && make) | tee log &`:               {{"cd", "w"}, {"make"}, {"tee", "log"}},
	}
	for in, want := range cases {
		if got := commands(in); !reflect.DeepEqual(got, want) {
			t.Errorf("commands(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRepoName(t *testing.T) {
	for in, want := range map[string]string{
		"git@github.com:blox-eng/blox.git\n": "blox-eng/blox",
		"https://github.com/blox-eng/blox":   "blox-eng/blox",
		"ssh://git@github.com/o/r.git":       "o/r",
		"github.com/o/r":                     "o/r",
		"o/r":                                "o/r",
		"https://gitlab.com/a/b/c":           "",
		`o/r"){x}`:                           "",
	} {
		if got := repoName(in); got != want {
			t.Errorf("repoName(%q) = %q, want %q", in, got, want)
		}
	}
}
