# Contributing to mini-agent

Thanks for your interest. mini-agent is a lightweight local AI agent designed
to run well on older, CPU-only hardware. Every contribution should respect that
constraint — simpler and more reliable beats ambitious and heavy.

---

## Before you start

Read [CLAUDE.md](CLAUDE.md) for the project mission, engineering priorities,
and what we deliberately will not build. If your idea conflicts with those
constraints, open an issue to discuss before writing code.

---

## Quick start

```bash
git clone https://github.com/geniobot/mini-agent
cd mini-agent
go build ./...   # verify it compiles
go test ./...    # run the test suite
go run ./cmd/mini-agent   # start the agent (uses ~/.mini-agent/config.yaml)
```

Requirements: Go 1.22+, [Ollama](https://ollama.com) (or any OpenAI-compatible
API key).

---

## Making changes

### Bugs

1. Open an issue first if the bug is non-trivial, so we can agree on the fix.
2. Add a test that reproduces the bug before fixing it.
3. Keep the fix minimal — don't clean up surrounding code in the same PR.

### Features

1. Open an issue describing the feature and how it fits the project mission.
2. Wait for a maintainer to indicate it's in scope before writing code.
3. Features that add dependencies, require cloud access, or increase startup
   time will not be accepted without a very strong justification.

### Performance improvements

Always welcome, especially changes that reduce latency, memory use, or
syscall count on slow hardware. Include a brief explanation of what you
measured and how.

---

## Code style

- Standard Go formatting (`gofmt`). Run `go vet ./...` before opening a PR.
- No new dependencies. The project has one external dependency
  (`gopkg.in/yaml.v3`) and that's intentional.
- No comments that explain *what* the code does — only comments that explain
  *why* something non-obvious is necessary.
- No error handling for things that cannot happen. Trust the compiler and
  framework guarantees.

---

## Tests

```bash
go test ./...
```

All packages must pass before a PR can be merged. If you add a tool, add
tests for it in the same package (see `internal/tools/memory_test.go` for
a reference).

---

## Commit messages

Use the conventional commit format:

```
<type>: <short summary>

<body — what changed and why, if non-obvious>
```

Types: `feat`, `fix`, `perf`, `docs`, `chore`, `test`.

---

## Pull requests

- One logical change per PR.
- Reference any related issue in the PR description.
- The PR description should explain *why* the change is needed, not just what
  it does.

---

## What we will not accept

- Browser automation, voice I/O, multi-agent orchestration, Docker sandboxing,
  plugin SDKs, web UIs, vector databases, or cloud-only features.
- New external Go dependencies without prior discussion.
- Changes that break the single-binary, near-zero-dependency contract.

See [CLAUDE.md](CLAUDE.md) for the full list.
