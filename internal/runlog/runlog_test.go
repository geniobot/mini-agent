package runlog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLogAndRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.log")

	lg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer lg.Close()

	lg.Log(Entry{TS: time.Now().UTC(), Tool: "write_file", Args: "hello.py", ResultBytes: 42, OK: true, DurationMS: 10})
	lg.Log(Entry{TS: time.Now().UTC(), Tool: "read_file", Args: "hello.py", ResultBytes: 42, OK: true, DurationMS: 5})
	lg.Close()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()

	var entries []Entry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanner: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Tool != "write_file" {
		t.Errorf("entry[0].Tool = %q, want write_file", entries[0].Tool)
	}
	if entries[1].Tool != "read_file" {
		t.Errorf("entry[1].Tool = %q, want read_file", entries[1].Tool)
	}
}

func TestRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.log")

	lg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer lg.Close()

	// Force rotation by setting size to exactly the limit; next write triggers rotate.
	lg.size = maxLogBytes
	lg.Log(Entry{TS: time.Now().UTC(), Tool: "write_file", ResultBytes: 100, OK: true})

	// After rotation, run.log.1 should exist.
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Errorf("rotated file not found: %v", err)
	}
	// New run.log should be small.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("new log not found: %v", err)
	}
	if info.Size() >= maxLogBytes {
		t.Errorf("new log size %d >= maxLogBytes", info.Size())
	}
}

func TestLogSilentOnError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.log")

	lg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	lg.Close() // close before logging — should not panic

	// Logging to a closed file should not panic.
	lg.Log(Entry{Tool: "test"})
}
