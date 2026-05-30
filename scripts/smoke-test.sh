#!/usr/bin/env bash
#
# smoke-test.sh — end-to-end test of mini-agent against a live Ollama.
#
# Drives the real agent with --run (quick goal mode) and --quiet, then asserts
# on the resulting filesystem state. This catches regressions in tool selection
# and execution that unit tests can't (they don't involve the model).
#
# Usage:
#   ./scripts/smoke-test.sh            # run all checks
#   ./scripts/smoke-test.sh --quick    # run a fast subset (file + edit only)
#   MODEL=qwen2.5-coder:3b ./scripts/smoke-test.sh
#
# Requirements: Ollama running locally with the configured model pulled.

set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/bin/mini-agent"
MODEL="${MODEL:-qwen2.5-coder:1.5b}"
QUICK=false
[ "${1:-}" = "--quick" ] && QUICK=true

# ── colors ────────────────────────────────────────────────────────────────
if [ -t 1 ]; then GRN='\033[92m'; RED='\033[91m'; DIM='\033[2m'; YEL='\033[33m'; RST='\033[0m'
else GRN='' RED='' DIM='' YEL='' RST=''; fi

PASS=0
FAIL=0
FAILED_NAMES=()

# ── build if needed ───────────────────────────────────────────────────────
if [ ! -x "$BIN" ]; then
  echo "Building mini-agent..."
  (cd "$ROOT" && go build -o bin/mini-agent ./cmd/mini-agent) || { echo "build failed"; exit 1; }
fi

# ── ollama reachability ─────────────────────────────────────────────────────
if ! curl -s http://localhost:11434/api/tags >/dev/null 2>&1; then
  echo -e "${RED}Ollama is not reachable at localhost:11434 — start it with 'ollama serve'${RST}"
  exit 1
fi

# ── test config: enable every tool, no confirmation prompts ─────────────────
WORK="$(mktemp -d)"
CONFIG="$WORK/config.yaml"
cat > "$CONFIG" <<EOF
ollama:
  host: "http://localhost:11434"
  model: "$MODEL"
  stream: true
  options:
    temperature: 0.1
    num_ctx: 4096
    num_predict: 1024
agent:
  max_history: 8
  step_timeout_seconds: 120
  max_goal_steps: 12
  goal_max_steps: 20
  system_prompt: |
    You are a local coding assistant.
    For file and shell operations output ONLY a JSON object — no explanation, no markdown.
    For questions reply in plain text.
    Write a new file: {"name":"write_file","arguments":{"path":"f.py","content":"..."}}
    Read a file: {"name":"read_file","arguments":{"path":"f.py"}}
    Edit part of a file: {"name":"edit_file","arguments":{"path":"f.py","old_string":"a","new_string":"b"}}
    Append: {"name":"append_file","arguments":{"path":"f.py","content":"..."}}
    List dir: {"name":"list_dir","arguments":{"path":"."}}
    Search files: {"name":"search_files","arguments":{"pattern":"TODO","path":"."}}
    Run command: {"name":"run_command","arguments":{"command":"ls","args":[]}}
    Use write_file for new files, edit_file to change existing files.
tools:
  enabled: true
  use_native_tools: false
  enable_read_file: true
  enable_write_file: true
  enable_edit_file: true
  enable_append_file: true
  enable_list_dir: true
  enable_run_command: true
  enable_search_files: true
  confirm_run_command: false
  confirm_write_file: false
  allowed_commands: [ls, cat, pwd, grep, find, head, tail]
EOF

cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT

# run <goal> — invokes the agent non-interactively in the scratch workdir
run() {
  ( cd "$WORK" && "$BIN" --config "$CONFIG" --quiet --no-save --fresh --no-context --run "$1" ) >/dev/null 2>&1
}

# check <name> <test-command...> — runs an assertion, records pass/fail
check() {
  local name="$1"; shift
  if "$@"; then
    echo -e "  ${GRN}✓${RST} $name"
    PASS=$((PASS+1))
  else
    echo -e "  ${RED}✗${RST} $name"
    FAIL=$((FAIL+1))
    FAILED_NAMES+=("$name")
  fi
}

# file_contains <file> <pattern>
file_contains() { [ -f "$WORK/$1" ] && grep -qi "$2" "$WORK/$1"; }
file_exists()   { [ -f "$WORK/$1" ]; }

echo ""
echo -e "${DIM}smoke-test — model: $MODEL — workdir: $WORK${RST}"
echo -e "${DIM}(each step calls the model; on CPU this takes a while)${RST}"
echo ""

# ── 1. write_file ───────────────────────────────────────────────────────────
echo -e "${YEL}file creation${RST}"
run "create a python file called calc.py containing a function add(a, b) that returns a + b"
check "calc.py created"           file_exists  "calc.py"
check "calc.py has add function"  file_contains "calc.py" "def add"

# ── 2. edit_file ──────────────────────────────────────────────────────────────
echo -e "${YEL}edit_file (find/replace)${RST}"
run "in calc.py rename the function add to sum_values"
check "calc.py renamed to sum_values" file_contains "calc.py" "def sum_values"
check "old add name is gone"          bash -c "! grep -q 'def add' '$WORK/calc.py'"

if [ "$QUICK" = true ]; then
  echo ""
  echo -e "${DIM}--quick: skipping remaining checks${RST}"
else
  # ── 3. append_file ──────────────────────────────────────────────────────────
  echo -e "${YEL}append_file${RST}"
  run "append a function called greet that prints hello to calc.py"
  check "calc.py has greet appended"  file_contains "calc.py" "greet"
  check "sum_values still present"    file_contains "calc.py" "sum_values"

  # ── 4. write structured file ──────────────────────────────────────────────
  echo -e "${YEL}structured file${RST}"
  run "create config.json with name set to myapp and version set to 1.0.0"
  check "config.json created"   file_exists "config.json"
  check "config.json has myapp" file_contains "config.json" "myapp"

  # ── 5. edit a value ─────────────────────────────────────────────────────────
  echo -e "${YEL}edit_file on json${RST}"
  run "in config.json change the version from 1.0.0 to 2.0.0"
  check "config.json version bumped"  file_contains "config.json" "2.0.0"

  # ── 6. search_files ─────────────────────────────────────────────────────────
  echo -e "${YEL}search_files${RST}"
  echo "# TODO: write tests" > "$WORK/scratch.py"
  out=$( cd "$WORK" && "$BIN" --config "$CONFIG" --quiet --no-save --fresh --no-context --run "search for the text TODO in the files here and report which file contains it" 2>/dev/null )
  check "search found scratch.py" bash -c "echo '$out' | grep -qi scratch"

  # ── 7. read_file ────────────────────────────────────────────────────────────
  echo -e "${YEL}read_file${RST}"
  out=$( cd "$WORK" && "$BIN" --config "$CONFIG" --quiet --no-save --fresh --no-context --run "read config.json and tell me the value of version" 2>/dev/null )
  check "read reported version 2.0.0" bash -c "echo '$out' | grep -q '2.0.0'"
fi

# ── summary ───────────────────────────────────────────────────────────────────
echo ""
echo -e "${DIM}──────────────────────────────────────────${RST}"
if [ "$FAIL" -eq 0 ]; then
  echo -e "  ${GRN}all $PASS checks passed${RST}"
else
  echo -e "  ${GRN}$PASS passed${RST}, ${RED}$FAIL failed${RST}"
  for n in "${FAILED_NAMES[@]}"; do echo -e "    ${RED}✗${RST} $n"; done
fi
echo -e "${DIM}──────────────────────────────────────────${RST}"
echo ""
echo -e "${DIM}Note: small models are non-deterministic. A failed check may mean${RST}"
echo -e "${DIM}the model picked the wrong tool, not that the tool is broken.${RST}"

[ "$FAIL" -eq 0 ]
