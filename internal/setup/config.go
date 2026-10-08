package setup

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"

	"github.com/blox-eng/matchblox/internal/config"
)

// Thresholds are the alert goals of a machine, from its size: the load
// average over the core count means work waits for a CPU, and less than a
// tenth of the memory free is close to swapping.
func Thresholds(cores int, memTotal uint64) config.Alerts {
	gb := float64(memTotal) / (1 << 30)
	return config.Alerts{
		Load1Over:           float64(cores),
		MemAvailableUnderGB: max(1, math.Floor(gb/10)),
	}
}

// WriteConfig writes the config file with the machine's thresholds, only
// when there is none. It reports whether it wrote it.
func WriteConfig(path string, cores int, memTotal uint64) (bool, error) {
	a := Thresholds(cores, memTotal)
	body := fmt.Sprintf(`# matchblox: the goals of this machine. Change them here.
# Made from this machine: %d cores, %.0f GB of memory.

[alerts]
# The load average over this for 2 minutes: work waits for a CPU.
load1_over = %.0f
# Less free memory than this, in GB.
mem_available_under_gb = %.0f
`, cores, float64(memTotal)/(1<<30), a.Load1Over, a.MemAvailableUnderGB)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // our own config path
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := f.WriteString(body); err != nil {
		_ = f.Close()
		return false, err
	}
	return true, f.Close()
}
