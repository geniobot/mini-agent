package scheduler

import (
	"testing"
	"time"
)

// anchor is a fixed reference time: Wednesday 2026-01-07 08:30
var anchor = time.Date(2026, 1, 7, 8, 30, 0, 0, time.UTC)

func mustParse(t *testing.T, expr string) *CronExpr {
	t.Helper()
	e, err := Parse(expr)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", expr, err)
	}
	return e
}

func TestNext_EveryMinute(t *testing.T) {
	e := mustParse(t, "* * * * *")
	got := e.Next(anchor)
	want := anchor.Add(time.Minute)
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNext_HourlyOnTheHour(t *testing.T) {
	e := mustParse(t, "0 * * * *")
	got := e.Next(anchor) // anchor is 08:30 → next is 09:00
	want := time.Date(2026, 1, 7, 9, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNext_DailyAt8am(t *testing.T) {
	e := mustParse(t, "0 8 * * *")
	// anchor is 08:30; next 08:00 is tomorrow
	got := e.Next(anchor)
	want := time.Date(2026, 1, 8, 8, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNext_DailyAt8am_Before8(t *testing.T) {
	e := mustParse(t, "0 8 * * *")
	before := time.Date(2026, 1, 7, 7, 55, 0, 0, time.UTC)
	got := e.Next(before)
	want := time.Date(2026, 1, 7, 8, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNext_Every30Min(t *testing.T) {
	e := mustParse(t, "*/30 * * * *")
	// anchor is 08:30 → next is 09:00
	got := e.Next(anchor)
	want := time.Date(2026, 1, 7, 9, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNext_Every15Min(t *testing.T) {
	e := mustParse(t, "*/15 * * * *")
	// anchor is 08:30 → next is 08:45
	got := e.Next(anchor)
	want := time.Date(2026, 1, 7, 8, 45, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNext_SpecificDayOfWeek(t *testing.T) {
	// Every Monday at 09:00; anchor is Wednesday
	e := mustParse(t, "0 9 * * 1")
	got := e.Next(anchor)
	want := time.Date(2026, 1, 12, 9, 0, 0, 0, time.UTC) // next Monday
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNext_SpecificDayOfMonth(t *testing.T) {
	// 1st of each month at midnight
	e := mustParse(t, "0 0 1 * *")
	got := e.Next(anchor)
	want := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNext_MonthRestriction(t *testing.T) {
	// Every March 1st at noon
	e := mustParse(t, "0 12 1 3 *")
	got := e.Next(anchor) // anchor is Jan
	want := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNext_CommaList(t *testing.T) {
	// At minutes 0, 20, 40
	e := mustParse(t, "0,20,40 * * * *")
	// anchor is 08:30 → next is 08:40
	got := e.Next(anchor)
	want := time.Date(2026, 1, 7, 8, 40, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNext_RangeField(t *testing.T) {
	// Weekdays (Mon-Fri) at 9am
	e := mustParse(t, "0 9 * * 1-5")
	// anchor is Wednesday; next is same day (anchor is 08:30)
	got := e.Next(anchor)
	want := time.Date(2026, 1, 7, 9, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNext_MidnightRollover(t *testing.T) {
	// 23:59 every day
	e := mustParse(t, "59 23 * * *")
	// anchor is 08:30 → same day 23:59
	got := e.Next(anchor)
	want := time.Date(2026, 1, 7, 23, 59, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestParse_InvalidFieldCount(t *testing.T) {
	if _, err := Parse("* * * *"); err == nil {
		t.Error("expected error for 4-field expression")
	}
	if _, err := Parse("* * * * * *"); err == nil {
		t.Error("expected error for 6-field expression")
	}
}

func TestParse_OutOfRange(t *testing.T) {
	if _, err := Parse("60 * * * *"); err == nil {
		t.Error("expected error for minute=60")
	}
	if _, err := Parse("* 24 * * *"); err == nil {
		t.Error("expected error for hour=24")
	}
	if _, err := Parse("* * 0 * *"); err == nil {
		t.Error("expected error for dom=0")
	}
	if _, err := Parse("* * * 13 *"); err == nil {
		t.Error("expected error for month=13")
	}
	if _, err := Parse("* * * * 7"); err == nil {
		t.Error("expected error for dow=7")
	}
}

func TestParse_InvalidStep(t *testing.T) {
	if _, err := Parse("*/0 * * * *"); err == nil {
		t.Error("expected error for step=0")
	}
}
