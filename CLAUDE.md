# CLAUDE.md

## Mission

Continue building a lightweight local agent that works well on old hardware and uses Ollama directly.

The target experience is:
- local
- simple
- fast enough on CPU-only machines
- useful for small local automation tasks

## Product summary

This is not meant to be a full OpenClaw replacement.
It is meant to capture the useful part of that experience with far less overhead.

Main principle:
- fewer layers
- fewer round trips
- shorter prompts
- smaller models
- simpler local tools

## User context

The target machine is an older Mac mini class box running Ubuntu server with limited performance.
The user specifically noticed that Ollama alone runs much faster than OpenClaw on this machine.
That observation is the core reason for this project.

## Stable baseline

Treat V2 as the practical baseline.
The user liked V2 best and pushed that version to GitHub.

Assume:
- V2 chat path works
- tool/action path still needs redesign or hardening
- V3 had interesting ideas but was not stable enough

## Engineering priorities

1. Do not break chat-only mode.
2. Keep startup and dependencies minimal.
3. Prefer zero or near-zero extra dependencies.
4. Keep CLI UX clean and easy to debug.
5. Make file actions reliable before adding more features.

## Preferred roadmap

### Phase 1

Harden the chat baseline:
- confirm config loading is simple
- confirm `go run` works cleanly
- confirm Ollama model switching is easy

### Phase 2

Implement reliable actions:
- `write_file`
- `read_file`
- optional file overwrite confirmation

### Phase 3

Add cautious shell support:
- keep whitelist
- keep explicit confirmation
- keep output small

### Phase 4

Improve UX:
- better prompts
- small TUI later if useful
- session persistence only if lightweight

## Constraints

Keep these constraints in mind:
- old CPU
- limited RAM
- local-only preference
- no giant framework stacks
- no cloud dependencies

## Architecture guidance

Good:
- Go CLI
- local config file
- direct Ollama HTTP usage
- deterministic/simple control flow
- one assistant loop

Bad:
- multi-agent orchestration
- long chain-of-thought prompting
- large browser-first app as MVP
- complex plugin ecosystems
- hidden magic that is hard to debug

## Action design guidance

If implementing local actions, prefer this order of reliability:
1. deterministic parser for narrow prompts
2. structured outputs with tiny schema
3. native tool-calling only if the selected model behaves reliably

For the target hardware, robustness matters more than elegance.

## Test prompts

Use these prompts when validating behavior:
- `hi`
- `Explain in 3 bullets what this program does`
- `Create a file named hello.txt with the text Hello from mini agent inside`
- `Read hello.txt`
- `Write a file named notes.txt with 3 short optimization tips for Ollama on older hardware`

## Acceptance criteria for next version

A next version is good if:
- it compiles cleanly
- it runs with simple setup
- normal chat works reliably
- file creation works reliably
- file reading works reliably
- shell commands remain optional and confirmed

## Repository hygiene

Keep repository contents simple:
- source
- config example
- README
- handoff docs
- avoid committing local junk

Good files to ignore in future:
- generated scratch files
- experiment outputs
- local test notes
- editor state
- temporary binaries

## Decision rule

When there is a tradeoff, choose:
- simpler over clever
- reliable over ambitious
- local over distributed
- transparent over magical
