# mini-agent — TODO

> Prioritized task list. Work top-to-bottom within each tier.
> Reference: PLAN.md for full context and architecture decisions.
> Last updated: 2026-06-04

---

## ✅ Completed

### Phase 4 — UX baseline
- [x] Context pressure coloring in prompt (dim → yellow → red)
- [x] `/history [N]` command
- [x] Richer tool feedback: `[write_file] hello.py (42 bytes) → wrote 42 bytes`
- [x] `/clear` missing `continue` bug fix
- [x] `~` path expansion in all file tools
- [x] Better system prompt examples (realistic values, not placeholders)
- [x] Version bump to v2.6.0

### Phase 5 — Reliability hardening
- [x] 5.2 Structured run log (`~/.mini-agent/run.log`, JSON lines, 10 MB rotation)
- [x] 5.3 Config validation (`Validate()` + startup check) + `--doctor` flag
- [x] 5.4 Persistent goal results appended to session on completion/stop
- [x] Prompt fixes: numbered RULE format, tighter fallback parser (starts-with-`{`)

### Phase 6 — Tool expansion (partial)
- [x] 6.1 `web_fetch` tool (stdlib HTTP, HTML strip, 32 KB cap)
- [x] 6.2 `search_files` tool (filepath.Walk, case-insensitive, grep output, binary skip)
- [x] 6.3 `git` tool (read-only + confirmed-write subcommands, blocked destructive ops/flags)
- [x] `edit_file` tool (find/replace, unique-match guard, replace_all) — borrowed from Hermes patch tool
- [x] `read_file` offset/limit (line-range reading of large files)
- [x] Telegram bot mode (`--telegram`, long-polling, allowlist security)
- [x] `/goal` persistent goal mode (pause/resume, state to disk, loop detection)
- [x] `/goal` premature-DONE fix (countToolCalls, adaptive directive)
- [x] CONTEXT.md injection (`--no-context` flag, 4 KB cap, shown in startup banner)
- [x] `/models` command (lists Ollama models, marks active)
- [x] `--version` flag (prints version and exits)

### Post v2.7.0 fixes
- [x] `--setup` wizard (provider selection, API key → ~/.zshrc, config patch)
- [x] Banner shows active provider model/host (not always Ollama block)
- [x] Token counter visible for cloud providers (default 8192 ctx)
- [x] Double `/v1` in cloud request URL fixed (strip trailing /v1 from base_url)
- [x] Empty model response shows actionable notice + model suggestion
- [x] `(compacted)` notice only fires when token count actually drops
- [x] Auto-retry on 429 rate limit (parse delay from error, wait, retry ×3)
- [x] Fallback parser handles prose-prefixed JSON blocks

### v2.9.1 — Reliability & UX polish
- [x] Atomic writes for all state/config files (goal state, goal history, session, config.yaml, ~/.zshrc) — prevents corruption on crash
- [x] `--debug` flag: prints raw LLM request/response JSON to stderr
- [x] `--log [N]`: prints last N entries from run.log and exits
- [x] `/inspect` command: per-message token breakdown + full system prompt
- [x] `/compact` command: manual context compaction keeping last 4 messages
- [x] CWD + git branch shown in prompt (`~/dir(branch) [tok] >`)

### Phase 9 — Multi-provider & Claude API support
- [x] 9.1 Model tier detection (weak/standard/frontier patterns)
- [x] 9.2 Tier-aware system prompts (system_prompt_weak, system_prompt_frontier)
- [x] 9.3 Tool parsing fallback for weak models (graceful JSON recovery)
- [x] 9.4 Claude API support (type: anthropic, API key via env var)
- [x] 9.5 Provider wizard update (--setup supports Claude)
- [x] 9.6 Doctor validation for anthropic providers
- [x] 9.7 Documentation and README updates

---

## 🔲 Open — Tier 1 (quick wins, do first)

### Bugs
- [x] Remove stale test files from repo root (`hello.txt`, `hello_world.txt`, `hi.txt`, `howareyou.txt`) — deleted
- [x] `run.sh` in repo root is undocumented — added comment explaining it as a convenience wrapper
- [x] Token counter decreases mid-session when compact fires — show `(compacted)` notice so users aren't confused

### 5.1 Model tier documentation
- [x] Test `/goal` with available models — documented in README Hardware Guide:
  - `qwen2.5-coder:1.5b`: multi-file goal false positive (reports done, no files created) — ~4.5 min
  - `llama3.2:3b`: loop detection after ~10 min (no files created)
  - `qwen2.5-coder:3b` not available on test machine; confirmed as minimum recommendation based on model family behavior
- [x] Update README Hardware Guide with model-tier notes:
  - 1.5B: chat + simple single-file ops (multi-file goals unreliable)
  - 3B: reliable file ops and goal mode (recommended minimum)
  - 7B: complex reasoning, multi-step goals, web tasks

---

## 🔲 Open — Tier 2 (Phase 6: remaining tools)

### 6.4 `json_query` tool
- [x] Create `internal/tools/json.go`
  - Dot-path extractor: `.user.name`, `.items[0].id`
  - Pure Go, no jq dependency
  - Returns `null` (not error) when path missing
- [x] Register as `json_query` + config key `enable_json_query: true`
- [x] Unit tests (14 cases, all passing)

---

## 🔲 Open — Tier 3 (Phase 7: context intelligence)

### 7.1 Context summarization (opt-in)
- [x] `internal/agent/summarize.go` — LLM-based summary of dropped messages (any provider)
- [x] `session.SetSummary()` — injects summary as protected system message (survives future compaction)
- [x] `session.DroppedMessages` — compact captures dropped messages for the agent to summarize
- [x] Config key `agent.summarize_on_compact: false` (default off — adds latency)
- [x] Unit tests (5 new session tests, all passing)

### 7.3 Goal memory (`/goals` command)
- [x] `internal/session/goals.go` — persist last 100 goal records to `~/.mini-agent/goals.json`
- [x] `/goals` command: list recent goals with status, summary, timestamp
- [x] Wire into goal.go on completion/failure
- [x] Unit tests

---

## 🔲 Open — Tier 4 (Phase 8: automation)

### 8.2 Batch mode
- [x] `--batch <file>` flag: run goals one per line, JSON results to stdout
- [x] Exit code: 0 all succeeded, 1 any failed
- [x] `--parallel N`: goroutine pool for concurrent goals

### 8.1 Scheduled goals (`--daemon`)
- [x] `schedule:` section in config.yaml with cron expressions
- [x] Minimal pure-Go cron parser (5-field POSIX) — `internal/scheduler/cron.go`
- [x] `--daemon` flag: sleep/wake loop, PID file, SIGTERM/SIGINT handling
- [x] Unit tests for cron parser (15 cases, all passing)

---

## 🔲 Open — Tier 5 (UX polish, low priority)

- [x] Multiline input: `\` at end of line continues to next line
- [x] `/save <name>`: copy session to `~/.mini-agent/sessions/<name>.json`
- [x] `/retry`: discard last response and regenerate
- [x] Pipe/stdin: `cat file | mini-agent "question"` — auto-quiet, one-shot
- [x] `@file` syntax: inline file reference in any message (`explain @main.go`)
- [x] `--system <text>` flag: override system prompt per session/invocation
- [x] `/copy`: copy last response to clipboard (wl-copy / xclip / xsel / pbcopy)
- [x] Better overwrite confirmation: show file size + first line before prompting
- [x] `install.sh`: bump version to v2.9.0
- [x] Auto-create default config on first run (no more "config not found" error)
- [x] `--completion bash|zsh` flag for tab completion
- [x] Small-model warning at start of `/run` and `/goal` for 1.5b/0.5b models
- [x] `edit_file` inline diff in tool feedback (shows `-` / `+` lines)
- [x] CWD + git branch shown in interactive prompt (`~/dir(branch) [tok] >`)
- [x] `/compact` command: manual context compaction keeping last 4 messages
- [x] `--model` flag now applies to `--batch` and `--run` (was silently ignored)
- [x] Version bump to v2.9.0
- [x] Thinking spinner: shows `| Xs` after 800ms if model is slow; clears cleanly on first token
- [x] `--doctor` now checks git binary availability, session file, and stale goal state
- [x] DONE false-positive guard: goal/goalcmd now require ≥1 tool call before accepting DONE
- [x] `countToolCalls` in goalcmd now covers all 10 tools (was missing edit_file, search_files, git, json_query)
- [x] `write_file` size guard: rejects content >64 KB with actionable error
- [x] `/inspect` command: shows per-message token breakdown and full system prompt
- [x] `--debug` flag: prints raw LLM request/response JSON to stderr

---

## Testing matrix (run before each release)

| Prompt | Expected behavior |
|---|---|
| `hello` | Plain text reply, no file created |
| `explain Python in 10 words` | Plain text, no file created |
| `create a file called hello.py with a hello world program` | `[write_file] hello.py` appears, file exists on disk |
| `read hello.py` | File content printed |
| `append a greet function to hello.py` | `[append_file] hello.py` appears |
| `list the files in the current directory` | `[list_dir]` appears, real file listing |
| `run ls -la` | Confirmation prompt, then real `ls` output |
| `/run create main.py with a main function and a README.md` | Both files created on disk, DONE reported |
| `/goal create a todo app: main.py, README.md, requirements.txt` | All 3 files created, DONE after 3+ tool calls |
| `fetch https://example.com and summarize it` | `[web_fetch]` appears (requires enable_web_fetch: true) |
| `/history 4` | Shows last 4 messages |
| `/status` | Shows model, host, tokens, history count |
| `mini-agent --doctor` | Config OK, Ollama reachable, model ready |

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
