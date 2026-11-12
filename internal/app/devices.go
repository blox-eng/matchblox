package app

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/blox-eng/matchblox/internal/sample"
)

// devPlus is devPick when the row "+ pair a device" is selected.
const devPlus = "+"

// devRow is a row under DEVICES: a key that opens this host, or the plus.
type devRow struct {
	d    PairDevice
	plus bool
}

func (r devRow) key() string {
	if r.plus {
		return devPlus
	}
	return r.d.Fingerprint
}

// loadDevices reads the keys again. It runs when the Machine tab opens and
// after a change, never per frame.
func (m *Model) loadDevices() {
	m.devices, m.devErr = nil, nil
	if m.remoteDevices() || m.opt.Pair.Devices == nil {
		return
	}
	ds, err := m.opt.Pair.Devices()
	if err != nil {
		m.devErr = err
		return
	}
	// Phones first; the order of the file holds inside each kind.
	slices.SortStableFunc(ds, func(a, b PairDevice) int {
		return strings.Compare(b.Kind, a.Kind)
	})
	m.devices = ds
}

// remoteDevices: the keys live on the host, so this console only says how
// to pair there.
func (m Model) remoteDevices() bool { return m.opt.Host != "" || m.opt.Pair == nil }

// deviceRows are the rows the cursor reaches: the devices, then the plus.
// A remote console has none.
func (m Model) deviceRows() []devRow {
	if m.remoteDevices() {
		return nil
	}
	rows := make([]devRow, 0, len(m.devices)+1)
	for _, d := range m.devices {
		rows = append(rows, devRow{d: d})
	}
	return append(rows, devRow{plus: true})
}

// machineView is a row of Machine that opens a screen of its own: ⏎ opens
// it, and esc comes back to Machine, on its row.
type machineView struct {
	name string
	tab  int
}

// machineViews are Machine's first rows.
var machineViews = []machineView{{"stoker", tabStoker}}

// machineRows are the views, the orphans, the device rows, then the host
// rows.
func (m Model) machineRows() int {
	return len(machineViews) + len(m.snap.Orphans) + len(m.deviceRows()) + len(m.hostRows())
}

// hostRows are Machine's rows of the hosts, in Hosts order: ⏎ on one opens
// Hosts, where a host is added or removed. A console with no Hosts above
// it has none.
func (m Model) hostRows() []string {
	if !m.opt.Layered {
		return nil
	}
	return m.hosts()
}

// machineIndex is the selected row of the Machine tab: its first view
// until another row is chosen.
func (m Model) machineIndex() int {
	nv, no := len(machineViews), len(m.snap.Orphans)
	if m.hostPicked {
		if i := slices.Index(m.hostRows(), m.hostPick); i >= 0 {
			return nv + no + len(m.deviceRows()) + i
		}
	}
	if m.devPick != "" {
		if i := slices.IndexFunc(m.deviceRows(), func(r devRow) bool { return r.key() == m.devPick }); i >= 0 {
			return nv + no + i
		}
	}
	if i := slices.IndexFunc(machineViews, func(v machineView) bool { return v.name == m.viewPick }); i >= 0 {
		return i
	}
	if i := slices.IndexFunc(m.snap.Orphans, func(o sample.Orphan) bool { return o.PID == m.orphanPID }); i >= 0 {
		return nv + i
	}
	return 0
}

func (m *Model) machinePick(i int) {
	nv, no, nd := len(machineViews), len(m.snap.Orphans), len(m.deviceRows())
	m.viewPick, m.orphanPID, m.devPick, m.hostPicked = "", 0, "", false
	switch {
	case i < nv:
		m.viewPick = machineViews[i].name
	case i < nv+no:
		m.orphanPID = m.snap.Orphans[i-nv].PID
	case i < nv+no+nd:
		m.devPick = m.deviceRows()[i-nv-no].key()
	default:
		m.hostPicked, m.hostPick = true, m.hostRows()[i-nv-no-nd]
	}
}

// selectedMachineView is the view row under the cursor.
func (m Model) selectedMachineView() (machineView, bool) {
	if i := m.machineIndex(); i < len(machineViews) {
		return machineViews[i], true
	}
	return machineView{}, false
}

// openedView is the view of Machine the screen shows, when it shows one.
func (m Model) openedView() (machineView, bool) {
	i := slices.IndexFunc(machineViews, func(v machineView) bool { return v.tab == m.tab })
	if i < 0 {
		return machineView{}, false
	}
	return machineViews[i], true
}

// openTab is the tab the tab row names open: Machine for a view of it.
func (m Model) openTab() int {
	if _, ok := m.openedView(); ok {
		return tabMachine
	}
	return m.tab
}

// openView is ⏎ on a view row: its screen, as today.
func (m Model) openView(v machineView) Model {
	m.setTab(v.tab)
	return m
}

// backToMachine is esc on a view's screen: Machine, on its row.
func (m Model) backToMachine() Model {
	v, ok := m.openedView()
	if !ok {
		return m
	}
	m.setTab(tabMachine)
	m.machinePick(slices.Index(machineViews, v))
	return m
}

// viewsBody is Machine's view rows, then a blank line.
func (m Model) viewsBody(w int) body {
	st := m.st
	var b body
	sel := m.machineIndex()
	for i, v := range machineViews {
		on := i == sel
		b.addRow(i, on, w, st, " "+st.ink(on).Render(pad(v.name, 10))+st.faint.Render(m.viewNote(v)))
	}
	b.add(-1, "")
	return b
}

// viewNote is what a view row says after its name: the stoker's state
// and how many of its steps wait for the builder.
func (m Model) viewNote(v machineView) string {
	if v.tab != tabStoker {
		return ""
	}
	switch n := m.stokerNeeds(); n {
	case 0:
		return m.stokerWord()
	case 1:
		return m.stokerWord() + " · 1 needs you"
	default:
		return m.stokerWord() + " · " + strconv.Itoa(n) + " need you"
	}
}

// selectedHostRow is the host of the host row under the cursor.
func (m Model) selectedHostRow() (string, bool) {
	if !m.hostPicked || !slices.Contains(m.hostRows(), m.hostPick) {
		return "", false
	}
	return m.hostPick, true
}

// hostsBody is HOSTS · N: a row for each host, ⏎ opens Hosts.
func (m Model) hostsBody(w int) body {
	hs := m.hostRows()
	if len(hs) == 0 {
		return body{}
	}
	st := m.st
	b := plain(m.region(fmt.Sprintf("hosts · %d", len(hs)), w))
	at, picked := m.selectedHostRow()
	base := len(machineViews) + len(m.snap.Orphans) + len(m.deviceRows())
	for i, h := range hs {
		on := picked && h == at
		line := " " + st.ink(on).Render(pad(m.nameOf(h), 24))
		if note := m.staleNote(h); note != "" {
			line += st.faint.Render(note)
		}
		b.addRow(base+i, on, w, st, line)
	}
	return b
}

// openHosts is ⏎ on a host row: Hosts, where a host is added or removed.
func (m Model) openHosts() (tea.Model, tea.Cmd) {
	return m, func() tea.Msg { return layerMsg{} }
}

// selectedDevice is the device row under the cursor.
func (m Model) selectedDevice() (devRow, bool) {
	rows := m.deviceRows()
	i := m.machineIndex() - len(machineViews) - len(m.snap.Orphans)
	if m.devPick == "" || i < 0 || i >= len(rows) {
		return devRow{}, false
	}
	return rows[i], true
}

// devicesBody is DEVICES · N: the keys that open this host.
func (m Model) devicesBody(w int) body {
	st := m.st
	if m.remoteDevices() {
		if m.opt.Host == "" {
			return body{} // no host to name, and no page to open
		}
		b := plain(m.region("devices", w))
		b.add(-1, fit(" "+st.muted.Render("run matchblox pair on "+m.opt.Host), w))
		return b
	}
	b := plain(m.region(fmt.Sprintf("devices · %d", len(m.devices)), w))
	if m.devErr != nil {
		b.add(-1, fit(" "+st.warn.Render("cannot read ~/.ssh/authorized_keys: "+m.devErr.Error()+". Check its owner and mode (600)."), w))
	}
	_, picked := m.selectedDevice()
	base := len(machineViews) + len(m.snap.Orphans)
	sel := m.machineIndex() - base
	for i, r := range m.deviceRows() {
		on := picked && i == sel
		row := base + i
		if r.plus {
			b.addRow(row, on, w, st, " "+st.accent.Render("+ pair a device"))
			continue
		}
		b.addRow(row, on, w, st, " "+st.muted.Render(pad(r.d.Kind, 8))+st.ink(on).Render(pad(r.d.Name, 24))+st.faint.Render(r.d.Fingerprint))
	}
	return b
}

// removeDevice is x on a device: it takes the key out after a typed y.
func (m Model) removeDevice(r devRow) *action {
	d := r.d
	return &action{
		label: "remove", destructive: true,
		say: fmt.Sprintf("remove %s (%s): it can no longer open this host", d.Name, d.Fingerprint),
		local: func(m Model) (Model, string) {
			if m.remoteDevices() || m.opt.Pair.RemoveDevice == nil {
				return m, "Remove devices from the host's own terminal."
			}
			if m.fromDevice() { // checked again: the last client may have changed
				return m, removeFromDeviceLine
			}
			// By fingerprint: two devices can share a name.
			at := m.machineIndex()
			backup, err := m.opt.Pair.RemoveDevice(d.Fingerprint)
			m.loadDevices()
			m.machinePick(min(at, max(m.machineRows()-1, 0))) // the cursor stays at the row
			if err != nil {
				return m, "not removed: " + err.Error()
			}
			msg := fmt.Sprintf("Removed %s (%s).", d.Name, d.Fingerprint)
			if backup != "" {
				msg += " Backup: " + tilde(backup)
			}
			return m, msg
		},
	}
}
