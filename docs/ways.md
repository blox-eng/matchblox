# Your ways

The ways file is where you tell the stoker how you work, in your own
words. It is one markdown file, `ways.md`, next to `config.toml`
(`~/.config/matchblox/ways.md` by default). The stoker reads it when you
launch a run, and it never changes it.

Open the **Write your ways** door in the console to create it. The door
shows the exact file first and never overwrites one you already have.

## Two conventions

- **Standing rules** are plain lines under any heading, such as
  `# Standing rules`. They hold for the whole run.
- **Recipes** are named procedures. A heading `# Recipe: <name>` starts
  one, and it runs to the next heading. The launch sheet offers each recipe
  by name, so you pick one without retyping it.

Everything else is yours: the stoker passes the file on as written.

## The starter

```markdown
# Standing rules

- When a session opens a PR: compact it, then send it /review.
- Past 80% of the week: hold every session that is not a priority until the week resets.
- Never touch a session I parked.

# Priority

Name the sessions or the issues that come first.

# Recipe: merge-order

1. Ready = a PR that is green, or only behind main.
2. Order: fewest conflicts first, and a stacked PR after its base.
3. Tell each session to read its issue, watch the PR before it, then rebase and merge on green.
4. A failed rebase: ask me, stop that lane, and go on with the rest.
```

## More examples

```markdown
# Standing rules

- Keep sessions on the billing service ahead of the docs sessions.
- If a session waits on a question for more than an hour, tell me in the morning.

# Recipe: release-notes

1. Ask each session for the user-facing change in one sentence.
2. Collect them in one list, grouped by service.
```
