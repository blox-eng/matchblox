package app

import (
	"bufio"
	"bytes"
	"os"
	"strconv"
	"strings"
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

// consoleRSSFromProc reads this process's VmRSS. The console must not import
// procfs (arch_test); /proc/self/status is enough for one figure.
func consoleRSSFromProc() uint64 {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return kb * 1024
	}
	return 0
}

func rssMiB(b uint64) string {
	return strconv.FormatUint(b/(1<<20), 10) + " MiB"
}
