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
