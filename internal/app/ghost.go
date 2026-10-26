package app

import (
	"path/filepath"
	"slices"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ghost is a ghost row of Sessions (#175): a place to start a session
// in. Its repo's path is "" when no place is known: N opens as it does.
type ghost struct {
	repo whereRepo
	here bool // the first row: "start a new session here"
}

// quickStarts is how many more repos an empty Sessions offers.
const quickStarts = 2

// ghosts are Sessions' ghost rows: here, then, while no session runs, the
// next most recent repos. Here is the repo the console was launched in,
// else the most recent one.
func (m Model) ghosts() []ghost {
	repos := slices.SortedFunc(slices.Values(m.wrepos), bySuggested)
	var here whereRepo
	switch {
	case m.opt.Here != "":
		here = m.repoAt(m.opt.Here)
	case len(repos) > 0:
		here = repos[0]
	}
	out := []ghost{{repo: here, here: true}}
	if len(m.snap.Sessions) > 0 {
		return out
	}
	for _, r := range repos {
		if len(out) > quickStarts {
			break
		}
		if r.path != here.path {
			out = append(out, ghost{repo: r})
		}
	}
	return out
}

// repoAt is the known repo or worktree at p, else a bare one.
func (m Model) repoAt(p string) whereRepo {
	p = filepath.Clean(p)
	for _, r := range m.wrepos {
		if r.path == p {
			return r
		}
		for _, t := range r.trees {
			if t.path == p {
				return t
			}
		}
	}
	return whereRepo{path: p, name: filepath.Base(p)}
}

// selectedGhost is the ghost row under the selection in Sessions.
func (m Model) selectedGhost() (ghost, bool) {
	if m.tab != tabSessions {
		return ghost{}, false
	}
	gs := m.ghosts()
	i := m.sessIndex() - len(m.doors) - len(m.snap.Sessions)
	if i < 0 || i >= len(gs) {
		return ghost{}, false
	}
	return gs[i], true
}

// ghostAction is ⏎ on a ghost row: N, on its folder.
func ghostAction(g ghost) *action {
	say := "start a new session"
	if g.repo.path != "" {
		say += " in " + tilde(g.repo.path)
	}
	return &action{label: "new session", say: say, ghost: true, where: g.repo.path}
}

// openNewIn is N with where picked, the cursor on agent; with no folder,
// N as it is.
func (m Model) openNewIn(where string) (tea.Model, tea.Cmd) {
	next, cmd := m.openNew()
	nm, ok := next.(Model)
	if !ok || nm.form == nil || where == "" {
		return next, cmd
	}
	f := *nm.form
	f.where, f.picked, f.query, f.extra = where, true, "", ""
	f.listOpen, f.at = false, fieldAgent
	nm.form = &f
	return nm, cmd
}

// ghostRow is one ghost row, drawn faint as a session row is laid out:
// the plus where the match is, the words where the name is, then the
// folder and its branch, or for a quick start its age.
func (m Model) ghostRow(g ghost, sel bool, w int) string {
	st := m.st
	mark := " "
	if sel {
		mark = st.accent.Render("▌")
	}
	words := "start a new session here"
	if !g.here {
		words = "start in " + g.repo.name
	}
	var where string
	if g.repo.path != "" {
		where = tilde(g.repo.path)
		switch {
		case g.here && g.repo.branch != "":
			where += "  " + g.repo.branch
		case !g.here && !g.repo.at.IsZero():
			where += "   " + whereAge(m.now().Sub(g.repo.at))
		}
	} else {
		words = "start a new session"
	}
	ink := st.faint
	if sel {
		ink = st.muted
	}
	left := mark + st.accent.Render("+") + " " + ink.Render(words)
	if t := m.sessTracks(w); t.wide {
		left = mark + st.accent.Render("+") + " " + ink.Render(pad(words, t.name))
	}
	if where == "" {
		return fit(left, w)
	}
	if room := max(w-lipgloss.Width(left)-3, 0); lipgloss.Width(where) > room {
		where = cutStartTo(where, room)
	}
	return fit(left+st.faint.Render("   "+where), w)
}

// ghostDetail is the detail of a ghost row: where ⏎ starts, and how.
func (m Model) ghostDetail(g ghost, w int) []string {
	st := m.st
	out := []string{"", fit(" "+st.accent.Bold(true).Render(ghostAction(g).say), w)}
	if g.repo.branch != "" {
		out = append(out, fit(" "+st.muted.Render("on "+g.repo.branch), w))
	}
	if len(m.snap.Sessions) == 0 {
		out = append(out, "", fit(" "+st.faint.Render(m.nothing("no agent sessions found in tmux")), w))
	}
	return append(out, "", " "+keyWord(m.keys.main.Open, st.accent, st.muted, " new session"))
}

// ghostRows add the ghost rows under the sessions, the first at row at;
// sel is the selected one, from 0.
func (m Model) ghostRows(b *body, at, sel, w int) {
	for i, g := range m.ghosts() {
		b.addRow(at+i, i == sel, w, m.st, m.ghostRow(g, i == sel, w))
	}
}
