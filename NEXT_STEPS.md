# NEXT_STEPS.md

## Immediate next tasks

1. Use V2 as the base branch.
2. Verify the current GitHub repo contents match the V2 baseline.
3. Add `.gitignore`.
4. Add a very small, reliable file action flow.
5. Test on the actual old hardware, not just a faster machine.

## Suggested implementation order

### Step 1: repo cleanup
- add `.gitignore`
- remove scratch files like `hello.txt` if they are not meant to be committed
- ensure README reflects the real current state

### Step 2: stable file actions
- support `write_file`
- support `read_file`
- keep file operations restricted to plain text
- return clear success/failure messages

### Step 3: action selection
- prefer a narrow action planner
- keep schema tiny
- avoid complex tool protocols until stability is proven

### Step 4: safety
- ask before overwrite
- ask before shell commands
- preserve command whitelist

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
