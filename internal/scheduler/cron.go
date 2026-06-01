// Package scheduler provides a minimal 5-field POSIX cron parser.
// Supported syntax per field: * */N N N-M N,M (and combinations).
// Day-of-week and day-of-month follow standard cron OR semantics when both are
// specified (either matching is enough to trigger the entry).
package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronExpr is a parsed 5-field cron expression.
type CronExpr struct {
	minute  [60]bool
	hour    [24]bool
	dom     [32]bool // indices 1-31
	month   [13]bool // indices 1-12
	dow     [7]bool  // indices 0-6 (0 = Sunday)
	domStar bool     // dom field was "*" (wildcard)
	dowStar bool     // dow field was "*" (wildcard)
}

// Parse parses a 5-field cron expression string.
// Fields: minute hour day-of-month month day-of-week
func Parse(expr string) (*CronExpr, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("expected 5 fields, got %d in %q", len(fields), expr)
	}
	e := &CronExpr{
		domStar: fields[2] == "*",
		dowStar: fields[4] == "*",
	}
	if err := parseField(fields[0], 0, 59, e.minute[:]); err != nil {
		return nil, fmt.Errorf("minute: %w", err)
	}
	if err := parseField(fields[1], 0, 23, e.hour[:]); err != nil {
		return nil, fmt.Errorf("hour: %w", err)
	}
	if err := parseField(fields[2], 1, 31, e.dom[:]); err != nil {
		return nil, fmt.Errorf("day-of-month: %w", err)
	}
	if err := parseField(fields[3], 1, 12, e.month[:]); err != nil {
		return nil, fmt.Errorf("month: %w", err)
	}
	if err := parseField(fields[4], 0, 6, e.dow[:]); err != nil {
		return nil, fmt.Errorf("day-of-week: %w", err)
	}
	return e, nil
}

// Next returns the first time strictly after `after` when the expression fires.
// Returns zero time if no match is found within one year.
func (e *CronExpr) Next(after time.Time) time.Time {
	t := after.Truncate(time.Minute).Add(time.Minute)
	limit := after.Add(366 * 24 * time.Hour)

	for !t.After(limit) {
		if !e.month[int(t.Month())] {
			t = advanceMonth(t)
			continue
		}
		if !e.matchDay(t) {
			t = advanceDay(t)
			continue
		}
		if !e.hour[t.Hour()] {
			t = advanceHour(t)
			continue
		}
		if !e.minute[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}
		return t
	}
	return time.Time{}
}

// matchDay checks whether t satisfies the dom/dow fields using standard cron
// OR semantics: if both dom and dow are restricted, either match is sufficient.
func (e *CronExpr) matchDay(t time.Time) bool {
	if e.domStar && e.dowStar {
		return true
	}
	if e.domStar {
		return e.dow[int(t.Weekday())]
	}
	if e.dowStar {
		return e.dom[t.Day()]
	}
	return e.dom[t.Day()] || e.dow[int(t.Weekday())]
}

// parseField parses a comma-separated cron field into a boolean slice.
func parseField(s string, min, max int, out []bool) error {
	for _, part := range strings.Split(s, ",") {
		if err := parsePart(part, min, max, out); err != nil {
			return err
		}
	}
	return nil
}

// parsePart parses one part of a cron field (handles *, N, N-M, */N, N-M/N).
func parsePart(s string, min, max int, out []bool) error {
	step := 1
	if i := strings.Index(s, "/"); i >= 0 {
		var err error
		step, err = strconv.Atoi(s[i+1:])
		if err != nil || step < 1 {
			return fmt.Errorf("invalid step %q", s[i+1:])
		}
		s = s[:i]
	}

	var lo, hi int
	switch {
	case s == "*":
		lo, hi = min, max
	case strings.Contains(s, "-"):
		parts := strings.SplitN(s, "-", 2)
		var err error
		lo, err = strconv.Atoi(parts[0])
		if err != nil {
			return fmt.Errorf("invalid range start %q", parts[0])
		}
		hi, err = strconv.Atoi(parts[1])
		if err != nil {
			return fmt.Errorf("invalid range end %q", parts[1])
		}
	default:
		v, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("invalid value %q", s)
		}
		lo, hi = v, v
	}

	if lo < min || hi > max || lo > hi {
		return fmt.Errorf("value %d-%d out of bounds %d-%d", lo, hi, min, max)
	}
	for v := lo; v <= hi; v += step {
		out[v] = true
	}
	return nil
}

func advanceMonth(t time.Time) time.Time {
	y, m, _ := t.Date()
	m++
	if m > 12 {
		m, y = 1, y+1
	}
	return time.Date(y, m, 1, 0, 0, 0, 0, t.Location())
}

func advanceDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, t.Location())
}

func advanceHour(t time.Time) time.Time {
	y, m, d := t.Date()
	h := t.Hour() + 1
	if h >= 24 {
		return time.Date(y, m, d+1, 0, 0, 0, 0, t.Location())
	}
	return time.Date(y, m, d, h, 0, 0, 0, t.Location())
}
