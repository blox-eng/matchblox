package setup

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/blox-eng/matchblox/internal/remote"
)

// Device is a key that the gate lets in: a phone, or a laptop console.
type Device struct{ Kind, Name, Fingerprint string }

// ValidDeviceName is a name the line comment can hold and a person can read.
func ValidDeviceName(s string) bool {
	if n := utf8.RuneCountInString(s); n < 1 || n > 40 || s[0] == ' ' || s[len(s)-1] == ' ' {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune(" ._-'’", r) {
			return false
		}
	}
	return true
}

// AuthorizeDevice lets a phone key start only `matchblox gate`, which then
// runs the console. Rules as for Authorize; the comment names the phone.
func AuthorizeDevice(home, exe, pubKey, name string) (backup string, err error) {
	if !remote.ValidDeviceKey(pubKey) {
		return "", errors.New("not one P-256 public key")
	}
	if !ValidDeviceName(name) {
		return "", errors.New("the name has 1 to 40 letters, digits, spaces and . _ - ' only, and no space at either end: " + name)
	}
	if !exePath.MatchString(exe) {
		return "", errors.New("the matchblox path has a character the gate line cannot hold (only letters, digits, space and ._+@/- ): " + exe +
			"; install matchblox in a plain directory, such as /usr/local/bin")
	}
	f := strings.Fields(pubKey)
	return putKeyLine(home, f[1], `restrict,pty,command="'`+exe+`' gate" `+f[0]+" "+f[1]+" matchblox:phone:"+name)
}

// Fingerprint is the SHA256 fingerprint of a key line, as ssh-keygen -lf
// prints it.
func Fingerprint(authorizedKey string) (string, error) {
	f := strings.Fields(authorizedKey)
	if len(f) < 2 {
		return "", errors.New("not a public key line")
	}
	blob, err := base64.StdEncoding.DecodeString(f[1])
	if err != nil || len(blob) == 0 {
		return "", errors.New("not a public key line")
	}
	sum := sha256.Sum256(blob)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:]), nil
}

var gateLine = regexp.MustCompile(`^[a-z,-]*command="'[^"]*' gate" (\S+ \S+)(?: (.*))?$`)

const phoneComment = "matchblox:phone:"

func parseDevice(line string) (Device, bool) {
	m := gateLine.FindStringSubmatch(line)
	if m == nil {
		return Device{}, false
	}
	fp, err := Fingerprint(m[1])
	if err != nil {
		return Device{}, false
	}
	if name, ok := strings.CutPrefix(m[2], phoneComment); ok {
		return Device{"phone", name, fp}, true
	}
	return Device{"laptop", m[2], fp}, true
}

// Devices are the keys that the gate lets in, in the order of the file.
func Devices(home string) ([]Device, error) {
	_, b, _, err := read(authorizedKeys(home))
	if err != nil {
		return nil, err
	}
	var out []Device
	for line := range strings.SplitSeq(string(b), "\n") {
		if d, ok := parseDevice(line); ok {
			out = append(out, d)
		}
	}
	return out, nil
}

// RemoveDevice takes out the line of one phone, by its name or, when
// names repeat, by its fingerprint.
func RemoveDevice(home, name string) (backup string, err error) {
	target, old, mode, err := read(authorizedKeys(home))
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(old), "\n")
	hit := -1
	n := 0
	for i, line := range lines {
		if d, ok := parseDevice(line); ok && (d.Fingerprint == name || (d.Kind == "phone" && d.Name == name)) {
			hit = i
			n++
		}
	}
	switch {
	case n == 0:
		return "", errors.New("no device named " + name)
	case n > 1:
		return "", fmt.Errorf("%d devices named %s; give the fingerprint", n, name)
	}
	next := strings.Join(append(lines[:hit:hit], lines[hit+1:]...), "\n")
	if backup, err = keep(target, old, mode); err != nil {
		return "", err
	}
	return backup, replace(target, []byte(next), mode)
}

func authorizedKeys(home string) string { return filepath.Join(home, ".ssh", "authorized_keys") }

var pairID = regexp.MustCompile(`^[0-9a-f]{12}$`)

func pairCommand(exe, id string) string { return `command="'` + exe + `' gate pair ` + id + `"` }

// AddPairKey lets a one-time key run only `matchblox gate pair <id>`, and
// tells sshd to refuse it after until.
func AddPairKey(home, exe, pubKey, id string, until time.Time) error {
	if !remote.ValidPubKey(pubKey) {
		return errors.New("not one ed25519 public key")
	}
	if !pairID.MatchString(id) {
		return errors.New("the pairing id is 12 lowercase hex characters: " + id)
	}
	if !exePath.MatchString(exe) {
		return errors.New("the matchblox path has a character the gate line cannot hold (only letters, digits, space and ._+@/- ): " + exe +
			"; install matchblox in a plain directory, such as /usr/local/bin")
	}
	f := strings.Fields(pubKey)
	_, err := putKeyLine(home, f[1], `restrict,expiry-time="`+until.UTC().Format("200601021504")+`Z",`+pairCommand(exe, id)+" "+f[0]+" "+f[1]+" matchblox:pair")
	return err
}

var pairLine = regexp.MustCompile(`^restrict,expiry-time="\d{12}Z",command="'[^"']*' gate pair ([0-9a-f]{12})" `)

func pairLines(home, id string) (lines []string, hits []int, err error) {
	if !pairID.MatchString(id) {
		return nil, nil, errors.New("the pairing id is 12 lowercase hex characters: " + id)
	}
	_, b, _, err := read(authorizedKeys(home))
	if err != nil {
		return nil, nil, err
	}
	lines = strings.Split(string(b), "\n")
	for i, line := range lines {
		if m := pairLine.FindStringSubmatch(line); m != nil && m[1] == id {
			hits = append(hits, i)
		}
	}
	return lines, hits, nil
}

// PairKeyPresent is whether the line of the one-time key is still there.
func PairKeyPresent(home, id string) (bool, error) {
	_, hits, err := pairLines(home, id)
	return len(hits) > 0, err
}

// RemovePairKey takes out the line of the one-time key. Every other line
// keeps its bytes.
func RemovePairKey(home, id string) (removed bool, err error) {
	lines, hits, err := pairLines(home, id)
	if err != nil || len(hits) == 0 {
		return false, err
	}
	target, _, mode, err := read(authorizedKeys(home))
	if err != nil {
		return false, err
	}
	var kept []string
	for i, line := range lines {
		if !slices.Contains(hits, i) {
			kept = append(kept, line)
		}
	}
	return true, replace(target, []byte(strings.Join(kept, "\n")), mode)
}
