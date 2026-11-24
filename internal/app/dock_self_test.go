package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestMachineSaysMatchbloxServe(t *testing.T) {
	st := queueState()
	st.Machine.SelfRSS = 34 << 20
	m, _ := loadedWith(t, 100, st)
	out := ansi.Strip(strings.Join(m.machineLines(60, false), "\n"))
	if !strings.Contains(out, "matchblox") || !strings.Contains(out, "34 MiB") {
		t.Fatalf("Machine lacks matchblox serve RSS:\n%s", out)
	}
}

func TestMachineOmitsMatchbloxWhenSelfRSSZero(t *testing.T) {
	st := queueState()
	st.Machine.SelfRSS = 0
	m, _ := loadedWith(t, 100, st)
	out := ansi.Strip(strings.Join(m.machineLines(60, false), "\n"))
	if strings.Contains(out, "matchblox") {
		t.Fatalf("Machine claims matchblox memory with SelfRSS 0:\n%s", out)
	}
}

func TestLocalMachineAddsConsoleRSS(t *testing.T) {
	st := queueState()
	st.Machine.SelfRSS = 34 << 20
	m, _ := loadedWith(t, 100, st)
	defer func() { readConsoleRSS = consoleRSSFromProc }()
	defer func() { thisMachineName = osHostname }()

	thisMachineName = func() string { return "desk" }
	readConsoleRSS = func() uint64 { return 38 << 20 }

	local := ansi.Strip(strings.Join(m.machineAbout(60, false, "desk"), "\n"))
	if !strings.Contains(local, "34 MiB") || !strings.Contains(local, "38 MiB") {
		t.Fatalf("local Machine should name serve and console:\n%s", local)
	}

	remote := ansi.Strip(strings.Join(m.machineAbout(60, false, "other-host"), "\n"))
	if strings.Contains(remote, "38 MiB") {
		t.Fatalf("remote Machine must not include this console RSS:\n%s", remote)
	}
	if !strings.Contains(remote, "34 MiB") {
		t.Fatalf("remote Machine should still show serve RSS:\n%s", remote)
	}
}
