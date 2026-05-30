package tools

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	searchDefaultMax = 50
	searchMaxLineLen = 200
	searchMaxFileBytes = 512 * 1024 // skip files larger than 512 KB
)

// directories that are never worth searching
var searchSkipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	".hg": true, ".svn": true, "__pycache__": true, ".cache": true,
	"dist": false, // intentionally not skipped — user code lives here too
}

// SearchFiles does a case-insensitive text search under root and returns
// matching lines in file:line: content format (same as grep -rn).
func SearchFiles(pattern, root string, maxResults int) (string, error) {
	if maxResults <= 0 {
		maxResults = searchDefaultMax
	}
	root = expandPath(root)
	if root == "" {
		root = "."
	}
	lower := strings.ToLower(pattern)

	var results []string
	capped := false

	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable entries silently
		}
		if info.IsDir() {
			if searchSkipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Size() > searchMaxFileBytes {
			return nil
		}
		if isBinaryFile(path) {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		raw, err := io.ReadAll(f)
		if err != nil {
			return nil
		}

		for lineNum, line := range strings.Split(string(raw), "\n") {
			if strings.Contains(strings.ToLower(line), lower) {
				display := strings.TrimSpace(line)
				if len(display) > searchMaxLineLen {
					display = display[:searchMaxLineLen] + "…"
				}
				results = append(results, fmt.Sprintf("%s:%d: %s", path, lineNum+1, display))
				if len(results) >= maxResults {
					capped = true
					return filepath.SkipAll
				}
			}
		}
		return nil
	})

	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return fmt.Sprintf("no matches for %q in %s", pattern, root), nil
	}

	out := strings.Join(results, "\n")
	if capped {
		out += fmt.Sprintf("\n[capped at %d results — use a more specific pattern to see more]", maxResults)
	}
	return out, nil
}

// isBinaryFile returns true if the first 512 bytes of a file contain a null byte.
func isBinaryFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	for _, b := range buf[:n] {
		if b == 0 {
			return true
		}
	}
	return false
}
