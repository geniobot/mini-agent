package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendGoalNotes_basic(t *testing.T) {
	notes := appendGoalNotes("", "step 1 [write_file]", "wrote hello.py")
	if !strings.Contains(notes, "step 1 [write_file]: wrote hello.py") {
		t.Errorf("unexpected notes: %q", notes)
	}
}

func TestAppendGoalNotes_trimsOldest(t *testing.T) {
	notes := ""
	for i := range 20 {
		notes = appendGoalNotes(notes, "step", strings.Repeat("x", 300))
		_ = i
	}
	if len(notes) > goalMaxNotes+300 {
		t.Errorf("notes length %d greatly exceeds goalMaxNotes %d", len(notes), goalMaxNotes)
	}
	if !strings.Contains(notes, "[...earlier steps omitted]") {
		t.Errorf("expected omission marker in notes")
	}
}

func TestGoalIsStuck_notStuck(t *testing.T) {
	sigs := []string{"a|b|c", "d|e|f", "g|h|i"}
	if goalIsStuck(sigs) {
		t.Error("should not be stuck with all-different signatures")
	}
}

func TestGoalIsStuck_stuck(t *testing.T) {
	sig := "write_file|hello.py|ok"
	sigs := []string{sig, sig, sig}
	if !goalIsStuck(sigs) {
		t.Error("should be stuck with 3 identical signatures")
	}
}

func TestGoalIsStuck_twoNotStuck(t *testing.T) {
	sig := "write_file|hello.py|ok"
	sigs := []string{sig, sig, "other|x|y"}
	if goalIsStuck(sigs) {
		t.Error("should not be stuck with only 2 identical signatures")
	}
}

func TestAppendGoalSig_capped(t *testing.T) {
	var sigs []string
	for i := range goalSigHistory + 3 {
		sigs = appendGoalSig(sigs, "sig"+string(rune('A'+i)))
	}
	if len(sigs) != goalSigHistory {
		t.Errorf("sigs length = %d, want %d", len(sigs), goalSigHistory)
	}
}

func TestGoalStatePersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goal.json")

	// Nothing yet.
	g, err := loadGoalState(path)
	if err != nil {
		t.Fatalf("load on missing file: %v", err)
	}
	if g != nil {
		t.Fatalf("expected nil for missing file, got %+v", g)
	}

	// Save a goal.
	original := &GoalState{
		Objective: "create a REST API",
		Status:    GoalActive,
		StartedAt: time.Now().UTC().Truncate(time.Second),
		Steps:     3,
		Notes:     "step 1: wrote main.go\n",
	}
	if err := saveGoalState(path, original); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Load it back.
	loaded, err := loadGoalState(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected loaded goal, got nil")
	}
	if loaded.Objective != original.Objective {
		t.Errorf("objective = %q, want %q", loaded.Objective, original.Objective)
	}
	if loaded.Steps != original.Steps {
		t.Errorf("steps = %d, want %d", loaded.Steps, original.Steps)
	}
	if loaded.Notes != original.Notes {
		t.Errorf("notes = %q, want %q", loaded.Notes, original.Notes)
	}

	// Clear it.
	if err := clearGoalFile(path); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("goal file should not exist after clearGoalFile")
	}

	// Clear again should not error.
	if err := clearGoalFile(path); err != nil {
		t.Errorf("second clear should not error: %v", err)
	}
}
