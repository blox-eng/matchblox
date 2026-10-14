package setup

import (
	"path/filepath"

	"github.com/blox-eng/matchblox/internal/doors"
)

const starterWays = `# Standing rules

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
`

// waysPath is config.WaysPath for this machine: next to config.toml.
func waysPath(e Env) string {
	dir := e.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(e.Home, ".config")
	}
	return filepath.Join(dir, "matchblox", "ways.md")
}

// waysDoor offers the starter file. It is done once any file is there:
// the door never changes what the person wrote.
func waysDoor(e Env) doors.Door {
	path := waysPath(e)
	d := doors.Door{ID: doors.Ways, Title: "Write your ways", Path: path,
		Why: "Tell the stoker your standing rules and recipes, in your words. It reads this file when you launch a run."}
	_, old, _, err := read(path)
	if err != nil {
		d.Problem = err.Error()
		return d
	}
	if old != nil {
		d.Done = true
		return d
	}
	d.Sum, d.Preview = Sum(nil), diff("", starterWays)
	return d
}

// WriteWays writes the starter file when there is none. A file made after
// the preview is not overwritten: the sums differ.
func WriteWays(path, seen string) error {
	_, err := edit(path, seen, func(old []byte) ([]byte, error) {
		if old != nil {
			return old, nil
		}
		return []byte(starterWays), nil
	})
	return err
}
