package app

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/blox-eng/matchblox/internal/accounts"
	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/sample"
)

// Usage and the machine: the strip says them on the board, and the Machine
// tab has them all (design/0006 §7).

// usageLines are the limits of each account a session runs under, as
// /usage says them: each window's name, a bar (warn from 60%, neg from
// 90%), the percent and its reset; the forecast and an old reading on the
// account's last line. Short of room (a phone's dock) an account takes
// two lines, and one of another provider than Anthropic one.
func (m Model) usageLines(w int, full bool) []string {
	st := m.st
	out := []string{spread(st.label.Render(" USAGE"), "", w)}
	tight := !full && w < 50
	rows, nameW := m.usageRows(tight)
	if len(rows) == 0 {
		return append(out, fit(" "+st.faint.Render("no session runs under a plan"), w))
	}
	if m.hostCount() > 1 {
		return append(out, m.usageCells(rows, nameW, w)...)
	}
	now := m.now()
	for _, r := range rows {
		a := r.a
		head := " " + m.accountMark(a.Provider) + " " + st.text.Render(pad(r.name, nameW+1)) + " "
		indent := strings.Repeat(" ", lipgloss.Width(head))
		if a.State == limits.Unmeasured {
			line := head + st.warn.Render("not measured · "+a.Fix)
			if lipgloss.Width(line) > w { // the fix goes whole, not cut
				line = head + st.warn.Render("not measured")
			}
			out = append(out, fit(line, w))
			continue
		}
		uses := a.Uses(now)
		if len(uses) == 0 {
			out = append(out, fit(head+st.faint.Render("no window read"), w))
			continue
		}
		if tight && a.Provider != "anthropic" && len(uses) > 1 {
			line := head + st.faint.Render(uses[0].Name+" ") + m.meter(uses[0].Pct, 3, m.useStyle(uses[0].Pct)) +
				st.text.Render(fmt.Sprintf(" %.0f%%", uses[0].Pct))
			for _, u := range uses[1:] {
				line += st.faint.Render(" · "+u.Name+" ") + st.text.Render(fmt.Sprintf("%.0f%%", u.Pct))
			}
			out = append(out, fit(line, w))
			continue
		}
		if tight {
			uses = uses[:min(2, len(uses))]
		}
		// The meter gives way to the resets on a narrow dock, as small as the
		// folded line's.
		resets := make([]string, len(uses))
		meterW := 8
		for j, u := range uses {
			if !u.Resets.IsZero() {
				resets[j] = " · " + limits.When(u.Resets, now, "")
			}
			meterW = min(meterW, w-lipgloss.Width(head)-10-lipgloss.Width(resets[j]))
		}
		meterW = max(meterW, 3)
		var lines []string
		for j, u := range uses {
			lead := head
			if j > 0 {
				lead = indent
			}
			line := lead + st.faint.Render(pad(u.Name, 5)) + m.meter(u.Pct, meterW, m.useStyle(u.Pct)) + st.text.Render(pad(fmt.Sprintf(" %.0f%%", u.Pct), 5))
			if resets[j] != "" && lipgloss.Width(line)+lipgloss.Width(resets[j]) <= w { // a reset goes whole, not cut
				line += st.faint.Render(resets[j])
			}
			lines = append(lines, line)
		}
		for _, p := range m.outlook(a) {
			last := lines[len(lines)-1]
			style := st.muted
			if p.Tone == limits.Warn {
				style = st.warn
			}
			switch more := last + st.faint.Render(" · ") + style.Render(p.Text); {
			case lipgloss.Width(more) <= w:
				lines[len(lines)-1] = more
			case !tight && lipgloss.Width(indent+p.Text) <= w:
				lines = append(lines, indent+style.Render(p.Text))
			case !tight: // under a narrow list the forecast takes its own line from the left
				lines = append(lines, " "+style.Render(p.Text))
			case p.Tone == limits.Warn:
				// A phone keeps the account on its lines: a warning takes the
				// place of the reset.
				cut, _, _ := strings.Cut(last, st.faint.Render(" · "))
				if warned := cut + st.faint.Render(" · ") + style.Render(p.Text); lipgloss.Width(warned) <= w {
					lines[len(lines)-1] = warned // else it waits for the room of the full lines, never cut
				}
			}
		}
		for _, l := range lines {
			out = append(out, fit(l, w))
		}
	}
	return out
}

// outlook is what an account's block ends with: the forecast of its week,
// and the time of a reading over an hour old.
func (m Model) outlook(a limits.Account) []limits.Part {
	var out []limits.Part
	for _, p := range limits.Parts(a, m.now()) {
		if p.Tone != "" {
			out = append(out, p)
		}
	}
	return out
}

func (m Model) useStyle(pct float64) lipgloss.Style {
	switch {
	case pct >= 90:
		return m.st.neg
	case pct >= 60:
		return m.st.warn
	}
	return m.st.muted
}

// providerMarks stand for the logos a terminal cannot draw: one cell each,
// none an emoji.
var providerMarks = map[string]string{"anthropic": "✻", "openai": "❋", "opencode": "▣", "cursor": "⬢"}

// providerNames name the provider of each account: the plans of two
// providers are often the same word ("Team"). OpenCode's accounts are
// under the provider its model runs on; another provider goes by its id.
var providerNames = map[string]string{"anthropic": "Claude", "openai": "OpenAI", "opencode": "OpenCode", "cursor": "Cursor", "google": "Google"}

// usageCells are the usage lines of two or more hosts: an account label's
// line for each window, with each host's own reading as a cell, in Hosts
// order: "✻ Claude work 5h   ws-1 58% · gpu-1 61%". A cell is never a sum
// or a mean of two hosts (R10); a stale host's cell says so. Short of
// room the line keeps its window and the cells that fit.
func (m Model) usageCells(rows []usageRow, nameW, w int) []string {
	st, now := m.st, m.now()
	var out []string
	for len(rows) > 0 {
		n := 1
		for n < len(rows) && rows[n].name == rows[0].name {
			n++
		}
		group := rows[:n]
		rows = rows[n:]
		head := " " + m.accountMark(group[0].a.Provider) + " " + st.text.Render(pad(group[0].name, nameW+1)) + " "
		indent := strings.Repeat(" ", lipgloss.Width(head))
		uses := make([][]limits.Use, len(group))
		var wins []string
		for i, r := range group {
			if r.a.State == limits.Unmeasured {
				continue
			}
			uses[i] = r.a.Uses(now)
			for _, u := range uses[i] {
				if !slices.Contains(wins, u.Name) {
					wins = append(wins, u.Name)
				}
			}
		}
		if len(wins) == 0 {
			if a := group[0].a; a.State == limits.Unmeasured {
				line := head + st.warn.Render("not measured · "+a.Fix)
				if lipgloss.Width(line) > w { // the fix goes whole, not cut
					line = head + st.warn.Render("not measured")
				}
				out = append(out, fit(line, w))
			} else {
				out = append(out, fit(head+st.faint.Render("no window read"), w))
			}
			continue
		}
		for j, win := range wins {
			var cells []string
			for i, r := range group {
				cells = append(cells, m.usageCell(r, uses[i], win))
			}
			lead := head
			if j > 0 {
				lead = indent
			}
			out = append(out, m.cellLines(lead+st.faint.Render(pad(win, 5)), cells, w, "   ")...)
		}
	}
	return out
}

// cellLines are line and as many whole cells after it as fit in w, then
// the rest, whole, as many to a line as fit, on the lines under it from
// cont: a cell is never cut, and no host's percent is left out.
func (m Model) cellLines(line string, cells []string, w int, cont string) []string {
	sep := m.st.faint.Render(" · ")
	var out []string
	first := true // the line holds no cell yet
	for _, c := range cells {
		next := c
		if !first {
			next = sep + c
		}
		if lipgloss.Width(line+next) <= w {
			line, first = line+next, false
			continue
		}
		out = append(out, line)
		line, first = cont+c, false
	}
	return append(out, fit(line, w))
}

// usageCell is host's reading of window win: "ws-1 58%".
func (m Model) usageCell(r usageRow, uses []limits.Use, win string) string {
	st, name := m.st, m.nameOf(r.host)
	if m.stale(r.host) {
		return st.faint.Render(name + " stale")
	}
	if r.a.State == limits.Unmeasured {
		return st.faint.Render(name + " not measured")
	}
	for _, u := range uses {
		if u.Name == win {
			return st.muted.Render(name+" ") + m.useStyle(u.Pct).Render(fmt.Sprintf("%.0f%%", u.Pct))
		}
	}
	return st.faint.Render(name + " —")
}

// usageRow is an account's limits on a host, with its short name.
type usageRow struct {
	host, name string
	a          limits.Account
}

// usageRows are the accounts of the dock and the widest name. Two or more
// hosts: a row per host and account, each named as on its own host, the
// same name together in the order it first shows, in Hosts order; the
// dock draws each name once, with a cell for each host (R10).
func (m Model) usageRows(tight bool) ([]usageRow, int) {
	hosts := []string{m.opt.Host}
	if m.hostCount() > 1 {
		hosts = m.statedHosts()
	}
	var rows []usageRow
	nameW := 0
	for _, h := range hosts {
		ls := m.limitsOf(h)
		names, w := accountNames(ls, tight)
		nameW = max(nameW, w)
		for i, a := range ls {
			rows = append(rows, usageRow{h, names[i], a})
		}
	}
	if len(hosts) > 1 {
		first := map[string]int{}
		for i, r := range rows {
			if _, ok := first[r.name]; !ok {
				first[r.name] = i
			}
		}
		slices.SortStableFunc(rows, func(a, b usageRow) int { return first[a.name] - first[b.name] })
	}
	return rows, nameW
}

// accountNames are the short names of the accounts, after their provider:
// one account of a provider by its plan, two by the name of their emails.
// A phone (tight) leaves out the plan.
func accountNames(ls []limits.Account, tight bool) ([]string, int) {
	per := map[string]int{}
	for _, a := range ls {
		if a.Dir == "" {
			per[a.Provider]++ // an account folder is named by its folder, not by its email
		}
	}
	names := make([]string, len(ls))
	nameW := 0
	for i, a := range ls {
		p := providerNames[a.Provider]
		if p == "" {
			p = a.Provider
		}
		email, plan, _ := strings.Cut(a.Account, " · ")
		name := plan
		switch {
		case a.Dir != "":
			name = accounts.Name(a.Dir)
		case per[a.Provider] > 1 || plan == "":
			name, _, _ = strings.Cut(email, "@")
		case tight:
			name = ""
		}
		names[i] = strings.TrimSpace(p + " " + name)
		nameW = max(nameW, lipgloss.Width(names[i]))
	}
	return names, nameW
}

// machineLines are the machine in a few lines: cpu with its last minute
// and the load, memory, disk, and the agents that run.
func (m Model) machineLines(w int, full bool) []string {
	return m.machineAbout(w, full, cmp.Or(m.opt.Host, m.host.Host))
}

// machineAbout is machineLines with host named on its first line ("":
// none).
func (m Model) machineAbout(w int, full bool, host string) []string {
	st, mc := m.st, m.snap.Machine
	about := ""
	if host != "" {
		about = host + " · "
	}
	about += plural(len(mc.Cores), "core")
	right := st.faint.Render(about + " ")
	sp := max(min(w-30, 24), 8)
	if full {
		sp = 24
	}
	used, memPct := memUse(mc)
	out := []string{
		spread(st.label.Render(" MACHINE"), right, w),
		" " + st.label.Render("cpu  ") + st.accent.Render(spark(m.history.cpu, sp, 100)) +
			st.text.Render(pad(fmt.Sprintf(" %.0f%%", mc.CPU), 6)) + st.label.Render("load ") + st.text.Render(fmt.Sprintf("%.1f", mc.Load[0])),
		" " + st.label.Render("mem  ") + m.meter(memPct, 8, st.muted) + st.text.Render(fmt.Sprintf(" %.0f of %.0f GB", gb(used), gb(mc.MemTotal))),
		" " + st.label.Render("disk ") + m.diskCell(),
		" " + st.label.Render("runs ") + st.text.Render(m.runs()),
	}
	for i := range out {
		out[i] = fit(out[i], w)
	}
	return out
}

func gb(b uint64) float64 { return float64(b) / (1 << 30) }

// memUse is the memory in use, in bytes and in percent.
func memUse(mc sample.Machine) (uint64, float64) {
	used := mc.MemTotal - mc.MemAvail
	if mc.MemTotal == 0 {
		return used, 0
	}
	return used, float64(used) / float64(mc.MemTotal) * 100
}

// diskCell is the use of the home's file system, "▰▰▰▰▰▱▱▱ 71% of 1 TB",
// and of / when it is another; the disk's traffic when none was read.
func (m Model) diskCell() string {
	st, mc := m.st, m.snap.Machine
	if len(mc.Disks) == 0 {
		return st.text.Render("r " + size(mc.DiskRead) + "/s · w " + size(mc.DiskWrite) + "/s")
	}
	d := mc.Disks[0]
	cell := m.meter(d.Pct(), 8, m.useStyle(d.Pct())) + st.text.Render(fmt.Sprintf(" %.0f%% of %s", d.Pct(), bytesWord(d.Total)))
	for _, o := range mc.Disks[1:] {
		cell += st.faint.Render(" · "+o.Path+" ") + m.useStyle(o.Pct()).Render(fmt.Sprintf("%.0f%%", o.Pct()))
	}
	return cell
}

// bytesWord is a size as a disk is named: 931 GB, 2 TB.
func bytesWord(b uint64) string {
	if tb := float64(b) / (1 << 40); tb >= 1 {
		return strings.TrimSuffix(fmt.Sprintf("%.1f", tb), ".0") + " TB"
	}
	return fmt.Sprintf("%.0f GB", gb(b))
}

// runs counts the agents by their command: "13 claude · 1 codex".
func (m Model) runs() string {
	n := map[string]int{}
	for _, s := range m.all.sessions {
		agent := s.Agent
		if agent == "" {
			agent = "agent"
		}
		n[agent]++
	}
	if len(n) == 0 {
		return "no agents"
	}
	var names []string
	for a := range n {
		names = append(names, a)
	}
	slices.SortFunc(names, func(a, b string) int {
		if n[a] != n[b] {
			return n[b] - n[a]
		}
		return strings.Compare(a, b)
	})
	parts := make([]string, len(names))
	for i, a := range names {
		parts[i] = fmt.Sprintf("%d %s", n[a], a)
	}
	return strings.Join(parts, " · ")
}

// machinePanel is the Machine tab: usage first, then the machine, then
// the detached busy loops and the top processes. Two or more hosts: a
// section for each host, in Hosts order, headed by its name; the busy
// loops and the devices are the home host's.
func (m Model) machinePanel(w int) body {
	var b body
	b.add(-1, m.usageLines(w, true)...)
	if m.hostCount() == 1 {
		b.add(-1, "")
		b.add(-1, m.machineLines(w, true)...)
		b.add(-1, m.machine(w)...)
		p := m.procs(w)
		b.lines, b.rows = append(b.lines, p.lines...), append(b.rows, p.rows...)
		return b
	}
	for _, h := range m.statedHosts() {
		on := m.viewOf(h)
		b.add(-1, "", m.hostHead(h, w))
		b.add(-1, on.machineAbout(w, true, "")...)
		b.add(-1, on.machine(w)...)
		if h == m.opt.Host {
			p := on.procs(w)
			b.lines, b.rows = append(b.lines, p.lines...), append(b.rows, p.rows...)
			continue
		}
		b.add(-1, on.region("top processes", w)...)
		b.add(-1, on.topRows(w)...)
	}
	return b
}

// statedHosts are the hosts that sent a state, in Hosts order.
func (m Model) statedHosts() []string {
	var out []string
	if m.have {
		out = append(out, m.opt.Host)
	}
	for _, l := range m.peers {
		if l.have {
			out = append(out, l.host)
		}
	}
	return out
}

// viewOf is m with host's machine in the home host's place: the Machine
// and History tabs and the dock draw that host with it.
func (m Model) viewOf(host string) Model {
	if host == m.opt.Host {
		m.snap.Sessions, m.all.sessions = m.homeAll.sessions, m.homeAll.sessions
		return m
	}
	if i := m.peerAt(host); i >= 0 {
		l := m.peers[i]
		m.snap, m.history, m.hist, m.push = l.st.Snapshot, l.cpu, l.hist, l.st.Push
		m.all.sessions = l.st.Sessions
	}
	return m
}

// hostHead heads a host's section: its name, and since when a stale
// host's numbers are.
func (m Model) hostHead(host string, w int) string {
	head := m.st.label.Render(" " + m.nameOf(host))
	if note := m.staleNote(host); note != "" {
		head += "  " + m.st.faint.Render(note)
	}
	return fit(head, w)
}

// staleNote is "stale · as of 14:02" for a stale host of two or more.
func (m Model) staleNote(host string) string {
	if m.hostCount() == 1 || !m.stale(host) {
		return ""
	}
	return "stale · as of " + m.stateAt(host).Format("15:04")
}
