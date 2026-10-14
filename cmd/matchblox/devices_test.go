package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/setup"
)

const (
	laptopKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIM4bfwfUcmyCX2tnMZ4mHkeEjSRJnkf0ehLEuB8iWLHU"
	phoneKey1 = "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBH7iriHk4uoQ5QFqWYraWlOwFNM2wikZzsWVilC6boso/FQAq1zahWnKxRjYJ2Z/EOtOHrgqLqjFZsrrjAmjJQA="
	phoneKey2 = "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBMvaEjNCnphfEsogXCHt0gNKw4Ek7XjbU/1H71OWJvVrSjYFsg1HSGRIpqX+VP2kHH5WQFlMn6djJG65WRbo/ME="
)

func homeWithDevices(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if _, err := setup.Authorize(home, testExe, laptopKey); err != nil {
		t.Fatal(err)
	}
	if _, err := setup.AuthorizeDevice(home, testExe, phoneKey1, "Martin's iPhone"); err != nil {
		t.Fatal(err)
	}
	if _, err := setup.AuthorizeDevice(home, testExe, phoneKey2, "iPad"); err != nil {
		t.Fatal(err)
	}
	return home
}

func fp(t *testing.T, key string) string {
	t.Helper()
	f, err := setup.Fingerprint(key)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestDevicesListAndRemove(t *testing.T) {
	home := homeWithDevices(t)
	var out bytes.Buffer
	if err := devicesCmd(nil, home, &out); err != nil {
		t.Fatal(err)
	}
	want := "phone   Martin's iPhone  " + fp(t, phoneKey1) + "\n" +
		"phone   iPad             " + fp(t, phoneKey2) + "\n" +
		"laptop                   " + fp(t, laptopKey) + "\n"
	if out.String() != want {
		t.Fatalf("list:\n%s\nwant:\n%s", out.String(), want)
	}

	out.Reset()
	if err := devicesCmd([]string{"rm", "iPad"}, home, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "Removed iPad. Backup: "+filepath.Join(home, ".ssh")) {
		t.Fatalf("rm printed %q", out.String())
	}
	b, _ := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	if strings.Contains(string(b), "matchblox:phone:iPad") || !strings.Contains(string(b), "Martin's iPhone") {
		t.Fatalf("authorized_keys after rm:\n%s", b)
	}

	out.Reset()
	if err := devicesCmd([]string{"rm", fp(t, laptopKey)}, home, &out); err != nil {
		t.Fatalf("rm by fingerprint: %v", err)
	}

	err := devicesCmd([]string{"rm", "nobody"}, home, &out)
	if err == nil || err.Error() != "no device named nobody" {
		t.Fatalf("rm unknown: %v", err)
	}
}

func TestDevicesRefusesWrongArguments(t *testing.T) {
	home := homeWithDevices(t)
	for _, args := range [][]string{{"rm"}, {"rm", "a", "b"}, {"--x"}, {"list"}} {
		if err := devicesCmd(args, home, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "usage: matchblox devices") {
			t.Errorf("%v: %v", args, err)
		}
	}
}

func TestDevicesWithoutKeysSaysHowToAddOne(t *testing.T) {
	var out bytes.Buffer
	if err := devicesCmd(nil, t.TempDir(), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "matchblox pair") {
		t.Fatalf("empty list: %q", out.String())
	}
}
