// Package gitremote parses and compares git remote URLs. It is pure: no
// commands, no files, no network.
package gitremote

import (
	"net/url"
	"regexp"
	"strings"
)

// Remote is a git remote split into its parts. URL is what git clones.
// Port is the URL's port when it is not the scheme's own.
type Remote struct {
	URL, Host, Port, Owner, Repo string
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
	if r.Repo == "" || dots(r.Repo) {
		return Remote{}, false
	}
	if protocol == "ssh" {
		r.URL = "git@github.com:" + m[1] + "/" + r.Repo + ".git"
	} else {
		r.URL = "https://github.com/" + m[1] + "/" + r.Repo + ".git"
	}
	return r, true
}

// Same reports whether two remote URLs name the same repo. The hostname
// ignores case; owner and repo ignore case only on the hosts in foldHosts,
// which compare them so. Scheme, user, ".git", a trailing "/" and the
// scheme's own port are ignored.
func Same(a, b string) bool {
	ra, oka := parseURL(strings.TrimSuffix(strings.TrimSpace(a), "/"))
	rb, okb := parseURL(strings.TrimSuffix(strings.TrimSpace(b), "/"))
	return oka && okb && key(ra) == key(rb)
}

// foldHosts compare owner and repo without case. Any other host may sit on
// a case-sensitive disk that holds team/Repo and team/repo as two repos.
var foldHosts = map[string]bool{"github.com": true, "gitlab.com": true, "bitbucket.org": true}

func key(r Remote) string {
	host, path := strings.ToLower(r.Host), r.Owner+"/"+r.Repo
	if foldHosts[host] {
		path = strings.ToLower(path)
	}
	return host + ":" + r.Port + "/" + path
}

// ownPort is each scheme's port: a URL that names it names none.
var ownPort = map[string]string{"https": "443", "ssh": "22", "http": "80"}

func dots(s string) bool { return s == "." || s == ".." }

// parseURL reads the https://, ssh:// and git@host:path forms. A password
// in the URL is refused: a token goes nowhere through matchblox.
func parseURL(s string) (Remote, bool) {
	var host, port, path string
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil || (u.Scheme != "https" && u.Scheme != "ssh") {
			return Remote{}, false
		}
		if u.User != nil && (!part.MatchString(u.User.Username()) || strings.HasPrefix(u.User.Username(), "-")) {
			return Remote{}, false
		}
		if _, set := u.User.Password(); set {
			return Remote{}, false
		}
		host, path = u.Hostname(), strings.TrimPrefix(u.Path, "/")
		if port = u.Port(); port == ownPort[u.Scheme] {
			port = ""
		}
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
	if repo == "" || dots(repo) {
		return Remote{}, false
	}
	return Remote{URL: s, Host: host, Port: port, Owner: strings.Join(segs[:len(segs)-1], "/"), Repo: repo}, true
}
