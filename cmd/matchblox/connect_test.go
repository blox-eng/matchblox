package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/hosts"
	"github.com/blox-eng/matchblox/internal/remote"
)

const testPub = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFakeFakeFakeFakeFakeFakeFakeFakeFakeFakeFake matchblox"

// keygen is a run that makes the key the way ssh-keygen would.
func keygen(t *testing.T, ran *[][]string, fail error) func([]string) error {
	return func(argv []string) error {
		*ran = append(*ran, argv)
		if argv[0] == "ssh-keygen" {
			key := argv[len(argv)-1]
			if err := os.WriteFile(key, []byte("private"), 0o600); err != nil {
				t.Fatal(err)
			}
			return os.WriteFile(key+".pub", []byte(testPub+"\n"), 0o600)
		}
		return fail
	}
}

func TestConnectMakesTheKeyThenBootstraps(t *testing.T) {
	home, list := t.TempDir(), filepath.Join(t.TempDir(), "hosts")
	var ran [][]string
	var out bytes.Buffer
	err := connectHost(connectEnv{Home: home, Hosts: list, Run: keygen(t, &ran, nil), Out: &out,
		Verify: func(remote.Target) (string, error) { return "v0.1.0", nil }}, "ws-1", false)
	if err != nil {
		t.Fatal(err)
	}
	key := remote.KeyPath(home)
	if len(ran) != 2 || !slices.Equal(ran[0], []string{"ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "matchblox", "-f", key}) {
		t.Fatalf("ran %q", ran)
	}
	script, _ := remote.Bootstrap(testPub, false)
	want := []string{"ssh", "-t", "-o", "ForwardAgent=no", "-o", "ClearAllForwardings=yes", "--", "ws-1", script}
	if !slices.Equal(ran[1], want) {
		t.Fatalf("bootstrap %q\nwant %q", ran[1], want)
	}
	if di, _ := os.Stat(filepath.Dir(key)); di.Mode().Perm() != 0o700 {
		t.Fatalf("~/.ssh mode %v", di.Mode().Perm())
	}
	for _, line := range []string{"$ ssh-keygen", "$ ssh -t", "connected ws-1 (matchblox v0.1.0)", "matchblox ws-1"} {
		if !strings.Contains(out.String(), line) {
			t.Fatalf("out lacks %q:\n%s", line, out.String())
		}
	}
	if got := hosts.Read(list); !slices.Equal(got, []string{"ws-1"}) {
		t.Fatalf("hosts %q", got)
	}
}

func TestConnectKeepsAnExistingKeyAndUpdates(t *testing.T) {
	home := t.TempDir()
	key := remote.KeyPath(home)
	if err := os.MkdirAll(filepath.Dir(key), 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(key, []byte("private"), 0o600)
	_ = os.WriteFile(key+".pub", []byte(testPub+"\n"), 0o600)
	var ran [][]string
	err := connectHost(connectEnv{Home: home, Hosts: filepath.Join(home, "hosts"), Run: keygen(t, &ran, nil), Out: &bytes.Buffer{},
		Verify: func(remote.Target) (string, error) { return "v0.1.0", nil }}, "ws-1", true)
	if err != nil {
		t.Fatal(err)
	}
	up, _ := remote.Bootstrap(testPub, true)
	if len(ran) != 1 || ran[0][len(ran[0])-1] != up {
		t.Fatalf("ran %q", ran)
	}
}

func TestConnectFailureAddsNoHost(t *testing.T) {
	home, list := t.TempDir(), filepath.Join(t.TempDir(), "hosts")
	var ran [][]string
	err := connectHost(connectEnv{Home: home, Hosts: list, Run: keygen(t, &ran, errors.New("exit status 255")), Out: &bytes.Buffer{},
		Verify: func(remote.Target) (string, error) { t.Fatal("verified after a failed ssh"); return "", nil }}, "ws-1", false)
	if err == nil {
		t.Fatal("no error")
	}
	if hosts.Read(list) != nil {
		t.Fatal("a host that did not connect was added")
	}
}

func TestConnectRefusesAnOptionAsHost(t *testing.T) {
	var ran [][]string
	err := connectHost(connectEnv{Home: t.TempDir(), Run: keygen(t, &ran, nil), Out: &bytes.Buffer{}}, "-oProxyCommand=x", false)
	if err == nil || len(ran) != 0 {
		t.Fatalf("err %v, ran %q", err, ran)
	}
}

// A failed connect keeps its reason on the screen until Enter: the console
// takes the terminal back as soon as it ends.
func TestAFailedConnectWaitsForEnter(t *testing.T) {
	var out bytes.Buffer
	err := holdFailure(errors.New("ssh ws-1: exit status 255"), true, strings.NewReader("\n"), &out)
	if err == nil || !errors.Is(err, errShown) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out.String(), "ssh ws-1: exit status 255") || !strings.Contains(out.String(), "press Enter") {
		t.Fatalf("out:\n%s", out.String())
	}
	if err := holdFailure(nil, true, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := holdFailure(errors.New("x"), false, strings.NewReader(""), &out); err == nil || errors.Is(err, errShown) || out.Len() != 0 {
		t.Fatalf("no terminal: the error goes back as it is: %v %q", err, out.String())
	}
}
