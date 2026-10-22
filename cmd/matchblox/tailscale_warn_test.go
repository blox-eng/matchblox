package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/setup"
)

func useTailscalePrefs(t *testing.T, prefsFile string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pair", prefsFile))
	if err != nil {
		t.Fatal(err)
	}
	old := setup.SetPairProbe(setup.PairProbe{
		LookPath: func(name string) (string, error) {
			if name == "tailscale" {
				return "/usr/bin/tailscale", nil
			}
			return "", errors.New("not found")
		},
		TailscalePrefs: func(string) ([]byte, error) { return b, nil },
	})
	t.Cleanup(func() { setup.SetPairProbe(old) })
}

func TestWarnTailscaleSSHGatePrintsWhenOn(t *testing.T) {
	useTailscalePrefs(t, "tailscale-prefs-ssh-on.json")
	var buf bytes.Buffer
	warnTailscaleSSHGate(&buf)
	got := buf.String()
	if !strings.Contains(got, setup.WarnTailscaleSSHGate) || !strings.HasPrefix(got, "matchblox: ") {
		t.Fatalf("got %q", got)
	}
}

func TestWarnTailscaleSSHGateSilentWhenOff(t *testing.T) {
	useTailscalePrefs(t, "tailscale-prefs-ssh-off.json")
	var buf bytes.Buffer
	warnTailscaleSSHGate(&buf)
	if buf.Len() != 0 {
		t.Fatalf("got %q", buf.String())
	}
}

func TestStatusWarnsTailscaleSSHGateText(t *testing.T) {
	path := isolate(t)
	serveInProcess(t, path)
	useTailscalePrefs(t, "tailscale-prefs-ssh-on.json")
	var out bytes.Buffer
	if err := status(&out, path, config.Default(), "", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), setup.WarnTailscaleSSHGate) {
		t.Fatalf("status said:\n%s", out.String())
	}
	useTailscalePrefs(t, "tailscale-prefs-ssh-off.json")
	out.Reset()
	if err := status(&out, path, config.Default(), "", true); err != nil || strings.Contains(out.String(), setup.WarnTailscaleSSHGate) {
		t.Fatalf("off prefs still warned: %v\n%s", err, out.String())
	}
}

func TestStatusWarnsTailscaleSSHGateJSONOnStderr(t *testing.T) {
	path := isolate(t)
	serveInProcess(t, path)
	useTailscalePrefs(t, "tailscale-prefs-ssh-on.json")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	var out bytes.Buffer
	err = status(&out, path, config.Default(), "", false)
	_ = w.Close()
	os.Stderr = old
	if err != nil {
		t.Fatal(err)
	}
	stderr, _ := io.ReadAll(r)
	_ = r.Close()
	if !strings.Contains(string(stderr), setup.WarnTailscaleSSHGate) {
		t.Fatalf("stderr %q", stderr)
	}
	if strings.Contains(out.String(), "Tailscale SSH") {
		t.Fatalf("JSON stdout mixed with warn:\n%s", out.String())
	}
}

func TestSetupDoorsWarnsTailscaleSSHGate(t *testing.T) {
	useTailscalePrefs(t, "tailscale-prefs-ssh-on.json")
	e := setupEnv(t)
	var out bytes.Buffer
	if err := setupDoors(e, strings.NewReader("\n\n\n\ny\n"), &out, func([]string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), setup.WarnTailscaleSSHGate) {
		t.Fatalf("setup said:\n%s", out.String())
	}
}
