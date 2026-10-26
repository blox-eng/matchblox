package gitscan

import "strings"

// gitConfig is the part of a git config this package reads. vals maps a
// section (lowercased name, subsection kept: `remote.origin`) to its
// variables (lowercased name) and their values in file order. order lists
// the sections as they first appear.
type gitConfig struct {
	vals  map[string]map[string][]string
	order []string
}

// parseGitConfig reads the subset of git's config syntax that matters here:
// case-insensitive sections and names, "…" quoting, \" \\ \n \t escapes, #
// and ; comments outside quotes, and \ line continuation. One pass, O(bytes).
func parseGitConfig(b []byte) gitConfig {
	c := gitConfig{vals: map[string]map[string][]string{}}
	s := string(b)
	section := ""
	for len(s) > 0 {
		line, rest := cutLine(s)
		s = rest
		line = strings.TrimLeft(line, " \t")
		for line != "" && line[0] != '#' && line[0] != ';' {
			if line[0] == '[' {
				name, after, ok := parseSection(line)
				if !ok {
					break
				}
				section = name
				if _, seen := c.vals[section]; !seen {
					c.vals[section] = map[string][]string{}
					c.order = append(c.order, section)
				}
				line = strings.TrimLeft(after, " \t")
				continue
			}
			if section == "" {
				break
			}
			i := 0
			for i < len(line) && (isAlnum(line[i]) || line[i] == '-') {
				i++
			}
			key := strings.ToLower(line[:i])
			line = strings.TrimLeft(line[i:], " \t")
			if key == "" || line == "" || line[0] != '=' {
				break // a bare name is a boolean: not a value we read
			}
			var v string
			v, s = parseValue(strings.TrimLeft(line[1:], " \t"), s)
			c.vals[section][key] = append(c.vals[section][key], v)
			break
		}
	}
	return c
}

func isAlnum(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9'
}

// cutLine splits s at the first newline, dropping a CR before it.
func cutLine(s string) (line, rest string) {
	line, rest, _ = strings.Cut(s, "\n")
	return strings.TrimSuffix(line, "\r"), rest
}

// parseSection reads `[name]` or `[name "sub"]` (also the legacy
// `[name.sub]`) and returns the section key and the text after `]`.
func parseSection(line string) (key, after string, ok bool) {
	i := 1
	for i < len(line) && (isAlnum(line[i]) || line[i] == '-' || line[i] == '.') {
		i++
	}
	name := strings.ToLower(line[1:i])
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	switch {
	case i < len(line) && line[i] == ']':
		return name, line[i+1:], name != ""
	case i < len(line) && line[i] == '"' && name != "":
		var sub strings.Builder
		for i++; i < len(line); i++ {
			switch ch := line[i]; {
			case ch == '\\' && i+1 < len(line):
				i++
				sub.WriteByte(line[i])
			case ch == '"':
				i++
				if i < len(line) && line[i] == ']' {
					return name + "." + sub.String(), line[i+1:], true
				}
				return "", "", false
			default:
				sub.WriteByte(ch)
			}
		}
	}
	return "", "", false
}

// parseValue reads a value from the rest of its line, taking the next lines
// of s while a backslash ends the line. It returns the value and what is
// left of s.
func parseValue(line, s string) (string, string) {
	var v strings.Builder
	quoted := false
	end := 0 // v.Len() after the last character that is not trailing space
	for {
		cont := false
	chars:
		for i := 0; i < len(line); i++ {
			ch := line[i]
			switch {
			case ch == '\\' && i+1 == len(line):
				cont = true
			case ch == '\\':
				i++
				switch line[i] {
				case 'n':
					v.WriteByte('\n')
				case 't':
					v.WriteByte('\t')
				case '"', '\\':
					v.WriteByte(line[i])
				default: // git rejects the rest; keep it as written
					v.WriteByte('\\')
					v.WriteByte(line[i])
				}
				end = v.Len()
			case ch == '"':
				quoted = !quoted
				end = v.Len()
			case !quoted && (ch == '#' || ch == ';'):
				break chars
			default:
				v.WriteByte(ch)
				if quoted || (ch != ' ' && ch != '\t') {
					end = v.Len()
				}
			}
		}
		if !cont || s == "" {
			break
		}
		line, s = cutLine(s)
	}
	return v.String()[:end], s
}

// hasRemote tells if [remote "<name>"] has a url.
func (c gitConfig) hasRemote(name string) bool { return len(c.vals["remote."+name]["url"]) > 0 }

// remoteURL is the first url of [remote "<name>"], else the first url of the
// first remote that has one, in file order.
func (c gitConfig) remoteURL(name string) string {
	if u := c.vals["remote."+name]["url"]; len(u) > 0 {
		return u[0]
	}
	for _, sec := range c.order {
		if strings.HasPrefix(sec, "remote.") {
			if u := c.vals[sec]["url"]; len(u) > 0 {
				return u[0]
			}
		}
	}
	return ""
}
