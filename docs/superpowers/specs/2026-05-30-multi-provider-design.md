# Multi-Provider Support — Design Spec
Date: 2026-05-30

## Goal

Allow mini-agent to use more than one LLM backend: local Ollama, any OpenAI-compatible
API (OpenRouter, Groq, LM Studio, Deepseek, Together, etc.), or a mix. Support three
deployment modes without changing code:

- **Local-only** — Ollama, no internet required (current default)
- **Cloud-only** — one OpenAI-compat provider (e.g. Raspberry Pi + frontier model)
- **Mixed** — local model for normal use, automatic escalation to a stronger cloud model
  when the local model fails repeatedly inside a goal

## Non-goals

- Anthropic-native API format (reachable via OpenRouter as `anthropic/claude-*`)
- Streaming provider *selection* mid-response
- Cost tracking or token budgets
- More than two active providers at once (default + fallback is sufficient)

---

## Config Shape

New top-level keys sit alongside the existing `ollama:` block.
If `providers:` is absent, the agent behaves exactly as before (backwards-compatible).

```yaml
providers:
  local:
    type: ollama
    host: "http://localhost:11434"
    model: "qwen2.5-coder:1.5b"
    options:
      temperature: 0.1
      num_ctx: 4096
      num_predict: 1024
  cloud:
    type: openai_compat
    base_url: "https://openrouter.ai/api/v1"
    api_key_env: "OPENROUTER_API_KEY"   # env var name — key never goes in the file
    model: "google/gemma-2-27b-it"
    stream: true

default_provider: local
fallback_provider: cloud
fallback_after_failures: 2   # consecutive tool-call failures before escalating
```

### Provider types

| `type` | Description |
|---|---|
| `ollama` | Local Ollama instance. Uses `host` + Ollama JSON streaming. |
| `openai_compat` | Any OpenAI-compatible `/v1/chat/completions` endpoint. Uses `base_url` + SSE streaming. |

### Deployment presets

**Local-only** (current default, backwards-compatible):
```yaml
ollama:
  host: "http://localhost:11434"
  model: "qwen2.5-coder:1.5b"
```

**Cloud-only** (Raspberry Pi):
```yaml
providers:
  main:
    type: openai_compat
    base_url: "https://api.groq.com/openai/v1"
    api_key_env: "GROQ_API_KEY"
    model: "llama-3.3-70b-versatile"
    stream: true
default_provider: main
```

**Mixed** (laptop — local cheap, cloud for complex goals):
```yaml
providers:
  local: { type: ollama, host: "...", model: "qwen2.5-coder:1.5b" }
  cloud: { type: openai_compat, base_url: "...", api_key_env: "OPENROUTER_API_KEY", model: "..." }
default_provider: local
fallback_provider: cloud
fallback_after_failures: 2
```

---

## Architecture

### New file: `internal/llm/openai.go`

`OpenAIClient` implements `Client` and `ModelLister`.

- Constructor: `NewOpenAI(baseURL, apiKey, model string) *OpenAIClient`
- `ChatStream`: POST `/v1/chat/completions`, parse `data: {...}` SSE lines
- `ListModels`: GET `/v1/models`, parse `data[].id` array
- No new dependencies — pure `net/http` + `bufio.Scanner`
- Translates `ChatRequest` (already OpenAI-shaped) to the wire format; maps Ollama
  `options.temperature` → top-level `temperature`, `num_predict` → `max_tokens`

### Config changes: `internal/config/config.go`

```go
type ProviderConfig struct {
    Type      string         `yaml:"type"`        // "ollama" | "openai_compat"
    Host      string         `yaml:"host"`        // ollama only
    BaseURL   string         `yaml:"base_url"`    // openai_compat only
    APIKeyEnv string         `yaml:"api_key_env"` // openai_compat only
    Model     string         `yaml:"model"`
    Stream    bool           `yaml:"stream"`
    Options   map[string]any `yaml:"options"`
}

// New fields on Config:
Providers             map[string]ProviderConfig `yaml:"providers"`
DefaultProvider       string                    `yaml:"default_provider"`
FallbackProvider      string                    `yaml:"fallback_provider"`
FallbackAfterFailures int                       `yaml:"fallback_after_failures"`
```

`OllamaConfig` stays untouched.

`Validate()` gains:
- If `providers` present: check `default_provider` names a valid key
- If `fallback_provider` set: check it names a valid key, and it differs from `default_provider`
- If an `openai_compat` provider is the `default_provider`: verify its `api_key_env` env var
  is non-empty (fatal). Warn (non-fatal) for fallback providers.

### Agent wiring: `internal/agent/loop.go`

`Loop` struct gains two new fields alongside the existing `client` field:

```go
client         llm.Client  // active client — used by all LLM calls
defaultClient  llm.Client  // original client, stored for post-goal restoration
fallbackClient llm.Client  // nil when no fallback is configured
```

`agent.New(cfg)` calls a new helper `buildClient(p config.ProviderConfig) llm.Client`:

```go
func buildClient(p config.ProviderConfig) llm.Client {
    switch p.Type {
    case "openai_compat":
        apiKey := os.Getenv(p.APIKeyEnv)
        return llm.NewOpenAI(p.BaseURL, apiKey, p.Model)
    default: // "ollama"
        return llm.NewOllama(p.Host)
    }
}
```

Backwards compat path (no `providers:` key):

```go
if len(cfg.Providers) == 0 {
    l.client = llm.NewOllama(cfg.Ollama.Host)
    l.defaultClient = l.client
    // fallbackClient stays nil
}
```

The existing `l.client` field continues to be the one all LLM calls use.
`defaultClient` is only stored for restoration after a goal escalation.

### Escalation logic: `internal/agent/goal.go`

`runGoal()` already tracks `consecutiveNoProgress`. When that counter reaches
`cfg.FallbackAfterFailures` and `l.fallbackClient != nil`:

```go
if consecutiveNoProgress >= l.cfg.Agent.FallbackAfterFailures && l.fallbackClient != nil {
    l.client = l.fallbackClient
    defer func() { l.client = l.defaultClient }()
    l.printf("[escalated to fallback provider for this goal]\n")
    consecutiveNoProgress = 0
}
```

`l.client` is swapped in-place for the duration of the goal, then restored by defer.
Regular chat (`Run()`) always uses `l.client` which equals `l.defaultClient` — no
escalation outside goal mode.

### `/status` and `/models` updates

- `/status` prints: `provider: local (ollama)` or `provider: cloud (openai_compat)`
- `/models` lists models for the currently-active client via the `ModelLister` interface
  (no change needed — already a type assertion)

---

## Error Handling

| Scenario | Behaviour |
|---|---|
| `api_key_env` unset, provider is `default_provider` | Fatal config error on startup |
| `api_key_env` unset, provider is `fallback_provider` | Warning from `--doctor`; runtime auth error if escalation triggers |
| Fallback client returns network/auth error | Step failure surfaces normally; goal continues on fallback (it already failed N times on primary) |
| `providers:` absent | Falls back to `ollama:` block — zero change for existing users |
| `--model` flag | Overrides model name on `default_provider`'s client only |

---

## Testing Plan

### Unit tests
- `buildClient()` for both `ollama` and `openai_compat` types
- Config loading: `providers` present vs. absent (fallback to `ollama:`)
- `Validate()`: missing `api_key_env` for default provider → error; for fallback → ok
- Escalation trigger: mock client that fails N times, assert `activeClient` swaps

### Integration / smoke test
- `smoke-test.sh` gains `--provider cloud` flag: skips Ollama checks, runs against a
  live OpenAI-compat endpoint when `$OPENROUTER_API_KEY` (or similar) is set
- Existing smoke test continues to run against Ollama unchanged

---

## File Changelist

| File | Change |
|---|---|
| `internal/llm/openai.go` | **New** — `OpenAIClient` |
| `internal/llm/openai_test.go` | **New** — unit tests |
| `internal/config/config.go` | Add `ProviderConfig`, new `Config` fields, update `Validate()` |
| `internal/config/config_test.go` | Add provider loading + validation tests |
| `internal/agent/loop.go` | `defaultClient`/`fallbackClient` fields, `buildClient()` helper, backwards-compat path |
| `internal/agent/goal.go` | Escalation logic (~10 lines) |
| `config.yaml` | Add commented-out `providers:` example block |
| `scripts/smoke-test.sh` | `--provider cloud` flag |
| `README.md` | Multi-provider section |
