package session

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// DefaultPath returns ~/.mini-agent/session.json.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".mini-agent", "session.json"), nil
}

// Save writes non-system messages to path, creating parent directories as needed.
// Returns nil without writing if there are no messages to save.
func Save(path string, msgs []Message) error {
	var toSave []Message
	for _, m := range msgs {
		if m.Role != "system" {
			toSave = append(toSave, m)
		}
	}
	if len(toSave) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(toSave, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// Load reads messages from path. Returns nil, nil if the file does not exist.
func Load(path string) ([]Message, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var msgs []Message
	if err := json.Unmarshal(b, &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}
