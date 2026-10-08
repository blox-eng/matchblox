package setup

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
func WriteConfig(path string, cores int, memTotal uint64, agents []string) (bool, error) {
	a := Thresholds(cores, memTotal)
	if len(agents) == 0 {
		agents = []string{"claude"}
	}
	quoted := make([]string, len(agents))
	for i, name := range agents {
		quoted[i] = strconv.Quote(name)
	}
	body := fmt.Sprintf(`# matchblox: the goals of this machine. Change them here.
# Made from this machine: %d cores, %.0f GB of memory.

# The coding agents found on this machine. A process with one of these
# command names in a tmux pane is a session. Add any other agent by name.
agents = [%s]

[alerts]
# The load average over this for 2 minutes: work waits for a CPU.
load1_over = %.0f
# Less free memory than this, in GB.
mem_available_under_gb = %.0f
`, cores, float64(memTotal)/(1<<30), strings.Join(quoted, ", "), a.Load1Over, a.MemAvailableUnderGB)
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

// KnownAgents are the coding agents the first run looks for, by the
// command that starts each one.
var KnownAgents = []string{"claude", "codex", "opencode", "gemini", "cursor-agent", "aider", "goose", "crush", "amp", "qwen"}

// FindAgents lists the known agents that are installed (lookPath is
// exec.LookPath). With none, it is Claude Code, the default.
func FindAgents(lookPath func(string) (string, error)) []string {
	var found []string
	for _, name := range KnownAgents {
		if _, err := lookPath(name); err == nil {
			found = append(found, name)
		}
	}
	if len(found) == 0 {
		return []string{"claude"}
	}
	return found
}
