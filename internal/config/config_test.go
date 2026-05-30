package config

import (
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
