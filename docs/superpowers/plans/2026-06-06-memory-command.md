# /memory Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `/memory` slash command that lists all key-value pairs stored in `~/.mini-agent/memory.json`.

**Architecture:** Single method `printMemory()` added to `Loop` in `loop.go`, wired into the existing slash-command switch and help table. Delegates entirely to the already-tested `tools.RunMemory` with op `list` — no new logic.

**Tech Stack:** Go, `internal/tools` (RunMemory already exists and is tested)

---

## File Map

| File | Change |
|---|---|
| `internal/agent/loop.go` | Add `printMemory()` method; add `/memory` case to switch; add entry to help table |

No new files. No new tests (RunMemory list op is covered by `internal/tools/memory_test.go`).

---

### Task 1: Add `printMemory()` method to Loop

**Files:**
- Modify: `internal/agent/loop.go` (after `printGoals`, around line 1262)

- [ ] **Step 1: Add the method**

Insert the following after the closing `}` of `printGoals` (after line 1262):

```go
func (l *Loop) printMemory() {
	result, err := tools.RunMemory(`{"op":"list"}`)
	if err != nil {
		fmt.Printf("[error] reading memory: %v\n", err)
		return
	}
	if result == "memory: (empty)" {
		l.printf("%s(empty)%s\n", ansiDim, ansiReset)
		return
	}
	fmt.Println()
	fmt.Println(result)
	fmt.Println()
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./...
```

Expected: no output, exit 0.

---

### Task 2: Wire `/memory` into the slash-command switch

**Files:**
- Modify: `internal/agent/loop.go` (slash-command switch, around line 277)

- [ ] **Step 1: Add the case**

Insert after the `/goals` case block (after line 277, before `case input == "/model":`):

```go
		case input == "/memory":
			l.printMemory()
			continue
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./...
```

Expected: no output, exit 0.

---

### Task 3: Add `/memory` to the help table

**Files:**
- Modify: `internal/agent/loop.go` (`printHelp`, around line 409)

- [ ] **Step 1: Add the help entry**

In `printHelp()`, insert after the `"/goals [N]"` line (after line 409):

```go
		{"/memory", "list all keys stored in memory"},
```

- [ ] **Step 2: Verify it compiles and tests pass**

```bash
go build ./... && go test ./...
```

Expected: BUILD OK, all packages pass.

---

### Task 4: Manual verification and commit

- [ ] **Step 1: Run the binary and test the empty case**

```bash
go run ./cmd/mini-agent
```

At the prompt, type `/memory`.

Expected output (if memory.json is empty or missing):
```
(empty)
```

- [ ] **Step 2: Ask the agent to set a memory key, then verify `/memory` shows it**

At the prompt, type:
```
remember that my preferred model is qwen2.5-coder:7b
```

Wait for the agent to call the `memory` tool with op `set`. Then type `/memory`.

Expected output:
```
preferred_model = qwen2.5-coder:7b
```

- [ ] **Step 3: Verify `/help` shows the new command**

Type `/help`. Confirm `/memory` appears in the command list with description `list all keys stored in memory`.

- [ ] **Step 4: Commit**

```bash
git add internal/agent/loop.go
git commit -m "feat: add /memory command to inspect stored key-value memory"
```
