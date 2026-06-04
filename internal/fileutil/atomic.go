// Package fileutil provides small filesystem helpers.
package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteAtomic writes data to path using a write-then-rename sequence so a crash
// mid-write never leaves a partial file at the destination.
func WriteAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("atomic rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}
