// Package gitremote parses and compares git remote URLs. It is pure: no
// commands, no files, no network.
package gitremote

import (
	"net/url"
	"regexp"
	"strings"
)

// Remote is a git remote split into its parts. URL is what git clones.
type Remote struct {
	URL, Host, Owner, Repo string
}

// Slug is "owner/repo".
func (r Remote) Slug() string { return r.Owner + "/" + r.Repo }

var (
	part  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	short = regexp.MustCompile(`^([A-Za-z0-9._-]+)/([A-Za-z0-9._-]+)$`)
	scp   = regexp.MustCompile(`^[A-Za-z0-9._-]+@([A-Za-z0-9._-]+):(.+)$`)
)

// Parse reads a remote URL or the shorthand "o/r" or "gh:o/r". The shorthand
// names github.com and uses protocol, "ssh" or "https". Parse refuses what is
// not a safe remote: a path, an option, or a transport other than https and ssh.
func Parse(s, protocol string) (Remote, bool) {
	s = strings.TrimSuffix(strings.TrimSpace(s), "/")
	if s == "" || strings.HasPrefix(s, "-") || strings.ContainsAny(s, " \t\r\n") || strings.Contains(s, "::") {
		return Remote{}, false
	}
	if r, ok := parseURL(s); ok {
		return r, true
	}
	m := short.FindStringSubmatch(strings.TrimPrefix(s, "gh:"))
	if m == nil || strings.HasPrefix(m[1], "-") || strings.HasPrefix(m[2], "-") || dots(m[1]) || dots(m[2]) {
		return Remote{}, false
	}
	r := Remote{Host: "github.com", Owner: m[1], Repo: strings.TrimSuffix(m[2], ".git")}
	if r.Repo == "" {
		return Remote{}, false
	}
	if protocol == "ssh" {
		r.URL = "git@github.com:" + m[1] + "/" + r.Repo + ".git"
	} else {
		r.URL = "https://github.com/" + m[1] + "/" + r.Repo + ".git"
	}
	return r, true
}

// Same reports whether two remote URLs name the same repo. Host and owner
// ignore case; scheme, user, ".git" and a trailing "/" are ignored.
func Same(a, b string) bool {
	ra, oka := parseURL(strings.TrimSuffix(strings.TrimSpace(a), "/"))
	rb, okb := parseURL(strings.TrimSuffix(strings.TrimSpace(b), "/"))
	return oka && okb && key(ra) == key(rb)
}

func key(r Remote) string {
	return strings.ToLower(r.Host) + "/" + strings.ToLower(r.Owner) + "/" + r.Repo
}

func dots(s string) bool { return s == "." || s == ".." }

// parseURL reads the https://, ssh:// and git@host:path forms.
func parseURL(s string) (Remote, bool) {
	var host, path string
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil || (u.Scheme != "https" && u.Scheme != "ssh") {
			return Remote{}, false
		}
		if u.User != nil && (!part.MatchString(u.User.Username()) || strings.HasPrefix(u.User.Username(), "-")) {
			return Remote{}, false
		}
		host, path = u.Hostname(), strings.TrimPrefix(u.Path, "/")
	} else if m := scp.FindStringSubmatch(s); m != nil {
		host, path = m[1], m[2]
	} else {
		return Remote{}, false
	}
	if host == "" || strings.HasPrefix(host, "-") {
		return Remote{}, false
	}
	segs := strings.Split(path, "/")
	if len(segs) < 2 {
		return Remote{}, false
	}
	for _, p := range segs {
		if !part.MatchString(p) || strings.HasPrefix(p, "-") || dots(p) {
			return Remote{}, false
		}
	}
	repo := strings.TrimSuffix(segs[len(segs)-1], ".git")
	if repo == "" {
		return Remote{}, false
	}
	return Remote{URL: s, Host: host, Owner: strings.Join(segs[:len(segs)-1], "/"), Repo: repo}, true
}
