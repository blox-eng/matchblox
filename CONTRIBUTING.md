# Contributing

Thank you for helping. One rule decides what belongs in matchblox:

> **matchblox shows what needs you and the one safe action that resolves it.
> It does not start agents, edit prompts or review diffs.**

Other tools do those well (design/0001-v0.1.md §15).

## Before you start

- Read [design/0001-v0.1.md](design/0001-v0.1.md). It is the contract.
- For a new feature, open an issue first with the feature template.

## Rules

- The console never reads the machine, and the service never imports the
  console (design §4.2). `internal/arch_test.go` checks this.
- Steps are argv lists. Never build a shell string from data.
- A destructive step needs a typed confirm and a guard that is checked again
  just before it runs.
- The service must stay under 1% of one core when the machine is idle.
- Tests and examples use the generic demo machine in `testdata/`. Never put a
  real machine name, person or company in the repository.
- Write docs and copy in short, plain sentences (ASD-STE100). Lead with
  what the reader gets and what to do; the why goes in design/
  (DESIGN.md §7).

## The dev loop

- Write the failing test first, watch it fail, then make it pass.
- `go test ./...` runs every test. `scripts/validate.sh` runs most of what
  CI runs: gofmt, vet, lint, tests, a smoke run and the cross builds.
- `go run ./cmd/matchblox --fixtures testdata/demo` opens the console on the
  demo machine `ws-1`.
- When the console's look changes, record the replay again:
  `go test ./cmd/matchblox -run TestReplay -update`. It writes
  `www/demo/frames.json`, `www/demo/replay.svg` and the still frame in
  `www/index.html`.

## The docs

The docs are Markdown in `docs/`, built by mkdocs (`mkdocs.yml`). A new key
or command goes into the docs in the same pull request: a test fails while a
key the console handles, or a command the binary has, is missing from them,
and while the docs name one that does not exist.

```sh
python3 -m venv .venv && .venv/bin/pip install --require-hashes --no-deps -r requirements-docs.txt
.venv/bin/mkdocs serve
```

## Workflow

1. Branch from `main`. Use a Conventional Commits title (`feat:`, `fix:`,
   `docs:`, …).
2. Add a line under `## [Unreleased]` in `CHANGELOG.md` for a user-visible change.
3. Run `scripts/validate.sh`.
4. Open a pull request. CI must pass. `main` takes squash merges only.

Questions go to [Discord](https://discord.gg/tYxBUpGfX3). Bugs and features
go to GitHub issues.
