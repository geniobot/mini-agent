# AGENTS.md

## Project

mini-agent is a lightweight local AI assistant meant to run on older hardware, especially a 2012 Mac mini / Ubuntu server class machine with limited CPU and RAM.

The main goal is to provide a local agent-like experience similar in spirit to OpenClaw, but much lighter, with less orchestration overhead and better performance on CPU-only systems.

## Core product goal

Build a small local assistant that:
- runs fully local with Ollama
- works acceptably on older computers
- supports normal chat
- gradually supports useful local actions such as reading files, writing files, and optionally running a small set of safe shell commands
- avoids heavy multi-agent frameworks, vector databases, large web UIs, or complex orchestration

## Why this project exists

OpenClaw + Ollama + Gemma worked too slowly on the target hardware.
Running Ollama alone with Gemma was much faster.
This project exists to keep the local/Ollama approach while removing most of the overhead.

## Hardware target

Primary target:
- older Intel machine
- example: 2012 Mac mini
- 16 GB RAM
- CPU-focused inference
- Ubuntu server environment

Design decisions should favor:
- low RAM usage
- short context windows
- small models when possible
- direct local API calls
- terminal-first workflow

## Current architecture direction

Preferred stack:
- Go CLI app
- direct calls to Ollama `/api/chat`
- short rolling history
- optional local actions
- explicit confirmation before dangerous actions

Backends considered:
- Ollama first
- llama.cpp server as an optional future backend, not required for MVP

## Versions created so far

### V1

Initial Go CLI MVP with:
- direct Ollama chat calls
- simple session memory
- tool registry
- YAML config

Problem:
- depended on native Ollama tool-calling behavior
- small local models were inconsistent

### V2

Improved MVP with:
- `go.sum`
- optional native tools mode (`use_native_tools`)
- highly reliable fallback parsing when the model prints structured JSON tool requests
- hidden JSON outputs in the terminal for seamless chat
- confirmation before `run_command`

Status:
- plain chat works seamlessly
- fallback JSON tool mode is now stable and reliable with small models like `qwen2.5-coder:1.5b`
- this is the version the user liked most overall and pushed to GitHub

### V3

Alternative design using structured JSON planning instead of native tool-calling.

Idea:
- ask the model for a single structured action: `answer`, `read_file`, `write_file`, or `run_command`
- execute locally after parsing the structured response

Status:
- architecture direction is promising
- generated packaging had compile issues
- not considered stable yet

## Current known-good behavior

Reliable today:
- local CLI chat through Ollama
- small-model operation on older hardware
- direct use with models like `gemma2:2b` in chat-only mode
- use with `qwen2.5-coder:1.5b` for experimentation
- robust action planning and file action execution using strict structured JSON fallback

Not yet fully reliable:
- seamless native tool behavior comparable to a polished agent product (depends heavily on model capabilities)

## Models used during development

Observed models:
- `gemma4:e2b`
- `gemma2:2b`
- `qwen2.5-coder:1.5b`
- `mistral-small:latest`

Practical guidance:
- Gemma is good for lightweight local chat
- Qwen coder small model is better for structured/action-oriented experiments
- keep contexts small on the target hardware

## Safety model

Action execution should stay minimal and explicit.

Current rule set:
- allow only a short whitelist of commands
- ask before running shell commands
- allow text file read/write only
- avoid arbitrary command execution by default

## What another AI agent should do next

Priority order:
1. Keep V2 as the stable baseline.
2. Extract the strongest ideas from V3 without breaking compile/run simplicity.
3. Implement a reliable file-action flow.
4. Test prompts like:
   - create a file with text
   - read that file
   - update a file
5. Keep everything terminal-first and lightweight.

## Recommended immediate direction

Best path forward:
- branch from V2
- keep normal chat working first
- add a simpler action layer for only `read_file` and `write_file`
- use structured output or deterministic prompt design for action selection
- do not depend on full native tool-calling unless the chosen model proves reliable

## Non-goals

Avoid turning this into:
- a heavy web app first
- a multi-agent platform
- a vector DB / RAG system for the MVP
- a complex OpenClaw clone
- a framework with large runtime overhead

## Developer notes

Important practical notes:
- the target user values local-first, simple, lightweight software
- performance on old hardware matters more than fancy architecture
- a stable 80% solution is better than an ambitious but fragile agent
- preserve straightforward local operation and easy debugging
