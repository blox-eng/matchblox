package sample

import (
	"regexp"
	"strings"
)

var versionSegment = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+_-]{0,39}$`)

// versionOf reads an agent's version from the install path in its exe or argv,
// <agent>/versions/<version>. It never runs the agent: a binary's own
// --version could run anything. An agent's updater deletes the old binary
// while its sessions run it, and the kernel then ends the exe link with
// " (deleted)": the version is still the one that runs.
func versionOf(agent, exe string, argv []string) (version, why string) {
	exe = strings.TrimSuffix(exe, " (deleted)")
	for _, path := range append([]string{exe}, argv...) {
		parts := strings.Split(path, "/")
		for i := 1; i+1 < len(parts); i++ {
			if parts[i-1] == agent && parts[i] == "versions" && versionSegment.MatchString(parts[i+1]) {
				return parts[i+1], ""
			}
		}
	}
	return "", "the install path names no version"
}

// AgentLabel names an agent for people.
func AgentLabel(agent string) string {
	switch agent {
	case "claude":
		return "Claude Code"
	case "cursor-agent":
		return "Cursor"
	case "codex":
		return "Codex"
	case "opencode":
		return "OpenCode"
	}
	return agent
}
