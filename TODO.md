# mini-agent — TODO

> Prioritized task list. Work top-to-bottom within each tier.
> Reference: PLAN.md for full context and architecture decisions.
> Last updated: 2026-05-29

---

## Tier 1 — Phase 5: Reliability hardening

These fix observed failures from live testing. No new features.

### 5.1 Tool prompt reliability
- [ ] Test with `qwen2.5-coder:3b` and document results
- [ ] Update README with recommended models by hardware tier:
  - 1.5B: basic chat, simple file ops (unreliable for code files)
  - 3B: reliable file ops and goal mode (recommended minimum)
  - 7B: complex reasoning, multi-step goals
- [ ] Add `num_predict` note in config.yaml comment: "raise to 2048 for long files"

### 5.2 Structured run log
- [ ] Create `internal/log/log.go`
  - JSON-lines writer to `~/.mini-agent/run.log`
  - Fields: `ts`, `session_id`, `tool`, `args_summary`, `result_bytes`, `ok`, `duration_ms`
  - Rotate at 10 MB (rename to `run.log.1`, start fresh)
  - Thread-safe (mutex or channel-based)
- [ ] Wire into `tools/registry.go`: wrap `Execute()` to log before/after
- [ ] Add unit test for rotation trigger
- [ ] Add `--log` flag to specify log path (default `~/.mini-agent/run.log`)

### 5.3 Config validation
- [ ] Add `Validate() error` method to `config.Config`
  - Check `ollama.host` is a valid URL
  - Check `ollama.num_ctx` > 0 and <= 131072
  - Check `ollama.num_predict` > 0
  - Check `agent.max_history` >= 1
  - Check `agent.step_timeout_seconds` >= 0
  - Check `agent.max_goal_steps` >= 1
  - Warn (not fatal) on unknown YAML keys
- [ ] Call `cfg.Validate()` in `main.go` after `config.Load()`
- [ ] Add `--doctor` flag to `main.go`
  - Loads config, runs Validate()
  - Pings Ollama (`llm.Ping`)
  - Lists available models (`llm.ListModels`)
  - Reports: config OK / Ollama reachable / model available or not
  - Exits 0 on success, 1 on any failure
- [ ] Add unit tests for each validation rule

### 5.4 Persistent goal results
- [ ] In `internal/agent/goal.go`, after `checkDone()` returns true:
  - Build a session message: `role: "assistant"`, content: `[Goal completed: "<goal>" → <summary>]`
  - Append it to `l.session` (so it persists when session is saved)
- [ ] In `runGoal()`, on "goal limit reached" or "no progress" exits:
  - Append: `[Goal stopped: "<goal>" — reason]`
- [ ] Add test: run a mock goal, verify session contains goal result message
- [ ] Verify `/history` shows the goal result on next session load

---

## Tier 2 — Phase 6: Lightweight tool expansion

Add 4 tools. Each must be pure Go or thin POSIX wrapper. No new module deps.

### 6.1 web_fetch tool
- [ ] Create `internal/tools/web.go`
  - `WebFetch(url string, timeoutSeconds int) (string, error)`
  - Use `net/http` from stdlib (no new dep)
  - Timeout via `http.Client{Timeout: ...}`
  - Truncate body at 32 KB, append `[truncated]` notice
  - Strip HTML tags for readability (simple regex, no library)
  - Return error on non-2xx status codes
- [ ] Register in `tools/registry.go` as `web_fetch`
- [ ] Add to config: `enable_web_fetch: false` (opt-in, default off)
- [ ] Add to system prompt examples in `config.yaml`
- [ ] Add unit tests (mock HTTP server)

### 6.2 git tool
- [ ] Create `internal/tools/git.go`
  - Allowed subcommands: `status`, `log`, `diff`, `branch`, `show`, `add`, `commit`, `push`, `pull`, `clone`, `stash`, `fetch`
  - Blocked subcommands: `reset`, `rebase`, `filter-branch`, `force-push equivalent`
  - Destructive subcommands (`add`, `commit`, `push`, `pull`) require user confirmation if `confirm_git_write: true` in config
  - Detect if `git` binary is in PATH; if not, disable tool gracefully with startup warning
  - Cap output at 8 KB
- [ ] Register as `git` tool in `tools/registry.go`
- [ ] Add to config: `enable_git: false`, `confirm_git_write: true`
- [ ] Add unit tests (uses `t.TempDir()` + `git init`)

### 6.3 search_files tool
- [ ] Create `internal/tools/search.go`
  - `SearchFiles(pattern, path string, maxResults int) (string, error)`
  - Uses `filepath.Walk` + case-insensitive `strings.Contains`
  - Returns `file:line: content` format (same as grep)
  - Skips: `.git/`, `node_modules/`, `vendor/`, binary files (check for null bytes)
  - Caps at `maxResults` matches (default 50)
  - Returns "no matches" string (not error) when nothing found
- [ ] Register as `search_files` in `tools/registry.go`
- [ ] Add to config: `enable_search_files: true`
- [ ] Add unit tests

### 6.4 json_query tool
- [ ] Create `internal/tools/json.go`
  - Parse JSON input, extract a value by dot-path (e.g., `.user.name`)
  - Support: object keys, array indices, nested paths
  - Pure Go, no jq dependency (stdlib `encoding/json` + recursive descent)
  - Return extracted value as formatted string
  - Return `null` (not error) when path doesn't exist
- [ ] Register as `json_query` in `tools/registry.go`
- [ ] Add to config: `enable_json_query: true`
- [ ] Add unit tests for nested paths, arrays, null cases

---

## Tier 3 — Phase 7: Context intelligence

### 7.1 Context summarization
- [ ] Create `internal/session/summarize.go`
  - Function: `Summarize(ctx context.Context, client llm.Client, msgs []Message, model string) (string, error)`
  - Prompt: send old messages + "Summarize the above conversation in 2-3 sentences, preserving key facts and file names."
  - Cache result in session: store as `summarized_at` + `summary` fields in session JSON
  - Only re-summarize if new messages were added since last summary
- [ ] In `session.go` `compact()`: before dropping oldest messages, call `Summarize`; prepend summary as a dim system note
- [ ] Make this opt-in: `agent.summarize_on_compact: false` in config (default off — adds LLM call latency)
- [ ] Add unit tests (mock LLM client)

### 7.2 Workspace context injection (CONTEXT.md)
- [ ] In `agent/loop.go`, `New()` function:
  - Check for `CONTEXT.md` or `.mini-agent/CONTEXT.md` in working directory
  - If found, read and prepend as a system message: `"Project context:\n" + content`
  - Cap at 4 KB (don't let it dominate context window)
  - Log: `[loaded CONTEXT.md — N tokens]`
- [ ] Add `--no-context` flag to skip this injection
- [ ] Document in README: create CONTEXT.md to give mini-agent project awareness

### 7.3 Goal memory
- [ ] Create `internal/session/goals.go`
  - `GoalRecord`: `{id, goal, status, summary, started_at, finished_at}`
  - `SaveGoal(path, record)` / `LoadGoals(path) []GoalRecord`
  - Store to `~/.mini-agent/goals.json`
  - Keep last 100 records; trim oldest beyond that
- [ ] Wire into `goal.go`: save record on goal start and update on completion/failure
- [ ] Add `/goals` command to `loop.go`: list last N goals with status and summary
- [ ] Add unit tests for save/load/trim

---

## Tier 4 — Phase 8: Automation

### 8.1 Scheduled goals
- [ ] Add `schedule` section to `config.yaml` and `config.Config` struct:
  ```yaml
  schedule:
    - cron: "0 8 * * *"
      goal: "Check disk usage and log if over 80%"
  ```
- [ ] Create `internal/scheduler/scheduler.go`
  - Parse cron expression (implement minimal parser: 5-field POSIX cron)
  - `NextRun(expr string, from time.Time) (time.Time, error)`
  - No external cron library
- [ ] Add `--daemon` flag to `main.go`:
  - Load scheduled goals from config
  - Sleep until next due goal, execute, log result, repeat
  - Write PID to `~/.mini-agent/daemon.pid`
  - Signal handling: SIGTERM / SIGINT = clean shutdown
  - SIGHUP = reload config
- [ ] Add unit tests for cron parser (standard and edge cases)

### 8.2 Batch mode
- [ ] Add `--batch <file>` flag to `main.go`
  - Read goals one per line (ignore blank lines and `#` comments)
  - Execute each goal sequentially via `loop.RunGoal()`
  - Write JSON result per line to stdout: `{"goal":"...","status":"done","summary":"..."}`
  - Exit code: 0 if all succeeded, 1 if any failed
- [ ] Add `--parallel N` flag: run up to N goals concurrently (goroutine pool)
- [ ] Add unit/integration tests

---

## Tier 5 — Phase 9: UX polish (low priority)

- [ ] `/models` command: call `llm.ListModels()`, print table with model name
- [ ] `--version` flag: print version and exit
- [ ] Multiline input: if line ends with `\`, concatenate with next line
- [ ] `/save <name>` command: copy current session to `~/.mini-agent/sessions/<name>.json`
- [ ] Better overwrite confirmation: show file size and first line before asking
- [ ] `install.sh` version: automate VERSION bump from `banner.go` constant

---

## Bugs / known issues (fix any time)

- [ ] `git commit` identity warning on machines with no global git config — add note to README
- [ ] Goal mode: model sometimes outputs `DONE` after empty step even with improved prompt — needs more testing with 3B model
- [ ] Token counter can decrease mid-session (compact fires) — consider showing "(compacted)" notice when this happens
- [ ] `/clear` resets session but doesn't clear the save path, so the empty session overwrites the file — verify this is intentional
- [ ] `hello_world.txt`, `hello.txt`, `hi.txt` created by early testing session left in repo root — add to `.gitignore`: `*.txt` (already there, check that it catches them)
- [ ] The `run.sh` in root is undocumented — clarify purpose or remove

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
| `can you browse the web?` | Honest "no" if web_fetch disabled |
| `/history 4` | Shows last 4 messages |
| `/status` | Shows model, host, tokens, history count |
| `/clear` then `hello` | Fresh session, no memory of previous messages |

---

## Dependency policy

> mini-agent has 1 Go module dependency. Keep it that way.

Allowed additions (only with strong justification):
- `golang.org/x/net` — only if needed for web_fetch and stdlib `net/http` is insufficient
- Nothing else without explicit decision

Never add:
- ORM or database driver
- Full HTTP framework (gin, echo, fiber)
- JavaScript/Python runtime
- Cloud SDK
- Browser automation library
