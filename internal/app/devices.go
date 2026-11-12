package app

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
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

// machineRows are the orphans, the device rows, then the host rows.
func (m Model) machineRows() int {
	return len(m.snap.Orphans) + len(m.deviceRows()) + len(m.hostRows())
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

// machineIndex is the selected row of the Machine tab.
func (m Model) machineIndex() int {
	if m.hostPicked {
		if i := slices.Index(m.hostRows(), m.hostPick); i >= 0 {
			return len(m.snap.Orphans) + len(m.deviceRows()) + i
		}
	}
	if m.devPick != "" {
		for i, r := range m.deviceRows() {
			if r.key() == m.devPick {
				return len(m.snap.Orphans) + i
			}
		}
	}
	return m.orphanIndex()
}

func (m *Model) machinePick(i int) {
	n, nd := len(m.snap.Orphans), len(m.deviceRows())
	m.hostPicked = false
	switch {
	case i < n:
		m.orphanPID, m.devPick = m.snap.Orphans[i].PID, ""
	case i < n+nd:
		m.devPick = m.deviceRows()[i-n].key()
	default:
		m.devPick, m.hostPicked, m.hostPick = "", true, m.hostRows()[i-n-nd]
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
	base := len(m.snap.Orphans) + len(m.deviceRows())
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
	i := m.machineIndex() - len(m.snap.Orphans)
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
	sel := m.machineIndex() - len(m.snap.Orphans)
	for i, r := range m.deviceRows() {
		on := picked && i == sel
		row := len(m.snap.Orphans) + i
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
