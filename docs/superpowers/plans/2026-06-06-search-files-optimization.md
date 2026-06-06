# search_files I/O Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite the per-file scan loop in `SearchFiles` to open each file once and stream lines with `bufio.Scanner` instead of reading the full file into memory.

**Architecture:** Single file change to `internal/tools/search.go`. Inline the binary detection (currently a separate open/close in `isBinaryFile`) into the main scan loop, then replace `io.ReadAll` + `strings.Split` with `io.MultiReader` + `bufio.Scanner`. No behavior change — same output format, same caps, same skip logic. Existing tests are the full acceptance criteria.

**Tech Stack:** Go stdlib — `bufio`, `bytes`, `io.MultiReader`

---

## File Map

| File | Change |
|---|---|
| `internal/tools/search.go` | Rewrite per-file block; remove `isBinaryFile`; update imports |
| `internal/tools/search_test.go` | No changes — existing tests are the spec |

---

### Task 1: Confirm baseline tests pass

**Files:**
- Test: `internal/tools/search_test.go`

- [ ] **Step 1: Run the existing tests and confirm they all pass**

```bash
go test ./internal/tools/ -v -run TestSearchFiles
```

Expected output (all pass):
```
--- PASS: TestSearchFiles_basic (0.00s)
--- PASS: TestSearchFiles_caseInsensitive (0.00s)
--- PASS: TestSearchFiles_noMatches (0.00s)
--- PASS: TestSearchFiles_skipGit (0.00s)
--- PASS: TestSearchFiles_cap (0.00s)
PASS
```

This is your baseline. The optimization must leave all five passing.

---

### Task 2: Rewrite `search.go` with single-pass per-file scan

**Files:**
- Modify: `internal/tools/search.go`

- [ ] **Step 1: Replace the entire file with the optimized version**

Write the following to `internal/tools/search.go`:

```go
package tools

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	searchDefaultMax   = 50
	searchMaxLineLen   = 200
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

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		// Read first 512 bytes for binary detection, then stream the rest.
		var head [512]byte
		n, _ := f.Read(head[:])
		if bytes.IndexByte(head[:n], 0) >= 0 {
			return nil // binary file — null byte found
		}

		scanner := bufio.NewScanner(io.MultiReader(bytes.NewReader(head[:n]), f))
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if strings.Contains(strings.ToLower(line), lower) {
				display := strings.TrimSpace(line)
				if len(display) > searchMaxLineLen {
					display = display[:searchMaxLineLen] + "…"
				}
				results = append(results, fmt.Sprintf("%s:%d: %s", path, lineNum, display))
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
```

Key changes from the original:
- `isBinaryFile` function removed — logic inlined as `bytes.IndexByte(head[:n], 0) >= 0`
- Single `os.Open` per file instead of two
- `var head [512]byte` stack buffer replaces the heap-allocated `make([]byte, 512)` in the old `isBinaryFile`
- `io.MultiReader(bytes.NewReader(head[:n]), f)` stitches the already-read 512 bytes back onto the file reader
- `bufio.NewScanner` replaces `io.ReadAll` + `strings.Split`
- Manual `lineNum` counter (1-based) replaces the loop index + `lineNum+1`
- Imports: added `bufio`, `bytes`; kept `fmt`, `io`, `os`, `path/filepath`, `strings`

- [ ] **Step 2: Verify it compiles**

```bash
go build ./...
```

Expected: no output, exit 0.

- [ ] **Step 3: Run all search tests**

```bash
go test ./internal/tools/ -v -run TestSearchFiles
```

Expected (all five pass, same as baseline):
```
--- PASS: TestSearchFiles_basic (0.00s)
--- PASS: TestSearchFiles_caseInsensitive (0.00s)
--- PASS: TestSearchFiles_noMatches (0.00s)
--- PASS: TestSearchFiles_skipGit (0.00s)
--- PASS: TestSearchFiles_cap (0.00s)
PASS
```

- [ ] **Step 4: Run the full test suite**

```bash
go test ./...
```

Expected: all packages pass, no failures.

- [ ] **Step 5: Commit**

```bash
git add internal/tools/search.go
git commit -m "perf: optimize search_files — single open, bufio.Scanner per file

Previously SearchFiles opened each file twice (once for binary detection,
once for content) and allocated the full file contents plus a line slice
via io.ReadAll + strings.Split. On embedded devices with SD card / eMMC
storage this caused redundant syscalls and unnecessary heap pressure.

Now: one os.Open per file, first 512 bytes used for binary detection via
bytes.IndexByte, then io.MultiReader + bufio.Scanner streams line-by-line.
No behavior change — same output format, caps, and skip logic."
```
