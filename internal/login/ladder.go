// Package login finds and sets up the builder's own key for their ssh
// login to a host, secure way first (design §3.3): a key the agent holds,
// a key in ~/.ssh, or a new key with a passphrase.
package login

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/remote"
)

// Source is where the ladder's key comes from.
type Source int

const (
	FromAgent Source = iota
	FromFile
	// NewKey: there is no key; the console makes one with a passphrase.
	NewKey
	// NotNow: the builder keeps the password; the console asks it once per
	// master.
	NotNow
)

// Step is the ladder's first step that applies. Path is the private key
// file (FromFile, NewKey); Line is the public key line to install.
type Step struct {
	Source       Source
	Path, Line   string
	Fingerprint  string
	NoPassphrase bool // a FromFile key that anyone who reads the file can use
}

// agentOrder and fileOrder are the ladder's keys, secure first.
var (
	agentOrder = []string{"sk-ssh-ed25519@openssh.com", "ssh-ed25519"}
	fileOrder  = []string{"id_ed25519_sk", "id_ed25519", "id_ecdsa_sk", "id_ecdsa", "id_rsa"}
)

// Ladder is the first step for the agent's keys (ssh-add -L lines) and the
// key files in sshDir. A key file counts with its .pub next to it.
func Ladder(agent []string, sshDir string) Step {
	for _, typ := range agentOrder {
		for _, a := range agent {
			if l, ok := keyLine(a); ok && strings.HasPrefix(l, typ+" ") {
				return Step{Source: FromAgent, Line: l, Fingerprint: Fingerprint(l)}
			}
		}
	}
	for _, name := range fileOrder {
		if s, ok := KeyFile(filepath.Join(sshDir, name)); ok {
			return s
		}
	}
	return Step{Source: NewKey, Path: filepath.Join(sshDir, "id_ed25519")}
}

// KeyFile is the FromFile step of the private key at path, when it and
// its .pub are there.
func KeyFile(path string) (Step, bool) {
	pub, err := os.ReadFile(path + ".pub") //nolint:gosec // the builder's own ~/.ssh
	if err != nil || !regular(path) {
		return Step{}, false
	}
	l, ok := keyLine(string(pub))
	if !ok {
		return Step{}, false
	}
	// A security key's file is no secret alone: it needs the token too.
	open := !strings.HasPrefix(l, "sk-") && HasNoPassphrase(path)
	return Step{Source: FromFile, Path: path, Line: l, Fingerprint: Fingerprint(l), NoPassphrase: open}, true
}

func regular(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

// keyLine is the line to install from a public key line: its type and
// blob, and its comment only when no shell reads it (ssh-add -L names a
// key by its path, which stays on this machine).
func keyLine(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if strings.ContainsAny(s, "\n\r") {
		return "", false
	}
	f := strings.Fields(s)
	if len(f) < 2 {
		return "", false
	}
	l := f[0] + " " + f[1]
	if len(f) == 3 && remote.ValidUserKey(l+" "+f[2]) {
		l += " " + f[2]
	}
	return l, remote.ValidUserKey(l)
}

// Fingerprint is the key's SHA256 fingerprint as ssh-keygen -l shows it;
// "" for a line that is not a key.
func Fingerprint(line string) string {
	f := strings.Fields(line)
	if len(f) < 2 {
		return ""
	}
	blob, err := base64.StdEncoding.DecodeString(f[1])
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(blob)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// HasNoPassphrase: the key file opens with an empty passphrase.
func HasNoPassphrase(path string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "ssh-keygen", "-y", "-P", "", "-f", path).Run() == nil //nolint:gosec // a path of the builder's ~/.ssh
}

// AgentKeys are the public keys the builder's ssh-agent holds (ssh-add
// -L); none when no agent runs.
func AgentKeys() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ssh-add", "-L").Output()
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}
