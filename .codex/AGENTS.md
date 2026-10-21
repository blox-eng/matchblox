# matchblox — Codex notes

Do not follow or index `testdata/`. Fixture `cwd` symlinks point at `/` (and other absolute paths); tools that walk with `--follow` hang on the whole disk. The fixtures stay for Go tests — leave them alone unless you are changing those tests.
