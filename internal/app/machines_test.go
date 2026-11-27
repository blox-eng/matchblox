package app

import (
	"cmp"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/sample"
)

// healthy is a host at rest: load 1 on 16 cores, memory and disk at half,
// up 3 days, nothing waiting on I/O.
func healthy() proto.State {
	st := hostState(sess(1, "api-auth", "%1", nil))
	zero := 0
	st.Machine = sample.Machine{
		CPU:      12,
		Cores:    make([]float64, 16),
		Load:     [3]float64{1, 1, 1},
		MemTotal: 64 << 30,
		MemAvail: 32 << 30,
		Disks:    []sample.Disk{{Path: "/", Total: 1 << 40, Used: 1 << 39, Avail: 1 << 39}},
		Booted:   lostAt.Add(-72 * time.Hour),
		IOWait:   &zero,
	}
	st.Heavy, st.Errors, st.Limits = nil, nil, nil
	return st
}

// fourHosts is a console at width 104 whose home host and peers gpu-1,
// box-3 and ws-1, in that Hosts order, sent these states at lostAt.
func fourHosts(t *testing.T, home, gpu, box, ws proto.State) (Model, map[string]*fakeConn) {
	t.Helper()
	now := lostAt
	m, _, conns := hostsAt(t, &now, "", 104, 30, "gpu-1", "box-3", "ws-1")
	m = upd(t, m, stateMsg(stateAt(home, now)))
	for host, st := range map[string]proto.State{"gpu-1": gpu, "box-3": box, "ws-1": ws} {
		m = upd(t, m, peerMsg{host: host, conn: conns[host], msg: stateMsg(stateAt(st, now))})
	}
	return m, conns
}

// lose drops host's connection with err.
func lose(t *testing.T, m Model, conns map[string]*fakeConn, host string, err error) Model {
	t.Helper()
	return upd(t, m, peerMsg{host: host, conn: conns[host], msg: lostMsg{err: err, conn: conns[host]}})
}

// machinesText is the region at width 104, without its styles.
func machinesText(m Model) []string { return machinesAt(m, 104) }

// machinesAt is the region at width w, without its styles.
func machinesAt(m Model, w int) []string {
	var out []string
	for _, l := range m.machinesBody(w).lines {
		out = append(out, ansi.Strip(l))
	}
	return out
}

// machineBlock is host's row and the lines under it, without styles.
func machineBlock(lines []string, host string) string {
	var out []string
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "     "):
			if len(out) > 0 {
				out = append(out, l)
			}
		case len(out) > 0:
			return strings.Join(out, "\n")
		case strings.HasPrefix(strings.TrimLeft(l, " ✗!"), host+" "):
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

func TestMachineRowAccountsHideTheEmail(t *testing.T) {
	ws := healthy()
	ws.Limits = []limits.Account{
		{Provider: "anthropic", Account: "attacker@example.com · Max"},
		{Provider: "openai", Account: "x@example.com · team · Pro"},
		{Provider: "cursor", Account: "x@example.com"},
		{Provider: "newco", Account: "x@example.com"},
		{Provider: "google", Account: "Pro · y@example.com"},
	}
	m, _ := fourHosts(t, healthy(), healthy(), healthy(), ws)
	lines := machinesText(m)
	block := machineBlock(lines, "ws-1")
	for _, want := range []string{"Claude Max", "OpenAI Pro", "Cursor", "newco", "Google"} {
		if !strings.Contains(block, want) {
			t.Errorf("ws-1's row has no %q:\n%s", want, block)
		}
	}
	if region := strings.Join(lines, "\n"); strings.Contains(region, "@") {
		t.Fatalf("the region shows an account name:\n%s", region)
	}
}

// machineRows are the names of the region's rows, in order.
func machineRows(m Model, lines []string) []string {
	var out []string
	for _, l := range lines {
		rest := strings.TrimLeft(l, " ✗!")
		if strings.TrimSpace(l) == "" || strings.HasPrefix(l, "     ") || strings.HasPrefix(rest, "MACHINES") {
			continue
		}
		for _, h := range m.hosts() {
			if name := m.nameOf(h); strings.HasPrefix(rest, name+" ") {
				out = append(out, name)
			}
		}
	}
	return out
}

// rawRow is the styled line of host's row.
func rawRow(m Model, host string) string {
	for _, l := range m.machinesBody(104).lines {
		if s := ansi.Strip(l); !strings.HasPrefix(s, "     ") && strings.HasPrefix(strings.TrimLeft(s, " ✗!"), host+" ") {
			return l
		}
	}
	return ""
}

func withLoad(st proto.State, load float64, cores int) proto.State {
	st.Machine.Load = [3]float64{load, load, load}
	st.Machine.Cores = make([]float64, cores)
	return st
}

func TestMachinesRowsInTroubleOrder(t *testing.T) {
	ws := withLoad(healthy(), 4, 32)
	ws.Machine.MemAvail = ws.Machine.MemTotal / 20
	m, conns := fourHosts(t, healthy(), healthy(), withLoad(healthy(), 50, 16), ws)
	m = lose(t, m, conns, "gpu-1", io.EOF)
	lines := machinesText(m)
	if !slices.ContainsFunc(lines, func(l string) bool { return strings.TrimSpace(l) == "MACHINES · 4" }) {
		t.Fatalf("no MACHINES · 4 head:\n%s", strings.Join(lines, "\n"))
	}
	want := []string{"gpu-1", "box-3", "ws-1", m.nameOf(m.opt.Host)}
	if got := machineRows(m, lines); !slices.Equal(got, want) {
		t.Fatalf("the rows are %q, want %q:\n%s", got, want, strings.Join(lines, "\n"))
	}
}

func TestMachineRowShowsLoadAgainstCores(t *testing.T) {
	m, _ := fourHosts(t, healthy(), healthy(), withLoad(healthy(), 50, 16), healthy())
	box := rawRow(m, "box-3")
	for _, want := range []string{"load 50 / 16 cores", "!"} {
		if !strings.Contains(box, m.st.neg.Render(want)) {
			t.Errorf("box-3's row has no neg %q: %q", want, box)
		}
	}
	x := m.st.neg.Render("x")
	neg := x[:strings.Index(x, "x")]
	if neg == "" {
		t.Fatal("the neg style draws no color here")
	}
	home := rawRow(m, m.nameOf(m.opt.Host))
	if home == "" || strings.Contains(home, neg) || strings.Contains(ansi.Strip(home), "!") {
		t.Fatalf("the healthy home row draws trouble: %q", home)
	}
}

func TestMachineRowCauseNamesTheSession(t *testing.T) {
	box := withLoad(healthy(), 50, 16)
	box.Sessions = append(box.Sessions, hostState(sess(11, "fetch-laws", "%4", nil)).Sessions...)
	box.Heavy = &sample.Tree{PID: 40, Comm: "xargs", Next: "gcloud", Procs: 257, CPU: 1400, Agent: 11}
	calm := healthy()
	calm.Heavy = &sample.Tree{PID: 40, Comm: "make", Procs: 3, CPU: 50, Agent: 1}
	m, _ := fourHosts(t, calm, healthy(), box, healthy())
	lines := machinesText(m)
	block := machineBlock(lines, "box-3")
	for _, want := range []string{"xargs → gcloud", "257 processes", "session fetch-laws"} {
		if !strings.Contains(block, want) {
			t.Errorf("box-3's cause has no %q:\n%s", want, block)
		}
	}
	if home := machineBlock(lines, m.nameOf(m.opt.Host)); strings.Contains(home, "make") || strings.Contains(home, "process") {
		t.Errorf("a host under its cores has a cause line:\n%s", home)
	}
	box.Heavy.Agent = 0
	m, _ = fourHosts(t, calm, healthy(), box, healthy())
	if block := machineBlock(machinesText(m), "box-3"); !strings.Contains(block, "no session") {
		t.Fatalf("a tree outside any session does not say so:\n%s", block)
	}
}

func TestLostMachineShowsNoFigures(t *testing.T) {
	for _, c := range []struct {
		err      error
		why, fix string
	}{
		{io.EOF, "", "run: matchblox connect gpu-1"},
		{remote.ErrHostKeyChanged, "host key changed: this can be an attack", "try again"},
	} {
		m, conns := fourHosts(t, healthy(), healthy(), healthy(), healthy())
		m = lose(t, m, conns, "gpu-1", c.err)
		l := m.peers[m.peerAt("gpu-1")]
		why, _ := m.blockedWords(blockedHost{host: "gpu-1", err: cmp.Or(l.blocked, l.dialErr)})
		if c.why != "" && why != c.why {
			t.Fatalf("blockedWords says %q, want %q", why, c.why)
		}
		lines := machinesText(m)
		block := machineBlock(lines, "gpu-1")
		row, under, _ := strings.Cut(block, "\n")
		if !strings.Contains(row, "not reached since 14:02") {
			t.Errorf("the lost row is %q", row)
		}
		for _, figure := range []string{"load", "cpu", "mem"} {
			if strings.Contains(block, figure) {
				t.Errorf("the lost host shows %q:\n%s", figure, block)
			}
		}
		if !strings.Contains(under, why) || !strings.Contains(under, c.fix) {
			t.Errorf("under the lost row %q, want %q and %q", under, why, c.fix)
		}
	}
}

func TestSilentAndOtherVersionRows(t *testing.T) {
	m, _ := fourHosts(t, healthy(), healthy(), healthy(), healthy())
	m.peers[m.peerAt("box-3")].silent = true
	ws := &m.peers[m.peerAt("ws-1")]
	ws.mismatch, ws.hello.Version = true, proto.Version-1
	lines := machinesText(m)
	for _, b := range []blockedHost{{host: "box-3", silent: true}, {host: "ws-1", other: true, version: proto.Version - 1}} {
		why, fix := m.blockedWords(b)
		if fix == "connect" {
			fix = "run: matchblox connect " + b.host
		}
		block := machineBlock(lines, b.host)
		if !strings.HasPrefix(strings.TrimSpace(block), "✗ "+b.host) {
			t.Errorf("%s has no ✗:\n%s", b.host, block)
		}
		for _, figure := range []string{"load", "cpu", "mem"} {
			if strings.Contains(block, figure) {
				t.Errorf("%s shows %q:\n%s", b.host, figure, block)
			}
		}
		if !strings.Contains(block, why) || !strings.Contains(block, fix) {
			t.Errorf("%s does not say %q and %q:\n%s", b.host, why, fix, block)
		}
	}
}

func TestOlderServeSaysUnknown(t *testing.T) {
	ws := withLoad(healthy(), 40, 16)
	ws.Machine.IOWait, ws.Machine.Booted = nil, time.Time{}
	m, _ := fourHosts(t, healthy(), healthy(), healthy(), ws)
	block := machineBlock(machinesText(m), "ws-1")
	row, _, _ := strings.Cut(block, "\n")
	for _, want := range []string{"io wait unknown", "up unknown"} {
		if !strings.Contains(row, want) {
			t.Errorf("the older serve's row has no %q: %q", want, row)
		}
	}
	for _, want := range []string{"cause unknown", "ws-1 runs an older matchblox · on ws-1: matchblox update"} {
		if !strings.Contains(block, want) {
			t.Errorf("under the older serve's row, no %q:\n%s", want, block)
		}
	}
	if strings.Contains(block, "io wait 0") {
		t.Fatalf("an unknown io wait shows as 0:\n%s", block)
	}
}

func TestNewServeOverCoresWithNoTree(t *testing.T) {
	for n, want := range map[int]string{5: "no busy process tree · the load is I/O wait", 0: "cause not sampled yet"} {
		ws := withLoad(healthy(), 40, 16)
		ws.Machine.IOWait = &n
		m, _ := fourHosts(t, healthy(), healthy(), healthy(), ws)
		if block := machineBlock(machinesText(m), "ws-1"); !strings.Contains(block, want) {
			t.Errorf("io wait %d: no %q:\n%s", n, want, block)
		}
	}
}

func TestNewServeWithoutBootSaysWhy(t *testing.T) {
	ws := healthy()
	ws.Machine.Booted = time.Time{}
	ws.Errors = []string{"boot time: no btime"}
	m, _ := fourHosts(t, healthy(), healthy(), healthy(), ws)
	block := machineBlock(machinesText(m), "ws-1")
	for _, want := range []string{"up unknown", "ws-1 did not read its boot time: no btime"} {
		if !strings.Contains(block, want) {
			t.Errorf("no %q:\n%s", want, block)
		}
	}
	if strings.Contains(block, "matchblox update") {
		t.Fatalf("a new serve says to update:\n%s", block)
	}
}

func TestAccountsFoldWhenTheRowIsFull(t *testing.T) {
	box := withLoad(healthy(), 50, 16)
	waits := 77
	box.Machine.IOWait = &waits
	box.Limits = []limits.Account{
		{Provider: "anthropic", Account: "a@example.com · Max"},
		{Provider: "openai", Account: "a@example.com · Pro"},
		{Provider: "cursor", Account: "a@example.com"},
	}
	ws := healthy()
	ws.Limits = box.Limits[:1]
	m, _ := fourHosts(t, healthy(), healthy(), box, ws)
	lines := machinesText(m)
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > 104 {
			t.Errorf("a line is %d wide: %q", w, l)
		}
	}
	block := machineBlock(lines, "box-3")
	row, under, _ := strings.Cut(block, "\n")
	if strings.Contains(row, "Claude") || !strings.Contains(row, "io wait 77") {
		t.Errorf("the full row did not fold its accounts: %q", row)
	}
	if !strings.Contains(under, "Claude Max · OpenAI Pro · Cursor") {
		t.Errorf("the folded accounts are not whole:\n%s", block)
	}
	if row, _, _ := strings.Cut(machineBlock(lines, "ws-1"), "\n"); !strings.Contains(row, "Claude Max") {
		t.Fatalf("an account that fits left its row: %q", row)
	}
}

func TestMachineRowNoPlanSessions(t *testing.T) {
	m, _ := fourHosts(t, healthy(), healthy(), healthy(), healthy())
	if block := machineBlock(machinesText(m), m.nameOf(m.opt.Host)); !strings.Contains(block, "no plan sessions") {
		t.Fatalf("a host with no plans does not say so:\n%s", block)
	}
}

func TestMachineRowMemoryOverTotalIsNoUse(t *testing.T) {
	ws := healthy()
	ws.Machine.MemAvail = ws.Machine.MemTotal + 1
	m, _ := fourHosts(t, healthy(), healthy(), healthy(), ws)
	row, _, _ := strings.Cut(machineBlock(machinesText(m), "ws-1"), "\n")
	if !strings.Contains(row, "mem 0%") || strings.Contains(row, "!") {
		t.Fatalf("a host with more memory free than it has reads as full: %q", row)
	}
}

func TestMachineRowShowsOnlyAPlanAsAPlan(t *testing.T) {
	ws := healthy()
	ws.Limits = []limits.Account{
		{Provider: "cursor", Account: "a@example.com · Acme Holdings Ltd"},
		{Provider: "anthropic", Account: "a@example.com · Max"},
		{Provider: "x@example.com", Account: "a@example.com · Pro"},
	}
	m, _ := fourHosts(t, healthy(), healthy(), healthy(), ws)
	lines := machinesText(m)
	block := machineBlock(lines, "ws-1")
	for _, want := range []string{"Cursor · ", "Claude Max", "account Pro"} {
		if !strings.Contains(block, want) {
			t.Errorf("ws-1's row has no %q:\n%s", want, block)
		}
	}
	region := strings.Join(lines, "\n")
	if strings.Contains(region, "Acme") || strings.Contains(region, "@") {
		t.Fatalf("the region shows a team or an account name:\n%s", region)
	}
}

func TestRemoteHomeLostShowsWhyAndFix(t *testing.T) {
	now := lostAt
	m, homeConn, conns := hostsAt(t, &now, "dev-1", 104, 30, "gpu-1")
	m = upd(t, m, stateMsg(stateAt(healthy(), now)))
	m = upd(t, m, peerMsg{host: "gpu-1", conn: conns["gpu-1"], msg: stateMsg(stateAt(healthy(), now))})
	m = upd(t, m, lostMsg{err: io.EOF, conn: homeConn})
	lines := machinesText(m)
	if got := machineRows(m, lines); !slices.Equal(got, []string{"dev-1", "gpu-1"}) {
		t.Errorf("the rows are %q, want the lost home first:\n%s", got, strings.Join(lines, "\n"))
	}
	why, fix := m.blockedWords(blockedHost{host: "dev-1"})
	if fix == "connect" {
		fix = "run: matchblox connect dev-1"
	}
	block := machineBlock(lines, "dev-1")
	row, under, _ := strings.Cut(block, "\n")
	if !strings.HasPrefix(strings.TrimSpace(row), "✗ dev-1") || !strings.Contains(row, "not reached since 14:02") {
		t.Errorf("the lost home's row is %q", row)
	}
	if !strings.Contains(under, why) || !strings.Contains(under, fix) {
		t.Fatalf("under the lost home's row %q, want %q and %q", under, why, fix)
	}
}

func TestRemoteHomeSilentShowsWhyAndFix(t *testing.T) {
	now := lostAt
	m, homeConn, conns := hostsAt(t, &now, "dev-1", 104, 30, "gpu-1")
	m = upd(t, m, peerMsg{host: "gpu-1", conn: conns["gpu-1"], msg: stateMsg(stateAt(healthy(), now))})
	m = upd(t, m, silentMsg{homeConn})
	why, fix := m.blockedWords(blockedHost{host: "dev-1", silent: true})
	lines := machinesText(m)
	block := machineBlock(lines, "dev-1")
	if !strings.HasPrefix(strings.TrimSpace(block), "✗ dev-1") || !strings.Contains(block, why) || !strings.Contains(block, fix) {
		t.Fatalf("the silent home does not say %q and %q:\n%s", why, fix, block)
	}
	m = upd(t, m, stateMsg(stateAt(healthy(), now)))
	if block := machineBlock(machinesText(m), "dev-1"); strings.Contains(block, "✗") || !strings.Contains(block, "load 1 / 16 cores") {
		t.Fatalf("the home that answered still reads as silent:\n%s", block)
	}
}

func TestNarrowRegionWrapsCellsWhole(t *testing.T) {
	ws := withLoad(healthy(), 4, 32)
	ws.Machine.MemAvail = ws.Machine.MemTotal / 20
	m, _ := fourHosts(t, healthy(), healthy(), withLoad(healthy(), 50, 16), ws)
	for _, w := range []int{60, 40} {
		lines := machinesAt(m, w)
		region := strings.Join(lines, "\n")
		for _, l := range lines {
			if lw := ansi.StringWidth(l); lw > w {
				t.Errorf("width %d: a line is %d wide: %q", w, lw, l)
			}
		}
		if strings.Contains(region, "…") {
			t.Errorf("width %d: a cell is cut:\n%s", w, region)
		}
		for host, cells := range map[string][]string{
			"box-3": {"up 3d", "load 50 / 16 cores", "cpu 12%", "mem 50%", "disk 50%", "io wait 0"},
			"ws-1":  {"up 3d", "load 4 / 32 cores", "cpu 12%", "mem 95%", "disk 50%", "io wait 0"},
		} {
			block := machineBlock(lines, host)
			for _, c := range cells {
				if !strings.Contains(block, c) {
					t.Errorf("width %d: %s has no whole %q:\n%s", w, host, c, block)
				}
			}
		}
	}
}

func TestMemoryTroubleIsTheShownFigure(t *testing.T) {
	for avail, c := range map[uint64]struct {
		figure  string
		trouble bool
	}{104: {"mem 90%", true}, 106: {"mem 89%", false}} {
		ws := healthy()
		ws.Machine.MemTotal, ws.Machine.MemAvail = 1000, avail
		m, _ := fourHosts(t, healthy(), healthy(), healthy(), ws)
		row := rawRow(m, "ws-1")
		if !strings.Contains(ansi.Strip(row), c.figure) {
			t.Errorf("%.1f%%: the row has no %q: %q", 100-float64(avail)/10, c.figure, ansi.Strip(row))
		}
		if got := strings.Contains(row, m.st.neg.Render(c.figure)); got != c.trouble {
			t.Errorf("%.1f%%: %q in neg is %v, want %v: %q", 100-float64(avail)/10, c.figure, got, c.trouble, row)
		}
		if got := strings.Contains(row, m.st.neg.Render("!")); got != c.trouble {
			t.Errorf("%.1f%%: the ! mark is %v, want %v: %q", 100-float64(avail)/10, got, c.trouble, ansi.Strip(row))
		}
	}
}

func TestUnderLinesWrapWhole(t *testing.T) {
	box := withLoad(healthy(), 50, 16)
	box.Sessions = append(box.Sessions, hostState(sess(11, "fetch-laws", "%4", nil)).Sessions...)
	box.Heavy = &sample.Tree{PID: 40, Comm: "xargs", Next: "gcloud", Procs: 257, CPU: 1400, Agent: 11}
	m, conns := fourHosts(t, healthy(), healthy(), box, healthy())
	m = lose(t, m, conns, "gpu-1", io.EOF)
	rows := map[string]machineRow{}
	for _, r := range m.machineOrder() {
		rows[r.host] = r
	}
	why, _ := m.blockedWords(*rows["gpu-1"].block)
	for host, want := range map[string]string{
		"gpu-1": why + " · run: matchblox connect gpu-1",
		"box-3": m.cause(rows["box-3"].on.snap),
	} {
		for _, w := range []int{40, 30} {
			var got []string
			for _, l := range m.underLines(rows[host], nil, w) {
				s := ansi.Strip(l)
				if lw := ansi.StringWidth(s); lw > w {
					t.Errorf("width %d: %s's line is %d wide: %q", w, host, lw, s)
				}
				if !strings.HasPrefix(s, "     ") {
					t.Errorf("width %d: %s's line is not indented: %q", w, host, s)
				}
				if strings.Contains(s, "…") {
					t.Errorf("width %d: %s's line is cut: %q", w, host, s)
				}
				got = append(got, strings.Fields(s)...)
			}
			if !slices.Equal(got, strings.Fields(want)) {
				t.Errorf("width %d: under %s the words are %q, want %q", w, host, got, strings.Fields(want))
			}
		}
	}
}

// machineTabText is the Machine tab's body at width 104, without its styles.
func machineTabText(m Model) []string {
	var out []string
	for _, l := range m.machinePanel(104).lines {
		out = append(out, ansi.Strip(l))
	}
	return out
}

func TestMachineOpensWithTheRegion(t *testing.T) {
	m, _ := twoHosts(t, hostState(sess(1, "api-auth", "%1", nil)), hostState(sess(2, "train", "%2", nil)))
	lines := machineTabText(m)
	head := slices.IndexFunc(lines, func(l string) bool { return strings.TrimSpace(l) == "MACHINES · 2" })
	usage := slices.IndexFunc(lines, func(l string) bool { return strings.TrimSpace(l) == "USAGE" })
	last := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(strings.TrimSpace(l), "panes") })
	if last < 0 || head <= last || usage <= head {
		t.Fatalf("the views end at %d, the region heads at %d and USAGE at %d:\n%s", last, head, usage, strings.Join(lines, "\n"))
	}
	for _, host := range []string{m.nameOf(m.opt.Host), "ws-1"} {
		if !slices.ContainsFunc(lines[head:usage], func(l string) bool { return strings.HasPrefix(strings.TrimLeft(l, " ✗!"), host+" ") }) {
			t.Errorf("the region has no row of %s:\n%s", host, strings.Join(lines[head:usage], "\n"))
		}
	}
}

func TestOneHostMachineHasOneRow(t *testing.T) {
	m, _ := loadedWith(t, 104, hostState(sess(1, "api-auth", "%1", nil)))
	lines := machineTabText(m)
	head := slices.IndexFunc(lines, func(l string) bool { return strings.TrimSpace(l) == "MACHINES · 1" })
	if head < 0 {
		t.Fatalf("one host's Machine has no region:\n%s", strings.Join(lines, "\n"))
	}
	end := head + slices.Index(lines[head:], "")
	var rows []string
	for _, l := range lines[head+1 : end] {
		if !strings.HasPrefix(l, "     ") {
			rows = append(rows, l)
		}
	}
	if len(rows) != 1 || m.rowBase(rowOrphans) != m.rowBase(rowMachines)+1 {
		t.Fatalf("one host's region has the rows %q, and %d selectable", rows, m.rowBase(rowOrphans)-m.rowBase(rowMachines))
	}
}

func TestOneBlockedHostHasNoRegion(t *testing.T) {
	f := newFake()
	next, _ := New(Options{NoMotion: true, Conn: f}).Update(tea.WindowSizeMsg{Width: 104, Height: 30})
	m := upd(t, next.(Model), lostMsg{err: io.EOF, conn: f})
	m.setTab(tabMachine)
	if m.have {
		t.Fatal("the host was reached")
	}
	if out := screen(m); strings.Contains(out, "MACHINES ·") || !strings.Contains(out, m.waitingWhy()) {
		t.Fatalf("one blocked host's screen:\n%s", out)
	}
	if body := strings.Join(machineTabText(m), "\n"); strings.Contains(body, "MACHINES ·") {
		t.Fatalf("one blocked host's Machine has the region:\n%s", body)
	}
	if m.rowBase(rowOrphans) != m.rowBase(rowMachines) {
		t.Fatal("one blocked host has machine rows")
	}
}

func TestDownFromTheViewsReachesTheMachines(t *testing.T) {
	m, conns := fourHosts(t, healthy(), healthy(), healthy(), healthy())
	m = lose(t, m, conns, "gpu-1", io.EOF)
	m.setTab(tabMachine)
	m.selectRow(len(machineViews) - 1)
	order := m.machineOrder()
	for i, want := range []string{order[0].host, order[1].host} {
		m = pressOn(m, "down")
		if !m.machineHostPicked || m.machineHost != want || m.rowIndex() != m.rowBase(rowMachines)+i {
			t.Fatalf("↓ %d: machine %q picked %v at row %d, want %q at %d", i+1, m.machineHost, m.machineHostPicked, m.rowIndex(), want, m.rowBase(rowMachines)+i)
		}
		b := m.machinePanel(104)
		at := slices.Index(b.rows, m.rowIndex())
		if at < 0 || !strings.HasPrefix(strings.TrimLeft(ansi.Strip(b.lines[at]), " ✗!"), m.nameOf(want)+" ") {
			t.Fatalf("↓ %d: the selected line is not %s's row", i+1, want)
		}
		off := m
		off.machinePick(0)
		if b.lines[at] == rawRow(off, want) {
			t.Fatalf("↓ %d: %s's row is not on the selected wash", i+1, want)
		}
		if keys := plainHints(m.view().keys(m)...); !strings.Contains(keys, "⏎ section") {
			t.Fatalf("↓ %d: a machine row offers %q", i+1, keys)
		}
	}
}

// threeMachines is a console at width 104 whose body is 20 lines: the home
// host and peers gpu-1 and ws-1, in that Hosts order; stated are the hosts
// that sent a state at lostAt, "" for the home host.
func threeMachines(t *testing.T, stated ...string) Model {
	t.Helper()
	now := lostAt
	m, _, conns := hostsAt(t, &now, "", 104, 25, "gpu-1", "ws-1")
	for _, h := range stated {
		if h == "" {
			m = upd(t, m, stateMsg(stateAt(healthy(), now)))
			continue
		}
		m = upd(t, m, peerMsg{host: h, conn: conns[h], msg: stateMsg(stateAt(healthy(), now))})
	}
	m.setTab(tabMachine)
	return m
}

// onMachineRow is m with host's machine row selected.
func onMachineRow(t *testing.T, m Model, host string) Model {
	t.Helper()
	i := slices.IndexFunc(m.machineList(), func(r machineRow) bool { return r.host == host })
	if i < 0 {
		t.Fatalf("no machine row of %q", host)
	}
	m.selectRow(m.rowBase(rowMachines) + i)
	return m
}

// machineWindow is the body the screen draws, without its styles: the
// lines under the tab rule.
func machineWindow(m Model) []string {
	f := frameOf(m)
	return f[3 : 3+m.bodyRows()]
}

func TestEnterOnAMachineMovesToItsSection(t *testing.T) {
	m := onMachineRow(t, threeMachines(t, "", "gpu-1", "ws-1"), "ws-1")
	if m.bodyRows() != 20 {
		t.Fatalf("the window has %d lines, want 20", m.bodyRows())
	}
	if keys := plainHints(m.view().keys(m)...); !strings.Contains(keys, "⏎ section") {
		t.Fatalf("ws-1's row offers %q", keys)
	}
	m = pressOn(m, "enter")
	if win := machineWindow(m); strings.TrimSpace(win[0]) != "ws-1" {
		t.Fatalf("after ⏎ the window starts at %q:\n%s", win[0], strings.Join(win, "\n"))
	}
	if !m.machineHostPicked || m.machineHost != "ws-1" {
		t.Fatalf("after ⏎ the selection is %q (%v), want ws-1's row", m.machineHost, m.machineHostPicked)
	}
}

func TestAKeyAfterTheJumpFollowsTheSelectionAgain(t *testing.T) {
	m := onMachineRow(t, threeMachines(t, "", "gpu-1", "ws-1"), "ws-1")
	twin := pressOn(m, "up")
	m = pressOn(m, "enter", "up")
	if m.rowIndex() != twin.rowIndex() || m.machineHost != "gpu-1" {
		t.Fatalf("↑ after ⏎ selects row %d (%q), want %d (gpu-1)", m.rowIndex(), m.machineHost, twin.rowIndex())
	}
	if got, want := strings.Join(machineWindow(m), "\n"), strings.Join(machineWindow(twin), "\n"); got != want {
		t.Fatalf("↑ after ⏎ draws:\n%s\nwant, as with no ⏎:\n%s", got, want)
	}
}

func TestEnterOnOneHostMovesToMachine(t *testing.T) {
	m, _ := loadedWith(t, 104, hostState(sess(1, "api-auth", "%1", nil)))
	m.setTab(tabMachine)
	m = onMachineRow(t, m, m.opt.Host)
	m = pressOn(m, "enter")
	if win := machineWindow(m); !strings.HasPrefix(win[0], " MACHINE ") {
		t.Fatalf("after ⏎ the window starts at %q:\n%s", win[0], strings.Join(win, "\n"))
	}
}

func TestEnterOnAnUnreachedHostMovesToItsHostRow(t *testing.T) {
	m := onMachineRow(t, threeMachines(t, "", "gpu-1"), "ws-1")
	before := strings.Join(machineWindow(m), "\n")
	if keys := plainHints(m.view().keys(m)...); strings.Contains(keys, "⏎") {
		t.Fatalf("with no hosts list, ws-1's row offers %q", keys)
	}
	if next := pressOn(m, "enter"); next.rowIndex() != m.rowIndex() || next.hostPicked || strings.Join(machineWindow(next), "\n") != before {
		t.Fatalf("with no hosts list, ⏎ moved to row %d (host row %v):\n%s", next.rowIndex(), next.hostPicked, strings.Join(machineWindow(next), "\n"))
	}
	m.opt.Layered = true
	m = onMachineRow(t, m, "ws-1")
	if keys := plainHints(m.view().keys(m)...); !strings.Contains(keys, "⏎ hosts") {
		t.Fatalf("ws-1's row offers %q", keys)
	}
	m = pressOn(m, "enter")
	if !m.hostPicked || m.hostPick != "ws-1" || m.rowIndex() != m.rowBase(rowHosts)+2 {
		t.Fatalf("after ⏎ the selection is row %d (host %q, %v), want ws-1's hosts row %d", m.rowIndex(), m.hostPick, m.hostPicked, m.rowBase(rowHosts)+2)
	}
}

// tapLine taps the first line of the screen whose text, trimmed of marks,
// starts with prefix.
func tapLine(t *testing.T, m Model, prefix string) Model {
	t.Helper()
	y := slices.IndexFunc(frameOf(m), func(l string) bool { return strings.HasPrefix(strings.TrimLeft(l, " ✗!"), prefix) })
	if y < 0 {
		t.Fatalf("no line starts with %q:\n%s", prefix, strings.Join(frameOf(m), "\n"))
	}
	next, cmd := tap(m, 4, y)
	drain(cmd)
	return next.(Model)
}

func TestASecondTapOnAMachineRowJumps(t *testing.T) {
	m := threeMachines(t, "", "gpu-1", "ws-1")
	m = tapLine(t, m, "ws-1 ")
	if !m.machineHostPicked || m.machineHost != "ws-1" {
		t.Fatalf("a tap selects %q (%v), want ws-1's row", m.machineHost, m.machineHostPicked)
	}
	if win := machineWindow(m); strings.TrimSpace(win[0]) == "ws-1" {
		t.Fatal("the first tap moved to ws-1's section")
	}
	m = tapLine(t, m, "ws-1 ")
	if win := machineWindow(m); strings.TrimSpace(win[0]) != "ws-1" {
		t.Fatalf("after a second tap the window starts at %q:\n%s", win[0], strings.Join(win, "\n"))
	}
	next, cmd := tap(m, 4, 3) // ws-1's head: the tap hits what the screen shows
	drain(cmd)
	if m = next.(Model); !m.machineHostPicked || m.machineHost != "ws-1" {
		t.Fatalf("a tap on ws-1's head selects row %d", m.rowIndex())
	}
	if win := machineWindow(m); strings.TrimSpace(win[0]) == "ws-1" {
		t.Fatal("after a tap the window still starts at ws-1's section")
	}
}

// TestASecondTapWithNoSectionSaysWhy: a second tap on a machine row that
// has nowhere to move says so and where the fix is, never nothing at all.
func TestASecondTapWithNoSectionSaysWhy(t *testing.T) {
	m := threeMachines(t, "", "gpu-1") // ws-1 never answered, and no hosts list
	m = tapLine(t, m, "ws-1 ")
	m = tapLine(t, m, "ws-1 ")
	if !strings.Contains(m.flash, "ws-1 has no section yet") || !strings.Contains(m.flash, "under its row") {
		t.Fatalf("a second tap on ws-1 says %q", m.flash)
	}
}
