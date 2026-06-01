package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"mini-agent/internal/config"
)

// BatchResult is the JSON-line record written to stdout for each goal.
type BatchResult struct {
	Index      int    `json:"index"`
	Goal       string `json:"goal"`
	Status     string `json:"status"`            // "ok" | "stopped" | "error"
	Summary    string `json:"summary,omitempty"` // DONE or stop-reason sentence
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

// ReadGoals reads goals from path: one per line, blank lines and # comments skipped.
func ReadGoals(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var goals []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		goals = append(goals, line)
	}
	return goals, sc.Err()
}

// RunBatch executes goals with up to parallel concurrent workers and writes one
// JSON line per goal to out as each completes (streaming order, not index order).
// Returns true if any goal ended in "error" status (for exit code 1).
func RunBatch(ctx context.Context, cfg *config.Config, goals []string, parallel int, out io.Writer) bool {
	if parallel < 1 {
		parallel = 1
	}

	sem := make(chan struct{}, parallel)
	var mu sync.Mutex
	var wg sync.WaitGroup
	anyFailed := false

	for i, goal := range goals {
		wg.Add(1)
		go func(idx int, g string) {
			defer wg.Done()

			// Acquire a slot in the pool.
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			if ctx.Err() != nil {
				return
			}

			start := time.Now()
			loop := New(cfg)
			loop.SetQuiet(true)

			err := loop.RunGoalCtx(ctx, g)
			elapsed := time.Since(start).Milliseconds()

			r := BatchResult{
				Index:      idx,
				Goal:       g,
				DurationMS: elapsed,
			}
			switch {
			case err != nil:
				r.Status = "error"
				r.Error = err.Error()
			case loop.lastGoalRecord != nil && loop.lastGoalRecord.status == "done":
				r.Status = "ok"
				r.Summary = loop.lastGoalRecord.detail
			case loop.lastGoalRecord != nil:
				r.Status = "stopped"
				r.Summary = loop.lastGoalRecord.detail
			default:
				r.Status = "ok"
			}

			b, _ := json.Marshal(r)
			mu.Lock()
			fmt.Fprintf(out, "%s\n", b)
			if r.Status == "error" {
				anyFailed = true
			}
			mu.Unlock()
		}(i, goal)
	}

	wg.Wait()
	return anyFailed
}
