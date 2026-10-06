package sample

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// A wedged daemon must not freeze the only sample loop of the service.
func TestHungCommandsTimeOut(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("no sleep")
	}
	dir := t.TempDir()
	for _, name := range []string{"docker", "nvidia-smi", "tmux"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexec "+sleep+" 30\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	defer func(d time.Duration) { cmdTimeout = d }(cmdTimeout)
	cmdTimeout = 100 * time.Millisecond
	for name, f := range map[string]func() ([]byte, error){"docker": DockerPS, "nvidia-smi": NvidiaSMI, "tmux": TmuxPanes} {
		start := time.Now()
		_, _ = f()
		if d := time.Since(start); d > 2*time.Second {
			t.Fatalf("%s blocked the sample for %v", name, d)
		}
	}
}
