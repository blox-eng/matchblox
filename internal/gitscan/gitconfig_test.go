package gitscan

import "testing"

func TestParseGitConfig(t *testing.T) {
	tests := []struct {
		name, in, remote, want string
	}{
		{"plain", "[remote \"origin\"]\n\turl = a", "origin", "a"},
		{"case of section and key", "[Remote \"origin\"]\n\tURL = a", "origin", "a"},
		{"subsection is case-sensitive", "[remote \"Origin\"]\n\turl = a", "origin", "a"},
		{"pushurl before url", "[remote \"origin\"]\n\tpushurl = p\n\turl = a", "origin", "a"},
		{"first of two urls", "[remote \"origin\"]\n\turl = a\n\turl = b", "origin", "a"},
		{"quotes and comment", "[remote \"origin\"]\n\turl = \"a b\" # c", "origin", "a b"},
		{"semicolon comment", "[remote \"origin\"]\n\turl = a ; c", "origin", "a"},
		{"escaped quote", "[remote \"origin\"]\n\turl = \"a\\\"b\"", "origin", `a"b`},
		{"continuation", "[remote \"origin\"]\n\turl = a\\\nb", "origin", "ab"},
		{"comment char in quotes", "[remote \"origin\"]\n\turl = \"a#b\"", "origin", "a#b"},
		{"escapes", "[remote \"origin\"]\n\turl = a\\\\b\\tc", "origin", "a\\b\tc"},
		{"crlf", "[remote \"origin\"]\r\n\turl = a\r\n", "origin", "a"},
		{"no remote", "[core]\n\tbare = false", "origin", ""},
		{"first remote in file order", "[remote \"up\"]\n\turl = u\n[remote \"origin\"]\n\turl = o", "x", "u"},
		{"configured remote wins", "[remote \"up\"]\n\turl = u\n[remote \"origin\"]\n\turl = o", "origin", "o"},
		{"url outside remote", "[core]\n\turl = z", "origin", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseGitConfig([]byte(tt.in)).remoteURL(tt.remote); got != tt.want {
				t.Errorf("remoteURL(%q) = %q, want %q", tt.remote, got, tt.want)
			}
		})
	}
}
