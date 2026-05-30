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

func TestEditFile_uniqueReplace(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.py")
	if err := os.WriteFile(p, []byte("print('hi')\nx = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := EditFile(p, "print('hi')", "print('hello')", false)
	if err != nil {
		t.Fatalf("EditFile: %v", err)
	}
	if !strings.Contains(out, "1 occurrence") {
		t.Errorf("expected 1 occurrence, got: %s", out)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "print('hello')\nx = 1\n" {
		t.Errorf("file content wrong: %q", string(b))
	}
}

func TestEditFile_notFound(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	os.WriteFile(p, []byte("hello\n"), 0o644)
	_, err := EditFile(p, "goodbye", "hi", false)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestEditFile_ambiguous(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	os.WriteFile(p, []byte("x\nx\nx\n"), 0o644)
	_, err := EditFile(p, "x", "y", false)
	if err == nil || !strings.Contains(err.Error(), "3 times") {
		t.Errorf("expected ambiguity error mentioning count, got: %v", err)
	}
}

func TestEditFile_replaceAll(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	os.WriteFile(p, []byte("a\na\na\n"), 0o644)
	out, err := EditFile(p, "a", "b", true)
	if err != nil {
		t.Fatalf("EditFile replaceAll: %v", err)
	}
	if !strings.Contains(out, "3 occurrence") {
		t.Errorf("expected 3 occurrences, got: %s", out)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "b\nb\nb\n" {
		t.Errorf("file content wrong: %q", string(b))
	}
}

func TestEditFile_deleteWithEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	os.WriteFile(p, []byte("keep\nDELETE_ME\nkeep2\n"), 0o644)
	_, err := EditFile(p, "DELETE_ME\n", "", false)
	if err != nil {
		t.Fatalf("EditFile delete: %v", err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "keep\nkeep2\n" {
		t.Errorf("delete failed: %q", string(b))
	}
}

func TestEditFile_identicalStrings(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	os.WriteFile(p, []byte("x\n"), 0o644)
	_, err := EditFile(p, "x", "x", false)
	if err == nil || !strings.Contains(err.Error(), "identical") {
		t.Errorf("expected identical error, got: %v", err)
	}
}

func TestReadFileRange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	os.WriteFile(p, []byte("line1\nline2\nline3\nline4\nline5\n"), 0o644)

	out, err := ReadFileRange(p, 2, 2)
	if err != nil {
		t.Fatalf("ReadFileRange: %v", err)
	}
	if !strings.Contains(out, "line2") || !strings.Contains(out, "line3") {
		t.Errorf("expected lines 2-3, got: %s", out)
	}
	if strings.Contains(out, "line1") || strings.Contains(out, "line4") {
		t.Errorf("range leaked extra lines: %s", out)
	}
}

func TestReadFileRange_wholeFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	os.WriteFile(p, []byte("hello world\n"), 0o644)
	out, err := ReadFileRange(p, 0, 0)
	if err != nil {
		t.Fatalf("ReadFileRange whole: %v", err)
	}
	if out != "hello world\n" {
		t.Errorf("whole-file read wrong: %q", out)
	}
}

func TestReadFileRange_offsetPastEnd(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	os.WriteFile(p, []byte("one\ntwo\n"), 0o644)
	out, err := ReadFileRange(p, 100, 10)
	if err != nil {
		t.Fatalf("ReadFileRange past end: %v", err)
	}
	if !strings.Contains(out, "past end") {
		t.Errorf("expected past-end notice, got: %s", out)
	}
}
