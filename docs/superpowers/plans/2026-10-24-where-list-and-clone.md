# N's where list and git-URL clone Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** N's `where` opens a ranked, filterable list of the builder's repos (worktrees under their repo), and a pasted git URL is cloned by the service into `clone_root` while the builder fills the rest of the form.

**Architecture:** A new `internal/clone` package holds everything about a remote that needs no service state: parsing a URL, comparing remotes, picking a free folder name, running `git clone` with progress, and saying a failure in plain words. The service adds a record of where sessions started (`used.json`), three acts (`clone:plan`, `clone:start`, `clone:stop`), clone progress in `proto.State.Clones`, and a URL path inside `new:session` for the CLI and old clients. The console builds the list from the Git tab, the found repos and the used record (`internal/app/wherelist.go`) and draws it in the form as layout C.

**Tech Stack:** Go, Bubble Tea v2 (`charm.land/bubbletea/v2`), bubbles `key.Binding` (`bkey`), lipgloss v2, git CLI.

**Spec:** `docs/superpowers/specs/2026-10-23-where-list-and-clone-design.md` (read it first; it wins where this plan is silent).

## Global Constraints

- Work only in the worktree `/Users/martinyankov/Lexroom/dev/matchblox-174` (branch `feat/174-176-where-list-clone`). Never touch `~/Lexroom/dev/matchblox`.
- Every commit is signed (`git commit -S`), conventional (`feat:` `fix:` `docs:` `test:` `refactor:`), names `(#174)` / `(#176)` / `(#174, #176)` at the end of the subject, and ends with the line `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Keys are `bkey.Binding` values in `internal/app/keys.go`; the router is `focus(k)`; hints and footers come from bindings (`hintFor` / `footFor`). No key string literals outside keys.go: `TestNoKeyStringLiterals` and `TestKeymapsHaveNoCollisions` stay green.
- DESIGN.md: colours only from tokens (`fg muted faint hair accent wash warn neg`); no match glyph (`✦ ╿ │`) on a repo row; no spinner; ASD-STE100 voice, lower case, no exclamation marks.
- No real people, companies, machine names or home paths in code, tests or docs (use `~/dev`, `~/src`, `o/r`, `api`, `notes`, `shop-api`).
- `matchblox new --where <path>` and an old client sending a path in `NewSession.Where` keep working unchanged. `proto.Version` stays the same; new wire fields are optional.
- Clone: a full clone (no `--depth`); `GIT_TERMINAL_PROMPT=0`; only `https://`, `ssh://`, `git@host:` URLs and the `o/r` / `gh:o/r` shorthand; the URL goes to git after `--`.
- Free-name words, in this order: `ember flint spark cinder tinder wick flare char soot coal`; when all are taken, `<word>-<4 chars of [a-z0-9]>`.
- `clone_root` default `~/src`, key `[git] clone_root`.
- Before each commit: `go vet ./...` and the package's tests pass. Before the last task's commit: `go test ./...` and `scripts/validate.sh`.

## Review Focus

1. **A URL that is really an option or a local transport** (`-uhack`, `--upload-pack=x`, `ext::sh -c …`, `file:///etc`, `/abs/path`): never cloned, never passed to git. Test in Task 1 (`TestParseRefusesOptionsAndTransports`).
2. **A folder appears at the target between plan and start, or the service dies mid-clone:** nothing is written into an existing folder, and a crash leaves no half-cloned folder under the real name. The clone goes into a hidden sibling `.<name>.cloning` and is renamed on success; cancel and failure delete only that sibling. Tests in Task 2 (`TestRunNeverWritesIntoAnExistingFolder`, `TestRunCancelLeavesNothing`).
3. **Two consoles (or a console and the CLI) clone to the same target at once:** the second is refused with `~/src/r is being cloned`. Test in Task 4 (`TestCloneStartTwiceRefusesTheSecond`).
4. **A narrow console (44 columns, a phone):** the list still draws in the box; the path cell is cut first (`…`), the name never. Test in Task 6 (`TestWhereListFitsAPhone`).
5. **A typed `~/o/r` or `./o/r`, or a pasted path with spaces:** stays a path, never a clone row. Test in Task 1 (`TestParseLeavesPathsAlone`) and Task 7 (`TestTildePathIsNoClone`).

---

## File map

| File | Does |
|---|---|
| `internal/clone/remote.go` (new) | `Remote`, `Parse`, `Same`, `Slug` |
| `internal/clone/name.go` (new) | `Words`, `FreeName` |
| `internal/clone/run.go` (new) | `Run`, `Progress`, `Explain` |
| `internal/clone/*_test.go` (new) | its tests |
| `internal/state/used.go` (new) | `Use`, `Used`, `ReadUsed`, `WriteUsed`, `(Used).Add`, `UsedPath` |
| `internal/proto/proto.go` | `ClonePlan`, `CloneStart`, `Clone`, `Used` alias, `State.Used`, `State.Clones`, `Result.Clone` |
| `internal/config/config.go` | `Git.CloneRoot` |
| `internal/service/used.go` (new) | load, record and send the used record |
| `internal/service/clone.go` (new) | `clone:plan`, `clone:start`, `clone:stop`, progress, the URL path of `new:session` |
| `internal/service/lifecycle.go`, `actions.go`, `service.go` | hooks into the above |
| `cmd/matchblox/main.go`, `life.go` | wire paths and config; CLI `--where <url>` |
| `internal/app/wherelist.go` (new) | the known repos, the rows, the fuzzy score |
| `internal/app/lifeform.go` | the list in the form, keys, the clone flow |
| `internal/app/keys.go` | no new binding expected; help text changes |
| docs: `docs/keys.md`, `docs/config.md`, `docs/console.md`, `README.md`, `CHANGELOG.md`, DESIGN.md (§6 if the list adds a rule), replay frames |

---

### Task 1: `internal/clone`: parse, compare and name

**Files:**
- Create: `internal/clone/remote.go`, `internal/clone/name.go`, `internal/clone/remote_test.go`, `internal/clone/name_test.go`

**Interfaces:**
- Produces:
  - `type Remote struct { URL, Host, Owner, Repo string }` — `URL` is what git clones.
  - `func (r Remote) Slug() string` — `"o/r"`.
  - `func Parse(s, protocol string) (Remote, bool)` — `protocol` is `"ssh"` or `"https"`, used only for `o/r` and `gh:o/r` (host `github.com`). ssh shorthand → `git@github.com:o/r.git`; https → `https://github.com/o/r.git`.
  - `func Same(a, b string) bool` — two remote URLs name the same repo.
  - `var Words = []string{"ember", "flint", "spark", "cinder", "tinder", "wick", "flare", "char", "soot", "coal"}`
  - `func FreeName(repo string, taken func(name string) bool, rnd *rand.Rand) string` — `math/rand/v2`.

- [ ] **Step 1: Write the failing tests**

```go
func TestParse(t *testing.T) {
	for _, c := range []struct{ in, proto, url, slug string }{
		{"https://github.com/o/r", "https", "https://github.com/o/r", "o/r"},
		{"https://github.com/o/r.git", "https", "https://github.com/o/r.git", "o/r"},
		{"git@github.com:o/r.git", "https", "git@github.com:o/r.git", "o/r"},
		{"ssh://git@gitlab.example/o/r.git", "https", "ssh://git@gitlab.example/o/r.git", "o/r"},
		{"gh:o/r", "ssh", "git@github.com:o/r.git", "o/r"},
		{"o/r", "https", "https://github.com/o/r.git", "o/r"},
		{" https://github.com/o/r/ ", "https", "https://github.com/o/r", "o/r"},
	} {
		r, ok := Parse(c.in, c.proto)
		if !ok || r.URL != c.url || r.Slug() != c.slug || r.Repo != "r" {
			t.Errorf("Parse(%q) = %+v %v", c.in, r, ok)
		}
	}
}

func TestParseLeavesPathsAlone(t *testing.T) {
	for _, s := range []string{"", "~/o/r", "./o/r", "/src/o/r", "o", "o/r/x", "~/My Work/r", "o/r x"} {
		if r, ok := Parse(s, "https"); ok {
			t.Errorf("Parse(%q) = %+v, want a path", s, r)
		}
	}
}

func TestParseRefusesOptionsAndTransports(t *testing.T) {
	for _, s := range []string{"-uhack", "--upload-pack=x", "ext::sh -c x", "file:///etc", "https://-x/o/r", "git@-x:o/r", "http://github.com/o/r"} {
		if r, ok := Parse(s, "https"); ok {
			t.Errorf("Parse(%q) = %+v, want refused", s, r)
		}
	}
}

func TestSame(t *testing.T) {
	if !Same("https://github.com/O/r.git", "git@github.com:o/r") || !Same("ssh://git@github.com/o/r", "https://github.com/o/r/") {
		t.Fatal("same repo not seen")
	}
	if Same("https://github.com/o/r", "https://github.com/o/r2") || Same("https://github.com/o/r", "https://gitlab.com/o/r") || Same("", "") {
		t.Fatal("different repos seen as one")
	}
}

func TestFreeName(t *testing.T) {
	rnd := rand.New(rand.NewPCG(1, 2))
	taken := map[string]bool{"r": true}
	got := FreeName("r", func(n string) bool { return taken[n] }, rnd)
	if !strings.HasPrefix(got, "r-") || !slices.Contains(Words, strings.TrimPrefix(got, "r-")) {
		t.Fatalf("FreeName = %q", got)
	}
	for _, w := range Words {
		taken["r-"+w] = true
	}
	got = FreeName("r", func(n string) bool { return taken[n] }, rnd)
	if !regexp.MustCompile(`^r-(ember|flint|spark|cinder|tinder|wick|flare|char|soot|coal)-[a-z0-9]{4}$`).MatchString(got) {
		t.Fatalf("FreeName with every word taken = %q", got)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/clone/`
Expected: FAIL (package does not build: `Parse` undefined).

- [ ] **Step 3: Implement `Parse`, `Slug`, `Same` in `remote.go` and `Words`, `FreeName` in `name.go`**

`Parse` trims spaces and one trailing `/`; refuses anything with a space inside, a leading `-`, a host or owner starting with `-`, `::`, or a scheme other than `https`/`ssh`. Shorthand is exactly `owner/repo` (one slash, no leading `~ . /`) or `gh:owner/repo`, each part `[A-Za-z0-9._-]+` and not starting with `-`. `Same` lowercases host and owner, drops scheme, user, `.git` and a trailing `/`, then compares `host/owner/repo`; empty is never the same. `FreeName` tries `repo` is the caller's job: it returns `repo-<word>` for a random free word (`rnd.Perm`), else `repo-<word>-<4 chars>` retried until free. Doc comments in the house voice.

- [ ] **Step 4: Run them to see them pass**

Run: `go test ./internal/clone/ && go vet ./internal/clone/`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/clone
git commit -S -m "feat: a git URL is parsed, compared and given a free folder name (#176)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: `internal/clone`: run git clone, with progress and plain-word failures

**Files:**
- Create: `internal/clone/run.go`, `internal/clone/run_test.go`

**Interfaces:**
- Consumes: `Remote` (Task 1).
- Produces:
  - `func Progress(line string) (pct int, ok bool)` — reads one git progress line (`Receiving objects:  42% (…)`, `Resolving deltas: 100% …`); only `Receiving objects` gives a percent.
  - `func Explain(stderr string, r Remote) string` — the failure in plain words (copy below).
  - `func Run(ctx context.Context, r Remote, target string, progress func(pct int)) error` — clones `r.URL` into `target` through the sibling `filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+".cloning")`, renamed to `target` on success. Errors when `target` or the sibling exists, before running git. On failure or ctx end it deletes the sibling only, and the error's text is `Explain(...)` (or `"stopped"` for ctx end).
  - `func Cloning(target string) string` — the sibling's path (the service and `FreeName`'s `taken` use it).

Failure copy (exact):

| git says (stderr contains) | Explain returns |
|---|---|
| `Permission denied (publickey)` | `no access to o/r: add an SSH key for github.com, or use its https URL` |
| `could not read Username`, `Authentication failed` | `no access to o/r: run gh auth login` |
| `Repository not found`, `not found` | `o/r is not on github.com, or you have no access: run gh auth login` |
| `Could not resolve host`, `Network is unreachable`, `Connection timed out` | `no network: github.com does not answer` |
| `No space left on device` | `the disk is full: o/r is not cloned` |
| anything else | `git clone o/r failed: <last non-empty stderr line>` |

(`github.com` is `r.Host`; `o/r` is `r.Slug()`.)

- [ ] **Step 1: Write the failing tests**

```go
func TestProgress(t *testing.T) {
	for line, want := range map[string]int{"Receiving objects:  42% (420/1000), 1.2 MiB | 2 MiB/s": 42, "Receiving objects: 100% (1000/1000), done.": 100} {
		if got, ok := Progress(line); !ok || got != want {
			t.Errorf("Progress(%q) = %d %v", line, got, ok)
		}
	}
	if _, ok := Progress("Resolving deltas:  50% (1/2)"); ok {
		t.Error("Resolving deltas gave a percent")
	}
}

func TestExplain(t *testing.T) {
	r := Remote{Host: "github.com", Owner: "o", Repo: "r"}
	if got := Explain("git@github.com: Permission denied (publickey).\nfatal: Could not read from remote repository.", r); got != "no access to o/r: add an SSH key for github.com, or use its https URL" {
		t.Error(got)
	}
	if got := Explain("fatal: unable to access 'https://github.com/o/r/': Could not resolve host: github.com", r); got != "no network: github.com does not answer" {
		t.Error(got)
	}
}

// localRepo makes a repo with one commit and returns its path, cloned as a URL by Run.
func TestRunClonesAndReportsProgress(t *testing.T)        // target has .git, no ".r.cloning" left, progress got ≥1 call or none (a tiny repo may give none)
func TestRunNeverWritesIntoAnExistingFolder(t *testing.T) // target exists with a file "keep": Run errors, "keep" is unchanged, nothing else in the folder
func TestRunCancelLeavesNothing(t *testing.T)             // ctx cancelled before Run: error "stopped", neither target nor sibling exists
func TestRunFailureLeavesNothing(t *testing.T)            // URL of a missing local repo: error non-empty, neither target nor sibling exists
```

The run tests use a local repo made with `git init` + one commit in `t.TempDir()`, passed as `Remote{URL: path, Host: "github.com", Owner: "o", Repo: "r"}` (Run does not re-check the URL: Parse is the gate).

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/clone/ -run 'Progress|Explain|Run'`
Expected: FAIL (`Progress` undefined).

- [ ] **Step 3: Implement `run.go`**

`exec.CommandContext(ctx, "git", "clone", "--progress", "--", r.URL, sibling)` with `Env: append(os.Environ(), "GIT_TERMINAL_PROMPT=0")`; read stderr split on `\r` and `\n`, call `progress` for each `Progress` hit, keep the last 20 lines for `Explain`. `os.Rename(sibling, target)` after success; if the rename fails (target appeared), delete the sibling and return `<target> is there already`.

- [ ] **Step 4: Run them to see them pass**

Run: `go test ./internal/clone/ && go vet ./internal/clone/`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/clone
git commit -S -m "feat: a clone runs beside its folder and leaves nothing when it stops (#176)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: the used record: where sessions started, sent to consoles

**Files:**
- Create: `internal/state/used.go`, `internal/state/used_test.go`, `internal/service/used.go`, `internal/service/used_test.go`
- Modify: `internal/proto/proto.go` (State), `internal/service/service.go` (fields, `stateFor`), `internal/service/lifecycle.go` (`newSession`), `cmd/matchblox/main.go` (path)

**Interfaces:**
- Produces:
  - `state`: `type Use struct { Path string \`json:"path"\`; At time.Time \`json:"at"\` }`; `type Used struct { Repos []Use \`json:"repos,omitempty"\` }`; `const MaxUsed = 200`; `func (u Used) Add(at time.Time, paths ...string) Used` (each path cleaned, moved to the front, newest first, deduped, cut to `MaxUsed`); `func UsedPath() string` (next to `ReposPath()`, file `used.json`); `func ReadUsed(path string) Used` (empty on a missing or broken file); `func WriteUsed(path string, u Used) error`.
  - `proto`: `type Used = state.Used`, `type Use = state.Use`; `State.Used *Used \`json:"used,omitempty"\`` — sent in a console's first snapshot and when it changes (a generation counter, as `Found`).
  - `service`: field `UsedPath string`; `func (s *Service) loadUsed()`; `func (s *Service) addUsed(paths ...string)` (adds, writes, bumps the generation, broadcasts).

- [ ] **Step 1: Write the failing tests**

```go
// internal/state/used_test.go
func TestUsedAddKeepsTheNewestFirstOnce(t *testing.T) {
	t0 := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	u := Used{}.Add(t0, "/src/a", "/src/b/").Add(t0.Add(time.Hour), "/src/a")
	if len(u.Repos) != 2 || u.Repos[0].Path != "/src/a" || !u.Repos[0].At.Equal(t0.Add(time.Hour)) || u.Repos[1].Path != "/src/b" {
		t.Fatalf("%+v", u)
	}
	for i := range MaxUsed + 5 {
		u = u.Add(t0, fmt.Sprintf("/src/x%d", i))
	}
	if len(u.Repos) != MaxUsed {
		t.Fatalf("%d repos kept", len(u.Repos))
	}
}

// internal/service/used_test.go
func TestNewSessionRecordsWhereItStarted(t *testing.T)   // a started session in /src/api adds "/src/api" to used.json and the next snapshot's State.Used; a refused one adds nothing
func TestNewSessionWithABranchRecordsRepoAndWorktree(t *testing.T) // both the repo path and the worktree path are recorded
func TestUsedIsSentOnceThenOnChange(t *testing.T)        // first snapshot has Used; the next has nil; after addUsed, the next has it again
func TestUsedNeedsNoScanConsent(t *testing.T)            // Find == nil, consent "no": Used is still recorded and sent
```

Use the existing service test helpers for `newSession` (see `internal/service/lifecycle_test.go` for how a fake tmux and `LookPath` are set).

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/state/ ./internal/service/ -run 'Used|Records'`
Expected: FAIL (`Used` undefined).

- [ ] **Step 3: Implement**

`state/used.go` as the interface says. In the service, `loadUsed` runs where `loadFound` runs; `addUsed(n.Where, dir)` runs in `newSession` after the steps ran (both cleaned; the same path once). `stateFor` sends `Used` when `cl.usedGen != s.usedGen`. `cmd/matchblox/main.go` sets `s.UsedPath = state.UsedPath()` beside `ConsentPath`/`ReposPath`. `scan off` does not touch `used.json`.

- [ ] **Step 4: Run them to see them pass**

Run: `go test ./internal/state/ ./internal/service/ ./internal/proto/ && go vet ./...`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/state internal/proto internal/service cmd/matchblox
git commit -S -m "feat: matchblox remembers where its sessions started, for N's list (#174)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: the service clones: plan, start, stop, progress

**Files:**
- Create: `internal/service/clone.go`, `internal/service/clone_test.go`
- Modify: `internal/proto/proto.go`, `internal/config/config.go` (+ its test), `internal/service/actions.go`, `internal/service/service.go`, `cmd/matchblox/main.go`

**Interfaces:**
- Consumes: `clone.Parse`, `clone.Same`, `clone.FreeName`, `clone.Run`, `clone.Cloning` (Tasks 1–2); `addUsed` (Task 3); `s.found` (its `Repos[].Remote`).
- Produces:
  - `config.Git.CloneRoot string \`toml:"clone_root"\`` default `"~/src"`; service field `CloneRoot string` (expanded, absolute).
  - `proto.ClonePlan { Repo string \`json:"repo"\`; URL string \`json:"url"\`; Target string \`json:"target"\`; Taken string \`json:"taken,omitempty"\`; Have []string \`json:"have,omitempty"\`; Fresh string \`json:"fresh,omitempty"\` }`.
    - `Repo`: `o/r`. `Target`: where a clone goes now: `<root>/<r>`, or a free name when `<root>/<r>` is taken by something else.
    - `Taken`: why `<root>/<r>` was not used: `"another repo"`, `"not a repo"`, `"a file"`; `""` when free or the same repo.
    - `Have`: checkouts with the same remote, `<root>/<r>` first when it is one, then known ones (found list, used record) by path. When `<root>/<r>` is the same repo, `Target` is `<root>/<r>` and `Fresh` is a free name for "clone anew".
  - `proto.Result.Clone *ClonePlan \`json:"clone,omitempty"\``.
  - `proto.CloneStart { URL string \`json:"url"\`; Target string \`json:"target"\` }`.
  - `proto.Clone { Target string \`json:"target"\`; Repo string \`json:"repo"\`; Pct int \`json:"pct"\` /* -1: no percent yet */; Err string \`json:"err,omitempty"\`; Done bool \`json:"done,omitempty"\` }`; `proto.State.Clones []Clone \`json:"clones,omitempty"\`` — every snapshot while any is listed; a done or failed clone stays listed 60 s.
  - Acts: `clone:plan` (Text: what was typed) → `Result.Clone` or `Err` (`not a git URL`); `clone:start` (Text: `CloneStart` JSON) → empty Result at once, the clone runs in the background; `clone:stop` (Text: target) → stops it.
  - `func (s *Service) gitProtocol() string` — `gh config get git_protocol` once (LookPath `gh`; 2 s bound), `"ssh"` or else `"https"`.

- [ ] **Step 1: Write the failing tests**

```go
func TestClonePlanFreeTarget(t *testing.T)          // root empty: plan {Repo "o/r", Target root+"/r", Taken "", Have nil}
func TestClonePlanSameRepoAtTarget(t *testing.T)    // root/r is a clone of the same remote (ssh vs https): Target root/r, Have[0] root/r, Fresh root/r-<word>
func TestClonePlanAnotherRepoAtTarget(t *testing.T) // root/r has another remote: Taken "another repo", Target root/r-<word>, Have nil
func TestClonePlanFileAtTarget(t *testing.T)        // root/r is a file: Taken "a file"
func TestClonePlanKnownElsewhere(t *testing.T)      // found has /dev/r with the same remote: Have == ["/dev/r"], Target root/r
func TestClonePlanIsOffline(t *testing.T)           // a plan runs no git that touches the network: LookPath for git is never asked to fetch (assert by a fake git on PATH that fails the test on "fetch"/"ls-remote"/"clone")
func TestClonePlanRefusesAPath(t *testing.T)        // Text "~/src/r" and "/src/r": Err "not a git URL"
func TestCloneStartRunsAndReports(t *testing.T)     // a local repo as URL (test hook skipping Parse): State.Clones shows the target, then Done; root/r has .git; used record has root/r
func TestCloneStartRefusesAnExistingTarget(t *testing.T)
func TestCloneStartTwiceRefusesTheSecond(t *testing.T) // second start to the same target: Err "~/src/r is being cloned" (path shown with ~)
func TestCloneStopDeletesThePartial(t *testing.T)   // start a clone of a slow fake git, stop: Clones shows Err "stopped", no target, no sibling
func TestCloneFromAnAgentsPaneIsRefused(t *testing.T) // clone:start with From = an agent's pane, not Console: Err notTheBuilders
func TestCloneRootDefault(t *testing.T)             // config.Default().Git.CloneRoot == "~/src"; a config with clone_root = "~/code" reads it
```

A test may set an unexported hook (`s.cloneRun func(ctx, clone.Remote, target string, progress func(int)) error`, nil → `clone.Run`) to use a slow fake.

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/service/ ./internal/config/ -run 'Clone'`
Expected: FAIL (undefined).

- [ ] **Step 3: Implement `internal/service/clone.go` and the hooks**

A map `clones map[string]*cloneRun` (target → its cancel, `proto.Clone`), under `s.mu`. `clone:start` re-plans the target (refuse if it now exists, if it is listed, or if it is not under `CloneRoot` and not the plan's), makes `CloneRoot` (`os.MkdirAll`, 0o755), runs the clone in a goroutine on the service's context, updates `Pct` (throttled to a change of the integer) and broadcasts, then `addUsed(target)` on success. Dispatch the three acts before the dedupe in `act`/`dispatch`, and refuse `clone:start` and `clone:plan` from an agent's pane as `new:session` is (extend the condition in `act`). Free-name `taken` is "exists on disk, or its `clone.Cloning` sibling exists, or a run is listed for it".

- [ ] **Step 4: Run them to see them pass**

Run: `go test -race ./internal/service/ ./internal/config/ ./internal/proto/ && go vet ./...`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/service internal/proto internal/config cmd/matchblox
git commit -S -m "feat: the service plans and runs a clone, and says how far it is (#176)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: a git URL in `new:session`, and `matchblox new --where <url>`

**Files:**
- Modify: `internal/service/clone.go`, `internal/service/lifecycle.go`, `internal/service/service.go` (`HandlePeer`), `cmd/matchblox/life.go`, `cmd/matchblox/cli.go`
- Test: `internal/service/clone_test.go`, `cmd/matchblox/life_test.go`

**Interfaces:**
- Consumes: Task 4's plan and run.
- Produces: `newSession` accepts `NewSession.Where` that `clone.Parse` reads (and that is not an absolute or `~` path): it plans; when `Have` is not empty it starts in `Have[0]`; otherwise it clones into `Target` synchronously, then goes on as today with `Where = Target`. The clone stops when the act's connection closes. `Result.Skipped` gets a first line naming what happened: `cloned o/r into ~/src/r` or `o/r is already at ~/src/r: started there`.
- CLI: `newSessionAct` sends `o.where` unchanged when it is not absolute, does not start with `~`, has no folder at `filepath.Join(here, o.where)`, and `clone.Parse(o.where, "https")` reads it; otherwise as today. `cliLife` for `new` uses `doActFor(path, a, cloneWait)` with `const cloneWait = 30 * time.Minute` when it sent a URL, and prints `cloning o/r…` first.

- [ ] **Step 1: Write the failing tests**

```go
func TestNewSessionClonesAURL(t *testing.T)            // Where "https://…/o/r" (hook-cloned from a local repo): session starts in root/r; Ran has the tmux step; Skipped[0] "cloned o/r into ~/src/r"
func TestNewSessionURLUsesTheExistingClone(t *testing.T) // root/r is the same repo: no clone run; starts there; Skipped[0] "o/r is already at ~/src/r: started there"
func TestNewSessionURLStopsWhenTheCLILeaves(t *testing.T) // slow fake clone; close the conn: no target, no sibling
func TestNewSessionPathStillWorks(t *testing.T)        // Where "/src/api" as before: unchanged result (guards the old client)
// cmd/matchblox/life_test.go
func TestNewWhereURLIsSentAsIs(t *testing.T)           // --where o/r with no folder o/r here: Where "o/r"; with a folder o/r here: Where here+"/o/r"; --where https://github.com/o/r: as is
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/service/ ./cmd/matchblox/ -run 'URL|NewSessionPath|NewWhere'`
Expected: FAIL.

- [ ] **Step 3: Implement**

In `HandlePeer`, make a context that ends when the connection's loop returns and give it to the URL path only (for example as a context value read by `newSession`), so other acts keep today's context.

- [ ] **Step 4: Run them to see them pass**

Run: `go test -race ./internal/service/ ./cmd/matchblox/ && go vet ./...`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/service cmd/matchblox
git commit -S -m "feat: matchblox new --where <url> clones and starts there (#176)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: the where list in the form (layout C)

**Files:**
- Create: `internal/app/wherelist.go`, `internal/app/wherelist_test.go`
- Modify: `internal/app/lifeform.go` (form state, `openNew`, `formKey`, `formBox`, `formFoot`, `tapForm`), `internal/app/keys.go` (help text only, if needed), `docs/keys.md`
- Test: `internal/app/lifeform_test.go`

**Interfaces:**
- Consumes: `m.git` (Git tab repos/worktrees with `PR`), `m.found` (`Repos[]`: `Path`, `Main`, `Branch`, `Active`), `State.Used` (Task 3, kept on the model as `m.used state.Used`), `m.snap.Sessions[].Cwd`.
- Produces:
  - `type whereRepo struct { path, name, branch string; pr int; at time.Time; live int; trees []whereRepo }` — a repo with its worktrees.
  - `func (m Model) whereRepos() []whereRepo` — every known repo once (Git tab, found, used), worktrees under their main checkout (`Checkout.Main`, the Git tab's repo), `at` = the newest of used `At` and `Checkout.Active`, `live` = sessions whose `Cwd` is the path or under it.
  - `func fuzzy(query, s string) (score int, hits []int)` — subsequence, case-insensitive; `score < 0` is no match; word starts (after `/ - _ .` or start) and runs score higher.
  - `type whereRow struct { kind int; repo whereRepo; tree string /* "", "├─ ", "╰─ " */; text string; hits []int }` with kinds `rowLabel, rowRepo, rowTree, rowPath, rowNone, rowClone, rowHave` (the last two filled in Task 7).
  - `func (m Model) whereRows(query string, now time.Time) []whereRow` — empty query: label `SUGGESTED`, up to 5 repos (live first, then newest used, then `Checkout.Active`, then name) each with its worktrees as tree rows, then label `ALL · N · by name` and the rest by name; with a query: one list by score (recency breaks ties within 10 % of the best score), no labels; no match: one `rowNone` row (`no repo matches "q" · ⏎ uses it as a path`).
  - form fields (replace `whereAt`): `sel int` (index into `whereRows`, skipping labels), `listOpen bool`, `picked bool` (the text came from a pick or ← →: it is not a filter; typing clears it and filters by the new text).

Behaviour to build (spec "What it looks like" and "Keys"):
- `openNew`: `f.where = ""`, `listOpen = true`, `sel` on the row of today's pre-fill path (session `Cwd`, Git tab worktree, else the first row). A pre-fill not among the known repos is added as the first SUGGESTED row (a `rowPath`).
- While `at == fieldWhere && listOpen`: ↑/↓ (`fk.Prev`/`fk.Next` keys `up`/`down` — split the bindings: `Next` keeps `tab`, a new `Down`/`Up` pair takes `down`/`up`, and the form's field moves accept both outside the list) move `sel`; ← → step `sel` to the previous / next repo row (not tree rows) and set `f.where` to its path with `picked = true`; ⏎ and tab pick the selected row (`f.where = path`, `listOpen = false`, `at = fieldBranch`); esc closes the list and keeps the text (a second esc closes the form, as today); typing appends to `f.where` and re-filters, `picked = false`.
- Focus back on where (shift+tab, a tap) sets `listOpen = true`.
- **Paste:** the console has no `tea.PasteMsg` handling today, so a paste reaches no field. With a form open, a `tea.PasteMsg` appends its text to the focused text field (line breaks dropped, spaces at both ends trimmed, cut at `maxField`), exactly as typing that text would (route it in `Update` to the form layer; on where it re-filters and clears `picked`).
- **No repos known** (no Git tab repos, no found, no used): the list is one line, `no repos known yet · type a path, or paste a git URL`, and, when the scan was never asked (`m.found == nil || m.found.Consent == ""`), a second line with the scan's key hint from its binding (`<key> finds your repos`).
- `formBox`: with the list open, after the where line: a blank line, the rows (in a fixed height: `min(12, terminal height − 10)` rows, scrolled so `sel` shows), `N of M` when filtered, a blank line, then `then  branch · name · agent <a> · first` in `faint`; the other fields are not drawn. Row cells as the spec: name `fg` (hits bold), short path `muted` (`tilde`, the parent folder for a repo, the folder for a worktree), branch `muted` (`detached` if empty), `#pr` `muted`, age `muted` (`ago()` as the Sessions tab uses), `N live` `faint`; selected row `▌` in accent on `wash`. The path is cut first when the row does not fit.
- `formFoot` with the list open: `⏎ pick · ↑↓ move · ←→ repo · tab next · esc keep the path`, each part from its binding.
- `docs/keys.md` "New session, Rename and Switch agent": rewrite the `Tab`/`↓` and `←`/`→` rows and add `Esc` on the list, per the spec's Keys table.

- [ ] **Step 1: Write the failing tests**

```go
func TestFuzzy(t *testing.T) {
	if s, _ := fuzzy("mbx", "~/dev/matchblox"); s < 0 { t.Fatal("mbx misses matchblox") }
	if s, _ := fuzzy("xyz", "~/dev/matchblox"); s >= 0 { t.Fatal("xyz matches") }
	a, _ := fuzzy("api", "shop-api"); b, _ := fuzzy("api", "a-pretty-index")
	if a <= b { t.Fatalf("a word start scores %d, scattered letters %d", a, b) }
}
func TestWhereRowsSuggestedFirst(t *testing.T)     // 52 known repos; the one with a live session first, then the newest used; SUGGESTED holds 5; ALL · 47 · by name follows; a repo is not in both
func TestWhereRowsWorktreesUnderTheirRepo(t *testing.T) // repo with two worktrees: rows repo, "├─ " tree, "╰─ " tree, in that order
func TestWhereRowsFilter(t *testing.T)             // query "mbx" among 52: first row matchblox, no label rows, "3 of 52" counted
func TestWhereRowsNoMatch(t *testing.T)            // query "zzz": one rowNone row
func TestNOpensTheListOnTheSessionsRepo(t *testing.T) // N on a session in /src/api: f.where == "", list open, selected row path /src/api, screen has "SUGGESTED" and "then  branch"
func TestWhereEnterPicksAndMovesOn(t *testing.T)   // ↓, ⏎: where = second row's path, list closed, at == fieldBranch, the full form drawn again
func TestWhereArrowsStillStepRepos(t *testing.T)   // → twice: where == third repo's path (tree rows skipped); the list is not filtered by it
func TestWhereEscKeepsTheText(t *testing.T)        // type "/tmp/x", esc: form open, list closed, where "/tmp/x"; esc again: form closed, nothing sent
func TestWhereListFitsAPhone(t *testing.T)         // width 44: every line of the box ≤ 44 cells; each row shows its name whole
func TestPasteFillsTheFocusedField(t *testing.T)    // tea.PasteMsg{Content: " /src/x\n"} on where: where == "/src/x", list filtered by it; on first: appended to first
func TestWhereListWithNoRepos(t *testing.T)         // no git, found nil, used empty: "no repos known yet · type a path, or paste a git URL" and the scan's key hint
func TestNewSessionSendsThePickedPath(t *testing.T) // the existing TestNOpensNewSessionWhereTheSessionWorks still passes: tab picks /src/api
```

Fixture: a `proto.State` with `Found` of 52 repos (`/src/r00`…, plus `/dev/matchblox`, `/src/matchblox-site`, `/src/tools/mbxctl`) and `Used` naming two of them; build it in a helper `whereState(t)` in `wherelist_test.go`.

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/app/ -run 'Fuzzy|WhereRows|Where|NOpens|NewSessionSends|Paste'`
Expected: FAIL.

- [ ] **Step 3: Implement `wherelist.go` and the form changes**

Remove `whereAt`, `gitPaths()` callers move to `whereRepos()`; keep `rebuildFoundPaths` only if something else needs `foundPaths` (else delete it and `foundPaths`). Add the up/down split in `keys.go` so `TestKeymapsHaveNoCollisions` holds.

- [ ] **Step 4: Run them, the key tests and the replay**

Run: `go test ./internal/app/ && go test ./cmd/matchblox -run TestReplay`
Expected: `ok` for the app. The replay fails only where the form's frames changed: check the diff is the new list, then `go test ./cmd/matchblox -run TestReplay -update` and run it again: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/app docs/keys.md cmd/matchblox www
git commit -S -m "feat: N's where lists the builder's repos, the most recent first (#174)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: the clone in the form

**Files:**
- Modify: `internal/app/lifeform.go`, `internal/app/wherelist.go`, `internal/app/model.go` (keep `State.Clones`)
- Test: `internal/app/lifeform_test.go`

**Interfaces:**
- Consumes: acts and types of Task 4 (`clone:plan`, `clone:start`, `clone:stop`, `ClonePlan`, `Clone`, `State.Clones`); `clone.Parse` (Task 1) to recognise a URL in the console without a round trip (protocol `"https"`; only to decide that a row is a clone row, the service's plan decides the rest).
- Produces: form fields `plan *proto.ClonePlan`, `planFor string` (the text the plan is for), `cloning string` (target of this form's clone), `startWhenDone bool`.

Behaviour (spec "Clone"):
- When `f.where` parses as a URL and is not a path: the list is one `rowClone` row at once (`clone o/r…` from the typed text), and a `clone:plan` act is sent (again only when the text changes). The plan's answer replaces the rows: each `Have` path as `start in <path> (already a clone of o/r)` (`rowHave`; the first selected), then `clone o/r into <Target>` — or, when `Fresh` is set, `clone anew into <Fresh>` — with ` (<root/r> is <Taken>)` when `Taken` is set. A plan `Err` shows as the `rowNone` text.
- ⏎/tab on `rowHave`: like a repo pick (where = that path).
- ⏎/tab on `rowClone`: send `clone:start` (`CloneStart{URL: plan.URL, Target: <target of the row>}`), `f.cloning = target`, `f.where` stays the URL, list closes, `at = fieldBranch`. The where line then shows `cloning o/r… 42%` (`muted`; `cloning o/r…` while `Pct < 0`) from `State.Clones` for `f.cloning`.
- Final ⏎ (formEnter) with `f.cloning` set: Done → send `new:session` with `Where = f.cloning`; still running → `startWhenDone = true`, the where line says `starts when the clone is done`; when a snapshot shows Done and `startWhenDone`, send `new:session` then. A clone `Err` shows as the form's error (`! …`) and clears `cloning`, `startWhenDone`.
- esc with `f.cloning` running: send `clone:stop`, clear `cloning`/`startWhenDone`, the form stays (where keeps the URL). Closing the form or editing `where` stops it the same way.
- A typed `~/o/r` or `./o/r` is never a clone row.

- [ ] **Step 1: Write the failing tests**

```go
func TestPastedURLShowsTheCloneRowAtOnce(t *testing.T) // paste "https://github.com/o/r": the screen has "clone o/r" before any result; one clone:plan act sent with that text
func TestPlanOffersTheExistingClone(t *testing.T)      // plan {Have ["/src/r"], Target "/src/r", Fresh "/src/r-ember"}: rows "start in ~/src/r (already a clone of o/r)" selected, "clone anew into ~/src/r-ember"
func TestPlanNamesTheTakenFolder(t *testing.T)         // plan {Target "/src/r-ember", Taken "another repo"}: row "clone o/r into ~/src/r-ember (~/src/r is another repo)"
func TestEnterOnCloneStartsItAndMovesOn(t *testing.T)  // ⏎: clone:start sent with {URL, Target}; at == fieldBranch; a snapshot with Clones [{Target, Pct 42}] shows "cloning o/r… 42%"
func TestEnterWhileCloningStartsWhenDone(t *testing.T) // ⏎ at first: no new:session, "starts when the clone is done"; snapshot Done: new:session sent with Where == target
func TestEscStopsTheClone(t *testing.T)                // esc while cloning: clone:stop sent with the target; form open, where still the URL
func TestCloneFailureShowsInTheForm(t *testing.T)      // snapshot Clones [{Err "no network: github.com does not answer"}]: "! no network" in the form; final ⏎ sends nothing
func TestTildePathIsNoClone(t *testing.T)              // type "~/o/r": no clone row, no clone:plan act
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/app/ -run 'URL|Plan|Clone|Tilde'`
Expected: FAIL.

- [ ] **Step 3: Implement**

The act results route through `formResult` (match `f.act` as today) for `clone:plan`; `clone:start` and `clone:stop` need no result beyond an `Err`, shown as the form's error.

- [ ] **Step 4: Run them to see them pass**

Run: `go test ./internal/app/ && go vet ./...`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/app
git commit -S -m "feat: a git URL pasted into N clones while the rest is filled in (#176)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: the docs, the changelog and the replay

**Files:**
- Modify: `docs/config.md` (`[git] clone_root`), `docs/console.md` (N, and `matchblox new --where <url>` in the CLI table), `docs/keys.md` (check Task 6's rows), `README.md` (where N is described), `CHANGELOG.md` (`## [Unreleased]`), DESIGN.md §6 (one bullet for the where list: labels, row cells, `N live`, no match glyph), `cmd/matchblox/replay_test.go` (a step that opens N, types `mbx`, and one that pastes a URL, if the replay's demo has a repo to show; else leave the frames from Task 6)

- [ ] **Step 1: Write the docs** in the house voice (ASD-STE100, short sentences, the spec's exact copy for every string shown on screen).
- [ ] **Step 2: Run the whole suite**

Run: `go test ./... && go vet ./... && scripts/validate.sh`
Expected: all `ok`; `validate.sh` exits 0 (it runs the docs checks: every key named in `docs/keys.md`).

- [ ] **Step 3: Commit**

```bash
git add docs README.md CHANGELOG.md DESIGN.md cmd/matchblox www
git commit -S -m "docs: N's where list and the clone in the docs, the changelog and DESIGN.md (#174, #176)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```
