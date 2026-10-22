package gitscan

import (
	"strings"
	"testing"
)

const mountinfoFixture = `22 1 0:21 / / rw,relatime - btrfs /dev/sda1 rw
23 22 0:21 /@repos /home/u/repos rw,relatime shared:2 - btrfs /dev/sda1 rw
24 22 0:40 / /home/u/net rw,relatime shared:3 - nfs srv:/x rw
25 22 0:41 / /home/u/my\040disk rw,relatime - ext4 /dev/sdb1 rw
26 22 0:42 / /home/u/auto rw,relatime - autofs systemd-1 rw
27 26 0:43 / /home/u/auto rw,relatime - nfs srv:/y rw
`

func TestParseMountinfo(t *testing.T) {
	m := parseMountinfo(strings.NewReader(mountinfoFixture))
	if fs, ok := m.network("/home/u/net"); fs != "nfs" || !ok {
		t.Errorf("net = %q, %v", fs, ok)
	}
	if _, ok := m.network("/home/u/repos"); ok {
		t.Error("btrfs subvolume reported as network")
	}
	if m["/home/u/my disk"] != "ext4" {
		t.Errorf("space path not unescaped: %v", m)
	}
	if fs, ok := m.network("/home/u/auto"); fs != "nfs" || !ok {
		t.Errorf("stacked = %q, %v", fs, ok)
	}
}

func TestReadMountsRuns(t *testing.T) {
	if readMounts() == nil {
		t.Error("readMounts returned nil")
	}
}
