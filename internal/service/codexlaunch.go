package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// tomlString encodes an argv config value, never a shell fragment. JSON string
// escapes are valid TOML basic-string escapes. Go emits non-ASCII as UTF-8.
func tomlString(s string) string { b, _ := json.Marshal(s); return string(b) }

// canonicalLaunchDir also resolves an as-yet uncreated worktree's parent.
func canonicalLaunchDir(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	var rest []string
	for {
		real, err := filepath.EvalSymlinks(dir)
		if err == nil {
			slices.Reverse(rest)
			return filepath.Join(append([]string{real}, rest...)...), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", err
		}
		rest = append(rest, filepath.Base(dir))
		dir = parent
	}
}

// codexLaunchConfig adds project trust (unless trust is false: a folder just
// cloned) and workspace-write network access for this invocation. It never changes sandbox mode, command approvals, hook trust
// or persisted configuration. Explicit untrusted choices stop the launch.
func (s *Service) codexLaunchConfig(dir string, trust bool, acct *onAccount, bootstrap string) ([]string, error) {
	real, err := canonicalLaunchDir(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve Codex workspace: %w", err)
	}
	home := s.accountFolder("codex", acct)
	files := []string{filepath.Join(home, "config.toml")}
	var local []string
	for d := real; ; d = filepath.Dir(d) {
		local = append(local, filepath.Join(d, ".codex", "config.toml"))
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil || filepath.Dir(d) == d {
			break
		}
	}
	slices.Reverse(local)
	files = append(files, local...)
	developer := ""
	for _, path := range files {
		cfg, err := readCodexLaunchConfig(path)
		if err != nil {
			return nil, err
		}
		if cfg == nil {
			continue
		}
		if projects, ok := cfg["projects"].(map[string]any); ok {
			for project, v := range projects {
				policy, ok := v.(map[string]any)
				if !ok || policy["trust_level"] != "untrusted" {
					continue
				}
				projectPath, e := canonicalLaunchDir(project)
				if e != nil {
					return nil, fmt.Errorf("read Codex trust in %s: %w", path, e)
				}
				rel, e := filepath.Rel(projectPath, real)
				if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					return nil, fmt.Errorf("the Codex configuration marks %s untrusted in %s: review that trust choice before launching it with matchblox", project, path)
				}
			}
		}
		if v, ok := cfg["developer_instructions"].(string); ok {
			developer = v
		}
		if profile, ok := cfg["profile"].(string); ok && profile != "" {
			if profiles, ok := cfg["profiles"].(map[string]any); ok {
				if p, ok := profiles[profile].(map[string]any); ok {
					if v, ok := p["developer_instructions"].(string); ok {
						developer = v
					}
				}
			}
			// Newer Codex versions also support CODEX_HOME/<name>.config.toml.
			if filepath.Base(profile) != profile {
				return nil, errors.New("the Codex profile must be a file name: fix the profile before launching")
			}
			p, err := readCodexLaunchConfig(filepath.Join(home, profile+".config.toml"))
			if err != nil {
				return nil, err
			}
			if v, ok := p["developer_instructions"].(string); ok {
				developer = v
			}
		}
	}
	if developer != "" {
		bootstrap = developer + "\n\n" + bootstrap
	}
	if len(tomlString(bootstrap)) > 64000 {
		return nil, errors.New("the Codex developer instructions exceed the launch limit: shorten them before launching with matchblox")
	}
	// Codex splits a -c key on every dot and keeps quotes in its segments, so
	// a path can only travel inside the TOML value.
	opts := []string{"-c", "sandbox_workspace_write.network_access=true", "-c", "developer_instructions=" + tomlString(bootstrap)}
	if trust {
		opts = append([]string{"-c", "projects={" + tomlString(real) + "={trust_level=\"trusted\"}}"}, opts...)
	}
	return opts, nil
}

func readCodexLaunchConfig(path string) (map[string]any, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Codex launch configuration %s: %w", path, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Size() > matchEvidenceLimit {
		return nil, fmt.Errorf("the Codex launch configuration %s must be a regular file under %d bytes", path, matchEvidenceLimit)
	}
	var cfg map[string]any
	// Never print parser errors: they can contain secret configuration values.
	if _, err := toml.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("the Codex launch configuration %s does not parse: fix its TOML before launching", path)
	}
	return cfg, nil
}
