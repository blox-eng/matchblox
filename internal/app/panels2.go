package app

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/history"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

type wtRow struct {
	host string
	repo string
	wt   proto.Worktree
	main bool
}

// wtKey is a worktree's identity across hosts: a path repeats on two.
type wtKey struct{ host, path string }

func (r wtRow) key() wtKey { return wtKey{r.host, r.wt.Path} }

// hostGit is a host's git scan.
type hostGit struct {
	host string
	r    *proto.GitReport
}

// rescan is r on Git: a new scan on the selected row's host.
func (m Model) rescan() (tea.Model, tea.Cmd) {
	host := m.opt.Host
	if r, ok := m.selectedWorktree(); ok {
		host = r.host
	}
	if why := m.refusal(host); why != "" {
		m.flash = why
		return m, nil
	}
	if host == m.opt.Host && (m.conn == nil || m.lost) {
		return m, nil
	}
	m.flash = "scanning git…"
	if m.hostCount() > 1 {
		m.flash = "scanning git on " + m.nameOf(host) + "…"
	}
	return m, m.sendTo(host, proto.Act{RecID: "rescan:git", Which: "primary"})
}

// gitOf is host's git scan; nil: none yet.
func (m Model) gitOf(host string) *proto.GitReport {
	if host == m.opt.Host {
		return m.git
	}
	return m.stateOf(host).Git
}

// gitReports are the git scans of the hosts that sent one, in Hosts order.
func (m Model) gitReports() []hostGit {
	var out []hostGit
	if m.git != nil {
		out = append(out, hostGit{m.opt.Host, m.git})
	}
	for _, l := range m.peers {
		if l.have && l.st.Git != nil {
			out = append(out, hostGit{l.host, l.st.Git})
		}
	}
	return out
}

// worktreeRows are the worktrees worth a line: the main checkout, the ones
// sessions work in, and merged ones. The rest are counted, not listed.
func (m Model) worktreeRows() []wtRow {
	rows := m.allWorktreeRows()
	if q := strings.ToLower(m.filter); m.tab == tabGit && q != "" {
		rows = slices.DeleteFunc(rows, func(r wtRow) bool {
			return !strings.Contains(strings.ToLower(r.wt.Path+" "+r.wt.Branch), q)
		})
	}
	return rows
}

func (m Model) allWorktreeRows() []wtRow {
	var rows []wtRow
	for _, g := range m.gitReports() {
		for _, r := range g.r.Repos {
			for i, wt := range r.Worktrees {
				if i == 0 || wt.Sessions > 0 || wt.Merged {
					rows = append(rows, wtRow{g.host, r.Path, wt, i == 0})
				}
			}
		}
	}
	return rows
}

func (m Model) selectedWorktree() (wtRow, bool) {
	rows := m.worktreeRows()
	if len(rows) == 0 {
		return wtRow{}, false
	}
	return rows[min(m.gitSel, len(rows)-1)], true
}

func (m Model) gitPanel(w int) body {
	st := m.st
	var b body
	if len(m.snap.GitPolling) > 0 {
		b.add(-1, m.region("git run by agents, last 10 min", w)...)
		for _, p := range m.snap.GitPolling[:min(5, len(m.snap.GitPolling))] {
			style := st.muted
			if p.Cores >= 0.5 {
				style = st.warn
			}
			b.add(-1, fit(" "+style.Render(pad(fmt.Sprintf("%.2f cores", p.Cores), 12))+st.muted.Render(tilde(p.Checkout)), w))
		}
	}
	reports := m.gitReports()
	if len(reports) == 0 {
		b.add(-1, "", st.faint.Render(" scanning repositories…"))
		return b
	}
	rows := m.worktreeRows()
	sel := min(m.gitSel, max(len(rows)-1, 0))
	i := 0
	for _, g := range reports {
		i = m.gitHost(&b, g, rows, i, sel, w)
	}
	if r, ok := m.selectedWorktree(); ok {
		wt := r.wt
		b.add(-1, fit(" "+st.label.Render(m.keys.main.Open.Help().Key+" ")+st.muted.Render("tmux new-window -c "+wt.Path), w))
		if wt.Safe {
			b.add(-1, fit(" "+st.label.Render(m.keys.main.Second.Help().Key+" ")+st.muted.Render(strings.Join(wt.Remove, " ")), w))
		}
	}
	return b
}

// gitHost adds host g's repositories to the Git tab, its rows from rows[i]
// on, and gives the index of the next host's first row. With two or more
// hosts each repository and the scan's line name the host.
func (m Model) gitHost(b *body, g hostGit, rows []wtRow, i, sel, w int) int {
	st, many := m.st, m.hostCount() > 1
	for _, r := range g.r.Repos {
		safe := 0
		for _, wt := range r.Worktrees {
			if wt.Safe {
				safe++
			}
		}
		rest := fmt.Sprintf("  ·  %d worktrees", len(r.Worktrees))
		if safe > 0 {
			rest += fmt.Sprintf("  ·  %d safe to remove", safe)
		}
		if many {
			b.add(-1, "", fit(" "+st.text.Render(m.nameOf(g.host))+st.muted.Render("  "+tilde(r.Path))+st.label.Render(strings.ToUpper(rest)), w))
		} else {
			b.add(-1, m.regionPath(r.Path, rest, w)...)
		}
		if r.Behind > 0 {
			b.add(-1, fit(" "+st.warn.Render("▲ ")+st.text.Render(fmt.Sprintf("%s is %d commits behind its remote", r.Main, r.Behind)), w))
		}
		b.add(-1, st.label.Render(fit(" "+pad("WORKTREE", 36)+pad("BRANCH", 28)+pad("AGENTS", 8)+pad("DIRTY", 8)+"STATE", w)))
		for ; i < len(rows) && rows[i].host == g.host && rows[i].repo == r.Path; i++ {
			wt := rows[i].wt
			name := filepath.Base(wt.Path)
			if rows[i].main {
				name += " (main)"
			}
			dirty := st.faint.Render(pad("—", 8))
			if wt.Dirty >= 0 {
				dirty = st.muted.Render(pad(fmt.Sprint(wt.Dirty), 8))
				if wt.Dirty >= 200 {
					dirty = st.warn.Render(pad(fmt.Sprint(wt.Dirty), 8))
				}
			}
			state := ""
			switch {
			case wt.Safe:
				state = st.accent.Render("✓ safe to remove")
			case wt.Merged && wt.InUse:
				state = st.muted.Render("merged, in use")
			case wt.Merged && wt.Dirty != 0:
				state = st.muted.Render("merged, has changes")
			}
			agents := ""
			if wt.Sessions > 0 {
				agents = fmt.Sprint(wt.Sessions)
			}
			mark := " "
			if m.picked[rows[i].key()] {
				mark = st.accent.Render("●")
			}
			line := mark + st.text.Render(pad(name, 36)) + st.muted.Render(pad(wt.Branch, 28)) + st.text.Render(pad(agents, 8)) + dirty + state
			b.addRow(i, i == sel, w, st, line)
		}
	}
	for _, e := range g.r.Errors {
		b.add(-1, fit(" "+st.neg.Render("! ")+st.muted.Render(e), w))
	}
	scanned := fmt.Sprintf("scanned %s ago in %s · %s rescans",
		sample.Human(m.now().Sub(g.r.At)), g.r.Took.Round(time.Millisecond), m.keys.main.Rename.Help().Key)
	if many {
		scanned = m.nameOf(g.host) + " · " + scanned
	}
	b.add(-1, "", fit(" "+st.faint.Render(scanned), w))
	return i
}

// recsSection is what to do next, under the sessions: its rows follow
// theirs. On a phone the selected one shows its evidence and its steps
// under the list; wider, the detail pane does. h folds the section to its
// label; d dismisses the selected one until its evidence changes.
func (m Model) recsSection(b *body, w int) {
	if len(m.recs) == 0 {
		return
	}
	st := m.st
	if len(b.lines) > 0 {
		b.add(-1, "")
	}
	if m.recPrefs.Folded {
		b.add(recsLabelRow, st.label.Render(fmt.Sprintf(" RECOMMENDATIONS · %d hidden", len(m.recs)))+st.faint.Render(" · ")+
			keyWord(m.keys.main.Fold, st.accent, st.faint, " shows"))
		return
	}
	b.add(recsLabelRow, st.label.Render(fmt.Sprintf(" RECOMMENDATIONS · %d", len(m.recs)))+st.faint.Render(" · ")+
		keyWord(m.keys.main.Fold, st.accent, st.faint, " hides"))
	n := len(m.doors) + len(m.snap.Sessions) + len(m.ghosts())
	sel := m.sessIndex() - n
	for i, r := range m.recs {
		mark := st.faint.Render("○ ")
		switch r.Level {
		case "crit":
			mark = st.neg.Render("! ")
		case "warn":
			mark = st.warn.Render("▲ ")
		}
		lead := " "
		if i == sel {
			lead = st.accent.Render("▌")
		}
		line := lead + mark + st.ink(i == sel).Render(tilde(r.Title))
		if run := m.recRun; run.rec == r.ID && run.out != "" {
			line = " " + st.accent.Render("✓ ") + st.text.Render(tilde(run.out))
			if run.failed {
				line = " " + st.neg.Render("✗ ") + st.text.Render(tilde(run.out))
			}
		}
		if i == sel {
			line = spread(line, dismissMark(st, m.keys.main.Dismiss)+" ", w)
		}
		b.addRow(n+i, i == sel, w, st, line)
	}
	if sel < 0 || sel >= len(m.recs) || !m.inPlace() {
		return
	}
	r := m.recs[sel]
	b.add(-1, wrap(" "+tilde(r.Evidence), w, st.muted.Render)...)
	b.add(-1, m.recActions(r, w)...)
}

// recActions are the two steps of a recommendation with their keys.
func (m Model) recActions(r advice.Rec, w int) []string {
	st := m.st
	var out []string
	for _, a := range []struct {
		key string
		act *advice.Action
	}{{m.keys.main.Open.Help().Key, r.Primary}, {m.keys.main.Second.Help().Key, r.Second}} {
		if a.act == nil {
			continue
		}
		cmd := strings.Join(a.act.Steps[0], " ")
		if n := len(a.act.Steps); n > 1 {
			cmd += fmt.Sprintf("  (+%d more)", n-1)
		}
		label := a.act.Label
		if a.act.Destructive {
			label += ", asks " + m.keys.confirm.Yes.Help().Key
		}
		out = append(out, fit(" "+st.accent.Render(a.key+" ")+st.text.Render(label+": ")+st.muted.Render(cmd), w))
	}
	return out
}

func wrap(s string, w int, render func(...string) string) []string {
	var out []string
	line := ""
	for _, word := range strings.Fields(s) {
		if len(line)+len(word)+1 > w-2 && line != "" {
			out = append(out, render(" "+line))
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	if line != "" {
		out = append(out, render(" "+line))
	}
	return out
}

// bucket reduces points to n values keeping each bucket's peak: a spike
// matters more than an average on a history chart.
func bucket(pts []history.Point, n int, f func(history.Point) float64) []float64 {
	if len(pts) == 0 || n <= 0 {
		return nil
	}
	out := make([]float64, 0, n)
	per := max((len(pts)+n-1)/n, 1) // ceil: never more columns than n
	for i := 0; i < len(pts); i += per {
		peak := 0.0
		for _, p := range pts[i:min(i+per, len(pts))] {
			peak = max(peak, f(p))
		}
		out = append(out, peak)
	}
	return out
}

// historyBody is the History tab. Two or more hosts: a section for each
// host, in Hosts order, headed by its name.
func (m Model) historyBody(w int) body {
	if m.hostCount() == 1 {
		return plain(m.historyPanel(w))
	}
	var b body
	for _, h := range m.statedHosts() {
		b.add(-1, "", m.hostHead(h, w))
		b.add(-1, m.viewOf(h).historyPanel(w)...)
	}
	return b
}

func (m Model) historyPanel(w int) []string {
	st := m.st
	pts := m.hist.Points
	if len(pts) < 2 {
		return []string{"", st.faint.Render(" collecting history: one point every 30 s")}
	}
	span := pts[len(pts)-1].At.Sub(pts[0].At)
	out := m.region(fmt.Sprintf("last %s · peak per column", sample.Human(span)), w)
	cw := min(w-28, 140)
	charts := []struct {
		name string
		unit string
		f    func(history.Point) float64
	}{
		{"cpu", "%", func(p history.Point) float64 { return p.CPU }},
		{"load1", "", func(p history.Point) float64 { return p.Load1 }},
		{"pressure cpu", "%", func(p history.Point) float64 { return p.PSICPU }},
		{"package temp", "°C", func(p history.Point) float64 { return p.TempC }},
		{"mem available", "G", func(p history.Point) float64 { return p.MemAvailGB }},
		{"swap", "G", func(p history.Point) float64 { return p.SwapGB }},
	}
	for _, c := range charts {
		xs := bucket(pts, cw, c.f)
		lo, hi := c.f(pts[0]), c.f(pts[0])
		for _, p := range pts {
			lo, hi = min(lo, c.f(p)), max(hi, c.f(p))
		}
		now := c.f(pts[len(pts)-1])
		// Scale min to max so two rows show the shape; the labels carry the
		// absolute values. The floor sits just under min so min still draws.
		floor := lo - (hi-lo)*0.05
		shifted := make([]float64, len(xs))
		for i, x := range xs {
			shifted[i] = x - floor
		}
		top := hi - floor
		if top <= 0 {
			top = 1 // a flat series: a zero range would divide by zero
		}
		lines := m.chart(shifted, len(shifted), 2, top)
		out = append(out, "")
		label := []string{
			st.label.Render(pad(strings.ToUpper(c.name), 24)),
			st.text.Render(pad(fmt.Sprintf("now %.1f%s", now, c.unit), 24)),
		}
		for i, l := range lines {
			out = append(out, fit(" "+label[min(i, 1)]+l, w))
		}
		out = append(out, fit(" "+pad("", 24)+st.faint.Render(fmt.Sprintf("min %.1f%s  max %.1f%s", lo, c.unit, hi, c.unit)), w))
	}
	return out
}

// dismissMark is the selected recommendation's tap target for d.
func dismissMark(st styles, b bkey.Binding) string {
	return keyWord(b, st.accent, st.muted, " "+b.Help().Desc)
}

// dismissWidth is the width of the dismiss mark and the space after it.
func (m Model) dismissWidth() int {
	return lipgloss.Width(dismissMark(m.st, m.keys.main.Dismiss)) + 1
}
