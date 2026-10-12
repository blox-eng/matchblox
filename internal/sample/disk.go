package sample

import (
	"os"
	"strconv"
	"strings"
)

// Disk is the use of one file system: the one that holds the home
// directory, and / when it is another.
type Disk struct {
	Path  string `json:"path"`
	Total uint64 `json:"total"`
	Used  uint64 `json:"used"`
	Avail uint64 `json:"avail"` // what a user may still write; root's reserve is neither
}

// Pct is the use as df counts it: used of what a user can have.
func (d Disk) Pct() float64 {
	if d.Used+d.Avail == 0 {
		return 0
	}
	return float64(d.Used) / float64(d.Used+d.Avail) * 100
}

// DiskFile reads the disks of a fixture tree: one per line, the path and
// its total, used and available bytes.
func DiskFile(path string) func() []Disk {
	return func() []Disk {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var out []Disk
		for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			f := strings.Fields(line)
			if len(f) != 4 {
				continue
			}
			n := func(s string) uint64 { v, _ := strconv.ParseUint(s, 10, 64); return v }
			out = append(out, Disk{Path: f[0], Total: n(f[1]), Used: n(f[2]), Avail: n(f[3])})
		}
		return out
	}
}

// Disks is the file system of each path once, in order: two paths on the
// same file system are one disk.
func Disks(paths ...string) func() []Disk {
	return func() []Disk {
		var out []Disk
		seen := map[uint64]bool{}
		for _, p := range paths {
			d, dev, ok := statfs(p)
			if !ok || seen[dev] {
				continue
			}
			seen[dev] = true
			out = append(out, d)
		}
		return out
	}
}
