# mini-agent v2

A lightweight local agent for older computers that talks directly to Ollama over `http://localhost:11434/api/chat`.

## What's fixed in V2

- Includes `go.sum` so `go run` works more cleanly.
- Defaults to `qwen2.5-coder:1.5b`, a smaller tool-capable Ollama model.
- Supports `tools.enabled: false` for chat-only mode with models like Gemma.
- Falls back to parsing JSON-like tool requests when a small model prints tool JSON instead of returning structured `tool_calls`.
- Asks for confirmation before `run_command` executes.

## Quick start

```bash
cd mini-agent-v2
chmod +x run.sh
./run.sh
```

## Recommended models

- Tool mode: `qwen2.5-coder:1.5b`
- Chat-only mode on older hardware: `gemma2:2b` or your existing Gemma tag with `tools.enabled: false`

## Switching to chat-only mode

Edit `config.yaml`:

```yaml
tools:
  enabled: false
ollama:
  model: "gemma2:2b"
```

## Example prompts

- `Read ./README.md and summarize it.`
- `Write a file named notes.txt with three optimization tips for Ollama.`
- `Find all mentions of model in config.yaml.`

## Safety note

`run_command` only allows whitelisted commands and asks for confirmation before execution.
