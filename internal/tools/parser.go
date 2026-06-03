package tools

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ToolCall represents a parsed tool invocation
type ToolCall struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ParseToolCall parses tool invocations from LLM output.
// For weak models, includes fallback recovery for malformed JSON.
func ParseToolCall(output string, modelTier string) (*ToolCall, error) {
	output = strings.TrimSpace(output)

	// Step 1: Try strict JSON parsing
	tc := &ToolCall{}
	if err := json.Unmarshal([]byte(output), tc); err == nil {
		if tc.Name != "" && tc.Arguments != nil {
			return tc, nil
		}
	}

	// Step 2: Fallback parser for weak models only
	if modelTier == "weak" {
		if recovered, err := tryFallbackParser(output); err == nil {
			return recovered, nil
		}
	}

	// Step 3: Error — couldn't parse
	return nil, fmt.Errorf("tool output unparseable (model may be too weak). output: %s", output)
}

// tryFallbackParser attempts to recover from common weak model JSON mistakes
func tryFallbackParser(output string) (*ToolCall, error) {
	// Try to find a JSON block in the output
	// Pattern: look for { ... } blocks
	re := regexp.MustCompile(`\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}`)
	matches := re.FindAllString(output, -1)

	for _, jsonBlock := range matches {
		tc := &ToolCall{}
		// Try to unmarshal this block
		if err := json.Unmarshal([]byte(jsonBlock), tc); err == nil {
			if tc.Name != "" && tc.Arguments != nil {
				return tc, nil
			}
		}
	}

	// Try to extract hints from prose
	// Look for patterns like "write_file" or "read_file"
	tc, err := extractFromProse(output)
	if err == nil {
		return tc, nil
	}

	return nil, fmt.Errorf("fallback parser failed")
}

// extractFromProse tries to extract tool calls from natural language.
// Very conservative: only handles common file operations.
func extractFromProse(text string) (*ToolCall, error) {
	lower := strings.ToLower(text)

	// Very simple heuristic: if text mentions write_file + a path, try to extract it
	if strings.Contains(lower, "write_file") || strings.Contains(lower, "write to") {
		// Try to find a filename pattern (word.extension)
		filenameRe := regexp.MustCompile(`(?i)(?:write\s+to\s+)?([a-zA-Z0-9_\-]{2,}\.[a-zA-Z]{2,5})`)
		matches := filenameRe.FindStringSubmatch(text)
		if len(matches) > 1 {
			path := matches[1]
			// Look for content hints
			contentRe := regexp.MustCompile(`(?i)(?:content|code|text|script)[\s:]*['"]?([^'"]+)`)
			contentMatches := contentRe.FindStringSubmatch(text)
			content := ""
			if len(contentMatches) > 1 {
				content = contentMatches[1]
			}

			return &ToolCall{
				Name: "write_file",
				Arguments: map[string]interface{}{
					"path":    path,
					"content": content,
				},
			}, nil
		}
	}

	return nil, fmt.Errorf("prose extraction failed")
}
