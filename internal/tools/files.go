package tools

import (
	"fmt"
	"os"
	"path/filepath"
)

func ReadFile(path string) (string, error) {
	clean := filepath.Clean(path)
	b, err := os.ReadFile(clean)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func WriteFile(path, content string) (string, error) {
	clean := filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(clean), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(clean, []byte(content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(content), clean), nil
}
