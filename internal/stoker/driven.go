package stoker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/blox-eng/matchblox/internal/hooks"
)

// drivenName is the file of the panes the stoker drove in the run on. The
// merge guard, a hook that must answer in a few milliseconds, reads it
// without asking the service.
const drivenName = "driven.json"

// Driven are the panes the stoker agent sent a message to in the run, each
// with the order's goes (Panes), the goes the builder gave a step with y
// (Steps), each good for one guarded command, and the panes of the stoker's own tmux
// server (Stoker, each id with its server), where no go ever holds.
// StokerSocket is that server's socket: pane ids repeat across servers,
// so a hook tells by its own $TMUX that it runs there, in a pane the last
// sample may not have listed yet. Own is, for a driven pane whose goes
// name a session (merge:@<session>), the session it runs and the pull
// requests it drives.
type Driven struct {
	Run          string              `json:"run"`
	Panes        map[string][]string `json:"panes"`
	Steps        map[string][]string `json:"steps,omitempty"`
	StokerSocket string              `json:"stoker_socket,omitempty"`
	Own          map[string]Own      `json:"own,omitempty"`
}

// Own is what a merge:@<session> go is checked against: the names of the
// session a pane runs (its name, pane and target) and the pull requests it
// drives.
type Own struct {
	Names []string `json:"names"`
	PRs   []int    `json:"prs,omitempty"`
}

// Allows tells if grants give the go for g in pane: Allowed, or a
// merge:@<session> when pane runs that session and the merge names a pull
// request the session drives, or none (its branch's own). A target the
// guard cannot read (a branch name, a graphql mutation) needs merge or
// merge:#<n> (Ruling AU).
func (d Driven) Allows(pane string, grants []string, g Guard) bool {
	if Allowed(grants, g.What, g.N) {
		return true
	}
	if g.What != "merge" {
		return false
	}
	own := d.Own[pane]
	for _, gr := range grants {
		if who, ok := strings.CutPrefix(gr, "merge:@"); ok && slices.Contains(own.Names, who) && (g.Bare || g.N > 0 && slices.Contains(own.PRs, g.N)) {
			return true
		}
	}
	return false
}

// ReadDriven reads driven.json. A missing file is fs.ErrNotExist.
func ReadDriven(dir string) (Driven, error) {
	var d Driven
	b, err := os.ReadFile(filepath.Join(dir, drivenName)) //nolint:gosec // our own state dir
	if err != nil {
		return d, err
	}
	return d, json.Unmarshal(b, &d)
}

// WriteDriven replaces driven.json atomically: the hook never reads half
// of it.
func WriteDriven(dir string, d Driven) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, drivenName+".*")
	if err != nil {
		return err
	}
	_, werr := f.Write(b)
	if err := errors.Join(werr, f.Close()); err != nil {
		_ = os.Remove(f.Name())
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, drivenName))
}

// RemoveDriven removes driven.json; a missing one is no error.
func RemoveDriven(dir string) error {
	if err := os.Remove(filepath.Join(dir, drivenName)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Guard is one command the order must give the go for: what is merge,
// release or deploy, and N the pull request a merge names (0: none). The
// hook sends the guards it read with the call's event.
type Guard = hooks.Guard

// Guarded is the first command of a shell line that needs a go.
func Guarded(cmd string) (what string, n int, ok bool) {
	gs := Guards(cmd)
	if len(gs) == 0 {
		return "", 0, false
	}
	return gs[0].What, gs[0].N, true
}

// Guards are the commands of a shell line that need a go: `gh pr merge`,
// `gh release create|upload|edit`, `gh workflow run` and the `gh api`
// calls that do the same, at the start of any command of the line (after
// &&, ||, ;, |, a subshell, `sh -c` or `timeout`). It keeps what any of
// three readings finds: the raw line with quotes read as spaces, the same
// for the line the hook scrubs, and the line split into words as the
// shell does (wordGuards). The raw readings skip the here-document bodies
// the shell reading finds are text. A line the shell reading cannot
// finish that names gh is Unread.
func Guards(cmd string) []Guard {
	var data [][2]int
	ws, ok := wordGuards(cmd, 0, &data)
	raw := withoutSpans(cmd, data)
	gs := union(lineGuards(raw), lineGuards(hooks.Scrub(raw)))
	if ok {
		gs = bareAsRead(gs, ws)
	}
	gs = union(gs, ws)
	if !ok && namesGH(cmd) {
		gs = append(gs, Guard{What: Unread})
	}
	return gs
}

// bareAsRead takes, from the shell reading, which merges name no pull
// request. The raw readings split a quoted subject into words, so
// `gh pr merge -t 'feat: x'` reads there as naming "x". Each bare merge
// of the shell reading clears one such merge, never more: a merge only
// the raw readings find keeps its target.
func bareAsRead(raw, ws []Guard) []Guard {
	bare := count(ws, Guard{What: "merge", Bare: true}) - count(raw, Guard{What: "merge", Bare: true})
	out := slices.Clone(raw)
	for i, g := range out {
		if bare > 0 && g == (Guard{What: "merge"}) {
			out[i].Bare = true
			bare--
		}
	}
	return out
}

// withoutSpans is s without the byte spans: the here-document bodies a
// command reads as text.
func withoutSpans(s string, spans [][2]int) string {
	if len(spans) == 0 {
		return s
	}
	slices.SortFunc(spans, func(a, b [2]int) int { return a[0] - b[0] })
	var out strings.Builder
	at := 0
	for _, sp := range spans {
		if sp[0] < at {
			continue
		}
		out.WriteString(s[at:sp[0]])
		at = sp[1]
	}
	out.WriteString(s[at:])
	return out.String()
}

// union is a with each guard b reads more often than a.
func union(a, b []Guard) []Guard {
	out := slices.Clone(a)
	for _, g := range b {
		if count(b, g) > count(out, g) {
			out = append(out, g)
		}
	}
	return out
}

func count(gs []Guard, g Guard) int {
	n := 0
	for _, x := range gs {
		if x == g {
			n++
		}
	}
	return n
}

// lineGuards is one reading of a line: quotes read as spaces.
func lineGuards(cmd string) []Guard {
	cmd = strings.ReplaceAll(cmd, "\\\n", " ")
	var out []Guard
	for _, seg := range strings.FieldsFunc(cmd, func(r rune) bool { return strings.ContainsRune(";&|\n()`", r) }) {
		words := strings.Fields(strings.NewReplacer(`"`, " ", `'`, " ").Replace(seg))
		if g, ok := guardOf(command(words)); ok {
			out = append(out, g)
		}
	}
	return out
}

// isGH tells if a word runs gh: by its base name, so /usr/bin/gh is gh,
// and zsh's =gh too.
func isGH(w string) bool {
	return filepath.Base(strings.TrimPrefix(w, "=")) == "gh"
}

// guardOf reads one command's words, past what leads it, and past gh's own
// flags before its subcommand and its verb.
func guardOf(words []string) (Guard, bool) {
	if len(words) == 0 || !isGH(words[0]) {
		return Guard{}, false
	}
	args := ghArgs(words[1:])
	if len(args) == 0 {
		return Guard{}, false
	}
	sub := args[0]
	if sub == "api" {
		return apiGuard(args[1:])
	}
	rest := ghArgs(args[1:])
	if len(rest) == 0 {
		return Guard{}, false
	}
	switch verb := rest[0]; {
	case sub == "pr" && verb == "merge":
		n, named := prTarget(rest[1:])
		return Guard{What: "merge", N: n, Bare: !named}, true
	case sub == "release" && (verb == "create" || verb == "new" || verb == "upload" || verb == "edit"):
		return Guard{What: "release"}, true
	case sub == "workflow" && verb == "run":
		return Guard{What: "deploy"}, true
	}
	return Guard{}, false
}

// ghArgs skips the flags gh reads before a subcommand or before its verb:
// -R and --repo take the next word.
func ghArgs(args []string) []string {
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		if (args[0] == "-R" || args[0] == "--repo") && len(args) > 1 {
			args = args[1:]
		}
		args = args[1:]
	}
	return args
}

// shellC is a shell flag that ends in c: -c, -lc, -ec, -ic.
var shellC = regexp.MustCompile(`^-[a-z]*c$`)

// command skips what runs a command without being it: variable
// assignments, the leaders, `timeout <duration>`, `find … -exec` and a
// shell's -c, whose script it reads as more words.
func command(words []string) []string {
	for {
		rest, script := lead(words)
		if !script {
			return rest
		}
		words = rest
	}
}

// leaders run the command after their options without being it, each
// with the options that take the next word.
var leaders = map[string][]string{
	"{": nil, "!": nil, "if": nil, "then": nil, "elif": nil, "else": nil, "do": nil, "while": nil, "until": nil,
	"env":     {"-u", "--unset", "-C", "--chdir"},
	"sudo":    {"-u", "--user", "-g", "--group", "-C", "-D", "-h", "-p", "-r", "-t", "-U"},
	"time":    {"-f", "--format", "-o", "--output"},
	"command": nil, "builtin": nil, "nohup": nil, "setsid": nil, "coproc": nil, "noglob": nil, "nocorrect": nil, "-": nil,
	"exec":   {"-a"},
	"nice":   {"-n", "--adjustment"},
	"ionice": {"-c", "--class", "-n", "--classdata", "-p", "--pid"},
	"stdbuf": {"-i", "-o", "-e", "--input", "--output", "--error"},
	"xargs": {"-I", "-a", "-n", "-P", "-L", "-d", "-E", "-s", "--arg-file", "--max-args", "--max-procs",
		"--delimiter", "--max-lines", "--eof", "--max-chars"},
	"watch": {"-n", "--interval"},
}

// shells run the script after their -c; script(1) runs one the same way.
var shells = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true, "mksh": true, "fish": true, "script": true}

// assignment is a variable assignment that leads a command.
var assignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\[[^]]*\])?\+?=`)

// lead skips what runs a command without being it, up to a shell's -c:
// script tells that rest starts with the script the shell runs. A leader
// is known by its base name, so /usr/bin/env is env.
func lead(words []string) (rest []string, script bool) {
	for len(words) > 0 {
		w := filepath.Base(words[0])
		valued, leader := leaders[w]
		switch {
		case leader:
			words = afterOptions(words[1:], valued)
		case w == "timeout":
			words = afterTimeout(words[1:])
		case w == "find":
			i := slices.IndexFunc(words, func(x string) bool {
				return x == "-exec" || x == "-execdir" || x == "-ok" || x == "-okdir"
			})
			if i < 0 {
				return words, false
			}
			words = words[i+1:]
		case shells[w]:
			rest, ok := afterShellC(words[1:])
			if !ok {
				return words, false
			}
			return rest, true
		case assignment.MatchString(words[0]):
			words = words[1:]
		default:
			return words, false
		}
	}
	return words, false
}

// afterOptions skips a leader's options: those in valued take the next
// word, and -- ends them.
func afterOptions(words, valued []string) []string {
	for len(words) > 0 && strings.HasPrefix(words[0], "-") {
		o := words[0]
		words = words[1:]
		if o == "--" {
			break
		}
		if slices.Contains(valued, o) && len(words) > 0 {
			words = words[1:]
		}
	}
	return words
}

// afterShellC is what follows a shell's -c style flag, past its other
// options (-l, --login, -o pipefail); false: the shell runs no -c.
func afterShellC(args []string) ([]string, bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case shellC.MatchString(a):
			return args[i+1:], true
		case a == "-o" || a == "+o" || a == "-O" || a == "+O":
			i++
		case strings.HasPrefix(a, "-") || strings.HasPrefix(a, "+"):
		default:
			return nil, false
		}
	}
	return nil, false
}

// afterTimeout skips timeout's options and its duration.
func afterTimeout(words []string) []string {
	for len(words) > 0 && strings.HasPrefix(words[0], "-") {
		if words[0] == "-s" || words[0] == "-k" || words[0] == "--signal" || words[0] == "--kill-after" {
			words = words[1:]
		}
		words = words[1:]
	}
	if len(words) > 0 {
		words = words[1:] // the duration
	}
	return words
}

var pullMerge = regexp.MustCompile(`/pulls/([0-9]+)/merge\b`)

// apiGuard reads a `gh api` call: a PR's /merge, a /dispatches (a workflow
// or repository dispatch), a write to /releases and a GraphQL mutation that
// merges or releases, whatever flags come before graphql.
func apiGuard(args []string) (Guard, bool) {
	method, fields := "", false
	var paths []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-X" || a == "--method":
			if i+1 < len(args) {
				method = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--method="):
			method = strings.TrimPrefix(a, "--method=")
		case strings.HasPrefix(a, "-X") && len(a) > 2:
			method = a[2:]
		case a == "-f" || a == "-F" || a == "--field" || a == "--raw-field" || a == "--input":
			fields = true
			i++
		case strings.HasPrefix(a, "--field=") || strings.HasPrefix(a, "--raw-field=") || strings.HasPrefix(a, "--input=") ||
			strings.HasPrefix(a, "-f") || strings.HasPrefix(a, "-F"):
			fields = true
		case strings.HasPrefix(a, "-"):
		default:
			paths = append(paths, a)
		}
	}
	method = strings.ToUpper(method)
	if method == "" && fields {
		method = "POST"
	}
	write := method == "POST" || method == "PATCH" || method == "PUT"
	if slices.Contains(paths, "graphql") {
		all := strings.Join(args, " ")
		if strings.Contains(all, "mergePullRequest") {
			return Guard{What: "merge"}, true
		}
		if strings.Contains(all, "createRelease") || strings.Contains(all, "updateRelease") {
			return Guard{What: "release"}, true
		}
	}
	for _, p := range paths {
		if m := pullMerge.FindStringSubmatch(p); m != nil {
			n, _ := strconv.Atoi(m[1])
			return Guard{What: "merge", N: n}, true
		}
		if strings.Contains(p, "/dispatches") {
			return Guard{What: "deploy", N: 0}, true
		}
		if strings.Contains(p, "/releases") && write {
			return Guard{What: "release", N: 0}, true
		}
	}
	return Guard{}, false
}

// mergeValueFlags are the flags of `gh pr merge` that take the next word.
var mergeValueFlags = []string{"-t", "--subject", "-b", "--body", "-F", "--body-file", "-R", "--repo", "-A", "--author-email", "--match-head-commit"}

// prTarget is the pull request `gh pr merge` names: 12, #12 or its URL. A
// branch name is 0 and named; no argument is 0 and not named.
func prTarget(args []string) (n int, named bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			if slices.Contains(mergeValueFlags, a) {
				i++
			}
			continue
		}
		if _, rest, ok := strings.Cut(a, "/pull/"); ok {
			a = strings.TrimRight(rest, "/")
		}
		n, err := strconv.Atoi(strings.TrimPrefix(a, "#"))
		if err != nil || n < 1 {
			return 0, true
		}
		return n, true
	}
	return 0, false
}

// Allowed tells if the grants give the go: "merge" for every merge,
// "merge:#<n>" for that one, "release" and "deploy" for their own.
func Allowed(grants []string, what string, n int) bool {
	for _, g := range grants {
		if g == what || what == "merge" && n > 0 && g == "merge:#"+strconv.Itoa(n) {
			return true
		}
	}
	return false
}

// The reasons a session reads when the guard stops a call, each naming
// why: DenyNoGo is the spec's own words.
const (
	DenyNoGo       = "the stoker's order did not give the go for this. Give it on the stoker's card (y) or stop the stoker."
	DenyStokerPane = "the stoker's own panes never merge, release or deploy during a run, whatever the order gives. Have a session the stoker drives do it, or stop the stoker."
	DenyUnread     = "the guard could not read this line to its end (a quote left open, or sh -c nested too deep), so it does not run in a session the stoker drives. Write it plainly."
	DenyLost       = "the stoker's run lost its record of the panes it drove, so nothing merges, releases or deploys until it is back. Stop the stoker, or wait for its next sample."
)

// Denies tells if Deny refuses the call.
func Denies(dir, socket, pane string, gs []Guard) bool {
	return Deny(dir, socket, pane, gs) != ""
}

// Deny is the merge guard of a pane, and why it refuses ("": it allows).
// socket is the tmux server the call runs on (from $TMUX), pane its pane
// there ($TMUX_PANE). A guarded command needs a go in a pane the stoker
// drove, and every pane of the stoker's own server never runs one,
// whatever the order gives: one the last sample did not list yet too.
// The builder's pane of the same id is not the stoker's. Without
// driven.json while a run is on, it denies in every pane: the mark is
// lost, so it fails closed.
func Deny(dir, socket, pane string, gs []Guard) string {
	if len(gs) == 0 {
		return ""
	}
	d, err := ReadDriven(dir)
	if err != nil {
		if m, err := ReadMode(dir); err == nil && m.On {
			return DenyLost
		}
		return ""
	}
	if d.StokerSocket != "" && socket == d.StokerSocket {
		return DenyStokerPane
	}
	grants, driven := d.Panes[pane]
	if !driven {
		return ""
	}
	if slices.Contains(gs, Guard{What: Unread}) {
		return DenyUnread
	}
	if _, short := d.Spend(pane, gs); len(short) > 0 {
		if who := d.notKnownYet(pane, slices.Concat(grants, d.Steps[pane]), short[0]); who != "" {
			return DenyNotKnown(who, short[0].N)
		}
		return DenyNoGo
	}
	return ""
}

// Spend matches the guarded commands of one line to the goes of pane: the
// order's goes cover any number, and each go given with y covers one. It
// returns the step goes left over and the guards nothing covered.
func (d Driven) Spend(pane string, gs []Guard) (left []string, short []Guard) {
	left = slices.Clone(d.Steps[pane])
	for _, g := range gs {
		if d.Allows(pane, d.Panes[pane], g) {
			continue
		}
		if i := slices.IndexFunc(left, func(x string) bool { return d.Allows(pane, []string{x}, g) }); i >= 0 {
			left = slices.Delete(left, i, i+1)
			continue
		}
		short = append(short, g)
	}
	return left, short
}

// DenyNotKnown is why a merge:@<who> go denies a pull request the ships
// poll has not tied to who yet.
func DenyNotKnown(who string, n int) string {
	return fmt.Sprintf("the order gave merge:@%s, and #%d is not known yet as %s's pull request: matchblox learns of a new one within about a minute. Try again then.", who, n, who)
}

// notKnownYet is the session a merge:@ go of pane names, when the merge
// is by a number no session is known to drive: one it may have just
// opened. A number another session drives is not its own.
func (d Driven) notKnownYet(pane string, grants []string, g Guard) string {
	if g.What != "merge" || g.N <= 0 {
		return ""
	}
	for _, o := range d.Own {
		if slices.Contains(o.PRs, g.N) {
			return ""
		}
	}
	for _, gr := range grants {
		if who, ok := strings.CutPrefix(gr, "merge:@"); ok && slices.Contains(d.Own[pane].Names, who) {
			return who
		}
	}
	return ""
}
