package remote

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// fixture is a recorded ssh -v stderr and its exit code (exits.txt).
func fixture(t *testing.T, name string) (string, int) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "login", name))
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join("testdata", "login", "exits.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for sc := bufio.NewScanner(f); sc.Scan(); {
		if n, code, ok := strings.Cut(sc.Text(), " "); ok && n == name {
			exit, err := strconv.Atoi(code)
			if err != nil {
				t.Fatal(err)
			}
			return string(b), exit
		}
	}
	t.Fatalf("%s has no exit code in exits.txt", name)
	return "", 0
}

func TestClassifyLogin(t *testing.T) {
	for _, c := range []struct {
		file      string
		want      Login
		detail    string
		tailscale bool
	}{
		{"tailscale-ok.stderr", LoginOK, "", true},
		{"tailscale-check.stderr", LoginOK, "", true}, // approved: no link to show any more
		{"tailscale-denied.stderr", LoginTailscaleDenied, `tailnet policy does not permit you to SSH as user "nosuchuser"`, true},
		{"openssh-password.stderr", LoginPassword, "", false},
		{"openssh-hostkey-unknown.stderr", LoginHostKeyUnknown, "", false},
		{"openssh-hostkey-changed.stderr", LoginHostKeyChanged, "", false},
		{"openssh-refused.stderr", LoginUnreachable, "ssh: connect to host 127.0.0.1 port 1: Connection refused", false},
	} {
		stderr, exit := fixture(t, c.file)
		got, detail := ClassifyLogin(stderr, exit)
		if got != c.want || detail != c.detail {
			t.Errorf("%s: %v %q, want %v %q", c.file, got, detail, c.want, c.detail)
		}
		if Tailscale(stderr) != c.tailscale {
			t.Errorf("%s: Tailscale = %v", c.file, !c.tailscale)
		}
	}
}

// publickeyOnly is ssh -v of a key the test sshd (PasswordAuthentication
// no) does not take, exit 255, recorded 2026-10-26 with OpenSSH 9.6 and
// cleaned as the files in testdata/login.
const publickeyOnly = `OpenSSH_9.6p1 Ubuntu-3ubuntu13.20, OpenSSL 3.0.13 30 Jan 2024
debug1: Reading configuration data /home/builder/.ssh/config
debug1: /home/builder/.ssh/config line 1: Applying options for *
debug1: Connecting to localhost [127.0.0.1] port 22.
debug1: fd 3 clearing O_NONBLOCK
debug1: Connection established.
debug1: identity file /home/builder/.ssh/id_ed25519 type 3
debug1: identity file /home/builder/.ssh/id_ed25519-cert type -1
debug1: Local version string SSH-2.0-OpenSSH_9.6p1 Ubuntu-3ubuntu13.20
debug1: Remote protocol version 2.0, remote software version OpenSSH_9.6p1 Ubuntu-3ubuntu13.20
debug1: compat_banner: match: OpenSSH_9.6p1 Ubuntu-3ubuntu13.20 pat OpenSSH* compat 0x04000000
debug1: Authenticating to localhost:22 as 'builder'
debug1: load_hostkeys: fopen /etc/ssh/ssh_known_hosts: No such file or directory
debug1: load_hostkeys: fopen /etc/ssh/ssh_known_hosts2: No such file or directory
debug1: SSH2_MSG_KEXINIT sent
debug1: SSH2_MSG_KEXINIT received
debug1: kex: algorithm: sntrup761x25519-sha512@openssh.com
debug1: kex: host key algorithm: ssh-ed25519
debug1: kex: server->client cipher: chacha20-poly1305@openssh.com MAC: <implicit> compression: none
debug1: kex: client->server cipher: chacha20-poly1305@openssh.com MAC: <implicit> compression: none
debug1: expecting SSH2_MSG_KEX_ECDH_REPLY
debug1: SSH2_MSG_KEX_ECDH_REPLY received
debug1: Server host key: ssh-ed25519 SHA256:fingerprint
debug1: load_hostkeys: fopen /etc/ssh/ssh_known_hosts: No such file or directory
debug1: load_hostkeys: fopen /etc/ssh/ssh_known_hosts2: No such file or directory
debug1: Host 'localhost' is known and matches the ED25519 host key.
debug1: Found key in /home/builder/.ssh/known_hosts:1
debug1: ssh_packet_send2_wrapped: resetting send seqnr 3
debug1: rekey out after 134217728 blocks
debug1: SSH2_MSG_NEWKEYS sent
debug1: Sending SSH2_MSG_EXT_INFO
debug1: expecting SSH2_MSG_NEWKEYS
debug1: ssh_packet_read_poll2: resetting read seqnr 3
debug1: SSH2_MSG_NEWKEYS received
debug1: rekey in after 134217728 blocks
debug1: SSH2_MSG_EXT_INFO received
debug1: kex_ext_info_client_parse: server-sig-algs=<ssh-ed25519,ecdsa-sha2-nistp256,ecdsa-sha2-nistp384,ecdsa-sha2-nistp521,sk-ssh-ed25519@openssh.com,sk-ecdsa-sha2-nistp256@openssh.com,rsa-sha2-512,rsa-sha2-256>
debug1: kex_ext_info_check_ver: publickey-hostbound@openssh.com=<0>
debug1: kex_ext_info_check_ver: ping@openssh.com=<0>
debug1: SSH2_MSG_SERVICE_ACCEPT received
debug1: SSH2_MSG_EXT_INFO received
debug1: kex_ext_info_client_parse: server-sig-algs=<ssh-ed25519,ecdsa-sha2-nistp256,ecdsa-sha2-nistp384,ecdsa-sha2-nistp521,sk-ssh-ed25519@openssh.com,sk-ecdsa-sha2-nistp256@openssh.com,rsa-sha2-512,rsa-sha2-256>
debug1: Authentications that can continue: publickey
debug1: Next authentication method: publickey
debug1: Will attempt key: /home/builder/.ssh/id_ed25519 ED25519 SHA256:fingerprint explicit
debug1: Offering public key: /home/builder/.ssh/id_ed25519 ED25519 SHA256:fingerprint explicit
debug1: Authentications that can continue: publickey
debug1: No more authentication methods to try.
builder@localhost: Permission denied (publickey).
`

// TestKeyOnlyHostIsNotAPasswordLogin: a host that takes keys only cannot
// take the password that installs a key: no ladder there. A refusal needs
// a failed ssh, and the methods in its parentheses decide.
func TestKeyOnlyHostIsNotAPasswordLogin(t *testing.T) {
	if got, _ := ClassifyLogin(publickeyOnly, 255); got != LoginKeyOnly {
		t.Fatalf("publickey only: %v", got)
	}
	for _, c := range []struct {
		stderr string
		exit   int
		want   Login
	}{
		{"me@ws-1: Permission denied (publickey).\r\n", 255, LoginKeyOnly},
		{"me@ws-1: Permission denied (publickey,gssapi-with-mic).\n", 255, LoginKeyOnly},
		{"me@ws-1: Permission denied (publickey,keyboard-interactive).\n", 255, LoginPassword},
		{"me@ws-1: Permission denied (password).\n", 255, LoginPassword},
		{"Permission denied, please try again.\n", 255, LoginPassword},
		{"me@ws-1: Permission denied (publickey,password).\n", 0, LoginOK},
	} {
		if got, _ := ClassifyLogin(c.stderr, c.exit); got != c.want {
			t.Errorf("%q exit %d: %v, want %v", c.stderr, c.exit, got, c.want)
		}
	}
}

// TestKeyCheckArgvSkipsTheMaster: the check after an install must log in
// with the key, not ride the master the install's password opened.
func TestKeyCheckArgvSkipsTheMaster(t *testing.T) {
	want := []string{"ssh", "-v", "-o", "BatchMode=yes", "-o", "ControlMaster=no", "-o", "ControlPath=none", "-o", "ConnectTimeout=10", "--", "ws-1", "true"}
	if got := KeyCheckArgv("ws-1"); !slices.Equal(got, want) {
		t.Fatalf("KeyCheckArgv = %q", got)
	}
}

// TestClassifyLoginShowsOnlyATailscaleURL: the URL comes from the server,
// so only a login.tailscale.com link of plain characters is shown.
func TestClassifyLoginShowsOnlyATailscaleURL(t *testing.T) {
	for _, url := range []string{
		"https://evil.example.com/a/0123",
		"https://login.tailscale.com.evil.example/a/0123",
		"https://login.tailscale.com/a/01\x1b]8;;x",
		"https://login.tailscale.com/a/0123?x=1",
		"http://login.tailscale.com/a/0123",
	} {
		stderr := tailscaleBanner + "# Tailscale SSH requires an additional check.\n# To authenticate, visit: " + url + "\n"
		if got, detail := ClassifyLogin(stderr, 255); got != LoginTailscaleCheck || detail != "" {
			t.Errorf("%q: %v %q", url, got, detail)
		}
	}
}

const tailscaleBanner = "debug1: Remote protocol version 2.0, remote software version Tailscale\n"

// TestTailscaleCheckWaits: a check that ends before the approval (the
// time limit, or esc) gives the URL.
func TestTailscaleCheckWaits(t *testing.T) {
	stderr, _ := fixture(t, "tailscale-check.stderr")
	stderr, _, _ = strings.Cut(stderr, "# Authentication checked")
	if got, detail := ClassifyLogin(stderr, -1); got != LoginTailscaleCheck || detail != "https://login.tailscale.com/a/0123456789abcd" {
		t.Fatalf("%v %q", got, detail)
	}
}

// TestTailscaleNeedsTheBanner: a host that is not Tailscale SSH can print
// Tailscale's lines too: without the version line they name nothing.
func TestTailscaleNeedsTheBanner(t *testing.T) {
	for _, c := range []struct {
		stderr string
		exit   int
	}{
		{"# To authenticate, visit: https://login.tailscale.com/a/0123\n", -1},
		{"tailscale: tailnet policy does not permit you\n", 255},
		{"debug1: Remote protocol version 2.0, remote software version OpenSSH_9.6\ntailscale: no\n", 255},
	} {
		if got, _ := ClassifyLogin(c.stderr, c.exit); got == LoginTailscaleCheck || got == LoginTailscaleDenied {
			t.Errorf("%q: %v", c.stderr, got)
		}
	}
}

// TestClassifyAsTailscale: the open-first runs without -v, so the host's
// kind comes from its last check.
func TestClassifyAsTailscale(t *testing.T) {
	denied := "tailscale: tailnet policy does not permit you to SSH as user \"x\"\nConnection closed by 100.64.0.10 port 22\n"
	if got, detail := ClassifyAs(denied, 255, true); got != LoginTailscaleDenied || detail != `tailnet policy does not permit you to SSH as user "x"` {
		t.Errorf("denied: %v %q", got, detail)
	}
	if got, _ := ClassifyAs(denied, 255, false); got == LoginTailscaleDenied {
		t.Errorf("an OpenSSH host: %v", got)
	}
	check := "# Tailscale SSH requires an additional check.\n# To authenticate, visit: https://login.tailscale.com/a/0123\n"
	if got, detail := ClassifyAs(check, -1, true); got != LoginTailscaleCheck || detail != "https://login.tailscale.com/a/0123" {
		t.Errorf("check: %v %q", got, detail)
	}
}

// TestVisitURL: the check's link line gives its URL, and only a plain
// login.tailscale.com link.
func TestVisitURL(t *testing.T) {
	for _, c := range []struct {
		line, url string
		ok        bool
	}{
		{"# To authenticate, visit: https://login.tailscale.com/a/0123\r", "https://login.tailscale.com/a/0123", true},
		{"# To authenticate, visit: https://evil.example/a/0123", "", true},
		{"# To authenticate, visit: https://login.tailscale.com/a/0\x1b]8;;x", "", true},
		{"# Tailscale SSH requires an additional check.", "", false},
		{"x # To authenticate, visit: https://login.tailscale.com/a/0123", "", false},
	} {
		if url, ok := VisitURL(c.line); url != c.url || ok != c.ok {
			t.Errorf("%q: %q %v", c.line, url, ok)
		}
	}
}

// TestServerSeen: a check through a master that runs has no version line,
// so it does not tell the host's kind.
func TestServerSeen(t *testing.T) {
	for _, f := range []string{"tailscale-ok.stderr", "openssh-password.stderr"} {
		if stderr, _ := fixture(t, f); !ServerSeen(stderr) {
			t.Errorf("%s: no server", f)
		}
	}
	if ServerSeen("debug1: auto-mux: Trying existing master\n") {
		t.Error("a check through the master saw a server")
	}
}

// TestUnreachableSaysTimedOut: a check that ssh did not end by itself
// (exit -1), or that said nothing, timed out.
func TestUnreachableSaysTimedOut(t *testing.T) {
	for _, c := range []struct {
		stderr string
		exit   int
	}{
		{"debug1: Connecting to ws-1 port 22.\n", -1},
		{"", 255},
		{"debug1: Connecting to ws-1 port 22.\n", 255},
	} {
		if got, detail := ClassifyLogin(c.stderr, c.exit); got != LoginUnreachable || detail != "timed out" {
			t.Errorf("%q %d: %v %q", c.stderr, c.exit, got, detail)
		}
	}
}

// TestClassifyLoginCleansTheReason: Tailscale's reason and ssh's last line
// come from the network: no control characters reach the screen.
func TestClassifyLoginCleansTheReason(t *testing.T) {
	_, detail := ClassifyLogin(tailscaleBanner+"tailscale: no\x1b[2J \x07way\n", 255)
	if detail != "no[2J way" {
		t.Errorf("detail %q", detail)
	}
	_, detail = ClassifyLogin("debug1: x\nssh: bad\x1b]0;t\x07 host\n", 255)
	if detail != "ssh: bad]0;t host" {
		t.Errorf("detail %q", detail)
	}
}

func TestMasterOpts(t *testing.T) {
	want := []string{"-o", "ControlMaster=auto", "-o", "ControlPath=/c/ssh/%C", "-o", "ControlPersist=10m"}
	if got := MasterOpts("/c/ssh"); !slices.Equal(got, want) {
		t.Errorf("MasterOpts = %q", got)
	}
	if got := MasterOpts(""); got != nil {
		t.Errorf("no directory, no master: %q", got)
	}
}

func TestCheckArgv(t *testing.T) {
	want := []string{"ssh", "-v", "-o", "BatchMode=yes",
		"-o", "ControlMaster=auto", "-o", "ControlPath=/c/ssh/%C", "-o", "ControlPersist=10m",
		"-o", "ConnectTimeout=10", "--", "ws-1", "true"}
	if got := CheckArgv("ws-1", "/c/ssh"); !slices.Equal(got, want) {
		t.Errorf("CheckArgv =\n %q\nwant\n %q", got, want)
	}
}

// TestMasterArgv: the master's check starts no connection; its open has
// no BatchMode, so ssh asks what it needs (through askpass), and runs
// nothing but true on the host.
func TestMasterArgv(t *testing.T) {
	master := []string{"-o", "ControlMaster=auto", "-o", "ControlPath=/c/ssh/%C", "-o", "ControlPersist=10m"}
	if got, want := MasterUpArgv("ws-1", "/c/ssh"), append(append([]string{"ssh"}, master...), "-O", "check", "--", "ws-1"); !slices.Equal(got, want) {
		t.Errorf("MasterUpArgv =\n %q\nwant\n %q", got, want)
	}
	want := append(append([]string{"ssh"}, master...), "-o", "ConnectTimeout=10", "--", "ws-1", "true")
	got := OpenArgv("ws-1", "/c/ssh")
	if !slices.Equal(got, want) {
		t.Errorf("OpenArgv =\n %q\nwant\n %q", got, want)
	}
	if slices.Contains(got, "BatchMode=yes") {
		t.Error("the open must be able to ask")
	}
}

// shortTemp is a temporary directory with a short path: a socket path
// under it must fit sun_path.
func shortTemp(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "mb") // short: macOS $TMPDIR is too long for sun_path
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestMasterDirMode(t *testing.T) {
	base := filepath.Join(shortTemp(t), "m")
	dir, err := EnsureMasterDir(base)
	if err != nil || dir != filepath.Join(base, "ssh") {
		t.Fatalf("EnsureMasterDir = %q, %v", dir, err)
	}
	for _, d := range []string{base, dir} {
		if fi, err := os.Stat(d); err != nil || fi.Mode().Perm() != 0o700 {
			t.Fatalf("%s: mode %v, %v", d, fi.Mode().Perm(), err)
		}
	}
	if _, err := EnsureMasterDir(base); err != nil {
		t.Fatalf("a second call: %v", err)
	}
	for _, d := range []string{dir, base} {
		if err := os.Chmod(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := EnsureMasterDir(base); err == nil {
			t.Fatalf("a 0755 %s was taken", d)
		}
		if err := os.Chmod(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
}

// TestMasterDirSymlinkRefused: another user can make /tmp/matchblox-<uid>
// a link to a directory of theirs first.
func TestMasterDirSymlinkRefused(t *testing.T) {
	tmp := shortTemp(t)
	if err := os.Mkdir(filepath.Join(tmp, "real"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(tmp, "real"), filepath.Join(tmp, "m")); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureMasterDir(filepath.Join(tmp, "m")); err == nil {
		t.Fatal("a link was taken")
	}
}

// TestMasterPathFits: ssh listens on ControlPath plus 17 bytes (".", 16
// random characters), and a path too long for sun_path ends ssh with 255.
// Such a directory gives no master.
func TestMasterPathFits(t *testing.T) {
	base := "/" + strings.Repeat("b", 69) // 70 bytes; the directory is base/ssh
	dir := base + "/ssh"
	if masterFits(dir, "darwin") || masterFits(dir, "linux") {
		t.Fatalf("%d bytes fit", len(dir))
	}
	if !masterFits("/tmp/matchblox-501/ssh", "darwin") || !masterFits("/tmp/matchblox-1000/ssh", "linux") {
		t.Fatal("the default directory does not fit")
	}
	if !masterFits(strings.Repeat("d", 49), "linux") || masterFits(strings.Repeat("d", 46), "darwin") {
		t.Fatal("the limits are 108 on linux and 104 on darwin, with the NUL")
	}
}

// TestHostTmux: a host's tmux version that is not a plain one is not known.
func TestHostTmux(t *testing.T) {
	long := strings.Repeat("9", 20)
	for v, want := range map[string]string{
		"none": "none", "3.4": "3.4", "3.3a": "3.3a", "next-3.6": "next-3.6", long: long,
		"": "", long + "9": "", "3.1\x1b]0;owned\x07": "", "3.4 x": "", "3.4\n": "",
	} {
		if got := HostTmux(v); got != want {
			t.Errorf("HostTmux(%q) = %q, want %q", v, got, want)
		}
	}
}

func TestTmuxTooOld(t *testing.T) {
	for v, old := range map[string]bool{
		"3.1c": true, "2.9a": true, "3.2": false, "3.2a": false, "3.4": false, "next-3.6": false,
		"": false, "master": false,
	} {
		if TmuxTooOld(v, MinHostTmux) != old {
			t.Errorf("TmuxTooOld(%q) = %v", v, !old)
		}
	}
}

// TestLineFoldsAJumpWithAMaster: the master options go first in the jump's
// ssh, and the line still folds them.
func TestLineFoldsAJumpWithAMaster(t *testing.T) {
	j := Jump{Host: "ws-1", Pane: "%1", Prefix: "C-b", ID: "0a1b2c3d"}
	got := Line(JumpSSH("ws-1", j, MasterOpts("/c/ssh")))
	if want := "ssh … ws-1 " + Quote(j.Remote()); got != want {
		t.Errorf("Line =\n %s\nwant\n %s", got, want)
	}
}
