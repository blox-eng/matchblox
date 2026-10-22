package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

// pairProbeFns is how CheckPairPath looks at this host: tests replace it.
// A tool the probe cannot run is a check that is skipped, never a refusal.
type pairProbeFns struct {
	LookPath        func(name string) (string, error)
	TailscalePrefs  func(path string) ([]byte, error)
	TailscaleStatus func(path string) ([]byte, error)
	SSHDConfig      func(path string) ([]byte, error)
	SSHDFile        func() ([]byte, error) // sshd_config when sshd -T cannot run
	Username        func() string
	HomeDir         func() string // passwd home for %h
	Uid             func() string // numeric uid for %U
}

var pairProbe = defaultPairProbe

// PairProbe is pairProbeFns for tests outside this package.
type PairProbe = pairProbeFns

// SetPairProbe replaces how CheckPairPath looks at this host and returns the
// one it replaced. It is for tests: a caller outside this package would
// otherwise run this machine's tailscale and sshd.
func SetPairProbe(p PairProbe) (old PairProbe) {
	old, pairProbe = pairProbe, p
	return old
}

var defaultPairProbe = pairProbeFns{
	LookPath: pairLookPath,
	TailscalePrefs: func(path string) ([]byte, error) {
		return pairRun(path, "debug", "prefs")
	},
	TailscaleStatus: func(path string) ([]byte, error) {
		return pairRun(path, "status", "--json")
	},
	SSHDConfig: defaultSSHDConfig,
	SSHDFile: func() ([]byte, error) {
		return os.ReadFile("/etc/ssh/sshd_config")
	},
	Username: func() string {
		if u, err := user.Current(); err == nil {
			return u.Username
		}
		return ""
	},
	HomeDir: func() string {
		if u, err := user.Current(); err == nil && u.HomeDir != "" {
			return u.HomeDir
		}
		return ""
	},
	Uid: func() string {
		if u, err := user.Current(); err == nil {
			return u.Uid
		}
		return ""
	},
}

// defaultSSHDConfig runs sshd -T with a -C connection so Match blocks for
// this user are applied when the daemon can run -T.
func defaultSSHDConfig(path string) ([]byte, error) {
	u, h := "nobody", "localhost"
	if cu, err := user.Current(); err == nil && cu.Username != "" {
		u = cu.Username
	}
	if name, err := os.Hostname(); err == nil && name != "" {
		h = name
	}
	return pairRun(path, "-T", "-C", "user="+u+",host="+h+",addr=127.0.0.1")
}

// pairLookPath finds name on PATH. sshd lives in sbin, which a user's PATH
// often leaves out.
func pairLookPath(name string) (string, error) {
	p, err := exec.LookPath(name)
	if err == nil || name != "sshd" {
		return p, err
	}
	for _, dir := range []string{"/usr/sbin", "/usr/local/sbin", "/sbin"} {
		if c := filepath.Join(dir, name); isExecutable(c) {
			return c, nil
		}
	}
	return "", err
}

func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&0o111 != 0
}

func pairRun(path string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, path, args...).Output()
}

// CheckPairPath refuses a pair that cannot work, before a key is written:
// Tailscale SSH answers port 22 on this host's Tailscale name and addresses
// instead of OpenSSH, and sshd may read keys from a file other than
// ~/.ssh/authorized_keys. A check whose tool cannot run is skipped.
func CheckPairPath(home, address string, port int) error {
	if err := checkTailscaleSSH(address, port); err != nil {
		return err
	}
	return checkAuthorizedKeysFile(home)
}

// ErrTailscaleSSH is the CLI refusal when Tailscale SSH would answer the
// advertised address. The console formats its own line from this.
var ErrTailscaleSSH = errors.New(errTailscaleSSH)

const errTailscaleSSH = "this address is answered by Tailscale SSH. Use matchblox pair --address with a host OpenSSH serves, or run tailscale set --ssh=false, then matchblox pair again"

// ErrTailscaleSSHConsole is the console refusal: Enter cannot take --address.
const ErrTailscaleSSHConsole = "this address is answered by Tailscale SSH. Run tailscale set --ssh=false, then press Enter. Or quit and run matchblox pair --address with a host OpenSSH serves"

func checkTailscaleSSH(address string, port int) error {
	p := pairProbe
	if port != 22 || p.LookPath == nil {
		return nil
	}
	path, err := p.LookPath("tailscale")
	if err != nil || p.TailscalePrefs == nil || p.TailscaleStatus == nil {
		return nil
	}
	b, err := p.TailscalePrefs(path)
	if err != nil {
		return nil
	}
	var prefs struct{ RunSSH bool }
	if json.Unmarshal(b, &prefs) != nil || !prefs.RunSSH {
		return nil
	}
	b, err = p.TailscaleStatus(path)
	if err != nil {
		return nil
	}
	var st struct {
		Self struct {
			DNSName      string
			TailscaleIPs []string
		}
	}
	if json.Unmarshal(b, &st) != nil {
		return nil
	}
	if addressIsSelfTailscale(address, st.Self.DNSName, st.Self.TailscaleIPs) {
		return ErrTailscaleSSH
	}
	return nil
}

// addressIsSelfTailscale: address is this host's MagicDNS name (FQDN or
// short label), or one of its Tailscale IPs (any spelling net.ParseIP knows).
func addressIsSelfTailscale(address, dnsName string, ips []string) bool {
	address = strings.TrimSpace(address)
	address = strings.Trim(address, "[]")
	address = strings.TrimSuffix(address, ".")
	if address == "" {
		return false
	}
	name := strings.TrimSuffix(dnsName, ".")
	if name != "" {
		if strings.EqualFold(address, name) {
			return true
		}
		if short, _, ok := strings.Cut(name, "."); ok && short != "" && strings.EqualFold(address, short) {
			return true
		}
	}
	if a := net.ParseIP(address); a != nil {
		for _, ip := range ips {
			if b := net.ParseIP(ip); b != nil && a.Equal(b) {
				return true
			}
		}
		return false
	}
	for _, ip := range ips {
		if ip == address {
			return true
		}
	}
	return false
}

func checkAuthorizedKeysFile(home string) error {
	p := pairProbe
	listed := ""
	if p.LookPath != nil && p.SSHDConfig != nil {
		if path, err := p.LookPath("sshd"); err == nil {
			if b, err := p.SSHDConfig(path); err == nil {
				listed = authorizedKeysFileListed(string(b))
			}
		}
	}
	// sshd -T usually needs root (host keys). Fall back to the world-readable
	// sshd_config for the global AuthorizedKeysFile outside Match blocks.
	if listed == "" && p.SSHDFile != nil {
		if b, err := p.SSHDFile(); err == nil {
			listed = authorizedKeysFileFromConfig(string(b))
		}
	}
	if listed == "" {
		return nil
	}
	username, uid := "", ""
	if p.Username != nil {
		username = p.Username()
	}
	if p.Uid != nil {
		uid = p.Uid()
	}
	// Expand %h with the passwd home when we have it: that is what sshd uses.
	expandHome := home
	if p.HomeDir != nil {
		if h := p.HomeDir(); h != "" {
			expandHome = h
		}
	}
	want := filepath.Clean(filepath.Join(home, ".ssh", "authorized_keys"))
	for _, f := range strings.Fields(listed) {
		for _, h := range []string{expandHome, home} {
			got, ok := expandKeysPath(f, h, username, uid)
			if !ok {
				// A token we do not expand: skip rather than refuse wrongly.
				return nil
			}
			if got == want {
				return nil
			}
		}
	}
	return fmt.Errorf("sshd reads AuthorizedKeysFile %s, not %%h/.ssh/authorized_keys where matchblox pair writes the one-time key. Set AuthorizedKeysFile to %%h/.ssh/authorized_keys (or include it), reload sshd, then matchblox pair again", listed)
}

// authorizedKeysFileListed is the value from sshd -T output.
func authorizedKeysFileListed(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if name, rest, ok := strings.Cut(strings.TrimSpace(line), " "); ok && strings.EqualFold(name, "authorizedkeysfile") {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// authorizedKeysFileFromConfig is the last global AuthorizedKeysFile in an
// sshd_config text. Lines under a Match block are skipped.
func authorizedKeysFileFromConfig(text string) string {
	listed := ""
	inMatch := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		key := strings.ToLower(fields[0])
		if key == "match" {
			inMatch = true
			continue
		}
		if inMatch {
			continue
		}
		if key == "authorizedkeysfile" && len(fields) > 1 {
			listed = strings.Join(fields[1:], " ")
		}
	}
	return listed
}

// expandKeysPath is one AuthorizedKeysFile entry as sshd reads it: %h is
// the home, %u the user, %U the uid, %% a percent sign, and a relative
// path is under the home. ok is false when the entry uses a token this
// code does not expand.
func expandKeysPath(entry, home, username, uid string) (path string, ok bool) {
	var sb strings.Builder
	for i := 0; i < len(entry); i++ {
		if entry[i] != '%' || i+1 == len(entry) {
			sb.WriteByte(entry[i])
			continue
		}
		i++
		switch entry[i] {
		case 'h':
			sb.WriteString(home)
		case 'u':
			sb.WriteString(username)
		case 'U':
			sb.WriteString(uid)
		case '%':
			sb.WriteByte('%')
		default:
			return "", false
		}
	}
	path = sb.String()
	if !filepath.IsAbs(path) {
		path = filepath.Join(home, path)
	}
	return filepath.Clean(path), true
}
