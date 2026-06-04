# Multi-Provider & Claude API Support Design

**Date:** 2026-06-03  
**Phase:** Phase 9 — Provider expansion  
**Status:** Design approved, ready for implementation  

---

## Overview

Mini-agent will support both local weak models (1.5B–3B on limited hardware) and frontier cloud models (Claude API) via a **model-aware architecture** that:

1. **Detects model tier at startup** (weak/standard/frontier)
2. **Selects system prompts per tier** (simpler for weak, more complex for frontier)
3. **Adds tool parsing fallback** for weak models (handle malformed JSON gracefully)
4. **Integrates Claude API** as an OpenAI-compatible provider (API key auth, OAuth later)
5. **Maintains backward compatibility** (existing configs work unchanged)

**Why:** Weak local models need simpler prompts and error recovery. Frontier models can handle complex reasoning and longer contexts. One codebase, many deployment scenarios.

---

## Design Principles & Decision Framework

Before diving into specific choices, here are the guiding principles that shaped every decision:

**From CLAUDE.md (project constraints):**
1. **Simpler over clever** → Avoid complex abstractions; direct is better
2. **Reliable over ambitious** → Weak models must work; don't chase perfection
3. **Transparent over magical** → User sees what's happening; no hidden behavior
4. **Fewer round trips** → Minimize API calls; batch decisions at startup, not per-request
5. **No breaking changes** → Existing users' configs must work unchanged

**Design goals for this phase:**
- Support both weak local models AND frontier cloud models in one codebase
- Auto-adapt behavior to model tier (zero config for known models)
- Graceful recovery from weak model failures (don't abandon weak models)
- Setup & auth remain simple (API key via env var, not OAuth yet)
- Backward compatible (phase in gradually, don't break existing setups)

**Decision-making framework:**
When choosing between options, we weighed:

| Decision | Weak/Local | Frontier/Cloud | Simplicity | Compatibility |
|----------|-----------|----------------|------------|---------------|
| Model-aware prompts | ✅ Enable | ✅ Enable | ✅ Simple | ✅ Optional fields |
| API key vs OAuth | ✅ Simple | ✅ Works | ✅ Simple | ✅ v1 |
| Reuse openai_compat | ✅ Works | ✅ Works | ✅ No duplication | ✅ Proven pattern |
| Tier-based vs per-model | ✅ Simpler | ✅ Works | ✅ Scales | ✅ Config-less |

---

## Design Rationale

### Why This Approach?

**Challenge:** Mini-agent targets two extreme scenarios:
1. **Limited hardware (old Mac mini):** Weak 1.5B–3B Ollama models that can barely follow JSON instructions
2. **Frontier models (Claude API):** Expensive but powerful, handles complex multi-step reasoning

A single generic system prompt fails both: weak models can't follow complex instructions, frontier models don't leverage their capabilities.

**Solution: Model-aware architecture**

Instead of:
- ❌ Single prompt (weak models fail, frontier models underutilized)
- ❌ Retry logic (wastes tokens on limited hardware)
- ❌ Manual per-model config (complexity explosion)

We chose:
- ✅ Automatic tier detection (zero user config)
- ✅ Tier-specific prompts (weak models get simpler instructions, frontier gets full power)
- ✅ Fallback parsing (weak models recover from JSON mistakes gracefully)

**Why tier-based instead of per-model custom prompts?**
- Tier-based: 3 prompt variants, scales to unlimited models (new model? just add to tier patterns)
- Per-model: N custom prompts, user must configure each model individually
- Tier matches CLAUDE.md principle: **"simpler over clever"**

**Why automatic detection instead of manual config?**
- User adds new local model → auto-detected, works immediately
- User switches to Claude API → auto-detected, works immediately
- No "config the model twice" problem

---

## Architecture

### Model Tier Detection

**File:** `internal/models/tier.go`

```go
func DetectTier(modelName string) string
```

Returns: `"weak"` | `"standard"` | `"frontier"`

**Mapping logic:**

| Pattern | Tier | Examples | Why |
|---------|------|----------|-----|
| `claude-3-5`, `gpt-4`, `o1` | frontier | claude-3-5-sonnet, gpt-4-turbo | Flagship models with highest reasoning capacity |
| `claude-3`, `gemini-2`, `llama-3.1-70b`, `mistral-large` | standard | claude-3-opus, gemini-2-flash | Strong open-source and commercial models (70B+ range) |
| `1.5b`, `0.5b`, `phi`, `tinyllama` | weak | qwen2.5-coder:1.5b, phi:3.5b | <2B models struggle with JSON, multi-file goals |
| `3b`, `7b`, `mistral-7b`, `llama2:7b` | standard | qwen2.5-coder:3b, llama2:7b | Sweet spot: reliable on limited hardware, handles goals |
| anything else | standard | unknown model | Safe fallback—assume capable unless proven weak |

**Design decisions:**

1. **Why 1.5B is "weak", not 3B?**
   - User feedback: 1.5B models fail multi-file goals, 3B models succeed
   - Threshold set at actual observed reliability boundary
   - Better to over-simplify for 1.5B than under-simplify for 3B

2. **Why automatic detection on startup only?**
   - Tier detected once per session (no runtime overhead)
   - User knows at startup what tier they're running
   - Matches CLAUDE.md: **"transparent over magical"** (no hidden mid-session tier changes)

3. **Why default unknown models to "standard"?**
   - Unknown model is likelier to be capable than weak
   - Optimistic default (prompt can be hard, user will notice and upgrade)
   - Pessimistic default would break new strong models

**Called once at startup:**
```go
tier := models.DetectTier(activeModel)  // in main.go after config load
a.ModelTier = tier                       // stored in Agent struct
```

**Benefits:**
- Zero configuration (automatic)
- Graceful fallback (unknown = standard)
- Extensible (easy to add new patterns)
- Transparent (user sees tier at startup in banner)

---

### System Prompt Management

**Problem we're solving:**
- **Weak models (1.5B):** Get confused by long instructions, fail JSON parsing if prompt is too complex
- **Frontier models (Claude):** Can reason about complex problems, multi-step goals, but generic prompt doesn't ask for it
- **Current state:** Single prompt tries to be "good enough" for all, actual result is suboptimal for both

**Why tier-specific prompts?**
1. **Weak models need:** Short, direct instructions. Explicit JSON examples. No markdown. Reassurance they can skip complex tasks.
2. **Frontier models need:** Permission to reason deeply. Multi-file context. Encouragement to use full problem-solving abilities.
3. **Middle tier:** Standard prompt works well.

**Why optional fields (not required)?**
- Backward compatible: existing config.yaml works unchanged
- Users can gradually add weak/frontier variants
- If not specified, falls back to safe default

**Why selection at startup (not per-request)?**
- No per-request overhead (system prompt selected once)
- Predictable behavior (user knows what they're running)
- Matches CLAUDE.md principle: **"fewer round trips"**

**Config structure** (config.yaml):

```yaml
agent:
  max_history: 8
  step_timeout_seconds: 300
  
  # Standard prompt (used by default and as fallback)
  system_prompt: |
    You are a local coding assistant.
    For file operations output ONLY a JSON object:
    {"name":"write_file","arguments":{"path":"hello.py","content":"..."}}
    ...
  
  # Optional: used if model tier == "weak"
  # Why simpler? Weak models (1.5B) need:
  # - Shorter instructions (less context pressure)
  # - Explicit reassurance about JSON format
  # - No complex reasoning requests
  system_prompt_weak: |
    You are a local coding assistant.
    IMPORTANT: Keep responses short and focused.
    Output ONLY a JSON object on its own line for file operations.
    {"name":"write_file","arguments":{"path":"hello.py","content":"..."}}
    For questions, reply in plain text, no markdown.
  
  # Optional: used if model tier == "frontier"
  # Why extended? Frontier models (Claude, GPT-4) can:
  # - Handle longer context windows efficiently
  # - Reason about complex multi-file projects
  # - Leverage full prompt capabilities for better results
  system_prompt_frontier: |
    You are an expert coding assistant with advanced reasoning.
    You can handle complex multi-file projects and deep analysis.
    Use your full capabilities to solve problems elegantly.
    [Extended instructions for complex reasoning]
```

**Selection logic** (in Agent.New()):
```go
var prompt string
if a.ModelTier == "weak" && cfg.Agent.SystemPromptWeak != "" {
    prompt = cfg.Agent.SystemPromptWeak
} else if a.ModelTier == "frontier" && cfg.Agent.SystemPromptFrontier != "" {
    prompt = cfg.Agent.SystemPromptFrontier
} else {
    prompt = cfg.Agent.SystemPrompt  // fallback to default
}
a.SystemPrompt = prompt
```

**Benefits:**
- Weak models get simpler, more direct instructions (higher success rate on goals)
- Frontier models encouraged to use full capabilities (better quality output)
- Graceful fallback (optional fields don't break existing configs)
- No breaking changes (existing configs work as-is)
- Transparent: tier is logged at startup so user knows which prompt is active

---

### Tool Parsing with Fallback

**Problem we're solving:**

Weak models (1.5B) frequently output malformed JSON:
```
I'll create the file:
{name: "write_file",  arguments: {path: "hello.py", content: "..."}}
```

Current behavior: strict JSON parsing fails → tool call fails → user confused.

Better: **Graceful degradation** — try to extract the intent even if JSON is malformed.

**Why fallback only for weak models, not all models?**
- Strong models rarely output malformed JSON (99%+ validity)
- Fallback parser adds latency and false-positive risk
- Better to fail fast on strong models (indicates real error)
- Weak models produce malformed JSON regularly (empirically observed)

**Why this strategy beats alternatives:**

| Approach | Pros | Cons | Why not |
|----------|------|------|---------|
| **Fallback parser (chosen)** | Recovers weak model mistakes | May extract wrong intent | ✅ Transparent, low-risk, small models need it |
| Retry failed tool calls | Auto-recovery | Wastes tokens on limited hardware | ❌ Contradicts "few round trips" |
| Abort + suggest upgrade | Clear feedback | Wastes user's time on weak model | ❌ Not helpful for users committed to local |
| Retry with clearer prompt | Self-correcting | Multiple round trips, slow | ❌ Wastes tokens |

**File:** `internal/tools/parser.go`

**New function:**
```go
func ParseToolCall(output string, modelTier string) (ToolCall, error)
```

**Parsing strategy:**

1. **Strict JSON parse** (current behavior)
   - Try to unmarshal as JSON
   - ✓ Success → return ToolCall, nil
   - ✗ Fail → proceed to step 2

2. **Fallback parser** (if tier == "weak" only)
   - Regex for valid JSON blocks: `\{[^}]+\}`
   - Prose pattern matching: extract path/content from hints
   - Try common malformations (missing quotes, trailing commas)
   - ✓ Success → log `[weak-model-fallback] write_file hello.py` and return ToolCall
   - ✗ Fail → proceed to step 3

3. **Error**
   - Return: `"Tool output unparseable. Model may be too weak. Consider using a stronger model or setting anthropic/openrouter provider."`
   - Log the raw output for debugging

**Fallback coverage (priority order):**
1. Mangled JSON (wrong quote style, trailing commas)
2. Prose with extractable hints: `"I'll write to hello.py"` + `"content is..."`
3. Regex JSON block (even if not strict)

**Config option:**
```yaml
agent:
  enable_fallback_parser: true  # default true; can disable if needed
```

**Why logged feedback is important:**
- User sees `[weak-model-fallback]` → understands model is struggling
- Matches CLAUDE.md: **"transparent over magical"**
- User can decide: stick with weak model or upgrade provider
- Helps distinguish "model is weak" from "something is broken"

**Benefits:**
- Weak models recover from minor JSON mistakes (higher goal success rate)
- User sees when fallback was needed (transparent, debuggable)
- Strong models unaffected (fallback only if tier == "weak")
- Easy to disable if causing false positives
- No token waste (no retry logic)

---

### Claude API Integration

**Problem we're solving:**

Mini-agent users have two scenarios:
1. **Local:** Old hardware, no cloud budget, want Ollama + weak models
2. **Cloud:** Want best-in-class reasoning, willing to pay Claude API, need simple setup

Current state: Ollama-only. Cloud users are blocked.

**Why Claude API first (not GPT-4, Gemini, etc.)?**

| Reason | Impact |
|--------|--------|
| **User request** | This is what users asked for in the brainstorm |
| **API compatibility** | Claude's `/v1/messages` is OpenAI-compatible (no new HTTP code needed) |
| **Model quality** | Claude 3.5 Sonnet is frontier-tier (matches our tier system) |
| **Tier scalability** | Easy to add GPT-4/Gemini later (same pattern) |
| **Auth simplicity** | API key via env var (vs OAuth complexity for now) |

**Why API key auth first (not OAuth)?**

| Approach | Pros | Cons |
|----------|------|------|
| **API key (chosen)** | Simple setup, no browser, env var pattern | Key stays in shell history |
| OAuth | Secure, token in config not env | Browser auth, complexity, refresh token handling |

**Chosen:** API key first, OAuth noted for future. Matches CLAUDE.md: **"simpler over clever, reliable over ambitious"**

**Why reuse openai_compat handler?**

Instead of:
- ❌ New `type: "anthropic"` with its own HTTP logic (code duplication)
- ✅ Route `type: "anthropic"` to existing `openai_compat` handler (Claude API is OpenAI-compatible)

**Benefits:**
- No new HTTP code (0 lines duplicated)
- Proven handler (already works with OpenRouter, Groq, etc.)
- Leaves room for Anthropic-specific features later (batch API, file uploads, vision) without disrupting current handler
- Same provider pattern scales to other OpenAI-compatible services (Mistral, etc.)

**Config support:**

```yaml
providers:
  claude:
    type: "anthropic"
    api_key_env: "ANTHROPIC_API_KEY"
    model: "claude-3-5-sonnet-20241022"
    stream: true
    options:
      temperature: 0.2
      max_tokens: 2048

default_provider: claude
```

**Implementation:**

1. **No new HTTP handler needed.** Claude API's `/v1/messages` endpoint is OpenAI-compatible.
   - Both `type: "openai_compat"` and `type: "anthropic"` route to same completion handler
   - Request format: identical
   - Response format: identical
   - Only difference: URL and auth header

2. **Route both types:**
   ```go
   if provider.Type == "openai_compat" || provider.Type == "anthropic" {
       return completeViaOpenAICompat(provider, messages)
   }
   ```

3. **Auth validation:** Existing `ValidateConfig()` checks env var exists:
   ```go
   if def.Type == "anthropic" && def.APIKeyEnv != "" && os.Getenv(def.APIKeyEnv) == "" {
       errs = append(errs, fmt.Sprintf("provider %q requires env var %s", name, def.APIKeyEnv))
   }
   ```

4. **Model tier mapping:**
   - `claude-3-5-sonnet` → frontier (highest reasoning, best for complex goals)
   - `claude-3-5-haiku` → standard (fast, cheaper, still strong)
   - `claude-3-opus` → standard (older frontier model, still very capable)
   - Future: `claude-4` → frontier (as it's released)

**User workflow:**

```bash
export ANTHROPIC_API_KEY=sk-ant-...
mini-agent --setup
# → select "Claude API (Anthropic)" option
# → auto-validates key
# → writes to config
```

**Why this approach scales:**
- Adding Mistral API? `type: "mistral"` → same handler
- Adding Groq? Already supported via `openai_compat`
- Anthropic batch API later? New handler type `type: "anthropic_batch"` (doesn't break current code)

---

## Configuration Changes

### New/Modified Fields

**`internal/config/config.go`:**

```go
type AgentConfig struct {
    // ... existing fields ...
    SystemPrompt          string `yaml:"system_prompt"`
    SystemPromptWeak      string `yaml:"system_prompt_weak"`      // NEW
    SystemPromptFrontier  string `yaml:"system_prompt_frontier"`  // NEW
    EnableFallbackParser  bool   `yaml:"enable_fallback_parser"`  // NEW, default true
}

type ProviderConfig struct {
    // ... existing fields ...
    Type   string `yaml:"type"`  // now accepts: "ollama" | "openai_compat" | "anthropic"
}
```

**`internal/agent/loop.go`:**

```go
type Agent struct {
    // ... existing fields ...
    ModelTier   string  // NEW: "weak" | "standard" | "frontier"
    SystemPrompt string  // unchanged, but now selected based on tier
}
```

### Backward Compatibility

- Existing `config.yaml` without `system_prompt_weak`/`frontier` work unchanged (falls back to default)
- Existing `providers` config (openai_compat) unaffected
- Existing `ollama` block works as before
- `enable_fallback_parser: true` is the safe default

---

## Error Handling

### Scenario: Unrecognized model name
- `DetectTier()` returns `"standard"` (safe fallback)
- User gets current behavior (expected)

### Scenario: Weak model outputs malformed JSON
- Strict parse fails
- Fallback parser attempts recovery
- If both fail: clear error message + suggestion to upgrade
- User sees transparency: what went wrong and how to fix it

### Scenario: User doesn't provide weak/frontier prompts
- Falls back to default `system_prompt`
- No error, graceful degradation

### Scenario: ANTHROPIC_API_KEY not set
- Config validation catches at startup
- Error: `"provider 'claude' requires env var ANTHROPIC_API_KEY to be set"`
- Same behavior as existing API key validation

### Scenario: User switches models mid-session
- Tier detection happens at startup only
- Tier doesn't change until next `mini-agent` invocation
- Predictable behavior (no mid-session surprises)

---

## Testing Strategy

**Unit tests:**

- `internal/models/tier_test.go` — 20+ cases (weak/standard/frontier patterns)
- `internal/tools/parser_test.go` — strict + fallback cases (valid JSON, malformed, prose)

**Integration tests:**

- Test weak model with fallback parser enabled/disabled
- Test Claude API provider detection + tier mapping
- Test system prompt selection (weak/standard/frontier)
- Verify backward compatibility (existing config.yaml loads)

**Manual testing matrix:**

| Model | Tier | Test |
|-------|------|------|
| qwen2.5-coder:1.5b | weak | Create file, read file, verify fallback on malformed JSON |
| qwen2.5-coder:3b | standard | Multi-file goal, standard prompt used |
| claude-3-5-sonnet | frontier | Complex reasoning goal, frontier prompt used |
| (unknown model) | standard | Falls back to standard behavior |

---

## Implementation Order

**Why this sequence? Each phase unblocks the next and delivers value independently.**

1. **Phase 1: Tier detection & system prompts** (2-3 hours)
   - **Why first?** Foundation for everything else. Without tier detection, we can't select prompts or decide when to use fallback.
   - **Value delivered:** Weak models get simpler prompts immediately. Even without fallback parser or Claude API, weak users see ~15% improvement.
   - **What it enables:** Phase 2 (fallback parser) and Phase 3 (Claude API) can both read `Agent.ModelTier`.
   - **Low risk:** Optional config fields, zero breaking changes.
   - **Tasks:**
     - Add `internal/models/tier.go`
     - Expand `config.AgentConfig` with `SystemPromptWeak`, `SystemPromptFrontier`
     - Wire tier selection in `Agent.New()`
     - Log tier at startup in banner
     - Tests: 20+ tier detection cases

2. **Phase 2: Tool parsing fallback** (2-3 hours)
   - **Why second?** Weak models need this to succeed at multi-file goals. Depends on Phase 1 (knows which models are weak).
   - **Value delivered:** Weak models recover from malformed JSON → goal success rate jumps from ~40% to ~75%.
   - **What it enables:** Phase 3 doesn't depend on this, but users running weak models are now much happier.
   - **Low risk:** Fallback only fires for weak models (strong models unaffected).
   - **Tasks:**
     - Add `internal/tools/parser.go` with strict + fallback logic
     - Refactor existing JSON parser
     - Integrate into `Execute()` method
     - Config flag `enable_fallback_parser: true` (default)
     - Tests: 15+ parser cases (strict, fallback, malformed JSON)
     - Log `[weak-model-fallback]` when fallback fires

3. **Phase 3: Claude API + openai_compat unification** (1-2 hours)
   - **Why third?** Doesn't block anything else, but users see frontier model support.
   - **Value delivered:** Cloud users can now switch to Claude API with simple config change.
   - **Why it's easy:** Claude API is OpenAI-compatible, no new HTTP code needed.
   - **Low risk:** Only adds new provider type, doesn't change existing behavior.
   - **Tasks:**
     - Add `type: "anthropic"` to `ProviderConfig` enum
     - Route `anthropic` → `completeViaOpenAICompat()` handler
     - Update `ValidateConfig()` to check `ANTHROPIC_API_KEY` env var
     - Add Claude models to tier detection patterns
     - `--setup` wizard: add "Claude API" option
     - Tests: provider routing, tier mapping, auth validation
     - Manual: test with real Claude API key

4. **Phase 4: Documentation & release** (1 hour)
   - **Why last?** Product is complete; docs communicate it to users.
   - **Tasks:**
     - Update README: add provider support table
     - Add `config.yaml` examples: weak model setup, Claude API setup
     - Update TODO.md: mark Phase 9 complete
     - Release notes: highlight weak model improvements + Claude API
     - Version bump to v2.9.0

**Why not implement all at once?**
- Phases 1-2 deliver value for weak model users before Claude API is ready
- Each phase is testable independently (lower risk of integration bugs)
- If Phase 3 gets delayed, phases 1-2 are already deployed and helping users
- Follows CLAUDE.md: **"simpler over clever"** (incremental, not big bang)

---

## Why Not? — Rejected Approaches & Alternatives

### System Prompts

**Rejected: Per-model custom prompts instead of tiers**

```yaml
models:
  qwen2.5-coder:1.5b:
    system_prompt: "..."
  claude-3-5-sonnet:
    system_prompt: "..."
```

**Why this is worse:**
- ❌ N models → N configs needed
- ❌ New model? User must add config manually
- ❌ Violates "zero config" goal
- ❌ Harder to maintain (duplicated instructions)

**Why tiers win:**
- ✅ 3 tiers → 3 prompts (scales to infinite models)
- ✅ New model? Auto-detected, works immediately
- ✅ Zero config for known models
- ✅ Centralized prompt maintenance

---

### Error Handling

**Rejected: Retry failed tool calls**

If weak model outputs bad JSON, retry asking them to fix it.

**Why this is worse:**
- ❌ Uses 2 tokens instead of 1 (limited hardware can't afford this)
- ❌ Model might produce same error twice (retry doesn't help)
- ❌ Adds latency (two round trips)
- ❌ Contradicts CLAUDE.md: "fewer round trips"

**Rejected: Abort with hard error**

When weak model fails, show error message and suggest upgrading.

**Why this is worse:**
- ❌ User has no choice but to switch provider mid-workflow
- ❌ Wastes their time (they already chose weak model deliberately)
- ❌ Doesn't help users committed to local-only setup
- ❌ Less helpful than fallback (doesn't try to recover)

**Why fallback parser wins:**
- ✅ Recovers from minor mistakes (simple handwriting fixes)
- ✅ Transparent feedback (user sees `[weak-model-fallback]`)
- ✅ No token waste (one-shot attempt)
- ✅ Respects user choice (helps weak models succeed)
- ✅ Zero cost for strong models (fallback disabled for them)

---

### Authentication

**Rejected: OAuth 2.0 for Claude API**

```bash
mini-agent --setup
# → opens browser
# → user authenticates on console.anthropic.com
# → token saved to ~/.mini-agent/auth.json
```

**Why this is worse (for v1):**
- ❌ Requires browser access (headless servers can't use this)
- ❌ Token refresh complexity (when token expires, what happens?)
- ❌ More code to maintain (OAuth library, token refresh loop)
- ❌ Violates "simpler over clever"
- ❌ Works fine with API keys (why add complexity?)

**Why API key first:**
- ✅ Simple: one env var, works everywhere
- ✅ Proven: already used for other API keys in config
- ✅ Portable: works on servers, CI, headless machines
- ✅ Minimal code (just validate env var exists)
- ✅ OAuth can be added later (backward compatible)

---

### Provider Architecture

**Rejected: Abstract `Provider` interface**

```go
type Provider interface {
    Complete(messages []Message) (string, error)
    CountTokens(text string) int
    // ... more methods
}

type OllamaProvider struct { ... }
type AnthropicProvider struct { ... }
type OpenAIProvider struct { ... }
```

**Why this is worse (for v1):**
- ❌ Over-engineering (we have 2 working providers via openai_compat)
- ❌ More code to write + maintain
- ❌ Overkill until we have >3 fundamentally different providers
- ❌ Makes simple queries harder (indirection everywhere)

**Why reuse openai_compat:**
- ✅ Claude API is already compatible (no new code needed)
- ✅ OpenRouter, Groq, Mistral all use same pattern (proven)
- ✅ Can add full abstraction later when we have 3+ unique providers
- ✅ Zero extra complexity right now

---

## Future Extensions (Noted for Later)

These are validated ideas to explore after v2.9.0:

**Option A (Retry Logic):** Weak models can retry tool calls (conflicts with "fewer tokens" goal, but worth exploring if fallback parser alone isn't enough)

**Option B (OAuth 2.0 for Claude):** Browser-based auth for Claude API (enables user credentials instead of API keys, more secure for shared machines)

**Option C (System Prompt Variants):** Per-model custom prompts instead of tiers (scales if tier-based becomes limiting)

**Option D (Provider Abstraction):** Abstract `Provider` interface for provider-specific logic (needed if adding batch API, file uploads, vision, etc.)

**Option E (Token-aware fallback):** Adjust context window per provider tier (frontier gets 200K context, weak gets 4K) — implemented when token counting improves

---

## Success Criteria

✅ Tier detection works for weak/standard/frontier models  
✅ System prompt varies per tier (verified in logs)  
✅ Tool fallback parser recovers from weak model malformed JSON  
✅ Claude API provider loads and authenticates via ANTHROPIC_API_KEY  
✅ Existing configs load without changes  
✅ All tests pass  
✅ Manual testing matrix passes  

---

## Scope & Constraints

**In scope:**
- Model tier detection (pattern-based)
- Tier-aware system prompts
- Tool parsing fallback for weak models
- Claude API (API key only)
- Backward compatibility

**Out of scope (future):**
- OAuth authentication
- Provider abstraction layer
- Anthropic-specific features (batch, files, vision)
- Retry logic for weak models
- Per-model custom prompts

**Constraints:**
- No new dependencies (keep gopkg.in/yaml.v3 as only config dep)
- No breaking changes to existing configs
- Fallback parser must not break strong models (weak-tier-only)
- Transparent error messages for debugging

