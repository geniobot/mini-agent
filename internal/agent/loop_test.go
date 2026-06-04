package agent

import (
	"context"
	"testing"

	"mini-agent/internal/config"
	"mini-agent/internal/llm"
	"mini-agent/internal/session"
)

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
		DefaultProvider:       "local",
		FallbackProvider:      "cloud",
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

func TestParseFallbackToolCall(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLen  int
		wantTool string
	}{
		{
			name:     "clean JSON",
			input:    `{"name":"write_file","arguments":{"path":"hello.txt","content":"hi"}}`,
			wantLen:  1,
			wantTool: "write_file",
		},
		{
			name:     "JSON in backtick fence",
			input:    "```json\n{\"name\":\"read_file\",\"arguments\":{\"path\":\"notes.txt\"}}\n```",
			wantLen:  1,
			wantTool: "read_file",
		},
		{
			// prose before JSON is intentionally NOT parsed — the model should only
			// output bare JSON; prose means it ignored the system prompt, so we
			// treat the whole response as plain text to avoid silent file operations.
			name:    "prose before JSON — not parsed",
			input:   "I will write the file now.\n{\"name\":\"write_file\",\"arguments\":{\"path\":\"out.txt\",\"content\":\"x\"}}",
			wantLen: 0,
		},
		{
			name:    "empty input",
			input:   "",
			wantLen: 0,
		},
		{
			name:    "unknown tool",
			input:   `{"name":"delete_file","arguments":{"path":"x"}}`,
			wantLen: 0,
		},
		{
			name:    "missing arguments field",
			input:   `{"name":"read_file"}`,
			wantLen: 0,
		},
		{
			name:    "malformed JSON",
			input:   `{"name":"read_file","arguments":{"path":`,
			wantLen: 0,
		},
		{
			name:     "list_dir",
			input:    `{"name":"list_dir","arguments":{"path":"."}}`,
			wantLen:  1,
			wantTool: "list_dir",
		},
		{
			name:     "run_command",
			input:    `{"name":"run_command","arguments":{"command":"ls","args":["-la"]}}`,
			wantLen:  1,
			wantTool: "run_command",
		},
		{
			name:     "append_file",
			input:    `{"name":"append_file","arguments":{"path":"log.txt","content":"line"}}`,
			wantLen:  1,
			wantTool: "append_file",
		},
		{
			name:    "empty name",
			input:   `{"name":"","arguments":{"path":"x"}}`,
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := parseFallbackToolCall(tt.input, "standard")
			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
				return
			}
			if tt.wantLen > 0 && got[0].Function.Name != tt.wantTool {
				t.Errorf("tool name = %q, want %q", got[0].Function.Name, tt.wantTool)
			}
		})
	}
}

func TestToolSummary(t *testing.T) {
	tests := []struct {
		name string
		tc   session.ToolCall
		want string
	}{
		{
			name: "read_file",
			tc:   mkCall("read_file", map[string]any{"path": "notes.txt"}),
			want: "notes.txt",
		},
		{
			name: "write_file",
			tc:   mkCall("write_file", map[string]any{"path": "out.txt", "content": "hello"}),
			want: "out.txt (5 bytes)",
		},
		{
			name: "append_file",
			tc:   mkCall("append_file", map[string]any{"path": "log.txt", "content": "line"}),
			want: "log.txt (4 bytes)",
		},
		{
			name: "list_dir",
			tc:   mkCall("list_dir", map[string]any{"path": "."}),
			want: ".",
		},
		{
			name: "run_command with args",
			tc:   mkCall("run_command", map[string]any{"command": "ls", "args": []any{"-la"}}),
			want: "ls -la",
		},
		{
			name: "run_command no args",
			tc:   mkCall("run_command", map[string]any{"command": "pwd"}),
			want: "pwd",
		},
		{
			name: "run_command empty args slice",
			tc:   mkCall("run_command", map[string]any{"command": "ls", "args": []any{}}),
			want: "ls",
		},
		{
			name: "write_file missing content",
			tc:   mkCall("write_file", map[string]any{"path": "out.txt"}),
			want: "out.txt",
		},
		{
			name: "unknown tool",
			tc:   mkCall("unknown_tool", map[string]any{}),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toolSummary(tt.tc)
			if got != tt.want {
				t.Errorf("toolSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func mkCall(name string, args map[string]any) session.ToolCall {
	return session.ToolCall{Function: session.ToolFunction{Name: name, Arguments: args}}
}
