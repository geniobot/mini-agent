# Changelog

All notable changes to mini-agent are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [v2.10.0] — 2026-06-09

### Added

- **`memory` tool** — persistent key-value store at `~/.mini-agent/memory.json`.
  Operations: `set`, `get`, `delete`, `list`. Atomic writes. Enabled via
  `enable_memory: true` in config.
- **`/memory` command** — lists all stored key-value pairs in the running session.
- **`notify` tool** — desktop notification via `notify-send` (Linux) or
  `osascript` (macOS). Silently no-ops if the binary is absent. Enabled via
  `enable_notify: true`. Useful for signalling completion of long-running goals.
- **`/model <name>` command** — switch model mid-session without restarting.
  Re-detects tier and updates the system prompt on switch.
- **Tool error retry** — retryable tool errors (file not found, permission
  denied, no unique match) are classified and tagged `[tool-error]` in goal
  notes. A corrective directive is injected into the next prompt so the model
  can recover rather than give up.

### Performance

- **`search_files` I/O optimisation** — rewrote per-file scan to open each
  file once and stream lines with `bufio.Scanner` instead of reading the full
  file into memory with `io.ReadAll`. Binary detection (null-byte check) is
  inlined into the main loop using a 512-byte stack buffer and
  `io.MultiReader`, eliminating a second `os.Open`. No behaviour change —
  same output format, caps, and skip logic.
- **Goal fast-path for read-only single-action goals** — `read_file`,
  `list_dir`, and `search_files` now auto-complete after step 1 without a
  follow-up LLM round-trip. On slow hardware the model is often evicted
  between steps, making the second step (just saying DONE) as expensive as a
  cold load. Write tools are excluded: step 1 of a multi-file goal is a write,
  and auto-completing there would skip remaining files.

### Fixed

- `memory.json` file permissions restricted to `0600` (owner-read/write only).
  Previously created world-readable, which could expose stored keys to other
  local users.
- Goal fast-path correctly restricted to read-only tools. Initial
  implementation included `write_file`, `edit_file`, and `append_file`, which
  caused multi-file goals to terminate after writing the first file.

---

## [v2.9.1] — 2026-06-04

### Added

- Atomic writes for all state and config files (goal state, goal history,
  session, `config.yaml`, `~/.zshrc`) — prevents file corruption on crash.
- `--debug` flag: prints raw LLM request/response JSON to stderr.
- `--log [N]` flag: prints last N entries from `run.log` and exits.
- `/inspect` command: per-message token breakdown and full system prompt.
- `/compact` command: manual context compaction keeping the last 4 messages.
- CWD and git branch shown in the interactive prompt
  (`~/dir(branch) [tok] >`).

---

## [v2.9.0] — 2026-06-03

### Added

- **Multi-provider support**: Ollama (default), OpenAI-compatible APIs (Groq,
  OpenRouter, LM Studio), and Anthropic/Claude via native endpoint.
- Model tier detection (`weak` / `standard` / `frontier`) with per-tier
  system prompts (`system_prompt_weak`, `system_prompt_frontier`).
- Fallback JSON parser for weak models that return prose-wrapped tool calls.
- `--setup` wizard: provider selection, API key written to `~/.zshrc`, config
  patched in place.
- Doctor validation for Anthropic providers.

---

## [v2.8.0] — 2026-06-01

### Added

- Context summarisation on compact (`summarize_on_compact: true`).
- Goal history persisted to `~/.mini-agent/goals.json`, browsable with
  `/goals`.
- Batch mode: `--batch <file>` runs goals one per line, JSON results to
  stdout; `--parallel N` uses a goroutine pool.
- Daemon mode: `--daemon` flag, cron-style `schedule:` section in
  `config.yaml`, pure-Go 5-field cron parser, PID file, SIGTERM/SIGINT
  handling.

---

## [v2.7.0] — 2026-05-30

### Added

- `web_fetch` tool (stdlib HTTP, HTML strip, 32 KB cap).
- `search_files` tool (filepath.Walk, case-insensitive, grep output, binary
  skip).
- `git` tool (read-only + confirmed-write subcommands, blocked destructive
  ops).
- `edit_file` tool (find/replace, unique-match guard, `replace_all` mode).
- `read_file` offset/limit for line-range reading of large files.
- Telegram bot mode (`--telegram`, long-polling, allowlist security).
- `/goal` persistent goal mode (pause/resume, state to disk, loop detection).
- CONTEXT.md auto-injection (`--no-context` flag, 4 KB cap).
- `/models` command (lists Ollama models, marks active).
- `--version` flag.

---

## [v2.6.0] — 2026-05-28

### Added

- Structured run log (`~/.mini-agent/run.log`, JSON lines, 10 MB rotation).
- Config validation (`Validate()` + startup check) and `--doctor` flag.
- Persistent goal results appended to session on completion or stop.
- Context pressure colouring in prompt (dim → yellow → red).
- `/history [N]` command.
- Richer tool feedback with byte counts and diff previews.
- `~` path expansion in all file tools.
