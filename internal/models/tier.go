package models

import "strings"

// DetectTier returns the capability tier of a model.
// Returns: "weak" | "standard" | "frontier"
func DetectTier(modelName string) string {
	lower := strings.ToLower(modelName)

	// Frontier tier: latest, most capable models
	if contains(lower, "claude-3-5", "gpt-4", "o1", "claude-opus") {
		return "frontier"
	}

	// Standard tier: strong, proven models
	if contains(lower, "claude-3", "gemini-2", "mistral-large", "llama-3.1-70b") {
		return "standard"
	}

	// Weak tier: small models that need simpler instructions
	if contains(lower, "1.5b", "0.5b", "phi", "tinyllama") {
		return "weak"
	}

	// 3B and 7B are standard (sweet spot for limited hardware)
	if contains(lower, "3b", "7b", "mistral-7b", "llama2:7b") {
		return "standard"
	}

	// Default: assume capable unless proven otherwise
	return "standard"
}

// contains checks if s contains any of the substrings
func contains(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}
