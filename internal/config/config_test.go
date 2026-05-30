package config

import (
	"os"
	"strings"
	"testing"
)

func TestValidate_valid(t *testing.T) {
	cfg := &Config{
		Ollama: OllamaConfig{Host: "http://localhost:11434", Model: "qwen2.5-coder:3b"},
		Agent:  AgentConfig{MaxHistory: 8, MaxGoalSteps: 10},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidate_missingModel(t *testing.T) {
	cfg := &Config{
		Ollama: OllamaConfig{Host: "http://localhost:11434"},
		Agent:  AgentConfig{MaxHistory: 8, MaxGoalSteps: 10},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing model")
	}
	if !strings.Contains(err.Error(), "ollama.model") {
		t.Errorf("error should mention ollama.model, got: %v", err)
	}
}

func TestValidate_badHost(t *testing.T) {
	cfg := &Config{
		Ollama: OllamaConfig{Host: "localhost:11434", Model: "x"},
		Agent:  AgentConfig{MaxHistory: 8, MaxGoalSteps: 10},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for bad host")
	}
	if !strings.Contains(err.Error(), "ollama.host") {
		t.Errorf("error should mention ollama.host, got: %v", err)
	}
}

func TestValidate_httpsAllowed(t *testing.T) {
	cfg := &Config{
		Ollama: OllamaConfig{Host: "https://ollama.example.com", Model: "x"},
		Agent:  AgentConfig{MaxHistory: 8, MaxGoalSteps: 10},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("https host should be valid, got: %v", err)
	}
}

func TestValidate_badNumCtx(t *testing.T) {
	cfg := &Config{
		Ollama: OllamaConfig{
			Host:    "http://localhost:11434",
			Model:   "x",
			Options: map[string]any{"num_ctx": 8},
		},
		Agent: AgentConfig{MaxHistory: 8, MaxGoalSteps: 10},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for num_ctx < 64")
	}
	if !strings.Contains(err.Error(), "num_ctx") {
		t.Errorf("error should mention num_ctx, got: %v", err)
	}
}

func TestValidate_multipleErrors(t *testing.T) {
	cfg := &Config{}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected errors for empty config")
	}
	// Should report both host and model problems.
	if !strings.Contains(err.Error(), "ollama.host") {
		t.Errorf("expected ollama.host error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "ollama.model") {
		t.Errorf("expected ollama.model error, got: %v", err)
	}
}

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
	// Missing API key on fallback-only provider is a warning, not a fatal error.
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for missing fallback key, got: %v", err)
	}
}

// writeTempConfig writes yaml content to a temp file and returns its path.
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
