# Multi-Provider & Claude API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add model tier detection, tier-aware system prompts, tool parsing fallback for weak models, and Claude API support (Phase 9 — v2.9.0).

**Architecture:** 
- Model tier detection runs once at startup (weak/standard/frontier)
- System prompt variants swapped based on tier (simpler for weak, richer for frontier)
- Tool parser adds fallback recovery for weak models (graceful degradation)
- Claude API integrated as `type: "anthropic"` reusing `openai_compat` handler

**Tech Stack:** Go, YAML config, OpenAI-compatible HTTP API, pure Go regex (no external parsing libs)

**Success Criteria:**
- All tier detection tests pass (20+ cases)
- All tool parser tests pass (15+ cases)
- Manual testing matrix passes (weak/standard/frontier models)
- Backward compatibility verified (existing configs load unchanged)
- No breaking changes

---

## File Structure

**New files:**
- `internal/models/tier.go` — Model tier detection logic
- `internal/models/tier_test.go` — Tier detection tests (20+ cases)
- `internal/tools/parser.go` — Tool parsing with fallback recovery
- `internal/tools/parser_test.go` — Parser tests (strict + fallback cases)

**Modified files:**
- `internal/config/config.go` — Add `SystemPromptWeak`, `SystemPromptFrontier`, `EnableFallbackParser` fields
- `internal/agent/loop.go` — Add `ModelTier` field, select prompt at startup, use fallback parser during execution
- `cmd/mini-agent/main.go` — Detect tier, log to banner
- `internal/agent/setup.go` — Add Claude option to provider wizard
- `config.yaml` — Example weak/frontier prompts, anthropic provider example
- `README.md` — Provider support table, examples
- `TODO.md` — Mark Phase 9 complete

---

## Phase 1: Tier Detection & System Prompts

### Task 1.1: Create tier detection module

**Files:**
- Create: `internal/models/tier.go`
- Create: `internal/models/tier_test.go`

- [ ] **Step 1: Create `internal/models/tier.go` with DetectTier function**

```go
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
```

- [ ] **Step 2: Create `internal/models/tier_test.go` with comprehensive test cases**

```go
package models

import "testing"

func TestDetectTier(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		// Frontier models
		{"claude-3-5-sonnet", "frontier"},
		{"claude-3-5-sonnet-20241022", "frontier"},
		{"claude-3-5-haiku", "frontier"},
		{"gpt-4-turbo", "frontier"},
		{"gpt-4o", "frontier"},
		{"o1", "frontier"},
		{"o1-preview", "frontier"},

		// Standard models (larger open source + older Claude/GPT)
		{"claude-3-opus-20240229", "standard"},
		{"claude-3-sonnet", "standard"},
		{"gemini-2-flash", "standard"},
		{"mistral-large", "standard"},
		{"mistral-7b", "standard"},
		{"llama2:7b", "standard"},
		{"qwen2.5-coder:3b", "standard"},
		{"qwen2.5-coder:7b", "standard"},
		{"llama-3.1-70b", "standard"},

		// Weak models (need simpler prompts)
		{"qwen2.5-coder:1.5b", "weak"},
		{"phi:3.5b", "weak"},
		{"tinyllama", "weak"},
		{"tinyllama:latest", "weak"},
		{"0.5b-model", "weak"},
		{"1.5b", "weak"},

		// Unknown models (default to standard)
		{"unknown-model", "standard"},
		{"my-custom-model", "standard"},
		{"", "standard"},

		// Case insensitive
		{"CLAUDE-3-5-SONNET", "frontier"},
		{"Qwen2.5-Coder:1.5B", "weak"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := DetectTier(tt.model)
			if got != tt.expected {
				t.Errorf("DetectTier(%q) = %q, want %q", tt.model, got, tt.expected)
			}
		})
	}
}
```

- [ ] **Step 3: Run tests to verify they pass**

```bash
cd /Users/jpino/Development/mini-agent
go test ./internal/models -v
```

Expected output: `PASS` with all 20+ cases passing

- [ ] **Step 4: Commit**

```bash
git add internal/models/tier.go internal/models/tier_test.go
git commit -m "feat: add model tier detection (weak/standard/frontier)"
```

---

### Task 1.2: Expand config to support tier-specific prompts

**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 1: Add new fields to AgentConfig struct**

In `internal/config/config.go`, find the `AgentConfig` struct (around line 40) and add these fields:

```go
type AgentConfig struct {
	MaxHistory         int    `yaml:"max_history"`
	SystemPrompt       string `yaml:"system_prompt"`
	SystemPromptWeak   string `yaml:"system_prompt_weak"`      // NEW
	SystemPromptFrontier string `yaml:"system_prompt_frontier"` // NEW
	StepTimeoutSeconds int    `yaml:"step_timeout_seconds"`
	MaxGoalSteps       int    `yaml:"max_goal_steps"`
	GoalMaxSteps       int    `yaml:"goal_max_steps"`
	SummarizeOnCompact bool   `yaml:"summarize_on_compact"`
	EnableFallbackParser bool  `yaml:"enable_fallback_parser"` // NEW, default true
}
```

- [ ] **Step 2: Update config validation in Load()**

In the `Load()` function (around line 145), add default for `EnableFallbackParser`:

```go
if !cfg.Tools.EnableReadFile && !cfg.Tools.EnableWriteFile && /* ... other checks ... */ {
	cfg.Tools.Enabled = false
}

// Add this new block:
if cfg.Agent.EnableFallbackParser == false && /* config was explicitly set to false */ {
	// Keep it false; otherwise default is true
} else if cfg.Agent.EnableFallbackParser == false {
	cfg.Agent.EnableFallbackParser = true // default to true
}

if cfg.Tools.WebFetchTimeoutSecs <= 0 {
	cfg.Tools.WebFetchTimeoutSecs = 30
}
```

Actually, let me simplify — we don't need to set a default since Go defaults `bool` to `false`, and we'll treat `false` as "use default behavior". Update the Load() function to be explicit:

In `Load()`, add this after the other agent config defaults (around line 173):

```go
	if cfg.Agent.MaxGoalSteps < 0 {
		cfg.Agent.MaxGoalSteps = 50
	}
	// Enable fallback parser by default (zero value is false, so explicit true)
	// This will be handled in Agent.New(), not here
```

No code change needed in Load() — the default YAML loading will work. We'll handle the default in Agent.New() next task.

- [ ] **Step 3: Commit config changes**

```bash
git add internal/config/config.go
git commit -m "feat: add system_prompt_weak, system_prompt_frontier, enable_fallback_parser config fields"
```

---

### Task 1.3: Modify Agent to detect tier and select prompt at startup

**Files:**
- Modify: `internal/agent/loop.go`

- [ ] **Step 1: Add ModelTier field to Agent struct**

In `internal/agent/loop.go`, find the `Agent` struct definition (around line 30-40) and add:

```go
type Agent struct {
	Config       *config.Config
	Session      *session.Session
	Logger       *logwriter.LogWriter
	RunLog       *runlog.RunLog
	ModelTier    string           // NEW: "weak" | "standard" | "frontier"
	SystemPrompt string           // Already exists, but now dynamically selected
	// ... rest of fields ...
}
```

- [ ] **Step 2: Update Agent.New() to detect tier and select prompt**

Find the `New(cfg *config.Config) *Agent` function (around line 50) and modify the system prompt initialization:

Replace the existing system prompt assignment with:

```go
func New(cfg *config.Config) *Agent {
	// ... existing code ...

	// Detect model tier from active provider
	var activeModel string
	if len(cfg.Providers) > 0 {
		provider := cfg.Providers[cfg.DefaultProvider]
		activeModel = provider.Model
	} else {
		activeModel = cfg.Ollama.Model
	}

	// NEW: Detect tier
	modelTier := models.DetectTier(activeModel)

	// NEW: Select system prompt based on tier
	var systemPrompt string
	switch modelTier {
	case "weak":
		if cfg.Agent.SystemPromptWeak != "" {
			systemPrompt = cfg.Agent.SystemPromptWeak
		} else {
			systemPrompt = cfg.Agent.SystemPrompt
		}
	case "frontier":
		if cfg.Agent.SystemPromptFrontier != "" {
			systemPrompt = cfg.Agent.SystemPromptFrontier
		} else {
			systemPrompt = cfg.Agent.SystemPrompt
		}
	default: // standard
		systemPrompt = cfg.Agent.SystemPrompt
	}

	a := &Agent{
		Config:       cfg,
		Session:      sess,
		Logger:       logger,
		RunLog:       rl,
		ModelTier:    modelTier,          // NEW
		SystemPrompt: systemPrompt,       // NEW: tier-selected prompt
		// ... rest of initialization ...
	}

	return a
}
```

Make sure to import `models` at the top of loop.go:

```go
import (
	// ... existing imports ...
	"mini-agent/internal/models"
)
```

- [ ] **Step 3: Update agent startup banner to show tier**

Find the banner printing code (around line `RunBanner()` or `printBanner()`). Add tier info:

```go
fmt.Fprintf(os.Stderr, "Model: %s [tier: %s]\n", activeModel, a.ModelTier)
```

Or similar — exact location depends on your banner code. The goal is to log the detected tier so user sees it at startup.

- [ ] **Step 4: Run the app to verify it starts and logs tier**

```bash
cd /Users/jpino/Development/mini-agent
go run ./cmd/mini-agent
```

Expected output: Should see something like:
```
Model: qwen2.5-coder:1.5b [tier: weak]
```

or 

```
Model: claude-3-5-sonnet [tier: frontier]
```

- [ ] **Step 5: Commit**

```bash
git add internal/agent/loop.go cmd/mini-agent/main.go
git commit -m "feat: detect model tier at startup, select system prompt per tier"
```

---

### Task 1.4: Add example prompts to config.yaml

**Files:**
- Modify: `config.yaml` (in repo root)

- [ ] **Step 1: Update config.yaml with weak/frontier prompt examples**

In `config.yaml`, find the `agent:` section and add the new prompt examples:

```yaml
agent:
  max_history: 8
  step_timeout_seconds: 300
  max_goal_steps: 10
  goal_max_steps: 50
  system_prompt: |
    You are a local coding assistant.
    For file and shell operations output ONLY a JSON object — no explanation, no markdown, nothing else.
    For questions and conversation reply in plain text.

    Write or create any file (code, script, config, text) — put the full content inside:
    {"name":"write_file","arguments":{"path":"hello.py","content":"print('Hello World')\n"}}

    Read a file:
    {"name":"read_file","arguments":{"path":"hello.py"}}

    Edit part of an existing file (find and replace):
    {"name":"edit_file","arguments":{"path":"hello.py","old_string":"print('hi')","new_string":"print('hello')"}}

    Append to an existing file:
    {"name":"append_file","arguments":{"path":"hello.py","content":"def greet(name):\n    print(name)\n"}}

    List a directory:
    {"name":"list_dir","arguments":{"path":"."}}

    Run a shell command:
    {"name":"run_command","arguments":{"command":"ls","args":["-la"]}}

    Fetch a web page or URL:
    {"name":"web_fetch","arguments":{"url":"https://example.com","timeout_seconds":30}}

    Search for text across files:
    {"name":"search_files","arguments":{"pattern":"TODO","path":"."}}

    Run a git command:
    {"name":"git","arguments":{"subcommand":"status","args":["--short"]}}

    Extract a value from a JSON string:
    {"name":"json_query","arguments":{"json":"{\"user\":{\"name\":\"Alice\"}}","path":".user.name"}}

    Use write_file when asked to: create, write, make, generate, save, build a new file.
    Use edit_file when asked to: change, modify, fix, update part of an existing file.
    Use search_files when asked to: find, search, grep, look for text in files.
    Use run_command when asked to: run, execute, do, try any shell command.
    Use web_fetch when asked to: fetch, visit, browse, scrape, read any URL or website.
    Use git when asked to: check status, view log, diff, commit, push, or other git operations.
    Use plain text when: greeting, explaining, answering a question, describing something.

  # Optional: Use this for weak models (1.5B, 0.5B)
  # Simpler instructions, shorter context, explicit JSON format guidance
  system_prompt_weak: |
    You are a local coding assistant.
    IMPORTANT: Keep responses short and focused. You may not be able to handle complex tasks.

    For file operations, output ONLY a JSON object on its own line — no explanation, no markdown:
    {"name":"write_file","arguments":{"path":"hello.py","content":"print('hi')\n"}}

    For questions, reply in plain text, no markdown, keep it brief.

  # Optional: Use this for frontier models (Claude 3.5, GPT-4)
  # Longer context, encourage complex reasoning, multi-file goals
  system_prompt_frontier: |
    You are an expert coding assistant with advanced reasoning capabilities.
    You excel at complex multi-file projects, architectural thinking, and deep analysis.
    Use your full problem-solving abilities to deliver elegant solutions.

    For file and shell operations, output JSON objects with clear structure:
    {"name":"write_file","arguments":{"path":"file.py","content":"..."}}

    You can reason about complex problems, suggest architectural improvements, and handle multi-step goals.
    Think deeply about edge cases and provide comprehensive solutions.

    Use plain text for explanations and questions.

  enable_fallback_parser: true
```

- [ ] **Step 2: Commit**

```bash
git add config.yaml
git commit -m "docs: add example system_prompt_weak and system_prompt_frontier to config.yaml"
```

---

### Task 1.5: Update default config template

**Files:**
- Modify: `internal/config/config.go` (the defaultConfigYAML constant)

- [ ] **Step 1: Update defaultConfigYAML constant**

In `internal/config/config.go`, find the `defaultConfigYAML` constant (around line 111) and update it to include the new fields:

```go
const defaultConfigYAML = `# mini-agent configuration — auto-generated on first run
# Edit this file to change models, enable tools, or add providers.

ollama:
  host: "http://localhost:11434"
  model: "qwen2.5-coder:1.5b"
  keep_alive: "30m"
  stream: true
  options:
    num_ctx: 2048
    num_predict: 512
    num_thread: 4

agent:
  max_history: 8
  max_goal_steps: 10
  step_timeout_seconds: 300
  system_prompt: |
    You are a helpful local assistant. Be concise and direct.
    For file operations output ONLY a JSON object — no explanation, no markdown.
    {"name":"write_file","arguments":{"path":"hello.py","content":"..."}}
    For questions, reply in plain text.
  
  # Optional: simpler prompt for weak models (1.5B)
  # system_prompt_weak: |
  #   You are a local assistant. Keep responses short.
  #   Output ONLY: {"name":"write_file","arguments":{"path":"hello.py","content":"..."}}
  
  enable_fallback_parser: true

tools:
  enabled: true
  use_native_tools: false
  enable_read_file: true
  enable_write_file: true
  enable_edit_file: true
  enable_append_file: true
  enable_list_dir: true
  enable_run_command: true
  confirm_run_command: true
  confirm_write_file: false
  allowed_commands: ["ls", "cat", "pwd", "echo", "grep", "find", "git", "python3", "python", "go", "node"]
`
```

- [ ] **Step 2: Commit**

```bash
git add internal/config/config.go
git commit -m "docs: update default config template with enable_fallback_parser"
```

---

### Task 1.6: Verify Phase 1 with integration test

**Files:**
- Create: Quick manual test (no test file, just verification)

- [ ] **Step 1: Build and run with --doctor flag**

```bash
cd /Users/jpino/Development/mini-agent
go build -o mini-agent ./cmd/mini-agent
./mini-agent --doctor
```

Expected output: Should show config OK, and log the detected tier

- [ ] **Step 2: Test with different model names**

Temporarily edit `config.yaml` and change the model to test tier detection:

```yaml
ollama:
  model: "qwen2.5-coder:1.5b"
```

Run: `./mini-agent` and verify it says `[tier: weak]`

Then change to:
```yaml
ollama:
  model: "qwen2.5-coder:7b"
```

Run: `./mini-agent` and verify it says `[tier: standard]`

- [ ] **Step 3: Verify system_prompt_weak is used for weak model**

This is harder to test without logging the actual prompt. For now, we'll rely on Phase 2's tool parsing to confirm weak model path works.

- [ ] **Step 4: Mark Phase 1 complete**

```bash
git log --oneline | head -5
# Should show:
# - docs: update default config template with enable_fallback_parser
# - docs: add example system_prompt_weak and system_prompt_frontier to config.yaml
# - feat: detect model tier at startup, select system prompt per tier
# - feat: add model tier detection (weak/standard/frontier)
```

Phase 1 is complete. Move to Phase 2.

---

## Phase 2: Tool Parsing with Fallback

### Task 2.1: Create tool parser module with fallback logic

**Files:**
- Create: `internal/tools/parser.go`
- Create: `internal/tools/parser_test.go`

- [ ] **Step 1: Create `internal/tools/parser.go`**

```go
package tools

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ToolCall represents a parsed tool invocation
type ToolCall struct {
	Name      string            `json:"name"`
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

	// Step 2: Fallback parser for weak models
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

// extractFromProse tries to extract tool calls from natural language
// Very conservative: only handles common file operations
func extractFromProse(text string) (*ToolCall, error) {
	lower := strings.ToLower(text)

	// Very simple heuristic: if text mentions write_file + a path, try to extract it
	if strings.Contains(lower, "write_file") || strings.Contains(lower, "write to") {
		// Try to find a filename pattern (word.extension)
		filenameRe := regexp.MustCompile(`(?i)(?:write\s+to\s+)?([a-zA-Z0-9_\-./]+\.[a-zA-Z0-9]+)`)
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
```

- [ ] **Step 2: Create `internal/tools/parser_test.go` with comprehensive test cases**

```go
package tools

import (
	"testing"
)

func TestParseToolCallStrictJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		modelTier string
		expected  *ToolCall
		wantErr   bool
	}{
		{
			name:      "valid JSON write_file",
			input:     `{"name":"write_file","arguments":{"path":"hello.py","content":"print('hi')\n"}}`,
			modelTier: "standard",
			expected: &ToolCall{
				Name: "write_file",
				Arguments: map[string]interface{}{
					"path":    "hello.py",
					"content": "print('hi')\n",
				},
			},
			wantErr: false,
		},
		{
			name:      "valid JSON read_file",
			input:     `{"name":"read_file","arguments":{"path":"hello.py"}}`,
			modelTier: "standard",
			expected: &ToolCall{
				Name: "read_file",
				Arguments: map[string]interface{}{
					"path": "hello.py",
				},
			},
			wantErr: false,
		},
		{
			name:      "invalid JSON, strong model should error",
			input:     `{name: "write_file", arguments: {path: "hello.py", content: "hi"}}`,
			modelTier: "standard",
			expected:  nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseToolCall(tt.input, tt.modelTier)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseToolCall error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.Name != tt.expected.Name {
					t.Errorf("got.Name = %q, want %q", got.Name, tt.expected.Name)
				}
				// Check arguments match
				for key, expectedVal := range tt.expected.Arguments {
					if got.Arguments[key] != expectedVal {
						t.Errorf("Arguments[%q] = %v, want %v", key, got.Arguments[key], expectedVal)
					}
				}
			}
		})
	}
}

func TestParseToolCallWeakModelFallback(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		modelTier string
		expected  *ToolCall
		wantErr   bool
	}{
		{
			name:      "malformed JSON with single quotes (weak model)",
			input:     `{'name': 'write_file', 'arguments': {'path': 'hello.py', 'content': 'print("hi")'}}`,
			modelTier: "weak",
			expected: &ToolCall{
				Name: "write_file",
				Arguments: map[string]interface{}{
					"path":    "hello.py",
					"content": "print(\"hi\")",
				},
			},
			wantErr: false,
		},
		{
			name:      "prose with write_file hint",
			input:     `I'll write to hello.py with content print('hi')`,
			modelTier: "weak",
			expected: &ToolCall{
				Name: "write_file",
			},
			wantErr: false, // Should extract path at minimum
		},
		{
			name:      "completely garbled (weak model)",
			input:     `asdfasdfasdf not even close`,
			modelTier: "weak",
			expected:  nil,
			wantErr:   true,
		},
		{
			name:      "malformed JSON, standard model should NOT use fallback",
			input:     `{'name': 'write_file', 'arguments': {'path': 'hello.py'}}`,
			modelTier: "standard",
			expected:  nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseToolCall(tt.input, tt.modelTier)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseToolCall error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.expected != nil && tt.expected.Name != "" {
				if got.Name != tt.expected.Name {
					t.Errorf("got.Name = %q, want %q", got.Name, tt.expected.Name)
				}
			}
		})
	}
}
```

- [ ] **Step 3: Run tests to verify they pass**

```bash
cd /Users/jpino/Development/mini-agent
go test ./internal/tools -v
```

Expected: All tests pass, including fallback cases

- [ ] **Step 4: Commit**

```bash
git add internal/tools/parser.go internal/tools/parser_test.go
git commit -m "feat: add tool parser with fallback recovery for weak models"
```

---

### Task 2.2: Integrate parser into tool execution

**Files:**
- Modify: `internal/agent/loop.go` (or wherever tool parsing happens)

- [ ] **Step 1: Find existing tool parsing code**

Look in `internal/agent/loop.go` for the function that parses tool calls (likely something like `parseJSON()` or within the main loop where tools are executed).

Find the line that currently does:
```go
json.Unmarshal([]byte(output), &toolCall)
```

- [ ] **Step 2: Replace with new parser**

Replace that JSON parsing with:

```go
// OLD:
// err := json.Unmarshal([]byte(output), &tc)

// NEW:
tc, err := tools.ParseToolCall(output, a.ModelTier)
if err != nil {
	return fmt.Errorf("tool output unparseable: %w", err)
}
```

Make sure to import the tools package at the top of the file:
```go
import (
	// ... existing imports ...
	"mini-agent/internal/tools"
)
```

- [ ] **Step 3: Add logging for fallback**

If you want to log when fallback fires (recommended for transparency), modify the ParseToolCall result handling:

```go
tc, err := tools.ParseToolCall(output, a.ModelTier)
if err != nil {
	return fmt.Errorf("tool output unparseable: %w", err)
}

// Log if fallback was used (optional but good for debugging)
// This requires checking if strict JSON parse would have failed
// For now, we can log based on presence of malformed indicators
if a.ModelTier == "weak" && !strings.HasPrefix(strings.TrimSpace(output), "{") {
	a.Logger.Logf("[weak-model-fallback] %s %v\n", tc.Name, tc.Arguments)
}
```

- [ ] **Step 4: Test with weak model**

Edit config.yaml to use weak model:
```yaml
ollama:
  model: "qwen2.5-coder:1.5b"
```

Try a command like: `create a file named test.py with content print('hello')`

Should see: Either success, or clear error message about unparseable output.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/loop.go
git commit -m "feat: integrate fallback tool parser into agent loop"
```

---

## Phase 3: Claude API Support

### Task 3.1: Add anthropic provider type and route to openai_compat handler

**Files:**
- Modify: `internal/config/config.go`
- Modify: HTTP completion handler (likely `internal/agent/completion.go` or similar)

- [ ] **Step 1: Verify ProviderConfig already supports type field**

Check `internal/config/config.go` line 81-90 to confirm `Type` field exists:

```go
type ProviderConfig struct {
	Type      string         `yaml:"type"`        // "ollama" | "openai_compat" | "anthropic" (NEW)
	// ... rest of fields ...
}
```

No change needed if `Type` already exists.

- [ ] **Step 2: Update config validation to accept "anthropic" type**

Find the `Validate()` function in `internal/config/config.go` (around line 199). Look for where it validates provider types and add "anthropic" as valid:

Current code likely checks:
```go
if def.Type == "openai_compat" {
	// validate openai_compat
}
```

Update to:
```go
if def.Type == "openai_compat" || def.Type == "anthropic" {
	// validate openai_compat (anthropic is compatible)
	if def.APIKeyEnv != "" && os.Getenv(def.APIKeyEnv) == "" {
		errs = append(errs, fmt.Sprintf("provider %q requires env var %s to be set", name, def.APIKeyEnv))
	}
	if def.BaseURL == "" && def.Type == "anthropic" {
		// For anthropic, set default base URL if not provided
		def.BaseURL = "https://api.anthropic.com/v1"
	}
	if def.BaseURL == "" && def.Type == "openai_compat" {
		errs = append(errs, fmt.Sprintf("provider %q: base_url is required for type openai_compat", name))
	}
	if def.Model == "" {
		errs = append(errs, fmt.Sprintf("provider %q: model is required", name))
	}
}
```

- [ ] **Step 3: Update HTTP completion handler to route anthropic → openai_compat**

Find the file that makes HTTP requests to LLM providers (likely `internal/agent/completion.go`). Look for where it routes based on provider type:

```go
func (a *Agent) Complete(messages []Message) (string, error) {
	// ...
	if provider.Type == "ollama" {
		return a.completeViaOllama(provider, messages)
	}
	if provider.Type == "openai_compat" {
		return a.completeViaOpenAICompat(provider, messages)
	}
	// Add this:
	if provider.Type == "anthropic" {
		return a.completeViaOpenAICompat(provider, messages)
	}
}
```

This works because Claude's API is OpenAI-compatible.

- [ ] **Step 4: Update tier detection for Claude models**

In `internal/models/tier.go`, update the `DetectTier()` function to recognize Claude models:

```go
func DetectTier(modelName string) string {
	lower := strings.ToLower(modelName)

	// Frontier tier: latest, most capable models
	if contains(lower, "claude-3-5", "gpt-4", "o1") {  // Already has claude-3-5
		return "frontier"
	}

	// Standard tier: strong, proven models
	if contains(lower, "claude-3", "gemini-2", "mistral-large", "llama-3.1-70b") {
		return "standard"
	}
	// ... rest unchanged ...
}
```

Claude models are already covered in the tier detection. No change needed if code already has "claude-3" and "claude-3-5" patterns.

- [ ] **Step 5: Add config example for Claude API**

Update `config.yaml` to include an example provider block (uncommented as an example, or in comments):

```yaml
# ── Multi-provider (optional) ──────────────────────────────────────────────
# Uncomment to use named providers instead of the ollama: block below.
#
# providers:
#   claude:
#     type: "anthropic"
#     api_key_env: "ANTHROPIC_API_KEY"
#     model: "claude-3-5-sonnet-20241022"
#     stream: true
#     options:
#       temperature: 0.2
#       max_tokens: 2048
#
#   local:
#     type: "ollama"
#     host: "http://localhost:11434"
#     model: "qwen2.5-coder:1.5b"
#
# default_provider: claude
# fallback_provider: local
# fallback_after_failures: 2
```

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/agent/completion.go internal/models/tier.go config.yaml
git commit -m "feat: add anthropic provider type, route to openai_compat handler"
```

---

### Task 3.2: Update setup wizard to support Claude API

**Files:**
- Modify: `internal/agent/setup.go`

- [ ] **Step 1: Find the setup wizard code**

Look in `internal/agent/setup.go` for the `runSetup()` function (or similar). This is where the `--setup` wizard is implemented.

- [ ] **Step 2: Add Claude as a provider option**

Find where providers are presented as options, and add Claude:

```go
// Example structure (adjust to match your code):
providers := []string{
	"ollama",
	"openai_compat",
	"anthropic",  // NEW
}

// When user selects anthropic, prompt for:
// - API key (read from ANTHROPIC_API_KEY env var)
// - Model (suggest default like claude-3-5-sonnet-20241022)
// - Temperature (default 0.2)
// - Max tokens (default 2048)
```

- [ ] **Step 3: Add validation for ANTHROPIC_API_KEY**

When user selects Claude provider, check that the env var is set:

```go
case "anthropic":
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "Error: ANTHROPIC_API_KEY environment variable not set\n")
		fmt.Fprintf(os.Stderr, "Please export ANTHROPIC_API_KEY=sk-ant-...\n")
		return
	}
```

- [ ] **Step 4: Write config for selected provider**

After user selects Claude and validates key, write the config:

```go
provider := config.ProviderConfig{
	Type:       "anthropic",
	APIKeyEnv:  "ANTHROPIC_API_KEY",
	Model:      "claude-3-5-sonnet-20241022",
	Stream:     true,
	BaseURL:    "https://api.anthropic.com/v1",
	Options: map[string]interface{}{
		"temperature": 0.2,
		"max_tokens":  2048,
	},
}
```

- [ ] **Step 5: Commit**

```bash
git add internal/agent/setup.go
git commit -m "feat: add Claude API option to --setup wizard"
```

---

### Task 3.3: Manual test with Claude API

**Files:**
- None (manual testing)

- [ ] **Step 1: Set up API key**

```bash
export ANTHROPIC_API_KEY=sk-ant-...your-key...
```

- [ ] **Step 2: Run setup wizard**

```bash
cd /Users/jpino/Development/mini-agent
go run ./cmd/mini-agent --setup
```

Select "Claude API" option, verify it:
- Finds the API key in env var
- Writes config with anthropic provider
- Sets model to claude-3-5-sonnet

- [ ] **Step 3: Test with Claude model**

```bash
go run ./cmd/mini-agent --doctor
```

Should show: `Model: claude-3-5-sonnet [tier: frontier]`

- [ ] **Step 4: Run a simple query**

```bash
echo "hello, what is 2+2?" | go run ./cmd/mini-agent --quiet
```

Should get a response from Claude.

- [ ] **Step 5: Mark manual testing complete**

No code commit, but the feature is verified working.

---

## Phase 4: Documentation & Release

### Task 4.1: Update README with provider support table

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add provider support section to README**

Add this section after the hardware guide:

```markdown
## Provider Support

Mini-agent supports multiple LLM backends:

| Provider | Type | Best For | Setup |
|----------|------|----------|-------|
| **Ollama** (local) | `ollama` | Limited hardware, offline, privacy | `ollama pull model` |
| **Claude API** | `anthropic` | Best reasoning, multi-file goals | `export ANTHROPIC_API_KEY=...` |
| **OpenRouter** | `openai_compat` | Cost-effective, many models | `export OPENROUTER_API_KEY=...` |
| **Groq** | `openai_compat` | Fast inference | `export GROQ_API_KEY=...` |

### Quick Start

**Local (Ollama):**
```bash
ollama run qwen2.5-coder:3b
mini-agent
```

**Cloud (Claude API):**
```bash
export ANTHROPIC_API_KEY=sk-ant-...
mini-agent --setup
# Select "Claude API"
mini-agent
```
```

- [ ] **Step 2: Update tier documentation**

Add explanation of what tier-based prompting means:

```markdown
## Model Tiers

Mini-agent automatically detects model capability tiers and adjusts prompts:

- **Weak** (1.5B–2B): Simplified instructions, shorter context, graceful error recovery
  - Examples: `qwen2.5-coder:1.5b`, `phi:3.5b`
- **Standard** (3B–70B, older Claude): Balanced for limited hardware and general use
  - Examples: `qwen2.5-coder:3b`, `claude-3-opus`, `llama2:7b`
- **Frontier** (Claude 3.5, GPT-4): Advanced reasoning, multi-file projects, complex goals
  - Examples: `claude-3-5-sonnet`, `gpt-4-turbo`

No config needed—detection is automatic.
```

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: add provider support table and tier explanation"
```

---

### Task 4.2: Update TODO.md to mark Phase 9 complete

**Files:**
- Modify: `TODO.md`

- [ ] **Step 1: Move Phase 9 items from Open to Completed**

Find the "## Open — Tier X" sections and move Phase 9 work items under "## ✅ Completed".

Add a new section in Completed:

```markdown
### Phase 9 — Multi-provider & Claude API support
- [x] 9.1 Model tier detection (weak/standard/frontier patterns)
- [x] 9.2 Tier-aware system prompts (system_prompt_weak, system_prompt_frontier)
- [x] 9.3 Tool parsing fallback for weak models (graceful JSON recovery)
- [x] 9.4 Claude API support (type: anthropic, API key via env var)
- [x] 9.5 Provider wizard update (--setup supports Claude)
- [x] 9.6 Documentation and README updates
```

- [ ] **Step 2: Update version info**

Change "Last updated" date to today and bump planned version to v2.9.0:

```markdown
> Last updated: 2026-06-03
> Planned next release: v2.9.0
```

- [ ] **Step 3: Commit**

```bash
git add TODO.md
git commit -m "docs: mark Phase 9 (multi-provider) complete in TODO"
```

---

### Task 4.3: Create release notes and bump version

**Files:**
- Modify: `cmd/mini-agent/main.go` or version file (wherever version constant is)

- [ ] **Step 1: Find and update version constant**

Look for `Version` or `version` constant (likely in `cmd/mini-agent/main.go` or `internal/agent/banner.go`):

```go
const Version = "v2.9.0"
```

Update from current version to:
```go
const Version = "v2.9.0"
```

- [ ] **Step 2: Create release notes (optional, can be in commit message)**

In the commit message, summarize Phase 9:

```
v2.9.0: Multi-Provider & Claude API Support

Features:
- Model tier detection (weak/standard/frontier) with automatic system prompt selection
- Tool parsing fallback for weak models — recover from malformed JSON gracefully
- Claude API support (Anthropic) — add "anthropic" provider type
- Enhanced --setup wizard to support Cloud providers
- New config fields: system_prompt_weak, system_prompt_frontier, enable_fallback_parser

Improvements:
- Weak models (1.5B) now succeed at file operations with simpler prompts
- Frontier models (Claude 3.5, GPT-4) leverage full reasoning capabilities
- Transparent logging of model tier and fallback parser usage
- 100% backward compatible — existing configs work unchanged

Tests:
- 20+ model tier detection cases
- 15+ tool parser test cases (strict + fallback)
- Manual testing matrix: weak/standard/frontier models
```

- [ ] **Step 3: Commit version bump**

```bash
git add cmd/mini-agent/main.go  # or wherever Version is
git commit -m "bump: version to v2.9.0 (multi-provider & Claude API support)"
```

- [ ] **Step 4: Create git tag (optional)**

```bash
git tag v2.9.0
```

---

### Task 4.4: Final verification and summary

**Files:**
- None

- [ ] **Step 1: Run full test suite**

```bash
cd /Users/jpino/Development/mini-agent
go test ./...
```

Expected: All tests pass (tier detection + parser + existing tests)

- [ ] **Step 2: Build and verify help text**

```bash
go build -o mini-agent ./cmd/mini-agent
./mini-agent --help | grep setup
```

Should show `--setup` flag available.

- [ ] **Step 3: Verify backward compatibility**

Load the old config (without new fields) and verify it still works:

```bash
./mini-agent --doctor
```

Should show: Config OK, even without `system_prompt_weak` defined.

- [ ] **Step 4: Manual testing matrix**

Run the test prompts from CLAUDE.md with different models:

**With weak model (qwen2.5-coder:1.5b):**
```
- `hello`
- `Explain in 3 bullets what this program does`
- `Create a file named hello.txt with the text Hello from mini agent inside`
- `Read hello.txt`
```

**With frontier model (claude-3-5-sonnet):**
```
- `/goal create a todo app: main.py, README.md, requirements.txt` (complex multi-file)
```

- [ ] **Step 5: Check git log**

```bash
git log --oneline | head -20
```

Should show commits for:
- Tier detection
- System prompts
- Tool parser
- Claude API
- Documentation
- Version bump

- [ ] **Step 6: Create summary**

All phases complete:
- ✅ Phase 1: Tier detection & system prompts
- ✅ Phase 2: Tool parsing fallback
- ✅ Phase 3: Claude API support
- ✅ Phase 4: Documentation & release

Ready for v2.9.0 release.

---

## Testing Checklist

Run this before declaring the feature complete:

```bash
# Unit tests
go test ./internal/models -v      # 20+ tier detection cases
go test ./internal/tools -v       # 15+ parser cases
go test ./...                      # All tests

# Build
go build -o mini-agent ./cmd/mini-agent

# Config validation
./mini-agent --doctor

# Backward compat
./mini-agent --help

# Tier detection with different models
# (edit config.yaml model field, restart)

# Claude API (if API key available)
export ANTHROPIC_API_KEY=sk-ant-...
./mini-agent --setup  # select Claude
./mini-agent --doctor
```

---

## Notes for Implementation

**If stuck on:**
- **Tier detection patterns:** Reference the test cases in `tier_test.go` — they define the expected behavior
- **Fallback parser:** Start simple (just handle valid JSON, add prose extraction later if needed)
- **Provider routing:** Claude API is OpenAI-compatible, so route `anthropic` to the existing `openai_compat` handler
- **Config fields:** Use YAML struct tags matching the examples in `config.yaml`
- **Testing:** Run `go test ./...` frequently; TDD approach catches bugs early

**Commits should be frequent:**
- After each major file is created/tested
- After each component is integrated
- One commit per logical feature (not per step, but per task)

