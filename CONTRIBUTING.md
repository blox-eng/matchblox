# Contributing

Thank you for helping. One rule decides what belongs in matchblox:

> **matchblox shows what needs you and the one safe action that resolves it.
> It does not start agents, edit prompts or review diffs.**

Other tools do those well (design/0001-v0.1.md §15).

## Before you start

- Read [design/0001-v0.1.md](design/0001-v0.1.md). It is the contract.
- For a new feature, open an issue first with the feature template.

## Rules

- The console never reads the machine. The service never imports the console.
  `internal/arch_test.go` checks this.
- Steps are argv lists. Never build a shell string from data.
- A destructive step needs a typed confirm and a guard that is checked again
  just before it runs.
- The service must stay under 1% of one core when the machine is idle.
- Tests and examples use the generic demo machine in `testdata/`. Never put a
  real machine name, person or company in the repository.
- Write docs and copy in short, plain sentences (ASD-STE100).

## Workflow

1. Branch from `main`. Use a Conventional Commits title (`feat:`, `fix:`, …).
2. Add a line under `## [Unreleased]` in `CHANGELOG.md` for a user-visible change.
3. Run `scripts/validate.sh`.
4. Open a pull request. CI must pass. `main` takes squash merges only.
