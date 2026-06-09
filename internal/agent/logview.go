package agent

import (
	"fmt"
	"strings"

	"github.com/geniobot/mini-agent/internal/runlog"
)

// PrintLog reads the last n entries from the default run log and prints them
// in a human-readable table. Called by the --log flag.
func PrintLog(n int) {
	path, err := runlog.DefaultPath()
	if err != nil {
		fmt.Printf("cannot determine log path: %v\n", err)
		return
	}
	entries, err := runlog.ReadTail(path, n)
	if err != nil {
		fmt.Printf("cannot read log %s: %v\n", path, err)
		return
	}
	if len(entries) == 0 {
		fmt.Printf("no entries in %s\n", path)
		return
	}

	const sep = "──────────────────────────────────────────────────────────────────"
	fmt.Printf("\n%s%s%s\n", ansiDim, sep, ansiReset)
	fmt.Printf("  %srun log — last %d entries%s  %s(%s)%s\n",
		ansiTeal, len(entries), ansiReset, ansiDim, path, ansiReset)
	fmt.Printf("%s%s%s\n\n", ansiDim, sep, ansiReset)

	for _, e := range entries {
		ts := e.TS.Format("2006-01-02 15:04:05")

		status := fmt.Sprintf("%s✓%s", ansiGreen, ansiReset)
		if !e.OK {
			status = fmt.Sprintf("%s✗%s", ansiRed, ansiReset)
		}

		// Extract a short arg summary from the args JSON string.
		argSummary := shortArgs(e.Args, 40)

		size := fmt.Sprintf("%dB", e.ResultBytes)
		if e.ResultBytes >= 1024 {
			size = fmt.Sprintf("%.1fKB", float64(e.ResultBytes)/1024)
		}

		dur := fmt.Sprintf("%.1fs", float64(e.DurationMS)/1000)

		fmt.Printf("  %s%s%s  %s%-14s%s  %-40s  %s  %6s  %5s\n",
			ansiDim, ts, ansiReset,
			ansiTeal, e.Tool, ansiReset,
			argSummary,
			status,
			size,
			dur,
		)
	}
	fmt.Printf("\n%s%s%s\n\n", ansiDim, sep, ansiReset)
}

// shortArgs extracts a readable key="value" summary from a JSON args string.
func shortArgs(raw string, maxLen int) string {
	if raw == "" {
		return ""
	}
	// Strip braces and quotes to get a compact representation.
	s := strings.NewReplacer(`"`, "", `{`, "", `}`, "", `\n`, " ", `\t`, " ").Replace(raw)
	// Collapse multiple spaces.
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen] + "…"
	}
	return s
}
