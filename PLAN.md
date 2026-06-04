# mini-agent — Engineering Plan

> Last updated: 2026-06-04
> Based on deep comparative analysis of Hermes Agent (Python) and OpenClaw (TypeScript).
> Phases 5–9 complete as of v2.9.1.

---

## Mission statement (unchanged)

mini-agent is a lightweight local agent that runs well on old hardware using Ollama directly.
It is not a full Hermes/OpenClaw replacement. It captures the useful 20% with 5% of the overhead.

**Core constraints that govern every decision:**
- Local-first; cloud providers supported but not required
- Single compiled Go binary, near-zero dependencies (1 Go module dep)
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

### What both do that we copied (lightweight versions)
1. **Persistent goal results** — both save what goals accomplished to session ✓
2. **Error classification and retry** — transient vs fatal, jittered backoff ✓
3. **Context summarization** — before trimming, summarize old turns ✓
4. **Lightweight tool expansion** — web fetch, git, json formatting ✓
5. **Config validation** — catch bad config early with clear messages ✓
6. **Structured logging** — JSON lines file for audit trail ✓
7. **Scheduled automation** — config-driven cron for recurring goals ✓

---

## Current state (v2.9.1)

### What works
- Interactive REPL with streaming responses (Ollama + OpenAI-compatible + Claude API)
- 10 tools: `read_file`, `write_file`, `edit_file`, `append_file`, `list_dir`, `run_command`, `web_fetch`, `search_files`, `git`, `json_query`
- `/goal` persistent goal mode, `/run` quick goal mode — both with loop detection and step limits
- Context summarization on compact (`summarize_on_compact: true` in config)
- Goal history persisted to `~/.mini-agent/goals.json`, browsable with `/goals`
- Batch mode (`--batch goals.txt`, `--parallel N`) and daemon mode (`--daemon`)
- Session persistence, CONTEXT.md auto-injection, `@file` references
- Config validation, `--doctor` flag, `--setup` wizard
- Structured run log (`~/.mini-agent/run.log`, JSON lines, 10 MB rotation), `--log N` flag
- Telegram bot mode
- Multi-provider support: Ollama (default), OpenAI-compatible (Groq, OpenRouter, LM Studio), Anthropic/Claude
- Model tier detection (weak / standard / frontier) with per-tier system prompts
- Fallback JSON parser for weak models that return prose-wrapped tool calls
- ANSI colors, context pressure indicator, CWD + git branch in prompt
- `/compact` (manual compaction), `/inspect` (token breakdown), `/copy` (clipboard)
- `--debug` flag (raw LLM JSON to stderr), `--completion bash|zsh`
- Atomic writes for all state files (crash-safe)
- 10 test packages, all green

### Known gaps
- Small models (1.5B) unreliable for multi-file goals
- Tool calling reliability varies by model — 3B+ recommended for goal mode
- No desktop notifications for long-running goals
- No persistent key-value memory across sessions (beyond goal history)

---

## Completed phases

| Phase | Summary | Version |
|---|---|---|
| 5 | Reliability hardening: run log, config validation, `--doctor`, goal results | v2.6.0 |
| 6 | Tool expansion: `web_fetch`, `search_files`, `git`, `edit_file`, `json_query` | v2.7.0 |
| 7 | Context intelligence: summarization, CONTEXT.md injection, goal memory | v2.8.0 |
| 8 | Automation: batch mode, `--daemon`, cron scheduler | v2.8.0 |
| 9 | Multi-provider: OpenAI-compat, Claude API, model tiers, fallback parser | v2.9.0 |
| 9.1 | UX polish: `/inspect`, `/compact`, `--debug`, `--log`, atomic writes | v2.9.1 |

---

## Phase 10 — Robustness & quality-of-life

Tighten what exists before adding scope. No new dependencies.

### 10.1 Tool error retry
- **Problem:** when a tool call fails (e.g. file not found, permission denied) the agent often gives up or hallucinates instead of trying a different path
- **Approach:** in goal/run loops, classify tool errors as retryable vs fatal; on retryable failure, inject a corrective system message and allow one more step
- **Files:** `internal/agent/goalcmd.go`, `internal/agent/loop.go`

### 10.2 `notify` tool
- Desktop notification on Linux (`notify-send`) and macOS (`osascript`); silently no-ops if binary absent
- Useful for long-running batch/daemon goals: agent calls `notify` when done
- Pure Go shell-out, zero deps, binary presence checked at startup like `git`
- Tool name: `notify`
- Arguments: `{"title": "mini-agent", "body": "Goal complete"}`
- **Files:** new `internal/tools/notify.go`, `internal/config/config.go` (enable_notify key)

### 10.3 `/model <name>` command
- Switch model mid-session without restarting (currently requires `--model` flag at startup)
- Updates `l.cfg` in place; prints confirmation with new tier detection result
- **Files:** `internal/agent/loop.go`

### 10.4 `memory` tool
- Simple persistent key-value store at `~/.mini-agent/memory.json`
- Operations: `set`, `get`, `delete`, `list`
- Lets the model persist facts across sessions without polluting conversation history
- Pure Go, atomic writes via `fileutil.WriteAtomic`
- **Files:** new `internal/tools/memory.go`, `internal/config/config.go` (enable_memory key)

---

## Architecture decisions

### Tool interface contract

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

### Logging contract

One JSON line per tool execution, written to `~/.mini-agent/run.log`:
```json
{"ts":"2026-06-04T20:00:00Z","session":"abc123","tool":"write_file","args":{"path":"hello.py"},"result_bytes":42,"ok":true,"duration_ms":12}
```

Log file rotates at 10 MB. No external logging library.

### Provider architecture

Providers are declared in `config.yaml` under a `providers:` map. Each has a `type` field:
- `ollama` — local Ollama instance (default)
- `openai_compat` — any OpenAI-compatible API (Groq, OpenRouter, LM Studio, etc.)
- `anthropic` — Claude API via native Anthropic endpoint

The active provider is selected via `default_provider:`. All providers speak the same internal `llm.Client` interface; the agent loop is provider-agnostic.

### Why not adopt Hermes/OpenClaw patterns wholesale

| Pattern | Hermes/OpenClaw approach | mini-agent approach | Reason |
|---|---|---|---|
| Session storage | Database | JSON file | No operational overhead |
| Tool registration | Auto-discovery via AST | Explicit registry | Explicit is debuggable |
| Async execution | asyncio / async/await | Synchronous Go | Simpler, Go is fast sync |
| Config validation | Zod / schema library | Hand-written checks | No new dep |
| Tool availability | Declarative expressions | Feature flags in config | Good enough for 10 tools |
| Error recovery | Provider failover | Trim + retry | Single provider typical |

---

## What we will NOT build

1. **Browser automation** — too heavy for target hardware
2. **Voice I/O** — hardware-specific, out of scope
3. **20+ messaging channels** — not a local automation tool
4. **Multi-agent orchestration** — contradicts simplicity mission
5. **Vision/image** — no reliable local model for old hardware
6. **Docker sandboxing** — operational overhead
7. **Plugin SDK** — premature abstraction for current scale
8. **Web UI / gateway** — CLI is the interface
9. **Vector database / embeddings** — too heavy, JSON file sufficient

---

## Definition of done for each phase

A phase is complete when:
1. All features compile cleanly (`go build ./...`)
2. All tests pass (`go test ./...`)
3. Manual testing with the test matrix in TODO.md passes
4. Commit is tagged (e.g., `v2.10.0` for Phase 10)

---

## Dependency policy

> mini-agent has 1 Go module dependency. Keep it that way.

Allowed (only with strong justification):
- `golang.org/x/net` — only if stdlib `net/http` is insufficient for web_fetch edge cases

Never add:
- ORM or database driver
- Full HTTP framework
- JavaScript/Python runtime
- Cloud SDK
- Browser automation library
