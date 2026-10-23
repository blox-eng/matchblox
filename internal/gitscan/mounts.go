package gitscan

import (
	"bufio"
	"io"
	"strings"
)

// mountTable maps a mount point to its filesystem type. The last entry
// for a path wins, as with stacked mounts.
type mountTable map[string]string

// networkFS are the filesystems a scan must not walk into: a dead server
// can hang any call on them.
var networkFS = map[string]bool{
	"nfs": true, "nfs4": true, "cifs": true, "smb3": true, "smbfs": true,
	"afpfs": true, "webdav": true, "davfs": true, "fuse.sshfs": true,
	"9p": true, "ceph": true, "glusterfs": true, "lustre": true, "autofs": true,
	"fuse.rclone": true, "fuse.s3fs": true, "fuse.gcsfuse": true, "fuse.goofys": true,
	"fuse.juicefs": true, "fuse.davfs2": true,
}

// network reports the type of path when it is a network mount point.
func (t mountTable) network(path string) (string, bool) {
	fs, ok := t[path]
	if !ok || !networkFS[fs] {
		return "", false
	}
	return fs, true
}

var mountUnescape = strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)

// parseMountinfo reads the Linux /proc/self/mountinfo format.
func parseMountinfo(r io.Reader) mountTable {
	t := mountTable{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 5 {
			continue
		}
		for i := 5; i+1 < len(f); i++ {
			if f[i] == "-" {
				t[mountUnescape.Replace(f[4])] = f[i+1]
				break
			}
		}
	}
	return t
}
