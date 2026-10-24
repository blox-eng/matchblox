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
	for _, s := range []string{"-uhack", "--upload-pack=x", "ext::sh -c x", "file:///etc", "https://-x/o/r", "git@-x:o/r", "http://github.com/o/r"} {
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
