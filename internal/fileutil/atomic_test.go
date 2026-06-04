package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")

	if err := WriteAtomic(path, []byte(`{"ok":true}`), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back failed: %v", err)
	}
	if string(got) != `{"ok":true}` {
		t.Fatalf("content mismatch: %q", got)
	}
	// No stray .tmp file should remain.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp file still present after successful write")
	}
}

func TestWriteAtomicCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c.json")

	if err := WriteAtomic(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

func TestWriteAtomicOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.json")

	WriteAtomic(path, []byte("first"), 0o644)
	if err := WriteAtomic(path, []byte("second"), 0o644); err != nil {
		t.Fatalf("overwrite failed: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "second" {
		t.Fatalf("expected 'second', got %q", got)
	}
}
