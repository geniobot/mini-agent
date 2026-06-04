package tools

import (
	"os"
	"path/filepath"
	"testing"
)

// patchMemoryPath points memoryPath at a temp file for the duration of the test.
func withTempMemory(t *testing.T, fn func()) {
	t.Helper()
	dir := t.TempDir()
	orig := memoryPathOverride
	memoryPathOverride = filepath.Join(dir, "memory.json")
	defer func() { memoryPathOverride = orig }()
	fn()
}

func TestMemory_SetGet(t *testing.T) {
	withTempMemory(t, func() {
		if _, err := RunMemory(`{"op":"set","key":"foo","value":"bar"}`); err != nil {
			t.Fatalf("set: %v", err)
		}
		got, err := RunMemory(`{"op":"get","key":"foo"}`)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got != "bar" {
			t.Fatalf("expected 'bar', got %q", got)
		}
	})
}

func TestMemory_GetMissing(t *testing.T) {
	withTempMemory(t, func() {
		out, err := RunMemory(`{"op":"get","key":"missing"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out == "missing" {
			t.Fatal("expected not-found message, got value")
		}
	})
}

func TestMemory_Delete(t *testing.T) {
	withTempMemory(t, func() {
		RunMemory(`{"op":"set","key":"x","value":"1"}`)
		if _, err := RunMemory(`{"op":"delete","key":"x"}`); err != nil {
			t.Fatalf("delete: %v", err)
		}
		out, _ := RunMemory(`{"op":"get","key":"x"}`)
		if out == "1" {
			t.Fatal("key still present after delete")
		}
	})
}

func TestMemory_List(t *testing.T) {
	withTempMemory(t, func() {
		RunMemory(`{"op":"set","key":"a","value":"1"}`)
		RunMemory(`{"op":"set","key":"b","value":"2"}`)
		out, err := RunMemory(`{"op":"list"}`)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if out == "" {
			t.Fatal("expected non-empty list")
		}
	})
}

func TestMemory_ListEmpty(t *testing.T) {
	withTempMemory(t, func() {
		out, err := RunMemory(`{"op":"list"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out != "memory: (empty)" {
			t.Fatalf("unexpected output: %q", out)
		}
	})
}

func TestMemory_UnknownOp(t *testing.T) {
	withTempMemory(t, func() {
		_, err := RunMemory(`{"op":"nope"}`)
		if err == nil {
			t.Fatal("expected error for unknown op")
		}
	})
}

func TestMemory_InvalidJSON(t *testing.T) {
	_, err := RunMemory(`not json`)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestMemory_SetMissingKey(t *testing.T) {
	withTempMemory(t, func() {
		_, err := RunMemory(`{"op":"set","value":"v"}`)
		if err == nil {
			t.Fatal("expected error for missing key in set")
		}
	})
}

// Verify atomic write leaves no .tmp file.
func TestMemory_NoTmpFile(t *testing.T) {
	withTempMemory(t, func() {
		RunMemory(`{"op":"set","key":"k","value":"v"}`)
		tmp := memoryPathOverride + ".tmp"
		if _, err := os.Stat(tmp); !os.IsNotExist(err) {
			t.Fatal(".tmp file still present after write")
		}
	})
}
