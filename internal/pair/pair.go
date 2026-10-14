// Package pair is the contract between `matchblox pair` and the phone: the
// payload in the QR code, the request and reply lines, and the one-time key.
package pair

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
)

const urlPrefix = "matchblox://pair?p="

type Payload struct {
	V       int    `json:"v"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	User    string `json:"user"`
	Name    string `json:"name"`
	HostKey string `json:"hostkey"`
	Key     string `json:"key"`
}

type Request struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type Reply struct {
	OK    bool   `json:"ok"`
	Host  string `json:"host,omitempty"`
	Error string `json:"error,omitempty"`
}

func (p Payload) URL() string {
	b, _ := json.Marshal(p)
	return urlPrefix + base64.RawURLEncoding.EncodeToString(b)
}

func Parse(raw string) (Payload, error) {
	var p Payload
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "matchblox" || u.Host != "pair" {
		return p, errors.New("not a matchblox pairing URL")
	}
	enc := u.Query().Get("p")
	if enc == "" {
		return p, errors.New("pairing URL has no payload")
	}
	b, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return p, errors.New("pairing payload is not base64url")
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return Payload{}, errors.New("pairing payload is not JSON")
	}
	if p.V != 1 {
		return Payload{}, errors.New("pairing payload version is not 1")
	}
	if p.Host == "" || p.User == "" || p.Name == "" || p.HostKey == "" || p.Key == "" {
		return Payload{}, errors.New("pairing payload is missing a field")
	}
	if p.Port < 1 || p.Port > 65535 {
		return Payload{}, errors.New("pairing port is outside 1..65535")
	}
	if seed, err := base64.StdEncoding.DecodeString(p.Key); err != nil || len(seed) != ed25519.SeedSize {
		return Payload{}, errors.New("pairing key is not a 32-byte seed")
	}
	return p, nil
}

// NewKey makes the one-time ed25519 key: its seed, its authorized_keys
// public part and a 12-hex id for the sshd command.
func NewKey() (seed []byte, authorized, id string, err error) {
	seed = make([]byte, ed25519.SeedSize)
	if _, err = rand.Read(seed); err != nil {
		return nil, "", "", err
	}
	pub := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	blob := []byte{0, 0, 0, 11}
	blob = append(blob, "ssh-ed25519"...)
	blob = append(blob, 0, 0, 0, byte(len(pub)))
	blob = append(blob, pub...)
	sum := sha256.Sum256(blob)
	return seed, "ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob), hex.EncodeToString(sum[:])[:12], nil
}
