package host

import (
	"errors"
	"time"

	"github.com/blox-eng/matchblox/internal/procfs"
)

// Host on iOS reads nothing: the app's sandbox shows no other processes or
// counters, and the demo reads its own fixture, never this host.
type Host struct{}

var _ procfs.Host = Host{}

var errIOS = errors.New("the machine is not read on iOS")

func (Host) Procs() (map[int]procfs.Proc, error) { return nil, errIOS }
func (Host) Proc(int) (procfs.Proc, bool)        { return procfs.Proc{}, false }
func (Host) Environ(int, string) (string, bool)  { return "", false }
func (Host) Cwd(int) string                      { return "" }
func (Host) Cmdline(int) string                  { return "" }
func (Host) Argv(int) []string                   { return nil }
func (Host) OpenFiles(int) ([]string, bool)      { return nil, false }
func (Host) Cgroup(int) string                   { return "" }
func (Host) UID(int) (int, bool)                 { return 0, false }
func (Host) Uptime() float64                     { return float64(time.Now().UnixMilli()) / 1000 }
func (Host) Stat() (procfs.CPUTimes, []procfs.CPUTimes, error) {
	return procfs.CPUTimes{}, nil, errIOS
}
func (Host) Loadavg() ([3]float64, error)         { return [3]float64{}, errIOS }
func (Host) Meminfo() (procfs.Mem, error)         { return procfs.Mem{}, errIOS }
func (Host) Pressure() procfs.PSI                 { return procfs.PSI{} }
func (Host) NetBytes() (rx, tx uint64, err error) { return 0, 0, errIOS }
func (Host) DiskBytes() (read, written uint64, err error) {
	return 0, 0, errIOS
}
