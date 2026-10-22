package setup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMain keeps these tests off this machine's tailscale and sshd: a test
// that wants a probe sets its own.
func TestMain(m *testing.M) {
	pairProbe = pairProbeFns{LookPath: func(string) (string, error) { return "", errors.New("not found") }}
	os.Exit(m.Run())
}

func pairTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pair", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// useProbe replaces the pair probe for one test. A nil body means that
// tool's command fails.
func useProbe(t *testing.T, tools map[string]bool, prefs, status, sshd []byte) {
	t.Helper()
	old := pairProbe
	t.Cleanup(func() { pairProbe = old })
	out := func(b []byte) func(string) ([]byte, error) {
		return func(string) ([]byte, error) {
			if b == nil {
				return nil, errors.New("command failed")
			}
			return b, nil
		}
	}
	pairProbe = pairProbeFns{
		LookPath: func(name string) (string, error) {
			if tools[name] {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
		TailscalePrefs:  out(prefs),
		TailscaleStatus: out(status),
		SSHDConfig:      out(sshd),
		Username:        func() string { return "alice" },
	}
}

func tailscaleProbe(t *testing.T, prefsFile string) {
	t.Helper()
	useProbe(t, map[string]bool{"tailscale": true}, pairTestdata(t, prefsFile), pairTestdata(t, "tailscale-status-self.json"), nil)
}

func wantTailscaleErr(t *testing.T, err error) {
	t.Helper()
	const want = "Tailscale SSH answers on this address. Use matchblox pair --address with a host OpenSSH serves, or run tailscale set --ssh=false, then matchblox pair again"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
	if !strings.Contains(err.Error(), "tailscale set --ssh=false") || !strings.Contains(err.Error(), "--address") {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckPairPathTailscaleSSHOnMagicDNS(t *testing.T) {
	tailscaleProbe(t, "tailscale-prefs-ssh-on.json")
	for _, addr := range []string{"ws-1.tail1234.ts.net", "ws-1.tail1234.ts.net.", "WS-1.tail1234.ts.net"} {
		wantTailscaleErr(t, CheckPairPath(t.TempDir(), addr, 22))
	}
}

func TestCheckPairPathTailscaleSSHAllowsOtherAddress(t *testing.T) {
	tailscaleProbe(t, "tailscale-prefs-ssh-on.json")
	if err := CheckPairPath(t.TempDir(), "192.0.2.10", 22); err != nil {
		t.Fatal(err)
	}
}

func TestCheckPairPathTailscaleSSHAllowsNon22(t *testing.T) {
	tailscaleProbe(t, "tailscale-prefs-ssh-on.json")
	if err := CheckPairPath(t.TempDir(), "ws-1.tail1234.ts.net", 2222); err != nil {
		t.Fatal(err)
	}
}

func TestCheckPairPathTailscaleSSHOnTailscaleIP(t *testing.T) {
	tailscaleProbe(t, "tailscale-prefs-ssh-on.json")
	wantTailscaleErr(t, CheckPairPath(t.TempDir(), "100.101.102.103", 22))
}

func TestCheckPairPathTailscaleSSHOff(t *testing.T) {
	tailscaleProbe(t, "tailscale-prefs-ssh-off.json")
	if err := CheckPairPath(t.TempDir(), "ws-1.tail1234.ts.net", 22); err != nil {
		t.Fatal(err)
	}
}

func TestCheckPairPathNoTailscale(t *testing.T) {
	useProbe(t, nil, pairTestdata(t, "tailscale-prefs-ssh-on.json"), pairTestdata(t, "tailscale-status-self.json"), nil)
	if err := CheckPairPath(t.TempDir(), "ws-1.tail1234.ts.net", 22); err != nil {
		t.Fatal(err)
	}
}

func TestCheckPairPathTailscaleProbeErrorDoesNotBlock(t *testing.T) {
	useProbe(t, map[string]bool{"tailscale": true}, nil, nil, nil)
	if err := CheckPairPath(t.TempDir(), "ws-1.tail1234.ts.net", 22); err != nil {
		t.Fatal(err)
	}
	useProbe(t, map[string]bool{"tailscale": true}, pairTestdata(t, "tailscale-prefs-ssh-on.json"), []byte("not json"), nil)
	if err := CheckPairPath(t.TempDir(), "ws-1.tail1234.ts.net", 22); err != nil {
		t.Fatal(err)
	}
}

func sshdProbe(t *testing.T, sshd []byte) {
	t.Helper()
	useProbe(t, map[string]bool{"sshd": true}, nil, nil, sshd)
}

func TestCheckPairPathAuthorizedKeysOther(t *testing.T) {
	sshdProbe(t, pairTestdata(t, "sshd-T-other.txt"))
	home := t.TempDir()
	err := CheckPairPath(home, "ws-1", 22)
	pairPath := filepath.Join(home, ".ssh", "authorized_keys")
	want := "sshd reads AuthorizedKeysFile /etc/ssh/keys/%u, not " + pairPath + " where matchblox pair writes the one-time key. Point AuthorizedKeysFile at " + pairPath + " (or include it), then matchblox pair again"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

func TestCheckPairPathAuthorizedKeysDefault(t *testing.T) {
	sshdProbe(t, pairTestdata(t, "sshd-T-default.txt"))
	if err := CheckPairPath(t.TempDir(), "ws-1", 22); err != nil {
		t.Fatal(err)
	}
}

func TestCheckPairPathAuthorizedKeysPercentU(t *testing.T) {
	// /etc/ssh/keys/%u becomes /etc/ssh/keys/alice: not the pair path.
	sshdProbe(t, pairTestdata(t, "sshd-T-other.txt"))
	if err := CheckPairPath(t.TempDir(), "ws-1", 22); err == nil {
		t.Fatal("a %u path outside home passed")
	}
	// A list that expands to the pair path passes: %u, %h and %%.
	parent := t.TempDir()
	home := filepath.Join(parent, "alice")
	for _, line := range []string{
		"authorizedkeysfile /etc/ssh/keys/%u " + parent + "/%u/.ssh/authorized_keys",
		"authorizedkeysfile %h/.ssh/authorized_keys",
		"authorizedkeysfile .ssh/../.ssh/authorized_keys",
	} {
		sshdProbe(t, []byte("port 22\n"+line+"\n"))
		if err := CheckPairPath(home, "ws-1", 22); err != nil {
			t.Fatalf("%q: %v", line, err)
		}
	}
	sshdProbe(t, []byte("authorizedkeysfile %%h/.ssh/authorized_keys\n"))
	if err := CheckPairPath(home, "ws-1", 22); err == nil {
		t.Fatal("%%h passed as %h")
	}
}

func TestCheckPairPathSSHDUnavailable(t *testing.T) {
	// Not installed.
	useProbe(t, nil, nil, nil, pairTestdata(t, "sshd-T-other.txt"))
	if err := CheckPairPath(t.TempDir(), "ws-1", 22); err != nil {
		t.Fatal(err)
	}
	// Installed, but sshd -T fails (it needs root on most hosts).
	sshdProbe(t, nil)
	if err := CheckPairPath(t.TempDir(), "ws-1", 22); err != nil {
		t.Fatal(err)
	}
	// Installed, but the output names no AuthorizedKeysFile.
	sshdProbe(t, []byte("port 22\n"))
	if err := CheckPairPath(t.TempDir(), "ws-1", 22); err != nil {
		t.Fatal(err)
	}
}

func TestNewPairOfferRefusesBeforeWritingKey(t *testing.T) {
	tailscaleProbe(t, "tailscale-prefs-ssh-on.json")
	home := t.TempDir()
	hostKey := filepath.Join(t.TempDir(), "host.pub")
	if err := os.WriteFile(hostKey, []byte(testHostKey), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := NewPairOffer(home, "/usr/local/bin/matchblox", "ws-1.tail1234.ts.net", 22, "ws-1", hostKey, time.Now().Add(time.Minute))
	wantTailscaleErr(t, err)
	b, rerr := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	if rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
		t.Fatal(rerr)
	}
	if strings.Contains(string(b), "gate pair") {
		t.Fatalf("a pair line was written:\n%s", b)
	}
}
