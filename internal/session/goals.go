package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"mini-agent/internal/fileutil"
)

const maxGoalRecords = 100

// GoalRecord is one entry in the goal history log.
type GoalRecord struct {
	Goal   string    `json:"goal"`
	Status string    `json:"status"` // "done" | "stopped"
	Detail string    `json:"detail"`
	At     time.Time `json:"at"`
}

// DefaultGoalsPath returns ~/.mini-agent/goals.json.
func DefaultGoalsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".mini-agent", "goals.json"), nil
}

// LoadGoalRecords reads goal history from path.
// Returns nil, nil if the file does not exist.
func LoadGoalRecords(path string) ([]GoalRecord, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var records []GoalRecord
	if err := json.Unmarshal(b, &records); err != nil {
		return nil, err
	}
	return records, nil
}

// AppendGoalRecord adds r to the history file at path, capping at maxGoalRecords.
// The file and its parent directories are created if needed.
func AppendGoalRecord(path string, r GoalRecord) error {
	records, err := LoadGoalRecords(path)
	if err != nil {
		return err
	}
	records = append(records, r)
	if len(records) > maxGoalRecords {
		records = records[len(records)-maxGoalRecords:]
	}
	b, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteAtomic(path, b, 0o644)
}
