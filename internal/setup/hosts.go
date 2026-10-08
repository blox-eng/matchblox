package setup

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/remote"
)

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

func hostsDoor(e Env, choices []string) doors.Door {
	d := doors.Door{ID: doors.Hosts, Title: "Pick your hosts", Path: e.Config, Choices: choices,
		Why: "matchblox <host> opens the console of a host in ~/.ssh/config. Pick the ones you work on."}
	_, old, _, err := read(e.Config)
	if err != nil {
		d.Problem = err.Error()
		return d
	}
	var raw map[string]any
	if _, err := toml.Decode(string(old), &raw); err != nil {
		d.Problem = fmt.Sprintf("%s: %v: fix it, then this door opens", e.Config, err)
		return d
	}
	if r, ok := raw["remote"]; ok {
		if t, ok := r.(map[string]any); ok {
			if hs, ok := t["hosts"].([]any); ok && len(hs) > 0 {
				d.Done = true
				return d
			}
		}
		d.Problem = fmt.Sprintf(`add hosts = ["%s"] under [remote] in %s`, choices[0], e.Config)
		return d
	}
	d.Sum = Sum(old)
	return d
}

// AddHosts appends the picked hosts to config.toml as a new [remote] table.
// Only hosts of ~/.ssh/config can be picked, in its order.
func AddHosts(e Env, seen string, picks []string) (backup string, err error) {
	choices := SSHHosts(e.Home)
	var hosts []string
	for _, h := range choices {
		if slices.Contains(picks, h) {
			hosts = append(hosts, h)
		}
	}
	switch {
	case len(picks) == 0:
		return "", errors.New("no host picked: space picks one")
	case len(hosts) != len(picks):
		return "", errors.New("pick only hosts of ~/.ssh/config")
	}
	return edit(e.Config, seen, func(old []byte) ([]byte, error) {
		s := string(old)
		if s != "" {
			if !strings.HasSuffix(s, "\n") {
				s += "\n"
			}
			s += "\n"
		}
		return []byte(s + doors.HostsBlock(hosts)), nil
	})
}
