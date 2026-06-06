# Design: `/memory` slash command

**Date:** 2026-06-06
**Status:** Approved

## Problem

The `memory` tool lets the agent persist key-value facts across sessions via `~/.mini-agent/memory.json`. There is no way for the user to inspect what has been stored without opening the JSON file directly. The store is a black box from the REPL.

## Goal

Add a `/memory` slash command that prints all stored keys and values so the user can see exactly what the agent has remembered.

## Scope

Read-only. List all entries. No sub-commands.

- Setting entries stays agent-only (keeps the mental model clean: agent writes, user reads).
- Deleting entries is done by asking the agent ("delete the key X from memory"), which uses the existing `memory` tool.

## Behavior

- `/memory` with no arguments prints all stored keys and values, sorted alphabetically:
  ```
  favorite_editor = neovim
  preferred_model = qwen2.5-coder:7b
  timezone = UTC-5
  ```
- If the store is empty or the file does not yet exist, prints `(empty)`.
- Output style matches other informational commands (`/status`, `/goals`): dim ANSI for the empty case, plain text for entries.

## Implementation

**Files changed:** `internal/agent/loop.go` only.

1. Add `l.printMemory()` method — calls `tools.RunMemory(`{"op":"list"}`)` and prints the result.
2. Add `case input == "/memory":` to the slash command switch, invoking `l.printMemory()`.
3. Add `{"/memory", "list all keys stored in memory"}` to the help table in `printHelp()`.

**Reuse:** `RunMemory` already handles the `list` operation correctly (sorted output, empty case, missing file). The slash command is a thin display wrapper — no new logic, no new file.

## Tests

No new unit tests required. `RunMemory` with op `list` is already covered by `internal/tools/memory_test.go`. The new code path is a one-line call plus a `fmt.Printf` — verifiable by running the binary and typing `/memory`.

## Out of scope

- `/memory <key>` (get single value) — YAGNI; list is sufficient for inspection
- `/memory delete <key>` — delegate to agent
- `/memory set <key> <value>` — agent-only write path
