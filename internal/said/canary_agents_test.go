//go:build canary

package said

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
)

type standIn interface {
	http.Handler
	toolRequests() int
}

type agent struct {
	key, name, bin string
	version        func(env []string) (string, error)
	setup          func(home, work, url string) error
	env            func(home, url, binDir string) []string
	argv           func() []string
	stand          func() standIn
}

const fakeAnthropicKey = "sk-ant-canary-not-a-real-key-00000000000000000000"

var agents = []agent{
	{
		key:  "claude",
		name: "Claude Code",
		bin:  "claude",
		version: func(env []string) (string, error) {
			return firstField(env, "claude", "--version")
		},
		setup: func(home, work, _ string) error {
			b, err := json.Marshal(map[string]any{
				"hasCompletedOnboarding": true,
				"theme":                  "dark",
				"customApiKeyResponses": map[string][]string{
					"approved": {fakeAnthropicKey[len(fakeAnthropicKey)-20:]},
					"rejected": {},
				},
				"projects": map[string]any{
					work: map[string]any{"hasTrustDialogAccepted": true, "allowedTools": []string{}},
				},
			})
			if err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(home, ".claude.json"), b, 0o600)
		},
		env: func(home, url, binDir string) []string {
			return []string{
				"HOME=" + home,
				"PATH=/usr/bin:/bin:" + binDir,
				"TERM=xterm-256color",
				"ANTHROPIC_BASE_URL=" + url,
				"ANTHROPIC_API_KEY=" + fakeAnthropicKey,
				"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
				"DISABLE_AUTOUPDATER=1",
			}
		},
		argv: func() []string {
			return []string{"claude", "--permission-mode", "default", "make the canary file"}
		},
		stand: func() standIn { return &anthropicStandIn{} },
	},
}

// firstField runs an agent's version command and returns the first word it
// prints.
func firstField(env []string, argv ...string) (string, error) {
	cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec // the agent's own version command
	cmd.Env = env
	b, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s: %v", strings.Join(argv, " "), err)
	}
	f := strings.Fields(string(b))
	if len(f) == 0 || !versionRe.MatchString(f[0]) {
		return "", fmt.Errorf("%s printed no version: %q", strings.Join(argv, " "), b)
	}
	return f[0], nil
}

// anthropicStandIn answers as the Anthropic Messages API: a request that
// offers a Bash tool gets one Bash call, and a request that carries its
// result gets "Done.".
type anthropicStandIn struct{ tools atomic.Int64 }

func (s *anthropicStandIn) toolRequests() int { return int(s.tools.Load()) }

func (s *anthropicStandIn) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method != http.MethodPost:
		_, _ = io.WriteString(w, "{}")
		return
	case strings.HasSuffix(r.URL.Path, "/count_tokens"):
		_, _ = io.WriteString(w, `{"input_tokens":10}`)
		return
	case r.URL.Path != "/v1/messages":
		_, _ = io.WriteString(w, "{}")
		return
	}

	var req struct {
		Model string `json:"model"`
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	bash := false
	for _, tool := range req.Tools {
		bash = bash || tool.Name == "Bash"
	}
	result := false
	for _, m := range req.Messages {
		var blocks []struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(m.Content, &blocks) == nil {
			for _, b := range blocks {
				result = result || b.Type == "tool_result"
			}
		}
	}

	block := map[string]any{"type": "text", "text": ""}
	delta := map[string]any{"type": "text_delta", "text": "Done."}
	stop := "end_turn"
	if bash && !result {
		s.tools.Add(1)
		block = map[string]any{"type": "tool_use", "id": "toolu_1", "name": "Bash", "input": map[string]any{}}
		delta = map[string]any{"type": "input_json_delta", "partial_json": `{"command":"touch canary-ran","description":"Make the canary file"}`}
		stop = "tool_use"
	}

	w.Header().Set("Content-Type", "text/event-stream")
	f, _ := w.(http.Flusher)
	for _, e := range []struct {
		name string
		data map[string]any
	}{
		{"message_start", map[string]any{"message": map[string]any{
			"id": "msg_canary", "type": "message", "role": "assistant", "model": req.Model,
			"content": []any{}, "usage": map[string]int{"input_tokens": 10, "output_tokens": 1},
		}}},
		{"content_block_start", map[string]any{"index": 0, "content_block": block}},
		{"content_block_delta", map[string]any{"index": 0, "delta": delta}},
		{"content_block_stop", map[string]any{"index": 0}},
		{"message_delta", map[string]any{
			"delta": map[string]any{"stop_reason": stop, "stop_sequence": nil},
			"usage": map[string]int{"output_tokens": 5},
		}},
		{"message_stop", map[string]any{}},
	} {
		e.data["type"] = e.name
		b, err := json.Marshal(e.data)
		if err != nil {
			return
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.name, b); err != nil {
			return
		}
		if f != nil {
			f.Flush()
		}
	}
}
