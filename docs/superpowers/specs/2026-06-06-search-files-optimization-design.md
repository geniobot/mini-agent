# Design: search_files I/O optimization

**Date:** 2026-06-06
**Status:** Approved

## Problem

`SearchFiles` in `internal/tools/search.go` performs two file opens and two full-memory allocations per searched file:

1. `isBinaryFile(path)` — opens the file, reads 512 bytes, closes it
2. `os.Open(path)` — opens the same file again
3. `io.ReadAll(f)` — reads the entire file into a `[]byte` (up to 512 KB)
4. `strings.Split(string(raw), "\n")` — allocates a `[]string` of every line

On embedded devices (Raspberry Pi, Jetson Nano) with SD card or eMMC storage, redundant syscalls and large heap allocations are measurably slower and harder on flash longevity.

## Goal

Replace the two-open, two-allocation sequence with a single-pass approach that:
- Opens each file exactly once
- Uses O(line-length) memory per file instead of O(file-size)
- Produces identical output and passes all existing tests unchanged

## Approach

For each file that passes the size check inside `filepath.Walk`:

1. `os.Open(path)` — single open
2. Read first 512 bytes into a fixed stack buffer (`var head [512]byte`)
3. Scan `head[:n]` for a null byte — if found, close and skip (binary)
4. Combine already-read bytes with the rest of the file: `io.MultiReader(bytes.NewReader(head[:n]), f)`
5. Wrap in `bufio.Scanner` for line-by-line streaming
6. Maintain a manual `lineNum` counter (1-based, same as current output)

## Implementation

**File:** `internal/tools/search.go` only.

**Changes:**
- Remove `isBinaryFile` function (only used in `SearchFiles`; logic inlined)
- Replace the `isBinaryFile(path)` call + `os.Open` + `io.ReadAll` + `strings.Split` block with the single-pass approach above
- Add imports: `bufio`, `bytes`
- Remove import: `io` (no longer needed after removing `io.ReadAll`)

**Scanner buffer:** Use default `bufio.Scanner` buffer (64 KB). Lines longer than 64 KB are rare in source files; the scanner returns an error on overflow which the walk callback silently ignores (same behavior as today for unreadable content).

## Behavior contract (unchanged)

- Output format: `file:line: content` (1-based line numbers)
- Lines truncated to `searchMaxLineLen` (200 chars) for display
- Results capped at `maxResults` (default 50) with cap notice
- Files larger than `searchMaxFileBytes` (512 KB) skipped
- Binary files (null byte in first 512 bytes) skipped
- Unreadable files/dirs silently skipped

## Tests

`internal/tools/search_test.go` — all existing tests are the acceptance criteria. No new test cases needed; the optimization is purely internal. All existing cases must pass unchanged.

## Out of scope

- Parallelizing the walk (adds goroutine complexity, contradicts simplicity mission)
- Caching file scan results (premature for current use)
- Changing output format or caps
