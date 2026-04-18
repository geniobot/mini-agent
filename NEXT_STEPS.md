# NEXT_STEPS.md

## Immediate next tasks

1. Test on the actual old hardware, not just a faster machine.
2. Polish the terminal experience further (e.g., colors, loading animations).

## Suggested implementation order

### Step 1: repo cleanup
- [x] add `.gitignore`
- [x] remove scratch files if they are not meant to be committed
- [x] ensure README reflects the real current state

### Step 2: stable file actions
- [x] support `write_file`
- [x] support `read_file`
- [x] keep file operations restricted to plain text
- [x] return clear success/failure messages

### Step 3: action selection
- [x] prefer a narrow action planner
- [x] keep schema tiny
- [x] avoid complex tool protocols until stability is proven

### Step 4: safety
- [x] ask before shell commands
- [x] preserve command whitelist

### Step 5: hardware tuning
- keep context low
- keep prompts short
- compare `gemma2:2b` vs `qwen2.5-coder:1.5b`
- measure perceived latency

## Good first acceptance test

- launch app
- say `hi`
- say `Create a file named hello.txt with the text Hello from mini agent inside`
- say `Read hello.txt`
- verify correct local behavior

## Out of scope for now

- web UI
- multi-agent system
- RAG
- embeddings database
- remote APIs
- complex agent memory systems
