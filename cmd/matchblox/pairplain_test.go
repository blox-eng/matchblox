package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/pair"
	"github.com/blox-eng/matchblox/internal/setup"
)

func TestPairCmdPlainOutputIsV2(t *testing.T) {
	f := loadPairFixture(t)
	hostKey := filepath.Join(t.TempDir(), "host.pub")
	if err := os.WriteFile(hostKey, []byte(f.HostKeyPub+" root@ws-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := pairSettings
	pairSettings = pairEnv{home: t.TempDir(), exe: testExe, hostKeyPath: hostKey, poll: time.Millisecond, limit: 5 * time.Minute}
	t.Cleanup(func() { pairSettings = old })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if err := pairCmd(ctx, []string{"--address", "ws-1.tail1234.ts.net", "--name", "ws-1"}, &out, time.Now); err != nil {
		t.Fatal(err)
	}
	var link string
	qr := 0
	for i, l := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(l, "matchblox://pair?p=") {
			link, qr = l, i
		}
	}
	p, err := pair.Parse(link)
	if err != nil || p.V != 2 {
		t.Fatalf("link %q: V %d, err %v", link, p.V, err)
	}
	if qr > 29 {
		t.Fatalf("the QR is %d lines, want at most 29", qr)
	}
}

func TestPairPollIsTenthOfASecond(t *testing.T) {
	if pairPollInterval != 100*time.Millisecond {
		t.Fatalf("pairPollInterval = %v", pairPollInterval)
	}
}

// The standalone page has no Enter to retry: its offer error keeps the
// command's own words.
func TestStandalonePairHostKeepsTheCommandError(t *testing.T) {
	h := pairHost(t.TempDir(), testExe, filepath.Join(t.TempDir(), "none.pub"), func() string { return "ws-1" }, 22, "ws-1", false)
	_, _, err := h.Offer(time.Now().Add(time.Minute))
	if err == nil || strings.Contains(err.Error(), "press Enter") {
		t.Fatalf("err = %v", err)
	}
	h = pairHost(t.TempDir(), testExe, filepath.Join(t.TempDir(), "none.pub"), func() string { return "ws-1" }, 22, "ws-1", true)
	if _, _, err = h.Offer(time.Now().Add(time.Minute)); err == nil || !strings.Contains(err.Error(), "press Enter") {
		t.Fatalf("console err = %v", err)
	}
}

const wantTailscaleSSH = "this address is answered by Tailscale SSH. Use matchblox pair --address with a host OpenSSH serves, or run tailscale set --ssh=false, then matchblox pair again"

// useTailscaleSSHProbe makes this host look like one where Tailscale SSH
// answers port 22 on ws-1.tail1234.ts.net, whatever this machine runs.
func useTailscaleSSHProbe(t *testing.T) {
	t.Helper()
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pair", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	prefs, status := read("tailscale-prefs-ssh-on.json"), read("tailscale-status-self.json")
	old := setup.SetPairProbe(setup.PairProbe{
		LookPath: func(name string) (string, error) {
			if name == "tailscale" {
				return "/usr/bin/tailscale", nil
			}
			return "", errors.New("not found")
		},
		TailscalePrefs:  func(string) ([]byte, error) { return prefs, nil },
		TailscaleStatus: func(string) ([]byte, error) { return status, nil },
	})
	t.Cleanup(func() { setup.SetPairProbe(old) })
}

func TestPairCmdPlainRefusesTailscaleSSHBeforeTheQR(t *testing.T) {
	useTailscaleSSHProbe(t)
	f := loadPairFixture(t)
	hostKey := filepath.Join(t.TempDir(), "host.pub")
	if err := os.WriteFile(hostKey, []byte(f.HostKeyPub+" root@ws-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	old := pairSettings
	pairSettings = pairEnv{home: home, exe: testExe, hostKeyPath: hostKey, poll: time.Hour, limit: time.Hour}
	t.Cleanup(func() { pairSettings = old })
	done := make(chan error, 1)
	var out bytes.Buffer
	go func() {
		done <- pairCmd(context.Background(), []string{"--address", "ws-1.tail1234.ts.net", "--name", "ws-1"}, &out, time.Now)
	}()
	select {
	case err := <-done:
		if err == nil || err.Error() != wantTailscaleSSH {
			t.Fatalf("err = %v, want %q", err, wantTailscaleSSH)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pairCmd waited for a phone instead of refusing")
	}
	if out.Len() != 0 {
		t.Fatalf("output before the refusal: %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh", "authorized_keys")); !os.IsNotExist(err) {
		t.Fatalf("a one-time key was written: %v", err)
	}
}

func TestPairHostOfferRefusesTailscaleSSH(t *testing.T) {
	useTailscaleSSHProbe(t)
	f := loadPairFixture(t)
	hostKey := filepath.Join(t.TempDir(), "host.pub")
	if err := os.WriteFile(hostKey, []byte(f.HostKeyPub+" root@ws-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		console bool
		want    string
	}{
		{false, wantTailscaleSSH},
		{true, strings.Replace(wantTailscaleSSH, "then matchblox pair again", "then press Enter", 1)},
	} {
		home := t.TempDir()
		h := pairHost(home, testExe, hostKey, func() string { return "ws-1.tail1234.ts.net" }, 22, "ws-1", c.console)
		id, link, err := h.Offer(time.Now().Add(time.Minute))
		if err == nil || err.Error() != c.want || id != "" || link != "" {
			t.Fatalf("console=%v: id %q link %q err %v, want %q", c.console, id, link, err, c.want)
		}
		if _, err := os.Stat(filepath.Join(home, ".ssh", "authorized_keys")); !os.IsNotExist(err) {
			t.Fatalf("console=%v: a one-time key was written: %v", c.console, err)
		}
	}
}
