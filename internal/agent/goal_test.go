package agent

import (
	"fmt"
	"strings"
	"testing"
)

func TestCheckDone(t *testing.T) {
	tests := []struct {
		input       string
		wantSummary string
		wantDone    bool
	}{
		{"DONE: wrote the file", "wrote the file", true},
		{"done: wrote the file", "wrote the file", true},
		{"Done: all tasks complete", "all tasks complete", true},
		{"DONE - created three files", "created three files", true},
		{"  DONE: leading whitespace", "leading whitespace", true},
		{"DONE:", "", true},
		{"not done yet", "", false},
		{"", "", false},
		{"DON", "", false},
		{"doing something", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			summary, done := checkDone(tt.input)
			if done != tt.wantDone {
				t.Errorf("done = %v, want %v", done, tt.wantDone)
			}
			if summary != tt.wantSummary {
				t.Errorf("summary = %q, want %q", summary, tt.wantSummary)
			}
		})
	}
}

func TestTruncStr(t *testing.T) {
	tests := []struct {
		s    string
		max  int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 5, "hello [+6 chars]"},
		{"", 5, ""},
		{"abcdef", 3, "abc [+3 chars]"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q max=%d", tt.s, tt.max), func(t *testing.T) {
			got := truncStr(tt.s, tt.max)
			if got != tt.want {
				t.Errorf("truncStr(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
			}
		})
	}
}

func TestCountToolCallsInNotes(t *testing.T) {
	notes := "step 1 [write_file]: wrote main.py\nstep 2 [read_file]: read config\nstep 3 [no tool]: thought about it\n"
	got := countToolCalls(notes)
	if got != 2 {
		t.Errorf("countToolCalls = %d, want 2", got)
	}
	empty := countToolCalls("")
	if empty != 0 {
		t.Errorf("empty notes should give 0, got %d", empty)
	}
}

func TestIsRetryableToolError(t *testing.T) {
	cases := []struct {
		msg      string
		expected bool
	}{
		{"no such file or directory", true},
		{"file not found", true},
		{"pattern not found in file", true},
		{"unique match required", true},
		{"permission denied", true},
		{"is a directory", true},
		{"file too large", false},
		{"unknown tool: foo", false},
		{"binary not available", false},
	}
	for _, c := range cases {
		got := isRetryableToolError(fmt.Errorf("%s", c.msg))
		if got != c.expected {
			t.Errorf("isRetryableToolError(%q) = %v, want %v", c.msg, got, c.expected)
		}
	}
	if isRetryableToolError(nil) {
		t.Error("nil error should not be retryable")
	}
}

func TestLastToolError(t *testing.T) {
	notes := "step 1 [write_file]: wrote a.txt\nstep 2 [tool-error]: tool error: no such file: missing.txt\n"
	got := lastToolError(notes)
	if got != "no such file: missing.txt" {
		t.Errorf("unexpected: %q", got)
	}
}

func TestBuildPersistentGoalPrompt_ToolErrorDirective(t *testing.T) {
	g := &GoalState{
		Objective: "create hello.py",
		Steps:     2,
		Notes:     "step 1 [tool-error]: tool error: no such file: missing.txt\n",
	}
	prompt := buildPersistentGoalPrompt(g, 0)
	if !strings.Contains(prompt, "Do NOT give up") {
		t.Errorf("expected retry directive in prompt, got: %q", prompt)
	}
	if !strings.Contains(prompt, "no such file") {
		t.Errorf("expected error text in prompt, got: %q", prompt)
	}
}

func TestAppendNotes(t *testing.T) {
	t.Run("empty notes gets entry", func(t *testing.T) {
		got := appendNotes("", "step 1 [write_file]", "wrote hello.txt")
		if !strings.Contains(got, "step 1 [write_file]: wrote hello.txt") {
			t.Errorf("unexpected notes: %q", got)
		}
	})

	t.Run("accumulates multiple entries", func(t *testing.T) {
		notes := appendNotes("", "step 1 [write_file]", "wrote a.txt")
		notes = appendNotes(notes, "step 2 [read_file]", "read b.txt")
		if !strings.Contains(notes, "step 1") || !strings.Contains(notes, "step 2") {
			t.Errorf("both steps should be present: %q", notes)
		}
	})

	t.Run("trims oldest when over limit", func(t *testing.T) {
		notes := ""
		for i := range 10 {
			notes = appendNotes(notes, fmt.Sprintf("step %d [write_file]", i+1), strings.Repeat("x", 300))
		}
		if len(notes) > maxNotesLen+200 {
			t.Errorf("notes length %d greatly exceeds maxNotesLen %d", len(notes), maxNotesLen)
		}
		if !strings.Contains(notes, "[...earlier steps omitted]") {
			t.Errorf("expected omission marker: %q", notes[:min(200, len(notes))])
		}
	})
}
