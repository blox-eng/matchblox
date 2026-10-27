package login

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// line is a public key line of typ with a random blob that names typ.
func line(typ, comment string) string {
	blob := binary.BigEndian.AppendUint32(nil, uint32(len(typ)))
	body := make([]byte, 32)
	_, _ = rand.Read(body)
	blob = append(append(blob, typ...), body...)
	l := typ + " " + base64.StdEncoding.EncodeToString(blob)
	if comment != "" {
		l += " " + comment
	}
	return l
}

// keyFiles puts a private file and its .pub in dir for each name.
func keyFiles(t *testing.T, dir string, names map[string]string) {
	t.Helper()
	for name, pub := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("private"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".pub"), []byte(pub+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLadderOrder(t *testing.T) {
	skAgent, edAgent, rsaAgent := line("sk-ssh-ed25519@openssh.com", "yubikey"), line("ssh-ed25519", "me@laptop"), line("ssh-rsa", "old")
	edFile, ecFile, rsaFile := line("ssh-ed25519", "me@laptop"), line("ecdsa-sha2-nistp256", ""), line("ssh-rsa", "")
	for _, c := range []struct {
		name   string
		agent  []string
		files  map[string]string
		source Source
		path   string
		line   string
	}{
		{"security key first", []string{rsaAgent, edAgent, skAgent}, map[string]string{"id_ed25519": edFile}, FromAgent, "", skAgent},
		{"then an agent ed25519", []string{rsaAgent, edAgent}, map[string]string{"id_ed25519": edFile}, FromAgent, "", edAgent},
		{"an agent rsa key is not taken", []string{rsaAgent}, map[string]string{"id_ecdsa": ecFile}, FromFile, "id_ecdsa", ecFile},
		{"ed25519 file before ecdsa and rsa", nil, map[string]string{"id_rsa": rsaFile, "id_ecdsa": ecFile, "id_ed25519": edFile}, FromFile, "id_ed25519", edFile},
		{"rsa last", nil, map[string]string{"id_rsa": rsaFile}, FromFile, "id_rsa", rsaFile},
		{"a bad .pub is skipped", nil, map[string]string{"id_ed25519": "ssh-ed25519 AAAA; rm -rf ~", "id_rsa": rsaFile}, FromFile, "id_rsa", rsaFile},
		{"no key", nil, nil, NewKey, "id_ed25519", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			keyFiles(t, dir, c.files)
			got := Ladder(c.agent, dir)
			wantPath := ""
			if c.path != "" {
				wantPath = filepath.Join(dir, c.path)
			}
			if got.Source != c.source || got.Path != wantPath || got.Line != c.line {
				t.Fatalf("Ladder = %+v\nwant %v %s %q", got, c.source, wantPath, c.line)
			}
			if c.line != "" && got.Fingerprint != fingerprintOf(c.line) {
				t.Fatalf("fingerprint %q", got.Fingerprint)
			}
		})
	}
}

// TestLadderDropsACommentAShellReads: ssh-add -L names a key by its path,
// which can hold a space; the line goes without it, and the path never
// reaches the host.
func TestLadderDropsACommentAShellReads(t *testing.T) {
	ed := line("ssh-ed25519", "")
	if got := Ladder([]string{ed + " /home/me/My Keys/id"}, t.TempDir()); got.Source != FromAgent || got.Line != ed {
		t.Fatalf("Ladder = %+v", got)
	}
}

// TestLadderNeedsThePrivateFile: a .pub alone logs nobody in.
func TestLadderNeedsThePrivateFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "id_ed25519.pub"), []byte(line("ssh-ed25519", "")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Ladder(nil, dir); got.Source != NewKey {
		t.Fatalf("Ladder = %+v", got)
	}
}

// fingerprintOf is what ssh-keygen -l says: SHA256 of the blob, base64
// without padding.
func fingerprintOf(l string) string {
	blob, _ := base64.StdEncoding.DecodeString(strings.Fields(l)[1])
	sum := sha256.Sum256(blob)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// TestFingerprintIsSSHKeygens: the fingerprint the console shows is the
// one ssh-keygen -l gives.
func TestFingerprintIsSSHKeygens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "k")
	keygen(t, path, "")
	pub, err := os.ReadFile(path + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("ssh-keygen", "-l", "-f", path+".pub").Output() //nolint:gosec // test
	if err != nil {
		t.Fatal(err)
	}
	if got := Fingerprint(strings.TrimSpace(string(pub))); got != strings.Fields(string(out))[1] {
		t.Fatalf("Fingerprint = %s, ssh-keygen -l: %s", got, out)
	}
}

func keygen(t *testing.T, path, pass string) {
	t.Helper()
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("no ssh-keygen here")
	}
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", pass, "-C", "test", "-f", path).CombinedOutput(); err != nil { //nolint:gosec // test
		t.Fatalf("ssh-keygen: %v %s", err, out)
	}
}

func TestHasNoPassphrase(t *testing.T) {
	dir := t.TempDir()
	open, locked := filepath.Join(dir, "open"), filepath.Join(dir, "locked")
	keygen(t, open, "")
	keygen(t, locked, "p4ss")
	if !HasNoPassphrase(open) {
		t.Error("a key made with -N \"\" has a passphrase?")
	}
	if HasNoPassphrase(locked) {
		t.Error("a key with a passphrase has none?")
	}
	if HasNoPassphrase(filepath.Join(dir, "missing")) {
		t.Error("a missing key has no passphrase?")
	}
}

// TestLadderSaysAFileKeyHasNoPassphrase: a key file anyone who reads it
// can use is still offered, and the step says so.
func TestLadderSaysAFileKeyHasNoPassphrase(t *testing.T) {
	dir := t.TempDir()
	keygen(t, filepath.Join(dir, "id_ed25519"), "")
	if got := Ladder(nil, dir); got.Source != FromFile || !got.NoPassphrase {
		t.Fatalf("Ladder = %+v", got)
	}
}

// TestSecurityKeyFileIsNoOpenKey: an sk- key file without a passphrase
// still needs its token: the step says nothing of a passphrase.
func TestSecurityKeyFileIsNoOpenKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "id_ed25519_sk")
	keygen(t, path, "") // a key file that opens empty, under an sk name
	sk := line("sk-ssh-ed25519@openssh.com", "yubikey")
	if err := os.WriteFile(path+".pub", []byte(sk+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Ladder(nil, dir); got.Source != FromFile || got.Line != sk || got.NoPassphrase {
		t.Fatalf("Ladder = %+v", got)
	}
}
