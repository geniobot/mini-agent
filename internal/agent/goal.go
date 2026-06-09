package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mini-agent/internal/session"
)

const goalSystem = `You are a local automation agent executing a goal step by step.
Each response must be EITHER a single JSON tool call OR the completion signal. Never both.

Write a new file (include full content):
{"name":"write_file","arguments":{"path":"hello.py","content":"print('Hello World')\n"}}
Read a file:
{"name":"read_file","arguments":{"path":"hello.py"}}
Edit part of an existing file (find and replace):
{"name":"edit_file","arguments":{"path":"hello.py","old_string":"old code","new_string":"new code"}}
Append to a file:
{"name":"append_file","arguments":{"path":"hello.py","content":"# extra\n"}}
List directory:
{"name":"list_dir","arguments":{"path":"."}}
Search for text in files:
{"name":"search_files","arguments":{"pattern":"TODO","path":"."}}
Run a command:
{"name":"run_command","arguments":{"command":"ls","args":[]}}
Fetch a URL:
{"name":"web_fetch","arguments":{"url":"https://example.com"}}
Run a git command:
{"name":"git","arguments":{"subcommand":"status","args":["--short"]}}

Use write_file for new files, edit_file to change existing files.
After each tool result, either call another tool or signal completion.
Only signal completion when ALL tasks are done: DONE: <one sentence summary>`

const maxNoteLen = 800   // chars per individual result before appending to notes
const maxNotesLen = 2000 // chars for total accumulated working notes (~500 tokens)

// stepSig identifies a tool action by its inputs and output.
// Two identical consecutive signatures mean the agent is stuck in a loop.
type stepSig struct {
	tool, args, result string
}

// smallModelPatterns are substrings found in model names known to be unreliable
// in goal/tool-use mode. A warning is shown but execution continues.
var smallModelPatterns = []string{"1.5b", "0.5b", "1b:"}

func isSmallModel(model string) bool {
	lower := strings.ToLower(model)
	for _, p := range smallModelPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func (l *Loop) runGoal(ctx context.Context, goal string) error {
	maxSteps := l.cfg.Agent.MaxGoalSteps
	timeout := time.Duration(l.cfg.Agent.StepTimeoutSeconds) * time.Second

	l.printf("\n%s[goal]%s %s\n", ansiCyan, ansiReset, goal)
	l.printf("%s[max %d steps — Ctrl+C to abort]%s\n\n", ansiDim, maxSteps, ansiReset)

	if isSmallModel(l.activeProvider.Model) {
		l.printf("%s[warning] %s may be too small for reliable goal/tool use.\n         Switch to a 3B+ model for best results: /model qwen2.5-coder:3b%s\n\n",
			ansiYellow, l.activeProvider.Model, ansiReset)
	}

	var notes string
	var prevSig stepSig
	var consecutiveNoTool int
	var countToolCalls int // require at least one real tool call before accepting DONE
	var blockedDoneCount int // times DONE was rejected for zero tool calls

	for step := 1; step <= maxSteps; step++ {
		l.printf("%s[step %d/%d]%s\n", ansiTeal, step, maxSteps, ansiReset)

		// Build a step context: child of the outer ctx (signal-cancellable) + step timeout.
		stepCtx := ctx
		var cancel context.CancelFunc
		if timeout > 0 {
			stepCtx, cancel = context.WithTimeout(ctx, timeout)
		}

		msgs := []session.Message{
			{Role: "system", Content: goalSystem},
			{Role: "user", Content: buildGoalPrompt(goal, step, notes, blockedDoneCount)},
		}

		resp, err := l.chatOnceWith(stepCtx, msgs)
		if cancel != nil {
			cancel()
		}

		if err != nil {
			if stepCtx.Err() == context.DeadlineExceeded {
				l.printf("\n%s[step %d timed out after %s]%s\n\n", ansiDim, step, timeout, ansiReset)
				return nil
			}
			// Propagate context.Canceled so the caller (Run) can print [interrupted].
			return fmt.Errorf("step %d: %w", step, err)
		}

		// Check for completion before attempting tool parsing.
		if summary, done := checkDone(resp.Content); done {
			if countToolCalls == 0 {
				blockedDoneCount++
				consecutiveNoTool++ // treat as a no-tool step for fallback purposes
				l.printf("%s[step %d] model signalled DONE without performing any actions — continuing%s\n", ansiYellow, step, ansiReset)
				notes = appendNotes(notes, fmt.Sprintf("step %d [warning]", step), "model claimed done without tool calls — ignored")
				continue
			}
			if l.quiet {
				fmt.Println(summary)
			} else {
				fmt.Printf("\n%s[✓ done]%s %s\n\n", ansiGreen, ansiReset, summary)
			}
			l.recordGoalResult(goal, "done", summary)
			return nil
		}

		// Try native tool calls first, then fall back to JSON extraction.
		if len(resp.ToolCalls) == 0 && l.registry.Enabled() {
			if tc, usedFallback := parseFallbackToolCall(resp.Content, l.ModelTier); len(tc) > 0 {
				if usedFallback {
					l.printf("[fallback tool parse — weak-model output required recovery]\n")
				} else {
					l.printf("[fallback tool parse]\n")
				}
				resp.ToolCalls = tc
			}
		}

		if len(resp.ToolCalls) == 0 {
			text := truncStr(strings.TrimSpace(resp.Content), maxNoteLen)
			notes = appendNotes(notes, fmt.Sprintf("step %d [reasoning]", step), text)
			consecutiveNoTool++
			if l.cfg.FallbackAfterFailures > 0 && consecutiveNoTool >= l.cfg.FallbackAfterFailures && l.fallbackClient != nil {
				fbProvider := l.cfg.Providers[l.cfg.FallbackProvider]
				savedFallback := l.fallbackClient
				l.client = l.fallbackClient
				l.activeProvider = fbProvider
				l.fallbackClient = nil // prevent re-triggering this block
				defer func() {
					l.client = l.defaultClient
					l.activeProvider = l.defaultProvider
					l.fallbackClient = savedFallback
				}()
				l.printf("%s[escalated to fallback provider for this goal]%s\n", ansiYellow, ansiReset)
				consecutiveNoTool = 0
			}
			continue
		}

		// Execute only the first tool call per step.
		// Small models are unreliable with multi-tool responses; chaining via notes is safer.
		tc := resp.ToolCalls[0]
		argsJSON, _ := json.Marshal(tc.Function.Arguments)

		if tc.Function.Name == "run_command" && l.registry.ConfirmRunCommand() {
			if !confirm("Allow run_command: " + l.registry.AllowedCommandPrompt(string(argsJSON)) + " ? [y/N]: ") {
				notes = appendNotes(notes, fmt.Sprintf("step %d [denied]", step), "user denied the command")
				continue
			}
		}
		if tc.Function.Name == "write_file" && l.registry.ConfirmWriteFile() {
			if path, ok := tc.Function.Arguments["path"].(string); ok {
				if _, err := os.Stat(filepath.Clean(path)); err == nil {
					if !confirmOverwrite(path) {
						notes = appendNotes(notes, fmt.Sprintf("step %d [denied]", step), "user denied the overwrite")
						continue
					}
				}
			}
		}

		toolStart := time.Now()
		result, err := l.registry.Execute(tc.Function.Name, string(argsJSON))
		if err != nil {
			result = "tool error: " + err.Error()
		}
		l.logTool(session.ToolCall{Function: session.ToolFunction{Name: tc.Function.Name, Arguments: tc.Function.Arguments}}, result, err, time.Since(toolStart))

		l.printf("  %s[%s]%s %s\n", ansiTeal, tc.Function.Name, ansiReset, truncStr(result, 300))

		// Loop detection: identical (tool, args, result) on back-to-back steps = stuck.
		sig := stepSig{tc.Function.Name, string(argsJSON), result}
		if step > 1 && sig == prevSig {
			l.printf("\n%s[no progress — same action produced the same result twice, stopping]%s\n\n", ansiDim, ansiReset)
			l.recordGoalResult(goal, "stopped", "no progress — same action repeated")
			return nil
		}
		prevSig = sig

		notes = appendNotes(notes, fmt.Sprintf("step %d [%s]", step, tc.Function.Name), truncStr(result, maxNoteLen))
		consecutiveNoTool = 0
		countToolCalls++

		// For single-action goals, skip the follow-up LLM round-trip and auto-complete.
		// On slow hardware the model is often evicted between steps, making step 2
		// (just saying DONE) take as long as a cold load. If this was step 1 and
		// the tool completed successfully, the goal is done.
		singleActionTools := map[string]bool{
			"read_file": true, "list_dir": true, "search_files": true,
			"write_file": true, "edit_file": true, "append_file": true,
		}
		if singleActionTools[tc.Function.Name] && step == 1 && err == nil {
			summary := tc.Function.Name + " completed"
			if l.quiet && (tc.Function.Name == "read_file" || tc.Function.Name == "search_files" || tc.Function.Name == "list_dir") {
				fmt.Println(result)
			} else if l.quiet {
				fmt.Println(summary)
			} else {
				fmt.Printf("\n%s[✓ done]%s %s\n\n", ansiGreen, ansiReset, summary)
			}
			l.recordGoalResult(goal, "done", summary)
			return nil
		}
	}

	l.printf("\n%s[goal limit reached after %d steps without DONE signal]%s\n\n", ansiDim, maxSteps, ansiReset)
	l.recordGoalResult(goal, "stopped", fmt.Sprintf("reached %d step limit without completion", maxSteps))
	return nil
}

func buildGoalPrompt(goal string, step int, notes string, blockedDoneCount int) string {
	if step == 1 {
		return "Goal: " + goal + "\n\nCall a tool to start."
	}
	base := fmt.Sprintf("Goal: %s\n\nProgress so far:\n%s", goal, notes)
	if blockedDoneCount > 0 {
		return base + fmt.Sprintf(
			"\n\nWARNING: you have signalled DONE %d time(s) without calling any tools. "+
				"The goal is NOT complete until you actually call the required tools (e.g. write_file). "+
				"Do NOT output DONE. Call a tool now.",
			blockedDoneCount,
		)
	}
	return base + "\n\nCall another tool to continue, or output DONE: <summary> only when every task is complete."
}

// appendNotes appends a labelled result to the running notes string,
// trimming the oldest lines if the total would exceed maxNotesLen.
func appendNotes(notes, label, result string) string {
	entry := label + ": " + result + "\n"
	combined := notes + entry
	if len(combined) <= maxNotesLen {
		return combined
	}
	excess := len(combined) - maxNotesLen
	cut := excess
	for cut < len(combined) && combined[cut] != '\n' {
		cut++
	}
	if cut+1 < len(combined) {
		return "[...earlier steps omitted]\n" + combined[cut+1:]
	}
	return combined[len(combined)-maxNotesLen:]
}

// checkDone returns the summary and true when content starts with DONE.
func checkDone(content string) (string, bool) {
	s := strings.TrimSpace(content)
	if len(s) < 4 {
		return "", false
	}
	if strings.EqualFold(s[:4], "done") {
		rest := strings.TrimLeft(s[4:], ":- ")
		return strings.TrimSpace(rest), true
	}
	return "", false
}

// truncStr shortens s to max chars and notes the omitted length.
func truncStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf(" [+%d chars]", len(s)-max)
}
