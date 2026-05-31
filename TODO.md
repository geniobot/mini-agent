# mini-agent — TODO

> Prioritized task list. Work top-to-bottom within each tier.
> Reference: PLAN.md for full context and architecture decisions.
> Last updated: 2026-05-31

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

---

## 🔲 Open — Tier 1 (quick wins, do first)

### Bugs
- [x] Remove stale test files from repo root (`hello.txt`, `hello_world.txt`, `hi.txt`, `howareyou.txt`) — deleted
- [x] `run.sh` in repo root is undocumented — added comment explaining it as a convenience wrapper
- [x] Token counter decreases mid-session when compact fires — show `(compacted)` notice so users aren't confused

### 5.1 Model tier documentation
- [ ] Test `/goal` with `qwen2.5-coder:3b` — document whether multi-file goals complete reliably
- [ ] Update README Hardware Guide with model-tier notes:
  - 1.5B: chat + simple single-file ops (multi-file goals unreliable)
  - 3B: reliable file ops and goal mode (recommended minimum)
  - 7B: complex reasoning, multi-step goals, web tasks

---

## 🔲 Open — Tier 2 (Phase 6: remaining tools)

### 6.4 `json_query` tool
- [ ] Create `internal/tools/json.go`
  - Dot-path extractor: `.user.name`, `.items[0].id`
  - Pure Go, no jq dependency
  - Returns `null` (not error) when path missing
- [ ] Register as `json_query` + config key `enable_json_query: true`
- [ ] Unit tests

---

## 🔲 Open — Tier 3 (Phase 7: context intelligence)

### 7.1 Context summarization (opt-in)
- [ ] `internal/session/summarize.go` — LLM-based summary before trimming old messages
- [ ] Config key `agent.summarize_on_compact: false` (default off — adds latency)
- [ ] Unit tests with mock LLM client

### 7.3 Goal memory (`/goals` command)
- [ ] `internal/session/goals.go` — persist last 100 goal records to `~/.mini-agent/goals.json`
- [ ] `/goals` command: list recent goals with status, summary, timestamp
- [ ] Wire into goal.go on completion/failure
- [ ] Unit tests

---

## 🔲 Open — Tier 4 (Phase 8: automation)

### 8.2 Batch mode
- [ ] `--batch <file>` flag: run goals one per line, JSON results to stdout
- [ ] Exit code: 0 all succeeded, 1 any failed
- [ ] `--parallel N`: goroutine pool for concurrent goals

### 8.1 Scheduled goals (`--daemon`)
- [ ] `schedule:` section in config.yaml with cron expressions
- [ ] Minimal pure-Go cron parser (5-field POSIX)
- [ ] `--daemon` flag: sleep/wake loop, PID file, SIGTERM/SIGHUP handling
- [ ] Unit tests for cron parser

---

## 🔲 Open — Tier 5 (UX polish, low priority)

- [ ] Multiline input: `\` at end of line continues to next line
- [ ] `/save <name>`: copy session to `~/.mini-agent/sessions/<name>.json`
- [ ] Better overwrite confirmation: show file size + first line before prompting
- [ ] `install.sh`: automate VERSION bump from `banner.go` constant

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
