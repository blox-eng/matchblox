package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
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

const pairPub = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJuq8ePD+piatVSqgsicCgoOmr96M/08H8oC9fvzMWs9"

func TestAddPairKeyLine(t *testing.T) {
	home := t.TempDir()
	until := time.Date(2026, 10, 14, 16, 37, 0, 0, time.FixedZone("EEST", 3*3600)).In(time.FixedZone("EEST", 3*3600))
	if err := AddPairKey(home, "/usr/local/bin/matchblox", pairPub, "0123456789ab", until); err != nil {
		t.Fatal(err)
	}
	want := `restrict,expiry-time="202610141337Z",command="'/usr/local/bin/matchblox' gate pair 0123456789ab" ` + pairPub + " matchblox:pair\n"
	if got := authKeys(home); got != want {
		t.Fatalf("line:\n%s\nwant:\n%s", got, want)
	}
	if ok, err := PairKeyPresent(home, "0123456789ab"); err != nil || !ok {
		t.Fatalf("present = %v, %v", ok, err)
	}
	if ok, _ := PairKeyPresent(home, "ba9876543210"); ok {
		t.Fatal("another id present")
	}
	if got, _ := Devices(home); len(got) != 0 {
		t.Fatalf("pair line listed as device: %+v", got)
	}
}

func TestAddPairKeyRefuses(t *testing.T) {
	home := t.TempDir()
	until := time.Now()
	for name, c := range map[string]struct{ exe, key, id string }{
		"short id":    {"/opt/mb/matchblox", pairPub, "0123"},
		"upper id":    {"/opt/mb/matchblox", pairPub, "0123456789AB"},
		"injected id": {"/opt/mb/matchblox", pairPub, `0123456789a" `},
		"quote exe":   {`/opt/mb/m"x`, pairPub, "0123456789ab"},
		"quote exe 2": {`/opt/mb/m'x`, pairPub, "0123456789ab"},
		"relative":    {"matchblox", pairPub, "0123456789ab"},
		"p256 key":    {"/opt/mb/matchblox", phoneKey, "0123456789ab"},
	} {
		if err := AddPairKey(home, c.exe, c.key, c.id, until); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if got := authKeys(home); got != "" {
		t.Fatalf("file written:\n%s", got)
	}
}

func TestRemovePairKey(t *testing.T) {
	home := t.TempDir()
	until := time.Date(2026, 10, 14, 14, 37, 0, 0, time.UTC)
	other := "ssh-rsa AAAAB3NzaC1yc2E other@host"
	keep2 := `restrict,expiry-time="202610141437Z",command="'/opt/mb/matchblox' gate pair ba9876543210" ` + "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOtherOtherOtherOtherOtherOtherOtherOtherOth matchblox:pair"
	dir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "authorized_keys"), []byte(other+"\n"+keep2), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AddPairKey(home, "/usr/local/bin/matchblox", pairPub, "0123456789ab", until); err != nil {
		t.Fatal(err)
	}
	removed, err := RemovePairKey(home, "0123456789ab")
	if err != nil || !removed {
		t.Fatalf("remove = %v, %v", removed, err)
	}
	if got := authKeys(home); got != other+"\n"+keep2+"\n" {
		t.Fatalf("after remove:\n%q", got)
	}
	if removed, err = RemovePairKey(home, "0123456789ab"); err != nil || removed {
		t.Fatalf("second remove = %v, %v", removed, err)
	}
	if got := authKeys(home); got != other+"\n"+keep2+"\n" {
		t.Fatalf("after second remove:\n%q", got)
	}
}

func TestRemovePairKeyKeepsBytesWithoutFinalNewline(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	pair := `restrict,expiry-time="202610141437Z",command="'/opt/mb/matchblox' gate pair 0123456789ab" ` + pairPub + " matchblox:pair"
	in := "ssh-rsa AAAA a\n" + pair + "\nssh-rsa BBBB b"
	if err := os.WriteFile(filepath.Join(dir, "authorized_keys"), []byte(in), 0o600); err != nil {
		t.Fatal(err)
	}
	if removed, err := RemovePairKey(home, "0123456789ab"); err != nil || !removed {
		t.Fatalf("remove = %v, %v", removed, err)
	}
	if got := authKeys(home); got != "ssh-rsa AAAA a\nssh-rsa BBBB b" {
		t.Fatalf("got %q", got)
	}
	if _, err := RemovePairKey(home, "nothex"); err == nil {
		t.Fatal("bad id: no error")
	}
}
