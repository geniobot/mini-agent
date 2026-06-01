package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "goals.json")

	r1 := GoalRecord{Goal: "create hello.py", Status: "done", Detail: "wrote hello.py", At: time.Now().UTC()}
	r2 := GoalRecord{Goal: "create notes.txt", Status: "stopped", Detail: "reached step limit", At: time.Now().UTC()}

	if err := AppendGoalRecord(path, r1); err != nil {
		t.Fatal(err)
	}
	if err := AppendGoalRecord(path, r2); err != nil {
		t.Fatal(err)
	}

	records, err := LoadGoalRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].Goal != r1.Goal {
		t.Errorf("expected %q, got %q", r1.Goal, records[0].Goal)
	}
	if records[1].Status != "stopped" {
		t.Errorf("expected stopped, got %q", records[1].Status)
	}
}

func TestLoadMissingFile(t *testing.T) {
	records, err := LoadGoalRecords("/nonexistent/path/goals.json")
	if err != nil {
		t.Fatal(err)
	}
	if records != nil {
		t.Errorf("expected nil, got %v", records)
	}
}

func TestCapAt100(t *testing.T) {
	path := filepath.Join(t.TempDir(), "goals.json")

	for i := range 110 {
		r := GoalRecord{
			Goal:   "goal",
			Status: "done",
			Detail: "step",
			At:     time.Unix(int64(i), 0).UTC(),
		}
		if err := AppendGoalRecord(path, r); err != nil {
			t.Fatal(err)
		}
	}

	records, err := LoadGoalRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 100 {
		t.Errorf("expected 100 records (capped), got %d", len(records))
	}
	// The oldest 10 should have been dropped; first kept record has Unix time 10.
	if records[0].At.Unix() != 10 {
		t.Errorf("expected oldest kept at unix=10, got %d", records[0].At.Unix())
	}
}

func TestCreatesParentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	path := filepath.Join(dir, "goals.json")

	r := GoalRecord{Goal: "test", Status: "done", Detail: "", At: time.Now().UTC()}
	if err := AppendGoalRecord(path, r); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestRoundTripTimestamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "goals.json")
	ts := time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC)
	r := GoalRecord{Goal: "g", Status: "done", Detail: "d", At: ts}

	if err := AppendGoalRecord(path, r); err != nil {
		t.Fatal(err)
	}
	records, err := LoadGoalRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	if !records[0].At.Equal(ts) {
		t.Errorf("timestamp mismatch: want %v got %v", ts, records[0].At)
	}
}
