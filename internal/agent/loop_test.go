package agent

import (
	"testing"

	"mini-agent/internal/session"
)

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
			name:     "prose before JSON",
			input:    "I will write the file now.\n{\"name\":\"write_file\",\"arguments\":{\"path\":\"out.txt\",\"content\":\"x\"}}",
			wantLen:  1,
			wantTool: "write_file",
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
			got := parseFallbackToolCall(tt.input)
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
			tc:   mkCall("read_file", map[string]interface{}{"path": "notes.txt"}),
			want: "notes.txt",
		},
		{
			name: "write_file",
			tc:   mkCall("write_file", map[string]interface{}{"path": "out.txt", "content": "hello"}),
			want: "out.txt (5 bytes)",
		},
		{
			name: "append_file",
			tc:   mkCall("append_file", map[string]interface{}{"path": "log.txt", "content": "line"}),
			want: "log.txt (4 bytes)",
		},
		{
			name: "list_dir",
			tc:   mkCall("list_dir", map[string]interface{}{"path": "."}),
			want: ".",
		},
		{
			name: "run_command with args",
			tc:   mkCall("run_command", map[string]interface{}{"command": "ls", "args": []interface{}{"-la"}}),
			want: "ls -la",
		},
		{
			name: "run_command no args",
			tc:   mkCall("run_command", map[string]interface{}{"command": "pwd"}),
			want: "pwd",
		},
		{
			name: "run_command empty args slice",
			tc:   mkCall("run_command", map[string]interface{}{"command": "ls", "args": []interface{}{}}),
			want: "ls",
		},
		{
			name: "write_file missing content",
			tc:   mkCall("write_file", map[string]interface{}{"path": "out.txt"}),
			want: "out.txt",
		},
		{
			name: "unknown tool",
			tc:   mkCall("unknown_tool", map[string]interface{}{}),
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

func mkCall(name string, args map[string]interface{}) session.ToolCall {
	return session.ToolCall{Function: session.ToolFunction{Name: name, Arguments: args}}
}
