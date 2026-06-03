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
		{
			name:      "valid JSON with extra whitespace",
			input:     "  {\"name\":\"write_file\",\"arguments\":{\"path\":\"out.txt\",\"content\":\"done\"}}  ",
			modelTier: "standard",
			expected: &ToolCall{
				Name: "write_file",
				Arguments: map[string]interface{}{
					"path":    "out.txt",
					"content": "done",
				},
			},
			wantErr: false,
		},
		{
			name:      "empty arguments map should fail",
			input:     `{"name":"write_file","arguments":{}}`,
			modelTier: "standard",
			expected:  nil,
			wantErr:   false, // parses fine; arguments is non-nil even if empty
		},
		{
			name:      "missing name field should fail",
			input:     `{"arguments":{"path":"hello.py"}}`,
			modelTier: "standard",
			expected:  nil,
			wantErr:   true,
		},
		{
			name:      "missing arguments field should fail",
			input:     `{"name":"read_file"}`,
			modelTier: "standard",
			expected:  nil,
			wantErr:   true,
		},
		{
			name:      "valid JSON for run_command",
			input:     `{"name":"run_command","arguments":{"command":"ls","args":["-la"]}}`,
			modelTier: "standard",
			expected: &ToolCall{
				Name: "run_command",
			},
			wantErr: false,
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
				if got == nil {
					t.Fatalf("got nil ToolCall, want non-nil")
				}
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
			name:      "prose before and after JSON (weak model)",
			input:     `I'll read the file for you: {"name":"read_file","arguments":{"path":"config.yaml"}} let me know what you find`,
			modelTier: "weak",
			expected: &ToolCall{
				Name: "read_file",
				Arguments: map[string]interface{}{
					"path": "config.yaml",
				},
			},
			wantErr: false,
		},
		{
			name:      "prose with write_file hint (weak model)",
			input:     `I'll write to hello.py with content print('hi')`,
			modelTier: "weak",
			expected: &ToolCall{
				Name: "write_file",
			},
			wantErr: false,
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
		{
			name:      "valid JSON works for weak model too",
			input:     `{"name":"write_file","arguments":{"path":"test.txt","content":"hello"}}`,
			modelTier: "weak",
			expected: &ToolCall{
				Name: "write_file",
				Arguments: map[string]interface{}{
					"path":    "test.txt",
					"content": "hello",
				},
			},
			wantErr: false,
		},
		{
			name:      "whitespace around JSON (weak model)",
			input:     `    {"name":"read_file","arguments":{"path":"file.txt"}}    `,
			modelTier: "weak",
			expected: &ToolCall{
				Name: "read_file",
				Arguments: map[string]interface{}{
					"path": "file.txt",
				},
			},
			wantErr: false,
		},
		{
			name:      "JSON embedded in explanation (weak model)",
			input:     `Sure! Here is the tool call: {"name":"write_file","arguments":{"path":"notes.txt","content":"tip 1"}}`,
			modelTier: "weak",
			expected: &ToolCall{
				Name: "write_file",
				Arguments: map[string]interface{}{
					"path":    "notes.txt",
					"content": "tip 1",
				},
			},
			wantErr: false,
		},
		{
			name:      "empty string (weak model)",
			input:     "",
			modelTier: "weak",
			expected:  nil,
			wantErr:   true,
		},
		{
			name:      "empty string (standard model)",
			input:     "",
			modelTier: "standard",
			expected:  nil,
			wantErr:   true,
		},
		{
			name:      "write_file prose with explicit filename (weak model)",
			input:     `Please use write_file on hello.txt with the content Hello world`,
			modelTier: "weak",
			expected: &ToolCall{
				Name: "write_file",
			},
			wantErr: false,
		},
		{
			name:      "partial valid JSON (weak model, no name)",
			input:     `{"arguments":{"path":"foo.txt"}}`,
			modelTier: "weak",
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
				if got == nil {
					t.Fatalf("got nil ToolCall, want non-nil")
				}
				if got.Name != tt.expected.Name {
					t.Errorf("got.Name = %q, want %q", got.Name, tt.expected.Name)
				}
				// Check any expected argument values
				for key, expectedVal := range tt.expected.Arguments {
					if got.Arguments[key] != expectedVal {
						t.Errorf("Arguments[%q] = %v, want %v", key, got.Arguments[key], expectedVal)
					}
				}
			}
		})
	}
}
