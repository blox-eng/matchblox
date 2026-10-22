package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	Username        func() string
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
	SSHDConfig: func(path string) ([]byte, error) {
		return pairRun(path, "-T")
	},
	Username: func() string {
		if u, err := user.Current(); err == nil {
			return u.Username
		}
		return ""
	},
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

const errTailscaleSSH = "this address is answered by Tailscale SSH. Use matchblox pair --address with a host OpenSSH serves, or run tailscale set --ssh=false, then matchblox pair again"

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
		return errors.New(errTailscaleSSH)
	}
	return nil
}

// addressIsSelfTailscale: address is this host's MagicDNS name (with or
// without the trailing dot, any case) or one of its Tailscale IPs.
func addressIsSelfTailscale(address, dnsName string, ips []string) bool {
	if name := strings.TrimSuffix(dnsName, "."); name != "" && strings.EqualFold(strings.TrimSuffix(address, "."), name) {
		return true
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
	if p.LookPath == nil || p.SSHDConfig == nil {
		return nil
	}
	path, err := p.LookPath("sshd")
	if err != nil {
		return nil
	}
	b, err := p.SSHDConfig(path)
	if err != nil {
		return nil
	}
	listed := ""
	for _, line := range strings.Split(string(b), "\n") {
		if name, rest, ok := strings.Cut(strings.TrimSpace(line), " "); ok && strings.EqualFold(name, "authorizedkeysfile") {
			listed = strings.TrimSpace(rest)
			break
		}
	}
	if listed == "" {
		return nil
	}
	username := ""
	if p.Username != nil {
		username = p.Username()
	}
	want := filepath.Clean(filepath.Join(home, ".ssh", "authorized_keys"))
	for _, f := range strings.Fields(listed) {
		if expandKeysPath(f, home, username) == want {
			return nil
		}
	}
	return fmt.Errorf("sshd reads AuthorizedKeysFile %s, not %s where matchblox pair writes the one-time key. Point AuthorizedKeysFile at %s (or include it), then matchblox pair again", listed, want, want)
}

// expandKeysPath is one AuthorizedKeysFile entry as sshd reads it: %h is
// the home, %u the user, %% a percent sign, and a relative path is under
// the home.
func expandKeysPath(entry, home, username string) string {
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
		case '%':
			sb.WriteByte('%')
		default:
			sb.WriteByte('%')
			sb.WriteByte(entry[i])
		}
	}
	path := sb.String()
	if !filepath.IsAbs(path) {
		path = filepath.Join(home, path)
	}
	return filepath.Clean(path)
}
