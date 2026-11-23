//go:build canary

package said

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

type standIn interface {
	http.Handler
	toolName() string
	toolRequests() int
}

type agent struct {
	key, name, bin string
	version        func(env []string) (string, error)
	setup          func(home, work, url string) error
	env            func(home, url, binDir string) []string
	argv           []string
	stand          func() standIn
	// check is what matchblox must read on the agent's screen, and reads
	// names it for the log.
	check func(screen string) (string, bool)
	reads string
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
				agentPath(binDir),
				"TERM=xterm-256color",
				"ANTHROPIC_BASE_URL=" + url,
				"ANTHROPIC_API_KEY=" + fakeAnthropicKey,
				"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
				"DISABLE_AUTOUPDATER=1",
			}
		},
		argv:  []string{"claude", "--permission-mode", "default", "make the canary file"},
		stand: func() standIn { return &anthropicStandIn{} },
		check: canaryCheck,
		reads: "waiting state, menu and options",
	},
	{
		key:  "codex",
		name: "Codex",
		bin:  "codex",
		version: func(env []string) (string, error) {
			return lastField(env, "codex", "--version")
		},
		setup: func(home, work, url string) error {
			dir := filepath.Join(home, ".codex")
			if err := os.Mkdir(dir, 0o700); err != nil {
				return err
			}
			config := fmt.Sprintf(`model = "stand-in"
model_provider = "canary"
approval_policy = "untrusted"
sandbox_mode = "read-only"
check_for_update_on_startup = false

[model_providers.canary]
name = "canary"
base_url = %q
wire_api = "responses"
env_key = "CANARY_KEY"

[projects.%q]
trust_level = "trusted"
`, url+"/v1", work)
			return os.WriteFile(filepath.Join(dir, "config.toml"), []byte(config), 0o600)
		},
		env: func(home, _, binDir string) []string {
			return []string{
				"HOME=" + home,
				"CODEX_HOME=" + filepath.Join(home, ".codex"),
				agentPath(binDir),
				"TERM=xterm-256color",
				"CANARY_KEY=canary-not-a-key",
			}
		},
		argv:  []string{"codex", "make the canary file"},
		stand: func() standIn { return &responsesStandIn{} },
		check: waitingCheck,
		reads: "waiting state",
	},
	{
		key:  "opencode",
		name: "OpenCode",
		bin:  "opencode",
		version: func(env []string) (string, error) {
			return lastField(env, "opencode", "--version")
		},
		setup: func(_, work, url string) error {
			b, err := json.Marshal(map[string]any{
				"$schema":    "https://opencode.ai/config.json",
				"autoupdate": false,
				"provider": map[string]any{
					"canary": map[string]any{
						"npm":     "@ai-sdk/openai-compatible",
						"name":    "canary",
						"options": map[string]any{"baseURL": url + "/v1", "apiKey": "canary-not-a-key"},
						"models":  map[string]any{"stand-in": map[string]any{}},
					},
				},
				"model":       "canary/stand-in",
				"small_model": "canary/stand-in",
				"permission":  map[string]any{"bash": "ask"},
			})
			if err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(work, "opencode.json"), b, 0o600)
		},
		env: func(home, _, binDir string) []string {
			return []string{
				"HOME=" + home,
				"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
				"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
				"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"),
				"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
				agentPath(binDir),
				"TERM=xterm-256color",
			}
		},
		argv:  []string{"opencode", "--prompt", "make the canary file"},
		stand: func() standIn { return &chatStandIn{} },
		check: waitingCheck,
		reads: "waiting state",
	},
}

var commandTimeout = 20 * time.Second

// agentPath is an agent's PATH: the system's, the agent's own folder, and
// node's, because an npm agent's launcher is a `#!/usr/bin/env node` script.
func agentPath(binDir string) string {
	path := "PATH=/usr/bin:/bin:" + binDir
	if node, err := exec.LookPath("node"); err == nil {
		path += ":" + filepath.Dir(node)
	}
	return path
}

func firstField(env []string, argv ...string) (string, error) {
	return versionField(env, argv, func(f []string) string { return f[0] })
}

func lastField(env []string, argv ...string) (string, error) {
	return versionField(env, argv, func(f []string) string { return f[len(f)-1] })
}

func versionField(env, argv []string, pick func([]string) string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // the agent's own version command
	cmd.Env = env
	// The whole group goes, so a child the agent started cannot hold its output open.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	b, err := cmd.Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("%s did not answer in %d s", strings.Join(argv, " "), int(commandTimeout.Seconds()))
	}
	if err != nil {
		reason := fmt.Sprintf("%s: %v", strings.Join(argv, " "), err)
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			if lines := strings.Split(strings.TrimSpace(string(ee.Stderr)), "\n"); lines[0] != "" {
				reason += ": " + strings.Join(lines[:min(len(lines), 3)], " / ")
			}
		}
		return "", errors.New(reason)
	}
	f := strings.Fields(string(b))
	if len(f) == 0 || !versionRe.MatchString(pick(f)) {
		return "", fmt.Errorf("%s printed no version: %q", strings.Join(argv, " "), b)
	}
	return pick(f), nil
}

// anthropicStandIn answers as the Anthropic Messages API: a request that
// offers a Bash tool gets one Bash call, and a request that carries its
// result gets "Done.".
type anthropicStandIn struct{ tools atomic.Int64 }

func (s *anthropicStandIn) toolName() string  { return "Bash" }
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
		bash = bash || tool.Name == s.toolName()
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
		block = map[string]any{"type": "tool_use", "id": "toolu_1", "name": s.toolName(), "input": map[string]any{}}
		delta = map[string]any{"type": "input_json_delta", "partial_json": `{"command":"` + standInCommand + `","description":"Make the canary file"}`}
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

// responsesStandIn answers as the OpenAI Responses API: a request that offers
// a shell tool gets one shell call, and a request that carries its output gets
// "Done.".
type responsesStandIn struct{ tools atomic.Int64 }

func (s *responsesStandIn) toolName() string  { return "shell" }
func (s *responsesStandIn) toolRequests() int { return int(s.tools.Load()) }

func (s *responsesStandIn) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
		_, _ = io.WriteString(w, `{"object":"list","data":[]}`)
		return
	case r.Method != http.MethodPost || r.URL.Path != "/v1/responses":
		_, _ = io.WriteString(w, "{}")
		return
	}

	var req struct {
		Model string `json:"model"`
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
		Input []struct {
			Type string `json:"type"`
		} `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	shell := false
	for _, tool := range req.Tools {
		shell = shell || tool.Name == s.toolName()
	}
	output := false
	for _, item := range req.Input {
		output = output || item.Type == "function_call_output"
	}

	var deltas []map[string]any
	item := map[string]any{"type": "message", "id": "msg_canary", "role": "assistant", "status": "in_progress", "content": []any{}}
	done := map[string]any{"type": "message", "id": "msg_canary", "role": "assistant", "status": "completed",
		"content": []any{map[string]any{"type": "output_text", "text": "Done.", "annotations": []any{}}}}
	if shell && !output {
		s.tools.Add(1)
		args := `{"command":["bash","-lc","` + standInCommand + `"]}`
		item = map[string]any{"type": "function_call", "id": "fc_canary", "call_id": "call_canary", "name": s.toolName(), "arguments": "", "status": "in_progress"}
		done = map[string]any{"type": "function_call", "id": "fc_canary", "call_id": "call_canary", "name": s.toolName(), "arguments": args, "status": "completed"}
		deltas = []map[string]any{
			{"type": "response.function_call_arguments.delta", "item_id": "fc_canary", "output_index": 0, "delta": args},
			{"type": "response.function_call_arguments.done", "item_id": "fc_canary", "output_index": 0, "arguments": args},
		}
	} else {
		deltas = []map[string]any{
			{"type": "response.content_part.added", "item_id": "msg_canary", "output_index": 0, "content_index": 0,
				"part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}},
			{"type": "response.output_text.delta", "item_id": "msg_canary", "output_index": 0, "content_index": 0, "delta": "Done."},
			{"type": "response.output_text.done", "item_id": "msg_canary", "output_index": 0, "content_index": 0, "text": "Done."},
			{"type": "response.content_part.done", "item_id": "msg_canary", "output_index": 0, "content_index": 0,
				"part": map[string]any{"type": "output_text", "text": "Done.", "annotations": []any{}}},
		}
	}
	response := func(status string, output []any) map[string]any {
		return map[string]any{"id": "resp_canary", "object": "response", "model": req.Model, "status": status, "output": output,
			"usage": map[string]any{
				"input_tokens": 10, "input_tokens_details": map[string]int{"cached_tokens": 0},
				"output_tokens": 5, "output_tokens_details": map[string]int{"reasoning_tokens": 0},
				"total_tokens": 15,
			}}
	}

	events := []map[string]any{
		{"type": "response.created", "response": response("in_progress", []any{})},
		{"type": "response.output_item.added", "output_index": 0, "item": item},
	}
	events = append(events, deltas...)
	events = append(events,
		map[string]any{"type": "response.output_item.done", "output_index": 0, "item": done},
		map[string]any{"type": "response.completed", "response": response("completed", []any{done})},
	)
	w.Header().Set("Content-Type", "text/event-stream")
	f, _ := w.(http.Flusher)
	for i, e := range events {
		e["sequence_number"] = i
		b, err := json.Marshal(e)
		if err != nil {
			return
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e["type"], b); err != nil {
			return
		}
		if f != nil {
			f.Flush()
		}
	}
}

// chatStandIn answers as the OpenAI chat completions API: a request that
// offers a bash tool gets one bash call, and a request that carries its result
// gets "Done.".
type chatStandIn struct{ tools atomic.Int64 }

func (s *chatStandIn) toolName() string  { return "bash" }
func (s *chatStandIn) toolRequests() int { return int(s.tools.Load()) }

func (s *chatStandIn) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
		_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"stand-in","object":"model","owned_by":"canary"}]}`)
		return
	case r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions":
		_, _ = io.WriteString(w, "{}")
		return
	}

	var req struct {
		Model string `json:"model"`
		Tools []struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	bash := false
	for _, tool := range req.Tools {
		bash = bash || tool.Function.Name == s.toolName()
	}
	result := false
	for _, m := range req.Messages {
		result = result || m.Role == "tool"
	}

	deltas := []map[string]any{{"role": "assistant", "content": "Done."}}
	finish := "stop"
	if bash && !result {
		s.tools.Add(1)
		deltas = []map[string]any{
			{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{
				"index": 0, "id": "call_canary", "type": "function",
				"function": map[string]any{"name": s.toolName(), "arguments": ""},
			}}},
			{"tool_calls": []any{map[string]any{
				"index":    0,
				"function": map[string]any{"arguments": `{"command":"` + standInCommand + `","description":"Make the canary file"}`},
			}}},
		}
		finish = "tool_calls"
	}

	chunk := func(delta map[string]any, finish any) map[string]any {
		return map[string]any{
			"id": "chatcmpl-canary", "object": "chat.completion.chunk", "created": 0, "model": req.Model,
			"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}},
		}
	}
	var chunks []map[string]any
	for _, d := range deltas {
		chunks = append(chunks, chunk(d, nil))
	}
	last := chunk(map[string]any{}, finish)
	last["usage"] = map[string]int{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
	chunks = append(chunks, last)

	w.Header().Set("Content-Type", "text/event-stream")
	f, _ := w.(http.Flusher)
	for _, c := range chunks {
		b, err := json.Marshal(c)
		if err != nil {
			return
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return
		}
		if f != nil {
			f.Flush()
		}
	}
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}
