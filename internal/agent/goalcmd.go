package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mini-agent/internal/session"
)

// persistentGoalSystem is the system prompt for /goal mode.
// Similar to goalSystem but tuned for longer multi-step runs.
const persistentGoalSystem = `You are a local automation agent working toward a goal.
Each response must be EITHER a single JSON tool call OR the completion signal. Never both.

Write a file (include full content):
{"name":"write_file","arguments":{"path":"hello.py","content":"print('Hello World')\n"}}
Read a file:
{"name":"read_file","arguments":{"path":"hello.py"}}
Append to a file:
{"name":"append_file","arguments":{"path":"hello.py","content":"# extra\n"}}
List directory:
{"name":"list_dir","arguments":{"path":"."}}
Run a command:
{"name":"run_command","arguments":{"command":"ls","args":[]}}

Rules:
- Always call a tool first. Never output DONE on the first step.
- After each tool result, either call another tool or signal completion.
- Only output DONE when every part of the goal is fully complete and verified.
- If a previous attempt did not work, try a different approach instead of repeating.
- Completion signal: DONE: <one sentence summary of what was accomplished>`

const (
	goalMaxNotes    = 4000 // larger budget than /run — goal mode runs longer
	goalStepNoteLen = 600  // chars per step in goal notes
	goalSigHistory  = 5    // number of recent step signatures to track
	goalStuckThresh = 3    // identical signatures within history = stuck
)

// GoalStatus represents the lifecycle state of a persistent goal.
type GoalStatus string

const (
	GoalActive   GoalStatus = "active"
	GoalAchieved GoalStatus = "achieved"
	GoalPaused   GoalStatus = "paused"
)

// GoalState is persisted to ~/.mini-agent/goal.json between runs.
type GoalState struct {
	Objective  string     `json:"objective"`
	Status     GoalStatus `json:"status"`
	StartedAt  time.Time  `json:"started_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	Steps      int        `json:"steps"`
	Notes      string     `json:"notes"`
	LastReason string     `json:"last_reason,omitempty"`
	// RecentSigs tracks the last goalSigHistory step signatures for loop detection.
	RecentSigs []string `json:"recent_sigs,omitempty"`
}

// GoalStatePath returns the default path for goal state persistence.
func GoalStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".mini-agent", "goal.json"), nil
}

func loadGoalState(path string) (*GoalState, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var g GoalState
	return &g, json.Unmarshal(b, &g)
}

func saveGoalState(path string, g *GoalState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func clearGoalFile(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// handleGoalCommand dispatches /goal subcommands.
func (l *Loop) handleGoalCommand(ctx context.Context, input string) error {
	statePath, err := GoalStatePath()
	if err != nil {
		return fmt.Errorf("cannot determine goal state path: %w", err)
	}

	rest := strings.TrimSpace(strings.TrimPrefix(input, "/goal"))

	switch rest {
	case "":
		return l.goalStatus(statePath)
	case "clear", "stop", "cancel", "off", "reset", "none":
		return l.goalClear(statePath)
	case "pause":
		return l.goalPause(statePath)
	case "resume":
		return l.goalResume(ctx, statePath)
	default:
		return l.goalSet(ctx, statePath, rest)
	}
}

func (l *Loop) goalStatus(statePath string) error {
	g, err := loadGoalState(statePath)
	if err != nil {
		return fmt.Errorf("load goal: %w", err)
	}
	if g == nil {
		l.printf("%s[no active goal — use /goal <objective> to set one]%s\n", ansiDim, ansiReset)
		return nil
	}
	const sep = "──────────────────────────────────────────────────"
	fmt.Printf("\n%s%s%s\n", ansiDim, sep, ansiReset)
	fmt.Printf("  %s◎%s  goal      %s\n", ansiTeal, ansiReset, g.Objective)
	fmt.Printf("  %s◆%s  status    %s\n", ansiTeal, ansiReset, g.Status)
	fmt.Printf("  %s◆%s  steps     %d\n", ansiTeal, ansiReset, g.Steps)
	elapsed := time.Since(g.StartedAt).Round(time.Second)
	fmt.Printf("  %s◆%s  started   %s (%s ago)\n", ansiTeal, ansiReset, g.StartedAt.Format("2006-01-02 15:04:05"), elapsed)
	if g.LastReason != "" {
		fmt.Printf("  %s◆%s  last      %s\n", ansiTeal, ansiReset, g.LastReason)
	}
	if g.Status == GoalPaused {
		fmt.Printf("\n  %s/goal resume%s to continue · %s/goal clear%s to stop\n", ansiTeal, ansiReset, ansiDim, ansiReset)
	}
	fmt.Printf("%s%s%s\n\n", ansiDim, sep, ansiReset)
	return nil
}

func (l *Loop) goalClear(statePath string) error {
	g, _ := loadGoalState(statePath)
	if g == nil {
		l.printf("%s[no active goal]%s\n", ansiDim, ansiReset)
		return nil
	}
	if err := clearGoalFile(statePath); err != nil {
		return err
	}
	l.goal = nil
	l.printf("%s[goal cleared: %q]%s\n", ansiDim, g.Objective, ansiReset)
	return nil
}

func (l *Loop) goalPause(statePath string) error {
	g, _ := loadGoalState(statePath)
	if g == nil || g.Status != GoalActive {
		l.printf("%s[no active goal to pause — use Ctrl+C during a run]%s\n", ansiDim, ansiReset)
		return nil
	}
	g.Status = GoalPaused
	g.UpdatedAt = time.Now()
	if err := saveGoalState(statePath, g); err != nil {
		return err
	}
	l.goal = g
	l.printf("%s[goal paused — type /goal resume to continue]%s\n", ansiDim, ansiReset)
	return nil
}

func (l *Loop) goalResume(ctx context.Context, statePath string) error {
	g, err := loadGoalState(statePath)
	if err != nil {
		return fmt.Errorf("load goal: %w", err)
	}
	if g == nil || g.Status == GoalAchieved {
		l.printf("%s[no paused goal to resume]%s\n", ansiDim, ansiReset)
		return nil
	}
	g.Status = GoalActive
	return l.runGoalLoop(ctx, g, statePath)
}

func (l *Loop) goalSet(ctx context.Context, statePath string, objective string) error {
	if existing, _ := loadGoalState(statePath); existing != nil && existing.Status == GoalActive {
		l.printf("%s[replacing active goal: %q]%s\n", ansiDim, existing.Objective, ansiReset)
	}
	g := &GoalState{
		Objective: objective,
		Status:    GoalActive,
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	return l.runGoalLoop(ctx, g, statePath)
}

// runGoalLoop is the persistent execution loop for /goal mode.
func (l *Loop) runGoalLoop(ctx context.Context, g *GoalState, statePath string) error {
	l.goal = g

	timeout := time.Duration(l.cfg.Agent.StepTimeoutSeconds) * time.Second
	maxSteps := l.cfg.Agent.GoalMaxSteps // 0 = unlimited

	l.printf("\n%s[◎ goal]%s %s\n", ansiCyan, ansiReset, g.Objective)
	if g.Steps > 0 {
		l.printf("%s[resuming — %d steps completed so far]%s\n\n", ansiDim, g.Steps, ansiReset)
	} else {
		l.printf("%s[Ctrl+C to pause · /goal clear to stop]%s\n\n", ansiDim, ansiReset)
	}

	for {
		if maxSteps > 0 && g.Steps >= maxSteps {
			l.printf("\n%s[goal paused: reached %d step limit — /goal resume to continue]%s\n\n",
				ansiDim, maxSteps, ansiReset)
			g.Status = GoalPaused
			g.LastReason = fmt.Sprintf("reached %d step limit without completion", maxSteps)
			g.UpdatedAt = time.Now()
			saveGoalState(statePath, g)
			l.goal = g
			return nil
		}

		g.Steps++
		l.printf("%s[step %d]%s\n", ansiTeal, g.Steps, ansiReset)

		stepCtx := ctx
		var cancel context.CancelFunc
		if timeout > 0 {
			stepCtx, cancel = context.WithTimeout(ctx, timeout)
		}

		msgs := []session.Message{
			{Role: "system", Content: persistentGoalSystem},
			{Role: "user", Content: buildPersistentGoalPrompt(g)},
		}

		resp, err := l.chatOnceWith(stepCtx, msgs)
		if cancel != nil {
			cancel()
		}

		if err != nil {
			if errors.Is(err, context.Canceled) {
				g.Status = GoalPaused
				g.UpdatedAt = time.Now()
				saveGoalState(statePath, g)
				l.goal = g
				l.printf("\n%s[goal paused at step %d — /goal resume to continue]%s\n\n",
					ansiDim, g.Steps, ansiReset)
				return nil // consume; let REPL continue normally
			}
			if stepCtx.Err() == context.DeadlineExceeded {
				g.LastReason = "step timed out"
				g.Status = GoalPaused
				g.UpdatedAt = time.Now()
				saveGoalState(statePath, g)
				l.goal = g
				l.printf("\n%s[step %d timed out — /goal resume to retry]%s\n\n", ansiDim, g.Steps, ansiReset)
				return nil
			}
			return fmt.Errorf("step %d: %w", g.Steps, err)
		}

		// Completion signal.
		if summary, done := checkDone(resp.Content); done {
			l.printf("\n%s[✓ goal achieved]%s %s\n\n", ansiGreen, ansiReset, summary)
			g.Status = GoalAchieved
			g.LastReason = summary
			g.UpdatedAt = time.Now()
			saveGoalState(statePath, g)
			l.goal = nil
			l.recordGoalResult(g.Objective, "done", summary)
			return nil
		}

		// Fallback JSON parsing.
		if len(resp.ToolCalls) == 0 && l.registry.Enabled() {
			if tc := parseFallbackToolCall(resp.Content); len(tc) > 0 {
				resp.ToolCalls = tc
			}
		}

		if len(resp.ToolCalls) == 0 {
			text := truncStr(strings.TrimSpace(resp.Content), goalStepNoteLen)
			g.Notes = appendGoalNotes(g.Notes, fmt.Sprintf("step %d [reasoning]", g.Steps), text)
			g.LastReason = "reasoning step — no tool called"
			g.UpdatedAt = time.Now()
			saveGoalState(statePath, g)
			continue
		}

		// Execute the first tool call.
		tc := resp.ToolCalls[0]
		argsJSON, _ := json.Marshal(tc.Function.Arguments)

		if tc.Function.Name == "run_command" && l.registry.ConfirmRunCommand() {
			if !confirm("Allow run_command: " + l.registry.AllowedCommandPrompt(string(argsJSON)) + " ? [y/N]: ") {
				g.Notes = appendGoalNotes(g.Notes, fmt.Sprintf("step %d [denied]", g.Steps), "user denied the command")
				g.UpdatedAt = time.Now()
				saveGoalState(statePath, g)
				continue
			}
		}
		if tc.Function.Name == "write_file" && l.registry.ConfirmWriteFile() {
			if path, ok := tc.Function.Arguments["path"].(string); ok {
				if _, statErr := os.Stat(filepath.Clean(path)); statErr == nil {
					if !confirm(fmt.Sprintf("Overwrite existing file %q? [y/N]: ", path)) {
						g.Notes = appendGoalNotes(g.Notes, fmt.Sprintf("step %d [denied]", g.Steps), "user denied overwrite")
						g.UpdatedAt = time.Now()
						saveGoalState(statePath, g)
						continue
					}
				}
			}
		}

		toolStart := time.Now()
		result, execErr := l.registry.Execute(tc.Function.Name, string(argsJSON))
		if execErr != nil {
			result = "tool error: " + execErr.Error()
		}
		toolTC := session.ToolCall{Function: session.ToolFunction{Name: tc.Function.Name, Arguments: tc.Function.Arguments}}
		l.logTool(toolTC, result, execErr, time.Since(toolStart))

		summary := toolSummary(toolTC)
		if summary != "" {
			l.printf("  %s[%s]%s %s → %s\n", ansiTeal, tc.Function.Name, ansiReset,
				summary, truncStr(strings.TrimSpace(result), 80))
		} else {
			l.printf("  %s[%s]%s %s\n", ansiTeal, tc.Function.Name, ansiReset,
				truncStr(strings.TrimSpace(result), 80))
		}

		// Loop detection: track recent signatures, stop if stuck.
		sig := tc.Function.Name + "|" + string(argsJSON) + "|" + result
		g.RecentSigs = appendGoalSig(g.RecentSigs, sig)
		if goalIsStuck(g.RecentSigs) {
			l.printf("\n%s[goal stuck — same action repeated %d+ times, pausing]%s\n\n",
				ansiDim, goalStuckThresh, ansiReset)
			g.Status = GoalPaused
			g.LastReason = "stuck in loop — same action repeated, try a different approach"
			g.UpdatedAt = time.Now()
			saveGoalState(statePath, g)
			l.goal = g
			return nil
		}

		g.Notes = appendGoalNotes(g.Notes, fmt.Sprintf("step %d [%s]", g.Steps, tc.Function.Name),
			truncStr(result, goalStepNoteLen))
		g.LastReason = fmt.Sprintf("step %d: %s", g.Steps, tc.Function.Name)
		g.UpdatedAt = time.Now()
		saveGoalState(statePath, g)
	}
}

func buildPersistentGoalPrompt(g *GoalState) string {
	if g.Steps == 1 {
		return "Goal: " + g.Objective + "\n\nCall a tool to start."
	}
	return fmt.Sprintf(
		"Goal: %s\n\nProgress so far:\n%s\nCall another tool to continue, or output DONE: <summary> only when every part of the goal is fully complete.",
		g.Objective, g.Notes,
	)
}

func appendGoalNotes(notes, label, result string) string {
	entry := label + ": " + result + "\n"
	combined := notes + entry
	if len(combined) <= goalMaxNotes {
		return combined
	}
	// Trim oldest lines to stay within budget, preserving note boundaries.
	excess := len(combined) - goalMaxNotes
	cut := excess
	for cut < len(combined) && combined[cut] != '\n' {
		cut++
	}
	if cut+1 < len(combined) {
		return "[...earlier steps omitted]\n" + combined[cut+1:]
	}
	return combined[len(combined)-goalMaxNotes:]
}

// appendGoalSig appends sig and keeps only the last goalSigHistory entries.
func appendGoalSig(sigs []string, sig string) []string {
	sigs = append(sigs, sig)
	if len(sigs) > goalSigHistory {
		sigs = sigs[len(sigs)-goalSigHistory:]
	}
	return sigs
}

// goalIsStuck returns true when goalStuckThresh or more of the recent
// signatures are identical — the agent is repeating the same action.
func goalIsStuck(sigs []string) bool {
	if len(sigs) < goalStuckThresh {
		return false
	}
	counts := make(map[string]int, len(sigs))
	for _, s := range sigs {
		counts[s]++
		if counts[s] >= goalStuckThresh {
			return true
		}
	}
	return false
}
