package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	tests := []struct {
		input string
		check func(got string) bool
		desc  string
	}{
		{
			input: "~/notes.txt",
			check: func(got string) bool {
				return strings.HasPrefix(got, home) && strings.HasSuffix(got, "notes.txt")
			},
			desc: "expands ~/",
		},
		{
			input: "~/a/b/c.txt",
			check: func(got string) bool {
				return got == filepath.Join(home, "a/b/c.txt")
			},
			desc: "expands nested ~/",
		},
		{
			input: "/absolute/path",
			check: func(got string) bool { return got == "/absolute/path" },
			desc:  "leaves absolute path unchanged",
		},
		{
			input: "relative/path",
			check: func(got string) bool { return got == "relative/path" },
			desc:  "leaves relative path unchanged",
		},
		{
			input: "~",
			check: func(got string) bool { return got == "~" },
			desc:  "bare ~ without slash is not expanded",
		},
		{
			input: "",
			check: func(got string) bool { return got == "" },
			desc:  "empty string unchanged",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := expandPath(tt.input)
			if !tt.check(got) {
				t.Errorf("expandPath(%q) = %q", tt.input, got)
			}
		})
	}
}
