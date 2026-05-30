package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchFiles_basic(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world\nfoo bar\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main\n// TODO: fix this\n"), 0o644))

	got, err := SearchFiles("todo", dir, 50)
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}
	if !strings.Contains(got, "b.go") {
		t.Errorf("expected b.go in results, got: %s", got)
	}
	if strings.Contains(got, "a.txt") {
		t.Errorf("a.txt should not match 'todo'")
	}
}

func TestSearchFiles_caseInsensitive(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "f.py"), []byte("# FIXME: broken\n"), 0o644))

	got, err := SearchFiles("fixme", dir, 50)
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}
	if !strings.Contains(got, "f.py") {
		t.Errorf("case-insensitive match failed: %s", got)
	}
}

func TestSearchFiles_noMatches(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "x.txt"), []byte("hello\n"), 0o644))

	got, err := SearchFiles("zzznomatch", dir, 50)
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}
	if !strings.Contains(got, "no matches") {
		t.Errorf("expected 'no matches' message, got: %s", got)
	}
}

func TestSearchFiles_skipGit(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	must(t, os.MkdirAll(gitDir, 0o755))
	must(t, os.WriteFile(filepath.Join(gitDir, "COMMIT_EDITMSG"), []byte("mypattern\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "src.go"), []byte("// mypattern\n"), 0o644))

	got, err := SearchFiles("mypattern", dir, 50)
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}
	if strings.Contains(got, ".git") {
		t.Errorf(".git directory should be skipped, got: %s", got)
	}
	if !strings.Contains(got, "src.go") {
		t.Errorf("src.go should be in results, got: %s", got)
	}
}

func TestSearchFiles_cap(t *testing.T) {
	dir := t.TempDir()
	// Write a file with 10 matching lines
	var sb strings.Builder
	for range 10 {
		sb.WriteString("match this line\n")
	}
	must(t, os.WriteFile(filepath.Join(dir, "big.txt"), []byte(sb.String()), 0o644))

	got, err := SearchFiles("match", dir, 3)
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}
	if !strings.Contains(got, "capped at 3") {
		t.Errorf("expected capped message, got: %s", got)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
