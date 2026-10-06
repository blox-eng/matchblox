**What this changes**

**Why**

**Checklist**
- [ ] Conventional commit title (`feat:`, `fix:`, `docs:`, …)
- [ ] Tests cover the new or changed behaviour
- [ ] `CHANGELOG.md` has an entry under `## [Unreleased]` for a user-visible change
- [ ] `scripts/validate.sh` passes locally

**Safety**
- [ ] No action runs without its exact command on screen, and no destructive
      action runs without a typed confirm and a re-checked guard.
- [ ] No real machine names, people or companies in code, tests or docs.
