package models

import "testing"

func TestDetectTier(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		expected string
	}{
		// Frontier tier tests
		{
			name:     "frontier: claude-3-5-sonnet",
			model:    "claude-3-5-sonnet",
			expected: "frontier",
		},
		{
			name:     "frontier: gpt-4-turbo",
			model:    "gpt-4-turbo",
			expected: "frontier",
		},
		{
			name:     "frontier: gpt-4o",
			model:    "gpt-4o",
			expected: "frontier",
		},
		{
			name:     "frontier: o1-preview",
			model:    "o1-preview",
			expected: "frontier",
		},
		{
			name:     "frontier: o1-mini",
			model:    "o1-mini",
			expected: "frontier",
		},
		{
			name:     "frontier: case insensitive CLAUDE-3-5-SONNET",
			model:    "CLAUDE-3-5-SONNET",
			expected: "frontier",
		},
		{
			name:     "frontier: case insensitive GPT-4",
			model:    "GPT-4",
			expected: "frontier",
		},

		// Standard tier tests
		{
			name:     "standard: claude-opus",
			model:    "claude-opus",
			expected: "standard",
		},
		{
			name:     "standard: claude-opus-4",
			model:    "claude-opus-4",
			expected: "standard",
		},
		{
			name:     "standard: claude-3-opus",
			model:    "claude-3-opus",
			expected: "standard",
		},
		{
			name:     "standard: claude-3-sonnet",
			model:    "claude-3-sonnet",
			expected: "standard",
		},
		{
			name:     "standard: gemini-2-flash",
			model:    "gemini-2-flash",
			expected: "standard",
		},
		{
			name:     "standard: mistral-large",
			model:    "mistral-large",
			expected: "standard",
		},
		{
			name:     "standard: llama-3.1-70b",
			model:    "llama-3.1-70b",
			expected: "standard",
		},
		{
			name:     "standard: mistral-7b",
			model:    "mistral-7b",
			expected: "standard",
		},
		{
			name:     "standard: llama2:7b",
			model:    "llama2:7b",
			expected: "standard",
		},
		{
			name:     "standard: qwen2.5-coder:3b",
			model:    "qwen2.5-coder:3b",
			expected: "standard",
		},
		{
			name:     "standard: qwen2.5-coder:7b",
			model:    "qwen2.5-coder:7b",
			expected: "standard",
		},
		{
			name:     "standard: claude-3-haiku-20240307",
			model:    "claude-3-haiku-20240307",
			expected: "standard",
		},
		{
			name:     "standard: claude-3-haiku",
			model:    "claude-3-haiku",
			expected: "standard",
		},
		{
			name:     "standard: phi3",
			model:    "phi3",
			expected: "standard",
		},
		{
			name:     "standard: phi3:mini",
			model:    "phi3:mini",
			expected: "standard",
		},
		{
			name:     "standard: phi3:medium",
			model:    "phi3:medium",
			expected: "standard",
		},
		{
			name:     "standard: case insensitive MISTRAL-7B",
			model:    "MISTRAL-7B",
			expected: "standard",
		},

		// Weak tier tests
		{
			name:     "weak: qwen2.5-coder:1.5b",
			model:    "qwen2.5-coder:1.5b",
			expected: "weak",
		},
		{
			name:     "weak: phi:3.5b",
			model:    "phi:3.5b",
			expected: "weak",
		},
		{
			name:     "weak: tinyllama",
			model:    "tinyllama",
			expected: "weak",
		},
		{
			name:     "weak: tinyllama:1.1b",
			model:    "tinyllama:1.1b",
			expected: "weak",
		},
		{
			name:     "weak: model-0.5b",
			model:    "model-0.5b",
			expected: "weak",
		},
		{
			name:     "weak: phi-1.5b",
			model:    "phi-1.5b",
			expected: "weak",
		},
		{
			name:     "weak: case insensitive TINYLLAMA",
			model:    "TINYLLAMA",
			expected: "weak",
		},
		{
			name:     "weak: case insensitive Qwen2.5-Coder:1.5B",
			model:    "Qwen2.5-Coder:1.5B",
			expected: "weak",
		},

		// Unknown/default tests
		{
			name:     "unknown: unknown-model",
			model:    "unknown-model",
			expected: "standard",
		},
		{
			name:     "unknown: my-custom-model",
			model:    "my-custom-model",
			expected: "standard",
		},
		{
			name:     "unknown: empty string",
			model:    "",
			expected: "standard",
		},
		{
			name:     "unknown: generic-llm",
			model:    "generic-llm",
			expected: "standard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectTier(tt.model)
			if result != tt.expected {
				t.Errorf("DetectTier(%q) = %q, want %q", tt.model, result, tt.expected)
			}
		})
	}
}
