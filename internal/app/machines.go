package app

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/sample"
)

// machineRow is one host of the machines region. A host not reached has
// gone in place of its figures; one marked ✗ has block, whose words
// blockedWords owns.
type machineRow struct {
	host  string
	gone  string
	block *blockedHost
	on    Model // viewOf(host), for a reached host
}

// machineRowOf is host's row: its state from its link or the home fields.
func (m Model) machineRowOf(host string) machineRow {
	lost, have, at, blocked, mismatch, version, silent := m.lost, m.have, m.at, m.blocked, m.mismatch, m.host.Version, m.silent
	var dialErr error
	if i := m.peerAt(host); host != m.opt.Host && i >= 0 {
		l := m.peers[i]
		lost, have, at, blocked, mismatch, version = l.lost, l.have, l.at, l.blocked, l.mismatch, l.hello.Version
		dialErr, silent = l.dialErr, l.silent
	}
	r := machineRow{host: host}
	unreached := &blockedHost{host: host, err: cmp.Or(blocked, dialErr)}
	switch {
	case mismatch:
		r.gone, r.block = "runs another version", &blockedHost{host: host, other: true, version: version}
	case silent:
		r.gone, r.block = "connected, no state yet", &blockedHost{host: host, silent: true}
	case have && (lost || blocked != nil):
		r.gone, r.block = "not reached since "+at.Format("15:04"), unreached
		if d := m.now().Sub(at); d >= time.Minute {
			r.gone += " (" + brief(d) + " ago)"
		}
	case !have && (lost || blocked != nil || dialErr != nil):
		r.gone, r.block = "never reached", unreached
	case !have:
		r.gone = "connecting…"
	default:
		r.on = m.viewOf(host)
	}
	return r
}

func (r machineRow) over() bool {
	mc := r.on.snap.Machine
	return r.gone == "" && len(mc.Cores) > 0 && mc.Load[0] > float64(len(mc.Cores))
}

func (r machineRow) full() bool {
	if r.gone != "" {
		return false
	}
	_, mem := memUse(r.on.snap.Machine)
	disk, _ := worstDisk(r.on.snap.Machine)
	return fullAt(mem) || fullAt(disk)
}

// fullAt: a use shown as 90% or more, so a figure in trouble is the one
// the row prints.
func fullAt(pct float64) bool { return math.RoundToEven(pct) >= 90 }

// group is the row's place in the trouble order.
func (r machineRow) group() int {
	switch {
	case r.block != nil:
		return 0
	case r.over():
		return 1
	case r.full():
		return 2
	}
	return 3
}

// machineOrder are the hosts' rows, trouble first, then in Hosts order.
func (m Model) machineOrder() []machineRow {
	var rows []machineRow
	for _, h := range m.hosts() {
		rows = append(rows, m.machineRowOf(h))
	}
	slices.SortStableFunc(rows, func(a, b machineRow) int { return a.group() - b.group() })
	return rows
}

// worstDisk is the fullest disk's use; false when none was read.
func worstDisk(mc sample.Machine) (float64, bool) {
	pct := 0.0
	for _, d := range mc.Disks {
		pct = max(pct, d.Pct())
	}
	return pct, len(mc.Disks) > 0
}

// machinesBody is the machines region: a row for each host, trouble first,
// and under a row what drives it or what went wrong.
func (m Model) machinesBody(w int) body {
	var b body
	rows := m.machineOrder()
	b.add(-1, m.region(fmt.Sprintf("machines · %d", len(rows)), w)...)
	nameW := 0
	for _, r := range rows {
		nameW = max(nameW, lipgloss.Width(m.nameOf(r.host)))
	}
	cells := make([][]string, len(rows))
	widths := []int{}
	for i, r := range rows {
		if r.gone != "" {
			continue
		}
		cells[i] = m.machineCells(r)
		for j, c := range cells[i] {
			if j == len(widths) {
				widths = append(widths, 0)
			}
			widths[j] = max(widths[j], lipgloss.Width(c))
		}
	}
	st := m.st
	for i, r := range rows {
		mark := " "
		switch {
		case r.block != nil:
			mark = st.neg.Render("✗")
		case r.group() < 3:
			mark = st.neg.Render("!")
		}
		head := " " + mark + " " + st.text.Render(pad(m.nameOf(r.host), nameW+2))
		if r.gone != "" {
			b.add(-1, fit(head+st.muted.Render(r.gone), w))
			b.add(-1, m.underLines(r, nil, w)...)
			continue
		}
		// A cell that does not fit starts a line under the first cell.
		indent := strings.Repeat(" ", lipgloss.Width(head))
		line := head
		for j, c := range cells[i] {
			if line != head && lipgloss.Width(line)+lipgloss.Width(c) > w {
				b.add(-1, fit(strings.TrimRight(line, " "), w))
				line = indent
			}
			line += c + strings.Repeat(" ", widths[j]-lipgloss.Width(c)+2)
		}
		accounts, style := strings.Join(accountWords(r.on.snap.Limits), " · "), st.text
		if accounts == "" {
			accounts, style = "no plan sessions", st.faint
		}
		var folded []string
		if lipgloss.Width(line+accounts) <= w {
			line += style.Render(accounts)
		} else {
			folded = foldWords(accounts, w-5)
		}
		b.add(-1, fit(strings.TrimRight(line, " "), w))
		b.add(-1, m.underLines(r, folded, w)...)
	}
	return b
}

// machineCells are a reached host's figures, a trouble figure in neg.
func (m Model) machineCells(r machineRow) []string {
	st, mc := m.st, r.on.snap.Machine
	text := func(trouble bool, s string) string {
		if trouble {
			return st.neg.Render(s)
		}
		return st.text.Render(s)
	}
	up := "up unknown"
	if !mc.Booted.IsZero() {
		up = "up " + brief(m.now().Sub(mc.Booted))
	}
	load := "load unknown"
	if len(mc.Cores) > 0 {
		load = fmt.Sprintf("load %s / %d cores", loadWord(mc.Load[0]), len(mc.Cores))
	}
	_, memPct := memUse(mc)
	mem := "mem unknown"
	if mc.MemTotal > 0 {
		mem = fmt.Sprintf("mem %.0f%%", memPct)
	}
	diskPct, read := worstDisk(mc)
	disk := "disk unknown"
	if read {
		disk = fmt.Sprintf("disk %.0f%%", diskPct)
	}
	io := "io wait unknown"
	if mc.IOWait != nil {
		io = fmt.Sprintf("io wait %d", *mc.IOWait)
	}
	return []string{
		text(false, up),
		text(r.over(), load),
		text(false, fmt.Sprintf("cpu %.0f%%", mc.CPU)),
		text(fullAt(memPct), mem),
		text(read && fullAt(diskPct), disk),
		text(false, io),
	}
}

// loadWord is a load average as the row says it: 50, 4, 0.6.
func loadWord(l float64) string {
	if l >= 10 {
		return fmt.Sprintf("%.0f", l)
	}
	return strings.TrimSuffix(fmt.Sprintf("%.1f", l), ".0")
}

// underLines are the lines under a host's row: its folded accounts, the
// cause of its load, and what went wrong with the fix in the words.
func (m Model) underLines(r machineRow, folded []string, w int) []string {
	var out []string
	say := func(s string) { out = append(out, fit("     "+m.st.muted.Render(s), w)) }
	for _, f := range folded {
		say(f)
	}
	if r.block != nil {
		why, fix := m.blockedWords(*r.block)
		if fix == "connect" {
			fix = "run: matchblox connect " + cmp.Or(r.host, m.nameOf(r.host))
		}
		say(why + " · " + fix)
		return out
	}
	if r.gone != "" {
		return out
	}
	snap, name := r.on.snap, m.nameOf(r.host)
	if r.over() {
		say(m.cause(snap))
	}
	if snap.Machine.IOWait == nil {
		say(name + " runs an older matchblox · on " + name + ": matchblox update")
	}
	if snap.Machine.Booted.IsZero() {
		for _, e := range snap.Errors {
			if why, ok := strings.CutPrefix(e, "boot time: "); ok {
				say(name + " did not read its boot time: " + why)
			}
		}
	}
	return out
}

// cause is what drives a host over its cores: its heaviest process tree
// and the session it runs in, or why none is named.
func (m Model) cause(snap sample.Snapshot) string {
	t := snap.Heavy
	switch {
	case t == nil && snap.Machine.IOWait == nil:
		return "cause unknown"
	case t == nil && *snap.Machine.IOWait > 0:
		return "no busy process tree · the load is I/O wait"
	case t == nil:
		return "cause not sampled yet"
	}
	cmd := t.Comm
	if t.Next != "" {
		cmd += " → " + t.Next
	}
	who := "no session"
	if i := slices.IndexFunc(snap.Sessions, func(s sample.Session) bool { return t.Agent != 0 && s.PID == t.Agent }); i >= 0 {
		who = "session " + snap.Sessions[i].Name
	}
	return fmt.Sprintf("%s · %s · %s%% cpu · %s", cmd, plural(t.Procs, "process"), grouped(int(t.CPU+0.5)), who)
}

// grouped is n with its thousands apart: 1,400.
func grouped(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// accountWords are the provider and plan of each account a host's sessions
// run under, never the account's name or team: "Claude Max", "OpenAI Pro".
func accountWords(ls []limits.Account) []string {
	words := make([]string, len(ls))
	for i, a := range ls {
		words[i] = cmp.Or(providerNames[a.Provider], a.Provider)
		if strings.Contains(words[i], "@") {
			words[i] = "account"
		}
		if at := strings.LastIndex(a.Account, " · "); at >= 0 {
			if plan := strings.TrimSpace(a.Account[at+len(" · "):]); sample.IsPlan(plan) {
				words[i] += " " + plan
			}
		}
	}
	return words
}

// foldWords breaks accounts at their " · " into lines of at most w cells,
// each account whole.
func foldWords(s string, w int) []string {
	var out []string
	line := ""
	for _, part := range strings.Split(s, " · ") {
		switch {
		case line == "":
			line = part
		case lipgloss.Width(line+" · "+part) <= w:
			line += " · " + part
		default:
			out, line = append(out, line), part
		}
	}
	return append(out, line)
}
