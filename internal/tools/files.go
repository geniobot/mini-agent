package tools

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxFileBytes = 64 * 1024 // 64 KB; larger files waste context on constrained hardware

func ReadFile(path string) (string, error) {
	clean := filepath.Clean(path)
	f, err := os.Open(clean)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Read one byte past the limit to detect truncation without a stat call.
	b, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return "", err
	}
	if len(b) > maxFileBytes {
		return string(b[:maxFileBytes]) + fmt.Sprintf("\n[truncated: file exceeds %d bytes]", maxFileBytes), nil
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

func AppendFile(path, content string) (string, error) {
	clean := filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(clean), 0o755); err != nil {
		return "", err
	}
	f, err := os.OpenFile(clean, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	n, err := f.WriteString(content)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("appended %d bytes to %s", n, clean), nil
}

func ListDir(path string) (string, error) {
	clean := filepath.Clean(path)
	if clean == "" {
		clean = "."
	}
	entries, err := os.ReadDir(clean)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, e := range entries {
		if e.IsDir() {
			b.WriteString(e.Name() + "/\n")
		} else {
			info, _ := e.Info()
			if info != nil {
				b.WriteString(fmt.Sprintf("%s (%d bytes)\n", e.Name(), info.Size()))
			} else {
				b.WriteString(e.Name() + "\n")
			}
		}
	}
	return strings.TrimSpace(b.String()), nil
}
