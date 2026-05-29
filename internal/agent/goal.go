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

const goalSystem = `You are a local automation agent. Complete the given goal step by step using tools.
To use a tool output ONLY a raw JSON object — no prose, no markdown, no explanation:
{"name":"write_file","arguments":{"path":"notes.txt","content":"Hello world"}}
{"name":"read_file","arguments":{"path":"notes.txt"}}
{"name":"append_file","arguments":{"path":"notes.txt","content":"extra line"}}
{"name":"list_dir","arguments":{"path":"."}}
{"name":"run_command","arguments":{"command":"ls","args":["-la"]}}
When the goal is fully complete output exactly: DONE: <one sentence summary>`

const maxNoteLen = 800   // chars per individual result before appending to notes
const maxNotesLen = 2000 // chars for total accumulated working notes (~500 tokens)

// stepSig identifies a tool action by its inputs and output.
// Two identical consecutive signatures mean the agent is stuck in a loop.
type stepSig struct {
	tool, args, result string
}

func (l *Loop) runGoal(ctx context.Context, goal string) error {
	maxSteps := l.cfg.Agent.MaxGoalSteps
	timeout := time.Duration(l.cfg.Agent.StepTimeoutSeconds) * time.Second

	l.printf("\n%s[goal]%s %s\n", ansiCyan, ansiReset, goal)
	l.printf("%s[max %d steps — Ctrl+C to abort]%s\n\n", ansiDim, maxSteps, ansiReset)

	var notes string
	var prevSig stepSig

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
			{Role: "user", Content: buildGoalPrompt(goal, step, notes)},
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
			if l.quiet {
				fmt.Println(summary)
			} else {
				fmt.Printf("\n%s[✓ done]%s %s\n\n", ansiGreen, ansiReset, summary)
			}
			return nil
		}

		// Try native tool calls first, then fall back to JSON extraction.
		if len(resp.ToolCalls) == 0 && l.registry.Enabled() {
			if tc := parseFallbackToolCall(resp.Content); len(tc) > 0 {
				resp.ToolCalls = tc
			}
		}

		if len(resp.ToolCalls) == 0 {
			text := truncStr(strings.TrimSpace(resp.Content), maxNoteLen)
			notes = appendNotes(notes, fmt.Sprintf("step %d [reasoning]", step), text)
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
					if !confirm(fmt.Sprintf("Overwrite existing file %q? [y/N]: ", path)) {
						notes = appendNotes(notes, fmt.Sprintf("step %d [denied]", step), "user denied the overwrite")
						continue
					}
				}
			}
		}

		result, err := l.registry.Execute(tc.Function.Name, string(argsJSON))
		if err != nil {
			result = "tool error: " + err.Error()
		}

		l.printf("  %s[%s]%s %s\n", ansiTeal, tc.Function.Name, ansiReset, truncStr(result, 100))

		// Loop detection: identical (tool, args, result) on back-to-back steps = stuck.
		sig := stepSig{tc.Function.Name, string(argsJSON), result}
		if step > 1 && sig == prevSig {
			l.printf("\n%s[no progress — same action produced the same result twice, stopping]%s\n\n", ansiDim, ansiReset)
			return nil
		}
		prevSig = sig

		notes = appendNotes(notes, fmt.Sprintf("step %d [%s]", step, tc.Function.Name), truncStr(result, maxNoteLen))
	}

	l.printf("\n%s[goal limit reached after %d steps without DONE signal]%s\n\n", ansiDim, maxSteps, ansiReset)
	return nil
}

func buildGoalPrompt(goal string, step int, notes string) string {
	if step == 1 {
		return "Goal: " + goal
	}
	return fmt.Sprintf(
		"Goal: %s\n\nWorking notes:\n%s\nContinue toward the goal, or output DONE: <summary> if it is complete.",
		goal, notes,
	)
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
