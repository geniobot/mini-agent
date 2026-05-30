package tools

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxFileBytes = 64 * 1024 // 64 KB; larger files waste context on constrained hardware

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func ReadFile(path string) (string, error) {
	return ReadFileRange(path, 0, 0)
}

// ReadFileRange reads a file, optionally limited to a line range.
// offset is a 1-based starting line (0 or 1 = from the top).
// limit is the maximum number of lines to return (0 = no line limit, byte-capped instead).
// When no range is given it behaves like a whole-file read capped at maxFileBytes.
func ReadFileRange(path string, offset, limit int) (string, error) {
	clean := filepath.Clean(expandPath(path))
	f, err := os.Open(clean)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Whole-file mode: byte-capped read, no line slicing.
	if offset <= 1 && limit <= 0 {
		b, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
		if err != nil {
			return "", err
		}
		if len(b) > maxFileBytes {
			return string(b[:maxFileBytes]) + fmt.Sprintf("\n[truncated: file exceeds %d bytes — use offset/limit to read more]", maxFileBytes), nil
		}
		return string(b), nil
	}

	// Line-range mode.
	if offset < 1 {
		offset = 1
	}
	b, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(b), "\n")
	start := offset - 1
	if start >= len(lines) {
		return fmt.Sprintf("[offset %d is past end of file — file has %d lines]", offset, len(lines)), nil
	}
	end := len(lines)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	selected := strings.Join(lines[start:end], "\n")
	return fmt.Sprintf("[lines %d-%d of %d]\n%s", start+1, end, len(lines), selected), nil
}

// EditFile performs a find-and-replace edit on a file.
// oldString must occur exactly once unless replaceAll is true.
// An empty newString deletes the matched text.
func EditFile(path, oldString, newString string, replaceAll bool) (string, error) {
	if oldString == "" {
		return "", fmt.Errorf("old_string must not be empty")
	}
	if oldString == newString {
		return "", fmt.Errorf("old_string and new_string are identical — nothing to change")
	}
	clean := filepath.Clean(expandPath(path))
	b, err := os.ReadFile(clean)
	if err != nil {
		return "", err
	}
	content := string(b)

	count := strings.Count(content, oldString)
	if count == 0 {
		return "", fmt.Errorf("old_string not found in %s", clean)
	}
	if count > 1 && !replaceAll {
		return "", fmt.Errorf("old_string appears %d times in %s — add more surrounding context to make it unique, or set replace_all", count, clean)
	}

	var updated string
	if replaceAll {
		updated = strings.ReplaceAll(content, oldString, newString)
	} else {
		updated = strings.Replace(content, oldString, newString, 1)
	}

	if err := os.WriteFile(clean, []byte(updated), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("edited %s — replaced %d occurrence(s)", clean, count), nil
}

func WriteFile(path, content string) (string, error) {
	clean := filepath.Clean(expandPath(path))
	if err := os.MkdirAll(filepath.Dir(clean), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(clean, []byte(content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(content), clean), nil
}

func AppendFile(path, content string) (string, error) {
	clean := filepath.Clean(expandPath(path))
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
	clean := filepath.Clean(expandPath(path))
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
			fmt.Fprintf(&b, "%s/\n", e.Name())
		} else {
			info, _ := e.Info()
			if info != nil {
				fmt.Fprintf(&b, "%s (%d bytes)\n", e.Name(), info.Size())
			} else {
				fmt.Fprintf(&b, "%s\n", e.Name())
			}
		}
	}
	return strings.TrimSpace(b.String()), nil
}
