package gitremote

import "testing"

func TestParse(t *testing.T) {
	for _, c := range []struct{ in, proto, url, slug string }{
		{"https://github.com/o/r", "https", "https://github.com/o/r", "o/r"},
		{"https://github.com/o/r.git", "https", "https://github.com/o/r.git", "o/r"},
		{"git@github.com:o/r.git", "https", "git@github.com:o/r.git", "o/r"},
		{"ssh://git@gitlab.example/o/r.git", "https", "ssh://git@gitlab.example/o/r.git", "o/r"},
		{"gh:o/r", "ssh", "git@github.com:o/r.git", "o/r"},
		{"o/r", "https", "https://github.com/o/r.git", "o/r"},
		{"o/r.git", "https", "https://github.com/o/r.git", "o/r"},
		{"gh:o/r.git", "ssh", "git@github.com:o/r.git", "o/r"},
		{" https://github.com/o/r/ ", "https", "https://github.com/o/r", "o/r"},
	} {
		r, ok := Parse(c.in, c.proto)
		if !ok || r.URL != c.url || r.Slug() != c.slug || r.Repo != "r" {
			t.Errorf("Parse(%q) = %+v %v", c.in, r, ok)
		}
	}
}

func TestParseLeavesPathsAlone(t *testing.T) {
	for _, s := range []string{"", "~/o/r", "./o/r", "/src/o/r", "o", "o/r/x", "~/My Work/r", "o/r x"} {
		if r, ok := Parse(s, "https"); ok {
			t.Errorf("Parse(%q) = %+v, want a path", s, r)
		}
	}
}

func TestParseRefusesOptionsAndTransports(t *testing.T) {
	for _, s := range []string{"-uhack", "--upload-pack=x", "ext::sh -c x", "file:///etc", "https://-x/o/r", "git@-x:o/r", "http://github.com/o/r", "ssh://-oProxyCommand=x@h/o/r", "https://-x@h/o/r"} {
		if r, ok := Parse(s, "https"); ok {
			t.Errorf("Parse(%q) = %+v, want refused", s, r)
		}
	}
}

func TestSame(t *testing.T) {
	if !Same("https://github.com/O/r.git", "git@github.com:o/r") || !Same("ssh://git@github.com/o/r", "https://github.com/o/r/") {
		t.Fatal("same repo not seen")
	}
	if Same("https://github.com/o/r", "https://github.com/o/r2") || Same("https://github.com/o/r", "https://gitlab.com/o/r") || Same("", "") {
		t.Fatal("different repos seen as one")
	}
}

// A repo named "", "." or ".." once ".git" is gone is no repo.
func TestParseRefusesADotRepo(t *testing.T) {
	for _, s := range []string{"o/...git", "gh:o/...git", "o/..git", "https://github.com/o/...git", "git@github.com:o/..git", "ssh://git@h/o/.git"} {
		if r, ok := Parse(s, "https"); ok {
			t.Errorf("Parse(%q) = %+v, want refused", s, r)
		}
	}
}

// A password in the URL is refused: a token never reaches git's argv,
// .git/config or a snapshot.
func TestParseRefusesAPassword(t *testing.T) {
	for _, s := range []string{"https://u:tok@github.com/o/r", "https://:tok@github.com/o/r", "ssh://git:pw@h/o/r"} {
		if r, ok := Parse(s, "https"); ok {
			t.Errorf("Parse(%q) = %+v, want refused", s, r)
		}
	}
	if Same("https://u:tok@github.com/o/r", "https://github.com/o/r") {
		t.Error("a URL with a password is the same repo")
	}
}

// Owner and repo ignore case only on the hosts that compare them so; a
// self-hosted server on a case-sensitive disk can hold team/Repo and
// team/repo as two repos. The hostname ignores case everywhere.
func TestSameCaseByHost(t *testing.T) {
	for _, c := range []struct {
		a, b string
		same bool
	}{
		{"https://github.com/o/Repo", "git@github.com:o/repo.git", true},
		{"https://github.com/O/r", "https://GitHub.com/o/r", true},
		{"https://gitlab.com/Team/Repo", "git@gitlab.com:team/repo", true},
		{"https://bitbucket.org/Team/Repo", "https://bitbucket.org/team/repo", true},
		{"https://git.example.com/team/Repo", "https://git.example.com/team/repo", false},
		{"https://git.example.com/Team/r", "https://git.example.com/team/r", false},
		{"https://Git.Example.com/team/r", "git@git.example.com:team/r", true},
	} {
		if got := Same(c.a, c.b); got != c.same {
			t.Errorf("Same(%q, %q) = %v, want %v", c.a, c.b, got, c.same)
		}
	}
}

func TestSamePort(t *testing.T) {
	for _, c := range []struct {
		a, b string
		same bool
	}{
		{"https://h:8443/o/r", "https://h:9443/o/r", false},
		{"https://h:8443/o/r", "https://h/o/r", false},
		{"https://h:443/o/r", "https://h/o/r", true},
		{"ssh://git@h:22/o/r", "git@h:o/r", true},
		{"ssh://git@h:2222/o/r", "git@h:o/r", false},
		{"ssh://git@h:2222/o/r", "ssh://git@h:2222/o/r.git", true},
	} {
		if got := Same(c.a, c.b); got != c.same {
			t.Errorf("Same(%q, %q) = %v, want %v", c.a, c.b, got, c.same)
		}
	}
}
