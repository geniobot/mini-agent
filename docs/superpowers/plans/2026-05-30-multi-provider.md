# Multi-Provider Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add support for OpenAI-compatible cloud providers alongside Ollama, with automatic escalation to a stronger model when the local model fails inside a goal.

**Architecture:** A new `OpenAIClient` implements the existing `llm.Client` interface. The `Loop` struct gains `defaultClient`/`fallbackClient` fields and an `activeProvider` config copy. During a goal, a `consecutiveNoTool` counter triggers a one-goal escalation when it hits `fallback_after_failures`. Backwards-compatible: if `providers:` is absent from config, the old `ollama:` block is used unchanged.

**Tech Stack:** Go stdlib only (`net/http`, `bufio`, `encoding/json`). No new module dependencies.

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `internal/llm/openai.go` | Create | `OpenAIClient` — SSE streaming + model listing |
| `internal/llm/openai_test.go` | Create | Unit tests with `httptest` mock server |
| `internal/config/config.go` | Modify | `ProviderConfig` struct, new `Config` fields, updated `Load()`/`Validate()` |
| `internal/config/config_test.go` | Modify | Provider loading + validation tests |
| `internal/agent/loop.go` | Modify | `defaultClient`/`fallbackClient`/`activeProvider`/`defaultProvider` fields, `buildClient()`, `chatOnceWith()`, `/model`, `/status`, `unloadModel()` |
| `internal/agent/doctor.go` | Modify | Multi-provider connectivity checks |
| `internal/agent/goal.go` | Modify | `consecutiveNoTool` counter + escalation block |
| `internal/agent/loop_test.go` | Modify | Escalation test |
| `cmd/mini-agent/main.go` | Modify | `--model` flag works with providers |
| `config.yaml` | Modify | Add commented `providers:` example |
| `scripts/smoke-test.sh` | Modify | `--provider cloud` flag |
| `README.md` | Modify | Multi-provider section |

---

## Task 1: OpenAI-compat LLM client

**Files:**
- Create: `internal/llm/openai.go`
- Create: `internal/llm/openai_test.go`

- [ ] **Step 1.1: Write the failing tests**

Create `internal/llm/openai_test.go`:

```go
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIClient_ChatStream_text(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing auth header")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}`)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":" world"},"finish_reason":null}]}`)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`)
		fmt.Fprintln(w, `data: [DONE]`)
	}))
	defer srv.Close()

	client := NewOpenAI(srv.URL, "test-key", "test-model")
	req := ChatRequest{Model: "test-model", Stream: true}

	var chunks []ChatChunk
	err := client.ChatStream(context.Background(), req, func(c ChatChunk) error {
		chunks = append(chunks, c)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var content strings.Builder
	var done bool
	for _, c := range chunks {
		content.WriteString(c.Message.Content)
		if c.Done {
			done = true
		}
	}
	if content.String() != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", content.String())
	}
	if !done {
		t.Error("expected Done=true in final chunk")
	}
}

func TestOpenAIClient_ChatStream_error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer srv.Close()

	client := NewOpenAI(srv.URL, "bad-key", "model")
	err := client.ChatStream(context.Background(), ChatRequest{}, func(ChatChunk) error { return nil })
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got: %v", err)
	}
}

func TestOpenAIClient_ListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "gpt-4o"},
				{"id": "gpt-4o-mini"},
			},
		})
	}))
	defer srv.Close()

	client := NewOpenAI(srv.URL, "key", "model")
	models, err := client.ListModels()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Errorf("expected 2 models, got %d", len(models))
	}
	if models[0] != "gpt-4o" {
		t.Errorf("expected gpt-4o, got %q", models[0])
	}
}
```

- [ ] **Step 1.2: Run tests — verify they fail**

```bash
cd /path/to/mini-agent && go test ./internal/llm/ -run TestOpenAI -v
```

Expected: compile error — `NewOpenAI` undefined.

- [ ] **Step 1.3: Implement `internal/llm/openai.go`**

```go
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mini-agent/internal/session"
)

// OpenAIClient implements Client and ModelLister against any OpenAI-compatible
// /v1/chat/completions endpoint (OpenRouter, Groq, LM Studio, Deepseek, etc.).
type OpenAIClient struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

func NewOpenAI(baseURL, apiKey, model string) *OpenAIClient {
	return &OpenAIClient{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     apiKey,
		Model:      model,
		HTTPClient: &http.Client{Timeout: 0},
	}
}

type openAIRequest struct {
	Model       string             `json:"model"`
	Messages    []session.Message  `json:"messages"`
	Stream      bool               `json:"stream"`
	Tools       []ToolSpec         `json:"tools,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
	MaxTokens   *int               `json:"max_tokens,omitempty"`
}

type openAIChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *OpenAIClient) ChatStream(ctx context.Context, req ChatRequest, onChunk func(ChatChunk) error) error {
	oaReq := openAIRequest{
		Model:    req.Model,
		Messages: req.Messages,
		Stream:   req.Stream,
		Tools:    req.Tools,
	}
	if t, ok := floatFromOptions(req.Options, "temperature"); ok {
		oaReq.Temperature = &t
	}
	if n, ok := intFromOptionsOAI(req.Options, "num_predict"); ok {
		oaReq.MaxTokens = &n
	}

	body, err := json.Marshal(oaReq)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("openai http %d: %s", resp.StatusCode, string(b))
	}

	// tool_calls[index] → accumulated arguments string
	toolArgsBuf := map[int]struct {
		id, name, args string
	}{}

	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line == "data: [DONE]" {
			if line == "data: [DONE]" {
				// Assemble any accumulated tool calls into the final chunk.
				var tcs []session.ToolCall
				for _, tc := range toolArgsBuf {
					var args map[string]any
					if tc.args != "" {
						_ = json.Unmarshal([]byte(tc.args), &args)
					}
					tcs = append(tcs, session.ToolCall{
						Function: session.ToolFunction{Name: tc.name, Arguments: args},
					})
				}
				return onChunk(ChatChunk{
					Message: session.Message{Role: "assistant", ToolCalls: tcs},
					Done:    true,
				})
			}
			continue
		}
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var chunk openAIChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil {
			return fmt.Errorf("openai error: %s", chunk.Error.Message)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		// Stream text content.
		if delta.Content != "" {
			if err := onChunk(ChatChunk{Message: session.Message{Role: "assistant", Content: delta.Content}}); err != nil {
				return err
			}
		}

		// Accumulate tool call argument fragments.
		for _, tc := range delta.ToolCalls {
			entry := toolArgsBuf[tc.Index]
			if tc.ID != "" {
				entry.id = tc.ID
			}
			if tc.Function.Name != "" {
				entry.name = tc.Function.Name
			}
			entry.args += tc.Function.Arguments
			toolArgsBuf[tc.Index] = entry
		}

		// finish_reason signals end — [DONE] line follows, but handle early stops too.
		if chunk.Choices[0].FinishReason != nil && *chunk.Choices[0].FinishReason != "" {
			continue // wait for [DONE] to deliver tool calls
		}
	}
	return scanner.Err()
}

// ListModels fetches the model list from /v1/models.
func (c *OpenAIClient) ListModels() ([]string, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	names := make([]string, len(payload.Data))
	for i, m := range payload.Data {
		names[i] = m.ID
	}
	return names, nil
}

func floatFromOptions(opts map[string]any, key string) (float64, bool) {
	if opts == nil {
		return 0, false
	}
	switch v := opts[key].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	}
	return 0, false
}

func intFromOptionsOAI(opts map[string]any, key string) (int, bool) {
	if opts == nil {
		return 0, false
	}
	switch v := opts[key].(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	}
	return 0, false
}
```

- [ ] **Step 1.4: Run tests — verify they pass**

```bash
go test ./internal/llm/ -run TestOpenAI -v
```

Expected: all 3 tests PASS.

- [ ] **Step 1.5: Run full test suite — verify no regressions**

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 1.6: Commit**

```bash
git add internal/llm/openai.go internal/llm/openai_test.go
git commit -m "feat: add OpenAI-compatible LLM client (SSE streaming)"
```

---

## Task 2: Config — ProviderConfig struct and loading

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 2.1: Write the failing tests**

Add to `internal/config/config_test.go`:

```go
func TestLoad_providers(t *testing.T) {
	yaml := `
providers:
  local:
    type: ollama
    host: "http://localhost:11434"
    model: "qwen2.5-coder:1.5b"
    stream: true
    options:
      temperature: 0.1
      num_ctx: 4096
  cloud:
    type: openai_compat
    base_url: "https://openrouter.ai/api/v1"
    api_key_env: "OPENROUTER_API_KEY"
    model: "google/gemma-2-27b-it"
    stream: true
default_provider: local
fallback_provider: cloud
fallback_after_failures: 2
agent:
  max_history: 8
  max_goal_steps: 10
`
	f := writeTempConfig(t, yaml)
	cfg, err := Load(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(cfg.Providers))
	}
	local := cfg.Providers["local"]
	if local.Type != "ollama" {
		t.Errorf("expected type ollama, got %q", local.Type)
	}
	if local.Model != "qwen2.5-coder:1.5b" {
		t.Errorf("unexpected local model: %q", local.Model)
	}
	cloud := cfg.Providers["cloud"]
	if cloud.Type != "openai_compat" {
		t.Errorf("expected type openai_compat, got %q", cloud.Type)
	}
	if cloud.APIKeyEnv != "OPENROUTER_API_KEY" {
		t.Errorf("unexpected api_key_env: %q", cloud.APIKeyEnv)
	}
	if cfg.DefaultProvider != "local" {
		t.Errorf("expected default_provider=local, got %q", cfg.DefaultProvider)
	}
	if cfg.FallbackProvider != "cloud" {
		t.Errorf("expected fallback_provider=cloud, got %q", cfg.FallbackProvider)
	}
	if cfg.FallbackAfterFailures != 2 {
		t.Errorf("expected fallback_after_failures=2, got %d", cfg.FallbackAfterFailures)
	}
}

func TestLoad_fallbackAfterFailures_default(t *testing.T) {
	yaml := `
providers:
  main:
    type: ollama
    host: "http://localhost:11434"
    model: "qwen2.5-coder:1.5b"
default_provider: main
agent:
  max_history: 8
  max_goal_steps: 10
`
	f := writeTempConfig(t, yaml)
	cfg, err := Load(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.FallbackAfterFailures != 2 {
		t.Errorf("expected default fallback_after_failures=2, got %d", cfg.FallbackAfterFailures)
	}
}

// writeTempConfig writes yaml to a temp file and returns its path.
// Add this helper if it does not already exist in the test file.
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "config*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}
```

Check whether `writeTempConfig` already exists in the test file before adding it — if so, skip that helper and just use the existing one.

- [ ] **Step 2.2: Run tests — verify they fail**

```bash
go test ./internal/config/ -run "TestLoad_providers|TestLoad_fallbackAfterFailures" -v
```

Expected: compile errors — `ProviderConfig`, `Providers`, `DefaultProvider` undefined.

- [ ] **Step 2.3: Add `ProviderConfig` and new fields to `internal/config/config.go`**

Add the `ProviderConfig` struct and new fields to `Config`, right after the `TelegramConfig` block:

```go
// ProviderConfig describes a single LLM backend.
// type "ollama"        — local Ollama instance (host + ollama streaming)
// type "openai_compat" — any OpenAI-compatible /v1/chat/completions endpoint
type ProviderConfig struct {
	Type      string         `yaml:"type"`        // "ollama" | "openai_compat"
	Host      string         `yaml:"host"`        // ollama: base URL
	BaseURL   string         `yaml:"base_url"`    // openai_compat: base URL
	APIKeyEnv string         `yaml:"api_key_env"` // openai_compat: env var holding the key
	Model     string         `yaml:"model"`
	Stream    bool           `yaml:"stream"`
	KeepAlive string         `yaml:"keep_alive"`
	Options   map[string]any `yaml:"options"`
}
```

Add new fields to the `Config` struct (after `Telegram`):

```go
// Multi-provider fields — optional. When absent, Ollama config is used.
Providers             map[string]ProviderConfig `yaml:"providers"`
DefaultProvider       string                    `yaml:"default_provider"`
FallbackProvider      string                    `yaml:"fallback_provider"`
FallbackAfterFailures int                       `yaml:"fallback_after_failures"`
```

In `Load()`, after the existing defaults block, add:

```go
if len(cfg.Providers) > 0 && cfg.FallbackAfterFailures <= 0 {
	cfg.FallbackAfterFailures = 2
}
```

- [ ] **Step 2.4: Run tests — verify they pass**

```bash
go test ./internal/config/ -run "TestLoad_providers|TestLoad_fallbackAfterFailures" -v
```

Expected: PASS.

- [ ] **Step 2.5: Run full test suite**

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 2.6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add ProviderConfig struct and multi-provider config loading"
```

---

## Task 3: Config — Validate() for providers

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 3.1: Write the failing tests**

Add to `internal/config/config_test.go`:

```go
func TestValidate_providers_valid(t *testing.T) {
	t.Setenv("MY_API_KEY", "sk-test")
	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"local": {Type: "ollama", Host: "http://localhost:11434", Model: "qwen2.5-coder:1.5b"},
			"cloud": {Type: "openai_compat", BaseURL: "https://openrouter.ai/api/v1", APIKeyEnv: "MY_API_KEY", Model: "google/gemma-2-27b-it"},
		},
		DefaultProvider:  "local",
		FallbackProvider: "cloud",
		Agent:            AgentConfig{MaxHistory: 8, MaxGoalSteps: 10},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidate_providers_unknownDefault(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"local": {Type: "ollama", Host: "http://localhost:11434", Model: "x"},
		},
		DefaultProvider: "missing",
		Agent:           AgentConfig{MaxHistory: 8, MaxGoalSteps: 10},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for unknown default_provider")
	}
	if !strings.Contains(err.Error(), "default_provider") {
		t.Errorf("error should mention default_provider, got: %v", err)
	}
}

func TestValidate_providers_missingAPIKey_default(t *testing.T) {
	os.Unsetenv("MISSING_KEY")
	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"cloud": {Type: "openai_compat", BaseURL: "https://api.example.com/v1", APIKeyEnv: "MISSING_KEY", Model: "x"},
		},
		DefaultProvider: "cloud",
		Agent:           AgentConfig{MaxHistory: 8, MaxGoalSteps: 10},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing API key env var on default provider")
	}
	if !strings.Contains(err.Error(), "MISSING_KEY") {
		t.Errorf("error should mention env var name, got: %v", err)
	}
}

func TestValidate_providers_missingAPIKey_fallback_ok(t *testing.T) {
	os.Unsetenv("MISSING_FALLBACK_KEY")
	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"local": {Type: "ollama", Host: "http://localhost:11434", Model: "x"},
			"cloud": {Type: "openai_compat", BaseURL: "https://api.example.com/v1", APIKeyEnv: "MISSING_FALLBACK_KEY", Model: "x"},
		},
		DefaultProvider:  "local",
		FallbackProvider: "cloud",
		Agent:            AgentConfig{MaxHistory: 8, MaxGoalSteps: 10},
	}
	// Missing API key on fallback-only provider is allowed (warning, not error).
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for missing fallback key, got: %v", err)
	}
}
```

- [ ] **Step 3.2: Run tests — verify they fail**

```bash
go test ./internal/config/ -run "TestValidate_providers" -v
```

Expected: FAIL — validation doesn't know about providers yet.

- [ ] **Step 3.3: Update `Validate()` in `internal/config/config.go`**

Replace the existing `Validate()` function body:

```go
func (c *Config) Validate() error {
	var errs []string

	if len(c.Providers) == 0 {
		// Legacy path: validate ollama block as before.
		if c.Ollama.Host == "" {
			errs = append(errs, "ollama.host is required")
		} else if !strings.HasPrefix(c.Ollama.Host, "http://") && !strings.HasPrefix(c.Ollama.Host, "https://") {
			errs = append(errs, "ollama.host must start with http:// or https://")
		}
		if c.Ollama.Model == "" {
			errs = append(errs, "ollama.model is required")
		}
		if nc := intFromOptions(c.Ollama.Options, "num_ctx"); nc != 0 && nc < 64 {
			errs = append(errs, "ollama.options.num_ctx must be >= 64")
		}
		if np := intFromOptions(c.Ollama.Options, "num_predict"); np != 0 && np < 1 {
			errs = append(errs, "ollama.options.num_predict must be >= 1")
		}
	} else {
		// Multi-provider path.
		if c.DefaultProvider == "" {
			errs = append(errs, "default_provider is required when providers: is set")
		} else if _, ok := c.Providers[c.DefaultProvider]; !ok {
			errs = append(errs, fmt.Sprintf("default_provider %q does not name a known provider", c.DefaultProvider))
		} else {
			def := c.Providers[c.DefaultProvider]
			if def.Type == "openai_compat" && def.APIKeyEnv != "" && os.Getenv(def.APIKeyEnv) == "" {
				errs = append(errs, fmt.Sprintf("default provider %q requires env var %s to be set", c.DefaultProvider, def.APIKeyEnv))
			}
			if def.Type == "openai_compat" && def.BaseURL == "" {
				errs = append(errs, fmt.Sprintf("provider %q: base_url is required for type openai_compat", c.DefaultProvider))
			}
			if def.Model == "" {
				errs = append(errs, fmt.Sprintf("provider %q: model is required", c.DefaultProvider))
			}
		}
		if c.FallbackProvider != "" {
			if _, ok := c.Providers[c.FallbackProvider]; !ok {
				errs = append(errs, fmt.Sprintf("fallback_provider %q does not name a known provider", c.FallbackProvider))
			}
			if c.FallbackProvider == c.DefaultProvider {
				errs = append(errs, "fallback_provider must differ from default_provider")
			}
		}
	}

	if c.Agent.MaxHistory < 1 {
		errs = append(errs, "agent.max_history must be >= 1")
	}
	if c.Agent.MaxGoalSteps < 1 {
		errs = append(errs, "agent.max_goal_steps must be >= 1")
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}
```

- [ ] **Step 3.4: Run tests — verify they pass**

```bash
go test ./internal/config/ -v
```

Expected: all pass (including pre-existing tests).

- [ ] **Step 3.5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: validate multi-provider config (default/fallback provider, API key env)"
```

---

## Task 4: Agent Loop — provider-aware wiring

**Files:**
- Modify: `internal/agent/loop.go`
- Modify: `internal/agent/doctor.go`
- Modify: `cmd/mini-agent/main.go`

- [ ] **Step 4.1: Add `defaultClient`, `fallbackClient`, `activeProvider`, `defaultProvider` fields to the `Loop` struct in `loop.go`**

Find the struct block (starts around line 27) and add after the existing `client` field:

```go
client         llm.Client            // current active client (swapped during goal escalation)
defaultClient  llm.Client            // original client — restored after each goal
fallbackClient llm.Client            // nil when no fallback is configured
activeProvider config.ProviderConfig // mirrors which client is active
defaultProvider config.ProviderConfig
```

The existing `client` field declaration should be **replaced** (not added alongside). It now carries the dual meaning of "active client."

- [ ] **Step 4.2: Add `buildClient()` helper to `loop.go`**

Add this function before `New()`:

```go
func buildClient(p config.ProviderConfig) llm.Client {
	switch p.Type {
	case "openai_compat":
		return llm.NewOpenAI(p.BaseURL, os.Getenv(p.APIKeyEnv), p.Model)
	default: // "ollama"
		return llm.NewOllama(p.Host)
	}
}
```

- [ ] **Step 4.3: Update `New()` in `loop.go` to use provider-aware construction**

Replace the existing `New()` function:

```go
func New(cfg *config.Config) *Loop {
	var defProvider config.ProviderConfig
	if len(cfg.Providers) == 0 {
		// Backwards-compat: synthesize a ProviderConfig from the ollama block.
		defProvider = config.ProviderConfig{
			Type:      "ollama",
			Host:      cfg.Ollama.Host,
			Model:     cfg.Ollama.Model,
			Stream:    cfg.Ollama.Stream,
			KeepAlive: cfg.Ollama.KeepAlive,
			Options:   cfg.Ollama.Options,
		}
	} else {
		defProvider = cfg.Providers[cfg.DefaultProvider]
	}

	defClient := buildClient(defProvider)
	var fbClient llm.Client
	var fbProvider config.ProviderConfig
	if cfg.FallbackProvider != "" {
		fbProvider = cfg.Providers[cfg.FallbackProvider]
		fbClient = buildClient(fbProvider)
	}

	ctx := numCtx(defProvider.Options)
	return &Loop{
		cfg:             cfg,
		client:          defClient,
		defaultClient:   defClient,
		fallbackClient:  fbClient,
		activeProvider:  defProvider,
		defaultProvider: defProvider,
		session:         session.New(cfg.Agent.SystemPrompt, cfg.Agent.MaxHistory, ctx),
		registry:        tools.New(cfg.Tools),
		maxCtx:          ctx,
	}
}
```

- [ ] **Step 4.4: Update `chatOnceWith()` to use `activeProvider` instead of `cfg.Ollama.*`**

In `chatOnceWith()`, replace the `ChatStream` call's request struct:

```go
err := l.client.ChatStream(ctx, llm.ChatRequest{
	Model:     l.activeProvider.Model,
	Messages:  msgs,
	Stream:    l.activeProvider.Stream,
	Tools:     reqTools,
	Options:   l.activeProvider.Options,
	KeepAlive: l.activeProvider.KeepAlive,
}, func(chunk llm.ChatChunk) error {
```

- [ ] **Step 4.5: Update `/model` command, `printStatus()`, and `unloadModel()` to use `activeProvider`**

Replace the `/model` command block:

```go
case input == "/model":
    fmt.Printf("  current model: %s\n", l.activeProvider.Model)
    continue
case strings.HasPrefix(input, "/model "):
    name := strings.TrimSpace(strings.TrimPrefix(input, "/model "))
    l.activeProvider.Model = name
    l.defaultProvider.Model = name
    l.printf("%s[model → %s]%s\n", ansiTeal, name, ansiReset)
    continue
```

Replace `printStatus()`:

```go
func (l *Loop) printStatus() {
	const sep = "──────────────────────────────────────────────────"
	fmt.Printf("\n%s%s%s\n", ansiDim, sep, ansiReset)
	providerName := l.cfg.DefaultProvider
	if providerName == "" {
		providerName = "ollama"
	}
	fmt.Printf("  %s◆%s  provider  %s (%s)\n", ansiTeal, ansiReset, providerName, l.activeProvider.Type)
	fmt.Printf("  %s◆%s  model     %s\n", ansiTeal, ansiReset, l.activeProvider.Model)
	var endpoint string
	if l.activeProvider.Type == "ollama" {
		endpoint = strings.TrimPrefix(l.activeProvider.Host, "http://")
		endpoint = strings.TrimPrefix(endpoint, "https://")
	} else {
		endpoint = l.activeProvider.BaseURL
	}
	fmt.Printf("  %s◆%s  endpoint  %s\n", ansiTeal, ansiReset, endpoint)
	tokens := session.EstimateTokens(l.session.Snapshot())
	fmt.Printf("  %s◆%s  tokens    %d / %d\n", ansiTeal, ansiReset, tokens, l.maxCtx)
	fmt.Printf("  %s◆%s  history   %d messages\n", ansiTeal, ansiReset, len(l.session.Messages))
	fmt.Printf("  %s◆%s  timeout   %ds / step\n", ansiTeal, ansiReset, l.cfg.Agent.StepTimeoutSeconds)
	fmt.Printf("  %s◆%s  max steps %d / goal\n", ansiTeal, ansiReset, l.cfg.Agent.MaxGoalSteps)
	if l.savePath != "" {
		fmt.Printf("  %s◆%s  session   %s\n", ansiTeal, ansiReset, l.savePath)
	}
	fmt.Printf("%s%s%s\n\n", ansiDim, sep, ansiReset)
}
```

Replace model references in `unloadModel()`:

```go
func (l *Loop) unloadModel() {
	u, ok := l.client.(llm.Unloader)
	if !ok {
		l.printf("%s[unload not supported by this backend]%s\n", ansiDim, ansiReset)
		return
	}
	l.printf("%s[unloading %s...]%s\n", ansiDim, l.activeProvider.Model, ansiReset)
	if err := u.Unload(l.activeProvider.Model); err != nil {
		l.printf("%s[unload failed: %v]%s\n", ansiDim, err, ansiReset)
		return
	}
	l.printf("%s[model unloaded — RAM freed]%s\n", ansiGreen, ansiReset)
}
```

Also fix the model-not-found warning in `pingOllama` (search for `cfg.Ollama.Model` references in loop.go and replace with `l.activeProvider.Model`).

- [ ] **Step 4.6: Update `--model` flag in `cmd/mini-agent/main.go`**

Find the `--model` flag handling (currently `cfg.Ollama.Model = *modelFlag`) and replace with:

```go
if *modelFlag != "" {
	if len(cfg.Providers) == 0 {
		cfg.Ollama.Model = *modelFlag
	} else {
		if p, ok := cfg.Providers[cfg.DefaultProvider]; ok {
			p.Model = *modelFlag
			cfg.Providers[cfg.DefaultProvider] = p
		}
	}
}
```

- [ ] **Step 4.7: Update `doctor.go` for multi-provider**

Replace `RunDoctor()` with a provider-aware version:

```go
func RunDoctor(cfg *config.Config) {
	const sep = "──────────────────────────────────────────────────"
	fmt.Printf("\n%s%s%s\n", ansiDim, sep, ansiReset)
	fmt.Printf("  %smini-agent doctor%s\n", ansiTeal, ansiReset)
	fmt.Printf("%s%s%s\n\n", ansiDim, sep, ansiReset)

	ok := true

	if err := cfg.Validate(); err != nil {
		fmt.Printf("  %s✗%s  config    %v\n", ansiRed, ansiReset, err)
		ok = false
	} else {
		fmt.Printf("  %s✓%s  config    OK\n", ansiGreen, ansiReset)
	}

	if len(cfg.Providers) == 0 {
		// Legacy: check Ollama only.
		host := strings.TrimPrefix(cfg.Ollama.Host, "http://")
		host = strings.TrimPrefix(host, "https://")
		if err := llm.Ping(cfg.Ollama.Host); err != nil {
			fmt.Printf("  %s✗%s  ollama    not reachable at %s\n", ansiRed, ansiReset, host)
			fmt.Printf("           is Ollama running? try: ollama serve\n")
			ok = false
		} else {
			fmt.Printf("  %s✓%s  ollama    reachable at %s\n", ansiGreen, ansiReset, host)
			var client llm.Client = llm.NewOllama(cfg.Ollama.Host)
			if ml, ok2 := client.(llm.ModelLister); ok2 {
				models, err := ml.ListModels()
				if err != nil {
					fmt.Printf("  %s?%s  models    could not list: %v\n", ansiYellow, ansiReset, err)
				} else {
					fmt.Printf("  %s✓%s  models    %d available\n", ansiGreen, ansiReset, len(models))
					if slices.Contains(models, cfg.Ollama.Model) {
						fmt.Printf("  %s✓%s  model     %q ready\n", ansiGreen, ansiReset, cfg.Ollama.Model)
					} else {
						fmt.Printf("  %s✗%s  model     %q not found — run: ollama pull %s\n",
							ansiRed, ansiReset, cfg.Ollama.Model, cfg.Ollama.Model)
						ok = false
					}
				}
			}
		}
	} else {
		// Multi-provider: check each named provider.
		for name, p := range cfg.Providers {
			tag := name
			switch p.Type {
			case "ollama":
				if err := llm.Ping(p.Host); err != nil {
					fmt.Printf("  %s✗%s  %-12s not reachable at %s\n", ansiRed, ansiReset, tag, p.Host)
					ok = false
				} else {
					fmt.Printf("  %s✓%s  %-12s ollama OK at %s — model: %s\n", ansiGreen, ansiReset, tag, p.Host, p.Model)
				}
			case "openai_compat":
				apiKey := os.Getenv(p.APIKeyEnv)
				if apiKey == "" {
					fmt.Printf("  %s✗%s  %-12s env var %s is not set\n", ansiRed, ansiReset, tag, p.APIKeyEnv)
					if name == cfg.DefaultProvider {
						ok = false
					}
				} else {
					fmt.Printf("  %s✓%s  %-12s api key set (%s) — model: %s\n", ansiGreen, ansiReset, tag, p.APIKeyEnv, p.Model)
				}
			}
		}
	}

	fmt.Printf("\n%s%s%s\n", ansiDim, sep, ansiReset)
	if ok {
		fmt.Printf("  %s✓  all checks passed — ready to run%s\n", ansiGreen, ansiReset)
	} else {
		fmt.Printf("  %s✗  fix the issues above before running mini-agent%s\n", ansiRed, ansiReset)
	}
	fmt.Printf("%s%s%s\n\n", ansiDim, sep, ansiReset)
}
```

Add `"os"` to the imports if not present.

- [ ] **Step 4.8: Build and run full tests**

```bash
go build ./... && go test ./...
```

Expected: compiles and all tests pass.

- [ ] **Step 4.9: Commit**

```bash
git add internal/agent/loop.go internal/agent/doctor.go cmd/mini-agent/main.go
git commit -m "feat: wire multi-provider clients into agent Loop (buildClient, activeProvider)"
```

---

## Task 5: Goal escalation logic

**Files:**
- Modify: `internal/agent/goal.go`
- Modify: `internal/agent/loop_test.go`

- [ ] **Step 5.1: Write the failing test**

Add to `internal/agent/loop_test.go`:

```go
// mockClient records calls and returns a canned response each time.
type mockClient struct {
	responses []string
	calls     int
}

func (m *mockClient) ChatStream(_ context.Context, _ llm.ChatRequest, onChunk func(llm.ChatChunk) error) error {
	if m.calls >= len(m.responses) {
		return onChunk(llm.ChatChunk{Message: session.Message{Role: "assistant", Content: "DONE: finished"}, Done: true})
	}
	content := m.responses[m.calls]
	m.calls++
	_ = onChunk(llm.ChatChunk{Message: session.Message{Role: "assistant", Content: content}})
	return onChunk(llm.ChatChunk{Done: true})
}

func TestGoalEscalation(t *testing.T) {
	// Primary returns prose (no tool call) twice, triggering escalation.
	primary := &mockClient{responses: []string{"let me think...", "still thinking..."}}
	// Fallback returns DONE immediately.
	fallback := &mockClient{responses: []string{"DONE: created file"}}

	cfg := &config.Config{
		Ollama: config.OllamaConfig{Host: "http://localhost:11434", Model: "test"},
		Agent:  config.AgentConfig{MaxHistory: 8, MaxGoalSteps: 10, StepTimeoutSeconds: 30},
		Providers: map[string]config.ProviderConfig{
			"local": {Type: "ollama", Host: "http://localhost:11434", Model: "test"},
			"cloud": {Type: "openai_compat", BaseURL: "https://x.com", Model: "big"},
		},
		DefaultProvider:      "local",
		FallbackProvider:     "cloud",
		FallbackAfterFailures: 2,
	}
	loop := New(cfg)
	loop.client = primary
	loop.defaultClient = primary
	loop.fallbackClient = fallback
	loop.SetQuiet(true)

	ctx := context.Background()
	err := loop.runGoal(ctx, "create a file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fallback.calls == 0 {
		t.Error("expected fallback client to be called after escalation")
	}
	// After the goal, active client should be restored to primary.
	if loop.client != primary {
		t.Error("expected client to be restored to primary after goal")
	}
}
```

- [ ] **Step 5.2: Run test — verify it fails**

```bash
go test ./internal/agent/ -run TestGoalEscalation -v
```

Expected: FAIL — escalation not implemented yet. The fallback client is never called.

- [ ] **Step 5.3: Implement escalation in `internal/agent/goal.go`**

In `runGoal()`, add a `consecutiveNoTool` counter and the escalation block. Find the section after the tool-parsing block (where `len(resp.ToolCalls) == 0` is handled) and the section where notes are accumulated.

Add the counter declaration near the top of `runGoal()`, alongside `var notes string` and `var prevSig stepSig`:

```go
var consecutiveNoTool int
```

In the section that handles no tool calls (after the `if len(resp.ToolCalls) == 0 {` block and the `continue`), add before the `continue`:

```go
consecutiveNoTool++
if consecutiveNoTool >= l.cfg.FallbackAfterFailures && l.fallbackClient != nil {
    fbProvider := l.cfg.Providers[l.cfg.FallbackProvider]
    l.client = l.fallbackClient
    l.activeProvider = fbProvider
    defer func() {
        l.client = l.defaultClient
        l.activeProvider = l.defaultProvider
    }()
    l.printf("%s[escalated to fallback provider for this goal]%s\n", ansiYellow, ansiReset)
    consecutiveNoTool = 0
}
```

Reset `consecutiveNoTool` when a tool call succeeds — add after `prevSig = sig`:

```go
consecutiveNoTool = 0
```

- [ ] **Step 5.4: Run test — verify it passes**

```bash
go test ./internal/agent/ -run TestGoalEscalation -v
```

Expected: PASS.

- [ ] **Step 5.5: Run full test suite**

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 5.6: Commit**

```bash
git add internal/agent/goal.go internal/agent/loop_test.go
git commit -m "feat: auto-escalate to fallback provider after N consecutive no-tool steps in goal"
```

---

## Task 6: Config example + README + smoke test

**Files:**
- Modify: `config.yaml`
- Modify: `README.md`
- Modify: `scripts/smoke-test.sh`

- [ ] **Step 6.1: Add commented providers example to `config.yaml`**

At the top of `config.yaml`, add a commented block before the existing `ollama:` section:

```yaml
# ── Multi-provider (optional) ──────────────────────────────────────────────────
# Uncomment to use named providers instead of the ollama: block below.
# Supports "ollama" (local) and "openai_compat" (any OpenAI-compat API).
#
# providers:
#   local:
#     type: ollama
#     host: "http://localhost:11434"
#     model: "qwen2.5-coder:1.5b"
#     options:
#       temperature: 0.1
#       num_ctx: 4096
#       num_predict: 1024
#   cloud:
#     type: openai_compat
#     base_url: "https://openrouter.ai/api/v1"
#     api_key_env: "OPENROUTER_API_KEY"  # export OPENROUTER_API_KEY=sk-or-...
#     model: "google/gemma-2-27b-it"
#     stream: true
#
# default_provider: local
# fallback_provider: cloud       # escalate to cloud after N failed steps in /goal
# fallback_after_failures: 2
#
# Cloud-only (e.g. Raspberry Pi without Ollama):
#   default_provider: cloud
#   (omit fallback_provider)
# ───────────────────────────────────────────────────────────────────────────────
```

- [ ] **Step 6.2: Add multi-provider section to README**

Find the configuration section in `README.md` and add a "Multi-Provider Support" subsection that explains:
- The three deployment modes (local-only, cloud-only, mixed)
- The `providers:`, `default_provider:`, `fallback_provider:`, `fallback_after_failures:` keys
- The `api_key_env:` pattern (never put the key in the file)
- Example snippets for each deployment mode (copy from the spec doc)
- A note that the old `ollama:` block continues to work unchanged

- [ ] **Step 6.3: Update `scripts/smoke-test.sh` to support `--provider cloud`**

After the `--quick` flag parsing block, add:

```bash
PROVIDER="${PROVIDER:-ollama}"
for arg in "$@"; do
  [ "$arg" = "--provider" ] && shift && PROVIDER="${1:-ollama}" && shift || true
done
```

Wrap the Ollama reachability check in a conditional:

```bash
if [ "$PROVIDER" != "cloud" ]; then
  if ! curl -s http://localhost:11434/api/tags >/dev/null 2>&1; then
    echo -e "${RED}Ollama is not reachable at localhost:11434 — start it with 'ollama serve'${RST}"
    exit 1
  fi
fi
```

Add at the top of the usage comment:

```bash
#   PROVIDER=cloud OPENROUTER_API_KEY=sk-... ./scripts/smoke-test.sh
```

Refactor the config generation so the `agent:` and `tools:` sections are shared. Replace the current single `cat > "$CONFIG"` block with:

```bash
# Shared agent + tools YAML (used by both ollama and openai_compat configs)
AGENT_TOOLS_YAML='agent:
  max_history: 8
  step_timeout_seconds: 120
  max_goal_steps: 12
  goal_max_steps: 20
  system_prompt: |
    You are a local coding assistant.
    For file and shell operations output ONLY a JSON object — no explanation, no markdown.
    For questions reply in plain text.
    Write a new file: {"name":"write_file","arguments":{"path":"f.py","content":"..."}}
    Read a file: {"name":"read_file","arguments":{"path":"f.py"}}
    Edit part of a file: {"name":"edit_file","arguments":{"path":"f.py","old_string":"a","new_string":"b"}}
    Append: {"name":"append_file","arguments":{"path":"f.py","content":"..."}}
    List dir: {"name":"list_dir","arguments":{"path":"."}}
    Search files: {"name":"search_files","arguments":{"pattern":"TODO","path":"."}}
    Run command: {"name":"run_command","arguments":{"command":"ls","args":[]}}
    Use write_file for new files, edit_file to change existing files.
tools:
  enabled: true
  use_native_tools: false
  enable_read_file: true
  enable_write_file: true
  enable_edit_file: true
  enable_append_file: true
  enable_list_dir: true
  enable_run_command: true
  enable_search_files: true
  confirm_run_command: false
  confirm_write_file: false
  allowed_commands: [ls, cat, pwd, grep, find, head, tail]'

if [ "$PROVIDER" = "cloud" ]; then
  MODEL="${MODEL:-google/gemma-2-27b-it}"
  BASE_URL="${OPENAI_BASE_URL:-https://openrouter.ai/api/v1}"
  KEY_ENV="${OPENAI_KEY_ENV:-OPENROUTER_API_KEY}"
  cat > "$CONFIG" <<EOF
providers:
  cloud:
    type: openai_compat
    base_url: "$BASE_URL"
    api_key_env: "$KEY_ENV"
    model: "$MODEL"
    stream: true
default_provider: cloud
$AGENT_TOOLS_YAML
EOF
else
  cat > "$CONFIG" <<EOF
ollama:
  host: "http://localhost:11434"
  model: "$MODEL"
  stream: true
  options:
    temperature: 0.1
    num_ctx: 4096
    num_predict: 1024
$AGENT_TOOLS_YAML
EOF
fi
```

Also add a guard at the top of the smoke test: if `PROVIDER=cloud` and the key env var is unset, print a warning and exit 0 (skip rather than fail CI):

```bash
if [ "$PROVIDER" = "cloud" ]; then
  KEY_ENV="${OPENAI_KEY_ENV:-OPENROUTER_API_KEY}"
  if [ -z "${!KEY_ENV}" ]; then
    echo -e "${YEL}Skipping cloud smoke test — $KEY_ENV is not set${RST}"
    exit 0
  fi
fi
```

- [ ] **Step 6.4: Build and run smoke test to verify nothing broke**

```bash
go build -o bin/mini-agent ./cmd/mini-agent && bash scripts/smoke-test.sh
```

Expected: 12/12 pass (Ollama path unchanged).

- [ ] **Step 6.5: Commit**

```bash
git add config.yaml README.md scripts/smoke-test.sh
git commit -m "docs: multi-provider config example, README section, smoke-test cloud flag"
```

---

## Task 7: Final integration check

- [ ] **Step 7.1: Run the full test suite one more time**

```bash
go test ./... -count=1
```

Expected: all pass.

- [ ] **Step 7.2: Build and verify `--doctor` works with providers config**

Create a temp config with one ollama provider and run:

```bash
cat > /tmp/test-provider.yaml <<'EOF'
providers:
  local:
    type: ollama
    host: "http://localhost:11434"
    model: "qwen2.5-coder:1.5b"
default_provider: local
agent:
  max_history: 8
  max_goal_steps: 10
  system_prompt: "You are a test assistant."
tools:
  enabled: false
EOF
./bin/mini-agent --config /tmp/test-provider.yaml --doctor
```

Expected: config OK, ollama reachable (if running), model check shown.

- [ ] **Step 7.3: Run smoke test**

```bash
bash scripts/smoke-test.sh
```

Expected: 12/12 pass.

- [ ] **Step 7.4: Final commit if anything was adjusted**

```bash
git add -A && git status
# Only commit if there are unstaged changes from the integration check.
```
