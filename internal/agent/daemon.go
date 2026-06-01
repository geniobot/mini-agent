package agent

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"mini-agent/internal/config"
	"mini-agent/internal/scheduler"
)

type daemonEntry struct {
	goal string
	expr *scheduler.CronExpr
}

// RunDaemon parses the schedule from cfg, writes a PID file, then loops:
// sleeping until the next due goal, running it, repeating until SIGTERM/SIGINT.
func RunDaemon(cfg *config.Config) error {
	if len(cfg.Schedule) == 0 {
		return fmt.Errorf("no schedule entries in config — add a 'schedule:' section")
	}

	entries := make([]daemonEntry, 0, len(cfg.Schedule))
	for _, s := range cfg.Schedule {
		if s.Cron == "" || s.Goal == "" {
			return fmt.Errorf("each schedule entry requires both 'cron' and 'goal' fields")
		}
		expr, err := scheduler.Parse(s.Cron)
		if err != nil {
			return fmt.Errorf("invalid cron %q: %w", s.Cron, err)
		}
		entries = append(entries, daemonEntry{goal: s.Goal, expr: expr})
	}

	// PID file — written on start, removed on clean exit.
	pidPath, pidErr := daemonPIDPath()
	if pidErr == nil {
		_ = os.MkdirAll(filepath.Dir(pidPath), 0o755)
		if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not write PID file: %v\n", err)
		} else {
			defer os.Remove(pidPath)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("[daemon] started PID %d — %d scheduled goal(s)\n", os.Getpid(), len(entries))
	now := time.Now()
	for _, e := range entries {
		if n := e.expr.Next(now); !n.IsZero() {
			fmt.Printf("[daemon]   %-50s → %s\n", truncStr(e.goal, 50), n.Format("2006-01-02 15:04"))
		}
	}
	fmt.Println()

	prev := time.Now()
	for {
		nextWake := nextFireTime(entries, prev)
		if nextWake.IsZero() {
			return fmt.Errorf("no future scheduled events found — check cron expressions")
		}

		wait := time.Until(nextWake)
		if wait < 0 {
			wait = 0
		}
		fmt.Printf("[daemon] next run at %s (in %s)\n",
			nextWake.Format("2006-01-02 15:04:05"), wait.Round(time.Second))

		select {
		case <-ctx.Done():
			fmt.Println("[daemon] shutting down cleanly")
			return nil
		case <-time.After(wait):
		}

		now := time.Now()
		for _, e := range entries {
			// An entry is due if its next fire time (after prev) falls at or before now.
			n := e.expr.Next(prev)
			if n.IsZero() || n.After(now) {
				continue
			}
			fmt.Printf("[daemon] → %s\n", e.goal)
			loop := New(cfg)
			loop.SetQuiet(true)
			if err := loop.RunGoalCtx(ctx, e.goal); err != nil && ctx.Err() == nil {
				fmt.Fprintf(os.Stderr, "[daemon] goal error: %v\n", err)
			}
			if ctx.Err() != nil {
				fmt.Println("[daemon] shutting down cleanly")
				return nil
			}
		}
		prev = now
	}
}

// nextFireTime returns the earliest next fire time across all entries after `after`.
func nextFireTime(entries []daemonEntry, after time.Time) time.Time {
	var earliest time.Time
	for _, e := range entries {
		n := e.expr.Next(after)
		if n.IsZero() {
			continue
		}
		if earliest.IsZero() || n.Before(earliest) {
			earliest = n
		}
	}
	return earliest
}

func daemonPIDPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".mini-agent", "daemon.pid"), nil
}
