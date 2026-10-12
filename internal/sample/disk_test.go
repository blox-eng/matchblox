package sample

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDiskFileReadsAFixture(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disks.txt")
	if err := os.WriteFile(path, []byte("/home/user 100 71 29\n/ 10 9 1\nbroken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ds := DiskFile(path)()
	if len(ds) != 2 || ds[0] != (Disk{Path: "/home/user", Total: 100, Used: 71, Avail: 29}) || ds[1].Pct() != 90 {
		t.Fatalf("disks %+v", ds)
	}
}

// TestDisksReadsEachFileSystemOnce: two paths on one file system are one
// disk, with its figures.
func TestDisksReadsEachFileSystemOnce(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("statfs is read on Linux and macOS")
	}
	dir := t.TempDir()
	ds := Disks(dir, filepath.Join(dir, "."), "/no/such/path")()
	if len(ds) != 1 || ds[0].Path != dir || ds[0].Total == 0 || ds[0].Used > ds[0].Total {
		t.Fatalf("disks %+v", ds)
	}
}
