// Package hosts is the list of hosts a console opens: one on each line of
// ~/.config/matchblox/hosts, "#" starts a comment. It adds or removes one
// line and never rewrites the others, so the builder's lines keep their
// bytes.
package hosts

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/remote"
)

// Path is the hosts file, next to config.toml.
func Path() string { return filepath.Join(filepath.Dir(config.Path()), "hosts") }

// HiddenPath is the file of the hosts the Hosts list keeps out of the
// console, in the same format as Path.
func HiddenPath() string { return filepath.Join(filepath.Dir(config.Path()), "hidden") }

// ThisMachine is the name the console gives its own machine; the hidden
// file may name it though it is not a host name.
const ThisMachine = "this machine"

// Read is the hosts of the file, in its order; a line that is not a host
// name is skipped.
func Read(path string) []string { return read(path, remote.ValidHost) }

// Hidden is the hosts of the hidden file, in its order, and ThisMachine
// when it is there.
func Hidden(path string) []string {
	return read(path, func(h string) bool { return h == ThisMachine || remote.ValidHost(h) })
}

func read(path string, valid func(string) bool) []string {
	b, err := os.ReadFile(path) //nolint:gosec // our own config file
	if err != nil {
		return nil
	}
	var out []string
	for line := range strings.SplitSeq(string(b), "\n") {
		if h := name(line); valid(h) {
			out = append(out, h)
		}
	}
	return out
}

func name(line string) string {
	line, _, _ = strings.Cut(line, "#")
	return strings.TrimSpace(line)
}

const (
	header       = "# The hosts `matchblox` opens, one on each line. Change them here.\n"
	hiddenHeader = "# The hosts `matchblox` keeps out of the console, one on each line.\n"
)

// Add appends the host when the file does not have it.
func Add(path, host string) error {
	if !remote.ValidHost(host) {
		return errors.New("not a host name: " + host)
	}
	return appendLine(path, host, header)
}

// Hide appends the host to the hidden file when it does not have it.
func Hide(path, host string) error {
	if host != ThisMachine && !remote.ValidHost(host) {
		return errors.New("not a host name: " + host)
	}
	return appendLine(path, host, hiddenHeader)
}

func appendLine(path, host, header string) error {
	b, err := os.ReadFile(path) //nolint:gosec // our own config file
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for line := range strings.SplitSeq(string(b), "\n") {
		if name(line) == host {
			return nil
		}
	}
	s := string(b)
	switch {
	case s == "":
		s = header
	case !strings.HasSuffix(s, "\n"):
		s += "\n"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return write(path, s+host+"\n")
}

// Show takes the host out of the hidden file and keeps every other line.
func Show(path, host string) error {
	if err := Remove(path, host); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Remove takes out the lines of the host and keeps every other line.
func Remove(path, host string) error {
	b, err := os.ReadFile(path) //nolint:gosec // our own config file
	if err != nil {
		return err
	}
	lines := strings.SplitAfter(string(b), "\n")
	var keep []string
	for _, l := range lines {
		if name(l) != host || name(l) == "" {
			keep = append(keep, l)
		}
	}
	return write(path, strings.Join(keep, ""))
}

// write replaces the file in one rename.
func write(path, s string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".hosts.*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // gone after the rename
	if _, err := tmp.WriteString(s); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// SSHHosts are the hosts ~/.ssh/config names, in their order: each name of
// a Host line that is not a pattern, and the hosts of the files it
// includes.
func SSHHosts(home string) []string {
	var hosts []string
	seen := map[string]bool{}
	sshHosts(filepath.Join(home, ".ssh"), filepath.Join(home, ".ssh", "config"), seen, &hosts)
	return hosts
}

func sshHosts(dir, path string, read map[string]bool, hosts *[]string) {
	if read[path] {
		return // an Include of itself
	}
	read[path] = true
	b, err := os.ReadFile(path) //nolint:gosec // the person's own ssh config
	if err != nil {
		return
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(strings.ReplaceAll(sc.Text(), "=", " "))
		if len(f) < 2 {
			continue
		}
		switch strings.ToLower(f[0]) {
		case "include":
			for _, pat := range f[1:] {
				if strings.HasPrefix(pat, "~/") {
					pat = filepath.Join(filepath.Dir(dir), pat[2:])
				} else if !filepath.IsAbs(pat) {
					pat = filepath.Join(dir, pat)
				}
				files, _ := filepath.Glob(pat)
				for _, inc := range files {
					sshHosts(dir, inc, read, hosts)
				}
			}
		case "host":
			for _, h := range f[1:] {
				if strings.ContainsAny(h, "*?!") || !remote.ValidHost(h) || slices.Contains(*hosts, h) {
					continue
				}
				*hosts = append(*hosts, h)
			}
		}
	}
}
