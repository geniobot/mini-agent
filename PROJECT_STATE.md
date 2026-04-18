# PROJECT_STATE.md

## Overview

Project: mini-agent

Purpose:
Create a lightweight local AI assistant for older computers using Ollama directly, with a terminal-first workflow and minimal overhead.

## Current state

The strongest usable version is V2 (now essentially V2.1).
That is the current baseline.

V2 characteristics:
- Go CLI
- local config file
- direct Ollama API use
- chat works seamlessly (JSON outputs are hidden for human readability)
- optional native tool mode exists conceptually, but relies on a solid strict structured JSON fallback parser for reliability
- tool reliability is solid for small models like `qwen2.5-coder:1.5b`

V3 explored a better control idea using structured planning, but the generated versions had compile/packaging issues and should not be treated as production-ready.

## Problem statement

The original pain point was performance.
OpenClaw was too slow on old CPU-based hardware when paired with local models.
Ollama alone felt much faster.

The project therefore aims to preserve:
- local execution
- agent usefulness
- lightweight footprint

while removing:
- heavy orchestration
- unnecessary layers
- excessive runtime overhead

## Working assumptions

- old hardware needs short contexts
- small models are preferred
- chat must always remain available even if actions are disabled
- action execution should be narrow and safe

## Current implementation themes

- direct HTTP calls to local Ollama
- lightweight session memory
- small config surface
- whitelisted local actions
- confirmation for risky operations

## Recommended next build target

Next real target should be a V2.1 or V4 based on V2, not on the unstable generated V3 packaging.

Recommended scope:
- preserve V2 baseline
- add reliable `write_file`
- add reliable `read_file`
- optionally add overwrite confirmation
- delay broader shell automation until file actions are solid

## Success criteria

A successful next version should let the user do the following reliably:
1. start the app quickly
2. chat with a local model
3. ask to create a text file with given contents
4. ask to read that file back
5. optionally ask for a short safe shell inspection command with confirmation

## Suggested file targets for future work

High priority:
- improve the agent loop
- separate stable chat mode from action mode
- create a minimal action planner

Medium priority:
- add tests for file actions
- add `.gitignore`
- improve README setup notes

Lower priority:
- TUI
- SQLite sessions
- alternative backend support

## Notes for future AI editors

Do not assume native tool-calling will be reliable on the smallest local models.
Do not optimize for feature count first.
Optimize for a stable core loop on low-end hardware.
