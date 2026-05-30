# mini-agent — Engineering Plan

> Last updated: 2026-05-29
> Based on deep comparative analysis of Hermes Agent (Python) and OpenClaw (TypeScript).

---

## Mission statement (unchanged)

mini-agent is a lightweight local agent that runs well on old hardware using Ollama directly.
It is not a full Hermes/OpenClaw replacement. It captures the useful 20% with 5% of the overhead.

**Core constraints that govern every decision:**
- Local-only, no cloud dependencies
- Single compiled Go binary, near-zero dependencies
- Must run on CPU-only machines (4-core, 8 GB RAM class)
- Simple enough to understand fully in one sitting
- Reliable over ambitious

---

## What we learned from Hermes and OpenClaw

### Hermes Agent (Python, asyncio)
- 50+ tools, 30+ dependencies, ~300 MB baseline RAM
- Multi-provider LLM failover, async tool execution
- Database-backed session storage, vector memory
- Delegation system for recursive sub-agents
- Production-grade: daemon, Docker, systemd
- **Too heavy for mini-agent's target machine**

### OpenClaw (TypeScript, Node.js)
- 20+ messaging channels, 15+ built-in tools
- Declarative tool availability system (config/auth/plugin-gated)
- Plugin SDK, Skills registry, Canvas live UI
- Full sandbox isolation (Docker/SSH per sub-agent)
- **Also too heavy, but has excellent ideas to borrow:**
  - Declarative tool descriptors (availability expressions)
  - Config schema validation + migration (doctor command)
  - Lightweight tools: web_fetch (curl), git pass-through

### What both do that we should copy (lightweight versions only)
1. **Persistent goal results** — both save what goals accomplished to session
2. **Error classification and retry** — transient vs fatal, jittered backoff
3. **Context summarization** — before trimming, summarize old turns
4. **Lightweight tool expansion** — web fetch, git, json formatting
5. **Config validation** — catch bad config early with clear messages
6. **Structured logging** — JSON lines file for audit trail
7. **Scheduled automation** — config-driven cron for recurring goals

### What NOT to copy
| Feature | Why not |
|---|---|
| Browser automation (Playwright) | 50+ MB, requires Chromium, too heavy |
| Vision / image understanding | No good local model yet |
| Multi-agent delegation | Contradicts "simple" mission |
| 20+ messaging channels | Out of scope for local tool |
| Database-backed sessions | JSON file is sufficient |
| Docker sandboxing | Adds operational complexity |
| Plugin SDK | Premature abstraction |
| Voice I/O | Hardware-specific, out of scope |

---

## Current state (v2.6.0)

### What works
- Interactive REPL with streaming Ollama responses
- 5 tools: read_file, write_file, append_file, list_dir, run_command
- Session persistence (JSON file, auto-save on exit)
- Goal mode (`/run`) with loop detection and step limits
- ANSI colors, context pressure indicator, /history command
- Token budget management with automatic history trimming
- Config auto-discovery, `~/.mini-agent/config.yaml`
- One-line installer with auto-config download
- 35 unit tests across 4 packages

### Known gaps (from live testing)
- Small models (1.5B) don't reliably call tools for code files
- Goal mode needs better prompt engineering for tiny models
- No web fetch / network tools
- No git integration
- No structured logging or audit trail
- Config has no validation (bad values fail silently)
- No cron / scheduled goals

---

## Roadmap

### Phase 5 — Reliability hardening (next)

Fix the root causes observed in live testing. No new features — make existing features work consistently.

#### 5.1 Tool prompt reliability
- **Problem:** 1.5B models return markdown instead of JSON for "create hello.py"
- **Approach:** Few-shot examples in system prompt (already done in v2.6.0); monitor with wider test matrix
- **Recommendation:** Add `qwen2.5-coder:3b` as recommended default in README
- **Files:** `config.yaml`, `README.md`

#### 5.2 Structured run log
- Append a machine-readable log line per tool call to `~/.mini-agent/run.log`
- Format: JSON lines `{"ts":"...","tool":"write_file","path":"hello.py","bytes":42,"ok":true}`
- Provides audit trail without cluttering stdout
- **Files:** `internal/tools/registry.go`, new `internal/log/log.go`

#### 5.3 Config validation
- Validate required fields on startup, warn on unknown keys
- Add `--doctor` flag: checks Ollama connectivity, model availability, config schema
- **Files:** `internal/config/config.go`, `cmd/mini-agent/main.go`

#### 5.4 Persistent goal results
- After a `/run` goal completes, append a summary message to the active session
- Format: `[goal: "description" → result: "DONE summary" at timestamp]`
- Searchable in `/history` and visible on next session load
- **Files:** `internal/agent/goal.go`, `internal/agent/loop.go`

---

### Phase 6 — Tool expansion (lightweight only)

Add tools that fit the local, lightweight philosophy. Each tool must:
- Be implemented in pure Go (no external binary required), OR
- Be a thin wrapper around a standard POSIX tool (git, curl, jq, ssh)
- Add no new Go module dependencies
- Have unit tests

#### 6.1 web_fetch
- Fetch a URL via HTTP GET, return body truncated to 32 KB
- Respects `--timeout` (default 30s)
- No JavaScript execution (curl equivalent, not Playwright)
- Tool name: `web_fetch`
- Arguments: `{"url": "https://...", "timeout_seconds": 30}`
- **Files:** new `internal/tools/web.go`

#### 6.2 git
- Thin pass-through to local `git` binary (must be installed)
- Allowlisted subcommands: `status`, `log`, `diff`, `branch`, `add`, `commit`, `push`, `pull`, `clone`
- Dangerous ops (reset --hard, force-push) require confirmation
- Tool name: `git`
- Arguments: `{"subcommand": "status", "args": ["--short"]}`
- **Files:** new `internal/tools/git.go`

#### 6.3 json_query
- Parse JSON from a string and extract a value using a jq-style path
- Pure Go implementation (no jq binary needed)
- Useful for model to process structured tool outputs
- Tool name: `json_query`
- Arguments: `{"json": "...", "path": ".name"}`
- **Files:** new `internal/tools/json.go`

#### 6.4 search_files
- Recursive grep across the working directory
- Returns file:line matches, capped at 50 results
- Pure Go (uses `filepath.Walk` + `strings.Contains`)
- Tool name: `search_files`
- Arguments: `{"pattern": "TODO", "path": ".", "max_results": 50}`
- **Files:** new `internal/tools/search.go`

---

### Phase 7 — Context intelligence

Improve what the model knows and remembers.

#### 7.1 Context summarization
- Before trimming old messages, send them to the model with a short "summarize in 2 sentences" prompt
- Cache the summary so it's not recomputed on every trim
- Insert the cached summary as a system-level note at the start of trimmed history
- This preserves facts from old turns without using full tokens
- **Tradeoff:** adds one extra LLM call when context is near capacity
- **Files:** `internal/session/session.go`, new `internal/session/summarize.go`

#### 7.2 Workspace context injection
- On startup, check for a `CONTEXT.md` file in the working directory
- Auto-inject it as a system message ("project context")
- Allows users to describe their project once and have mini-agent always know
- Similar to how CLAUDE.md works for Claude Code
- **Files:** `internal/agent/loop.go`, `internal/config/config.go`

#### 7.3 Goal memory
- Store the last N goal summaries in `~/.mini-agent/goals.json`
- Expose via `/goals` command: list recent goals with status and timestamp
- Let model read goal history via a `read_goals` tool (optional)
- **Files:** new `internal/session/goals.go`, `internal/agent/loop.go`

---

### Phase 8 — Automation

Lightweight scheduling without daemon overhead.

#### 8.1 Cron-style goal scheduling
- Add `schedule` section to `config.yaml`:
  ```yaml
  schedule:
    - cron: "0 8 * * *"       # every day at 8am
      goal: "Check system disk usage and warn if over 80%"
    - cron: "*/30 * * * *"    # every 30 minutes
      goal: "Append current timestamp to ~/heartbeat.log"
  ```
- Add `mini-agent --daemon` flag: runs scheduled goals, exits when no more pending
- Use OS-level cron (`crontab`) or launchd for actual scheduling; mini-agent is the executor
- Alternatively: `mini-agent --run-scheduled` checks a lock file and runs due goals
- **Files:** `internal/config/config.go`, new `internal/scheduler/scheduler.go`, `cmd/mini-agent/main.go`

#### 8.2 Batch mode
- `mini-agent --batch goals.txt` reads goals one per line, executes each, writes results
- Output: JSON lines to stdout (compatible with shell pipelines)
- Enables scripting mini-agent into other tools
- **Files:** `cmd/mini-agent/main.go`, `internal/agent/loop.go`

---

### Phase 9 — UX polish (deferred)

Low-priority items to address after Phases 5-8 are stable.

- `/models` command: list available Ollama models interactively
- Multiline input: allow `\` at end of line to continue input on next line
- `/save <name>` command: save current session to a named file
- Configurable prompt prefix (e.g., show git branch, CWD)
- Better diff view for file edits (show before/after excerpt)
- `--version` flag output

---

## Architecture decisions

### Why not adopt Hermes/OpenClaw patterns wholesale

| Pattern | Hermes/OpenClaw approach | mini-agent approach | Reason |
|---|---|---|---|
| Session storage | Database | JSON file | No operational overhead |
| Tool registration | Auto-discovery via AST | Explicit registry | Explicit is debuggable |
| Async execution | asyncio / async/await | Synchronous Go | Simpler, Go is fast sync |
| Config validation | Zod / schema library | Hand-written checks | No new dep |
| Tool availability | Declarative expressions | Feature flags in config | Good enough for 10 tools |
| Error recovery | Provider failover | Trim + retry | Single provider (Ollama) |

### Tool interface contract (extended for Phase 6)

Every tool must implement:
```go
type Tool interface {
    Name() string
    Description() string
    Schema() map[string]any      // JSON schema for arguments
    Execute(args string) (string, error)
    RequiresBinary() string      // "" if pure Go, "git" if external
}
```

Tools are registered at startup. If `RequiresBinary()` returns a non-empty string and the binary is not found in PATH, the tool is disabled with a warning (not a fatal error).

### Logging contract (Phase 5.2)

One JSON line per tool execution, written to `~/.mini-agent/run.log`:
```json
{"ts":"2026-05-29T20:00:00Z","session":"abc123","tool":"write_file","args":{"path":"hello.py"},"result_bytes":42,"ok":true,"duration_ms":12}
```

Log file rotates at 10 MB. No external logging library.

---

## What we will NOT build (ever, unless mission changes)

1. **Browser automation** — too heavy for target hardware
2. **Voice I/O** — hardware-specific, out of scope
3. **20+ messaging channels** — not a local automation tool
4. **Multi-agent orchestration** — contradicts simplicity mission
5. **Vision/image** — no local model available for old hardware
6. **Cloud model providers** — local-first is non-negotiable
7. **Docker sandboxing** — operational overhead
8. **Plugin SDK** — premature abstraction for current scale
9. **Web UI / gateway** — CLI is the interface

---

## Definition of done for each phase

A phase is complete when:
1. All features in the phase compile cleanly (`go build ./...`)
2. All tests pass (`go test ./...`)
3. The CLAUDE.md acceptance criteria for that phase are met
4. Manual testing with the test prompts in CLAUDE.md passes
5. Commit is tagged (e.g., `v2.7.0` for Phase 5)
