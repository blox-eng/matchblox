package app

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/history"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

type wtRow struct {
	repo string
	wt   proto.Worktree
	main bool
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
	if m.git == nil {
		return nil
	}
	var rows []wtRow
	for _, r := range m.git.Repos {
		for i, wt := range r.Worktrees {
			if i == 0 || wt.Sessions > 0 || wt.Merged {
				rows = append(rows, wtRow{r.Path, wt, i == 0})
			}
		}
	}
	return rows
}

func (m Model) selectedWorktree() (proto.Worktree, bool) {
	rows := m.worktreeRows()
	if len(rows) == 0 {
		return proto.Worktree{}, false
	}
	return rows[min(m.gitSel, len(rows)-1)].wt, true
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
	if m.git == nil {
		b.add(-1, "", st.faint.Render(" scanning repositories…"))
		return b
	}
	rows := m.worktreeRows()
	sel := min(m.gitSel, max(len(rows)-1, 0))
	i := 0
	for _, r := range m.git.Repos {
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
		b.add(-1, m.regionPath(r.Path, rest, w)...)
		if r.Behind > 0 {
			b.add(-1, fit(" "+st.warn.Render("▲ ")+st.text.Render(fmt.Sprintf("%s is %d commits behind its remote", r.Main, r.Behind)), w))
		}
		b.add(-1, st.label.Render(fit(" "+pad("WORKTREE", 36)+pad("BRANCH", 28)+pad("AGENTS", 8)+pad("DIRTY", 8)+"STATE", w)))
		for ; i < len(rows) && rows[i].repo == r.Path; i++ {
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
			if m.picked[wt.Path] {
				mark = st.accent.Render("●")
			}
			line := mark + st.text.Render(pad(name, 36)) + st.muted.Render(pad(wt.Branch, 28)) + st.text.Render(pad(agents, 8)) + dirty + state
			b.addRow(i, i == sel, w, st, line)
		}
	}
	for _, e := range m.git.Errors {
		b.add(-1, fit(" "+st.neg.Render("! ")+st.muted.Render(e), w))
	}
	b.add(-1, "", fit(" "+st.faint.Render(fmt.Sprintf("scanned %s ago in %s · r rescans",
		sample.Human(m.now().Sub(m.git.At)), m.git.Took.Round(time.Millisecond))), w))
	if wt, ok := m.selectedWorktree(); ok {
		b.add(-1, fit(" "+st.label.Render("⏎ ")+st.muted.Render("tmux new-window -c "+wt.Path), w))
		if wt.Safe {
			b.add(-1, fit(" "+st.label.Render("x ")+st.muted.Render(strings.Join(wt.Remove, " ")), w))
		}
	}
	return b
}

// recsSection is what to do next, under the queue: its rows follow the
// queue's, and the selected one shows its evidence and its steps.
func (m Model) recsSection(b *body, w int) {
	st := m.st
	b.add(-1, m.region(fmt.Sprintf("%d recommendations", len(m.recs)), w)...)
	if len(m.recs) == 0 {
		b.add(-1, st.faint.Render(m.nothing(" nothing to do")))
		return
	}
	n := len(m.doors) + len(m.queue)
	sel := m.queueIndex() - n
	for i, r := range m.recs {
		mark := st.faint.Render("○ ")
		switch r.Level {
		case "crit":
			mark = st.neg.Render("! ")
		case "warn":
			mark = st.warn.Render("▲ ")
		}
		line := " " + mark + st.text.Render(tilde(r.Title))
		if run := m.recRun; run.rec == r.ID && run.out != "" {
			line = " " + st.accent.Render("✓ ") + st.text.Render(tilde(run.out))
			if run.failed {
				line = " " + st.neg.Render("✗ ") + st.text.Render(tilde(run.out))
			}
		}
		b.addRow(n+i, i == sel, w, st, line)
	}
	if sel < 0 {
		return // a queue row is selected: the detail is for a recommendation
	}
	r := m.recs[sel]
	b.add(-1, "", st.hair.Render(strings.Repeat("─", w)), fit(" "+st.text.Render(tilde(r.Title)), w))
	b.add(-1, wrap(" "+tilde(r.Evidence), w, st.muted.Render)...)
	for _, a := range []struct {
		key string
		act *advice.Action
	}{{"⏎", r.Primary}, {"x", r.Second}} {
		if a.act == nil {
			continue
		}
		cmd := strings.Join(a.act.Steps[0], " ")
		if n := len(a.act.Steps); n > 1 {
			cmd += fmt.Sprintf("  (+%d more)", n-1)
		}
		label := a.act.Label
		if a.act.Destructive {
			label += ", asks y"
		}
		b.add(-1, fit(" "+st.label.Render(a.key+" ")+st.text.Render(label+": ")+st.muted.Render(cmd), w))
	}
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
