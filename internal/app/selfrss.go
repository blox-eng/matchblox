package app

import (
	"os"
	"strconv"

	"github.com/blox-eng/matchblox/internal/procfs"
)

// readConsoleRSS is this console process's RSS in bytes. Tests replace it.
var readConsoleRSS = consoleRSSFromProc

// thisMachineName is the hostname of the machine this console runs on.
var thisMachineName = osHostname

func osHostname() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}

func consoleRSSFromProc() uint64 {
	p, ok := procfs.FS{Root: "/proc"}.Proc(os.Getpid())
	if !ok {
		return 0
	}
	return p.RSS
}

func rssMiB(b uint64) string {
	return strconv.FormatUint(b/(1<<20), 10) + " MiB"
}
