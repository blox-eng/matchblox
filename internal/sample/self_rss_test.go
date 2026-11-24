package sample

import (
	"bufio"
	"bytes"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/procfs"
)

func TestSampleMachineSetsSelfRSS(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("SelfRSS reads /proc")
	}
	s := &Sampler{FS: procfs.FS{Root: "/proc"}, Rules: DefaultRules}
	snap := s.Sample()
	if snap.Machine.SelfRSS == 0 {
		t.Fatal("SelfRSS: got 0, want this process's RSS")
	}
	want := selfVmRSS(t)
	lo, hi := want/2, want+want/2
	if snap.Machine.SelfRSS < lo || snap.Machine.SelfRSS > hi {
		t.Fatalf("SelfRSS %d not within ±50%% of VmRSS %d", snap.Machine.SelfRSS, want)
	}
}

func TestSampleMachineSelfRSSMissingStat(t *testing.T) {
	root := copyFixture(t)
	s := newFixtureSampler(root)
	snap := s.Sample()
	if snap.Machine.SelfRSS != 0 {
		t.Fatalf("SelfRSS: got %d, want 0 when this pid has no stat under the fixture FS", snap.Machine.SelfRSS)
	}
}

func selfVmRSS(t *testing.T) uint64 {
	t.Helper()
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			t.Fatalf("VmRSS line: %q", line)
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		return kb * 1024
	}
	t.Fatal("VmRSS missing from /proc/self/status")
	return 0
}
