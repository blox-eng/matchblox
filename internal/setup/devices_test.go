package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	phoneKey  = "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBH7iriHk4uoQ5QFqWYraWlOwFNM2wikZzsWVilC6boso/FQAq1zahWnKxRjYJ2Z/EOtOHrgqLqjFZsrrjAmjJQA="
	phoneKey2 = "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBMvaEjNCnphfEsogXCHt0gNKw4Ek7XjbU/1H71OWJvVrSjYFsg1HSGRIpqX+VP2kHH5WQFlMn6djJG65WRbo/ME="
)

type pairFixture struct {
	Names struct {
		Good, Bad []string
	} `json:"names"`
	HostkeyPub string `json:"hostkey_pub"`
	HostkeyFp  string `json:"hostkey_fp"`
}

func loadPairFixture(t *testing.T) pairFixture {
	t.Helper()
	b, err := os.ReadFile("../../testdata/pair/v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var f pairFixture
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func authKeys(home string) string {
	b, _ := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	return string(b)
}

func TestValidDeviceName(t *testing.T) {
	f := loadPairFixture(t)
	for _, s := range f.Names.Good {
		if !ValidDeviceName(s) {
			t.Errorf("%q refused, want accepted", s)
		}
	}
	for _, s := range f.Names.Bad {
		if ValidDeviceName(s) {
			t.Errorf("%q accepted, want refused", s)
		}
	}
	if ValidDeviceName("trail ") {
		t.Error("trailing space accepted")
	}
}

func TestAuthorizeDeviceWritesTheGateLine(t *testing.T) {
	home := t.TempDir()
	if _, err := AuthorizeDevice(home, "/usr/local/bin/matchblox", phoneKey, "Martin’s iPhone"); err != nil {
		t.Fatal(err)
	}
	want := `restrict,pty,command="'/usr/local/bin/matchblox' gate" ` + phoneKey + " matchblox:phone:Martin’s iPhone\n"
	if got := authKeys(home); got != want {
		t.Fatalf("authorized_keys:\n%s\nwant:\n%s", got, want)
	}
}

func TestAuthorizeDeviceRefuses(t *testing.T) {
	home := t.TempDir()
	cases := map[string][3]string{
		"ed25519 key": {"/opt/mb/matchblox", pub, "iPhone"},
		"bad name":    {"/opt/mb/matchblox", phoneKey, "a\"b"},
		"bad path":    {"/opt/mb/it's", phoneKey, "iPhone"},
	}
	for name, c := range cases {
		if _, err := AuthorizeDevice(home, c[0], c[1], c[2]); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if got := authKeys(home); got != "" {
		t.Fatalf("authorized_keys written: %q", got)
	}
}

func TestAuthorizeDeviceKeepsOtherLines(t *testing.T) {
	home := t.TempDir()
	laptop := `restrict,pty,command="'/opt/mb/matchblox' gate" ` + pub
	old := "ssh-rsa AAAAother other@box\n" + laptop + "\nssh-rsa AAAAlast last@box"
	writeFile(t, filepath.Join(home, ".ssh", "authorized_keys"), old)
	backup, err := AuthorizeDevice(home, "/opt/mb/matchblox", phoneKey, "iPhone")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(backup); string(b) != old {
		t.Fatalf("backup %q: %q", backup, b)
	}
	want := old + "\n" + `restrict,pty,command="'/opt/mb/matchblox' gate" ` + phoneKey + " matchblox:phone:iPhone\n"
	if got := authKeys(home); got != want {
		t.Fatalf("authorized_keys:\n%s\nwant:\n%s", got, want)
	}
}

func TestAuthorizeDeviceSameKeyReplacesItsLine(t *testing.T) {
	home := t.TempDir()
	for _, c := range [][2]string{{"/old/matchblox", "iPhone"}, {"/new/matchblox", "Desk-2"}} {
		if _, err := AuthorizeDevice(home, c[0], phoneKey, c[1]); err != nil {
			t.Fatal(err)
		}
	}
	want := `restrict,pty,command="'/new/matchblox' gate" ` + phoneKey + " matchblox:phone:Desk-2\n"
	if got := authKeys(home); got != want {
		t.Fatalf("authorized_keys:\n%s\nwant:\n%s", got, want)
	}
	if backup, err := AuthorizeDevice(home, "/new/matchblox", phoneKey, "Desk-2"); err != nil || backup != "" {
		t.Fatalf("unchanged file: backup %q, err %v", backup, err)
	}
}

func TestAuthorizeDeviceThroughASymlink(t *testing.T) {
	home := t.TempDir()
	real := filepath.Join(home, "dotfiles", "authorized_keys")
	writeFile(t, real, "ssh-rsa AAAAother other@box\n")
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".ssh", "authorized_keys")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := AuthorizeDevice(home, "/opt/mb/matchblox", phoneKey, "iPhone"); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link replaced: %v %v", fi, err)
	}
	if b, _ := os.ReadFile(real); !strings.HasPrefix(string(b), "ssh-rsa AAAAother other@box\n") || !strings.Contains(string(b), "matchblox:phone:iPhone") {
		t.Fatalf("target: %q", b)
	}
}

func TestDevicesAndRemoveDevice(t *testing.T) {
	home := t.TempDir()
	laptopKey := loadPairFixture(t).HostkeyPub
	laptop := `restrict,pty,command="'/opt/mb/matchblox' gate" ` + laptopKey
	other := "ssh-rsa AAAAother other@box"
	pair := `restrict,expiry-time="202610141200Z",command="'/opt/mb/matchblox' gate pair abc" ` + pub + " matchblox:pair"
	writeFile(t, filepath.Join(home, ".ssh", "authorized_keys"), other+"\n"+laptop+"\n"+pair+"\n")
	for name, key := range map[string]string{"Martin’s iPhone": phoneKey, "Desk-2": phoneKey2} {
		if _, err := AuthorizeDevice(home, "/opt/mb/matchblox", key, name); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Devices(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Kind != "laptop" || got[1].Kind != "phone" || got[2].Kind != "phone" {
		t.Fatalf("devices: %+v", got)
	}
	names := []string{got[1].Name, got[2].Name}
	if !slices.Contains(names, "Martin’s iPhone") || !slices.Contains(names, "Desk-2") {
		t.Fatalf("phone names: %v", names)
	}
	fp, err := Fingerprint(laptopKey)
	if err != nil || got[0].Fingerprint != fp {
		t.Fatalf("laptop fingerprint %q, want %q (%v)", got[0].Fingerprint, fp, err)
	}

	backup, err := RemoveDevice(home, "Martin’s iPhone")
	if err != nil || backup == "" {
		t.Fatalf("remove: backup %q, err %v", backup, err)
	}
	if left := authKeys(home); strings.Contains(left, phoneKey) || !strings.Contains(left, other) || !strings.Contains(left, pair) || !strings.Contains(left, phoneKey2) {
		t.Fatalf("after remove:\n%s", left)
	}
	if got, _ = Devices(home); len(got) != 2 {
		t.Fatalf("devices after remove: %+v", got)
	}
	if _, err := RemoveDevice(home, "Nobody"); err == nil || err.Error() != "no device named Nobody" {
		t.Fatalf("unknown name: %v", err)
	}
}

func TestRemoveDeviceByFingerprintWhenNamesRepeat(t *testing.T) {
	home := t.TempDir()
	for _, key := range []string{phoneKey, phoneKey2} {
		if _, err := AuthorizeDevice(home, "/opt/mb/matchblox", key, "iPhone"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := RemoveDevice(home, "iPhone"); err == nil || err.Error() != "2 devices named iPhone; give the fingerprint" {
		t.Fatalf("repeated name: %v", err)
	}
	fp, err := Fingerprint(phoneKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveDevice(home, fp); err != nil {
		t.Fatal(err)
	}
	if got := authKeys(home); strings.Contains(got, phoneKey) || !strings.Contains(got, phoneKey2) {
		t.Fatalf("authorized_keys:\n%s", got)
	}
}

func TestFingerprintMatchesSSHKeygen(t *testing.T) {
	f := loadPairFixture(t)
	got, err := Fingerprint(f.HostkeyPub)
	if err != nil || got != f.HostkeyFp {
		t.Fatalf("Fingerprint = %q, %v; want %q", got, err, f.HostkeyFp)
	}
	for _, bad := range []string{"", "x", "ssh-ed25519 !!!"} {
		if _, err := Fingerprint(bad); err == nil {
			t.Errorf("Fingerprint(%q): no error", bad)
		}
	}
}
