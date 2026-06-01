package session

import (
	"fmt"
	"strings"
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}
	got := EstimateTokens(msgs)
	if got <= 0 {
		t.Errorf("EstimateTokens = %d, want > 0", got)
	}
	// Empty slice should return 0.
	if n := EstimateTokens(nil); n != 0 {
		t.Errorf("EstimateTokens(nil) = %d, want 0", n)
	}
}

func TestDropOldest(t *testing.T) {
	s := New("system prompt", 10, 0)
	s.Add("user", "msg1")
	s.Add("assistant", "resp1")
	s.Add("user", "msg2")
	s.Add("assistant", "resp2")

	before := len(s.Messages)
	s.DropOldest(2)

	if len(s.Messages) != before-2 {
		t.Errorf("after DropOldest(2): got %d messages, want %d", len(s.Messages), before-2)
	}
	if s.Messages[0].Role != "system" {
		t.Errorf("system message not preserved after DropOldest")
	}
	if s.Messages[1].Content != "msg2" {
		t.Errorf("first remaining message = %q, want %q", s.Messages[1].Content, "msg2")
	}
}

func TestDropOldest_MoreThanAvailable(t *testing.T) {
	s := New("sys", 10, 0)
	s.Add("user", "only message")
	s.DropOldest(100) // should not panic or go negative
	if len(s.Messages) < 1 {
		t.Error("system message should survive DropOldest(100)")
	}
	if s.Messages[0].Role != "system" {
		t.Error("system message not preserved")
	}
}

func TestCompact_MaxHistory(t *testing.T) {
	s := New("sys", 2, 0) // max 2 pairs = 4 history messages
	for i := range 6 {
		s.Add("user", fmt.Sprintf("msg%d", i))
		s.Add("assistant", fmt.Sprintf("resp%d", i))
	}
	// 1 system + at most 2*2 = 4 history messages
	want := 5
	if len(s.Messages) != want {
		t.Errorf("len(Messages) = %d, want %d", len(s.Messages), want)
	}
	if s.Messages[0].Role != "system" {
		t.Error("system message not preserved")
	}
	// Most recent pair should be last.
	last := s.Messages[len(s.Messages)-1]
	if last.Content != "resp5" {
		t.Errorf("last message = %q, want %q", last.Content, "resp5")
	}
}

func TestCompact_NoSystemPrompt(t *testing.T) {
	s := New("", 2, 0)
	for i := range 5 {
		s.Add("user", fmt.Sprintf("u%d", i))
		s.Add("assistant", fmt.Sprintf("a%d", i))
	}
	if len(s.Messages) > 4 {
		t.Errorf("len(Messages) = %d, want <= 4", len(s.Messages))
	}
}

func TestSetSummary_InsertsAfterSystemPrompt(t *testing.T) {
	s := New("you are helpful", 10, 0)
	s.Add("user", "hello")
	s.SetSummary("user greeted the assistant")

	if len(s.Messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(s.Messages))
	}
	if s.Messages[0].Role != "system" || s.Messages[0].Content != "you are helpful" {
		t.Error("main system prompt should remain at index 0")
	}
	if s.Messages[1].Role != "system" {
		t.Errorf("summary should be a system message at index 1, got role %q", s.Messages[1].Role)
	}
	if !strings.HasPrefix(s.Messages[1].Content, summaryMarker) {
		t.Errorf("summary message should start with marker, got %q", s.Messages[1].Content)
	}
}

func TestSetSummary_ReplacesExisting(t *testing.T) {
	s := New("sys", 10, 0)
	s.SetSummary("first summary")
	s.SetSummary("updated summary")

	count := 0
	for _, m := range s.Messages {
		if m.Role == "system" && strings.HasPrefix(m.Content, summaryMarker) {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 summary message, got %d", count)
	}
	for _, m := range s.Messages {
		if strings.HasPrefix(m.Content, summaryMarker) {
			if !strings.Contains(m.Content, "updated summary") {
				t.Errorf("expected updated summary content, got %q", m.Content)
			}
		}
	}
}

func TestSetSummary_SurvivesCompaction(t *testing.T) {
	s := New("sys", 2, 0) // max 2 pairs
	s.SetSummary("earlier context: user asked about Go")
	for i := range 5 {
		s.Add("user", fmt.Sprintf("msg%d", i))
		s.Add("assistant", fmt.Sprintf("resp%d", i))
	}
	// Summary should still be present after compaction.
	hasSummary := false
	for _, m := range s.Messages {
		if m.Role == "system" && strings.HasPrefix(m.Content, summaryMarker) {
			hasSummary = true
		}
	}
	if !hasSummary {
		t.Error("summary message was dropped by compaction; it should survive in the protected zone")
	}
}

func TestCompact_CapturesDroppedMessages(t *testing.T) {
	s := New("sys", 2, 0) // max 2 pairs
	s.Add("user", "msg1")
	s.Add("assistant", "resp1")
	s.Add("user", "msg2")
	s.Add("assistant", "resp2")
	s.DroppedMessages = nil // clear baseline
	// Adding one more pair pushes the oldest out.
	s.Add("user", "msg3")
	s.Add("assistant", "resp3")

	if len(s.DroppedMessages) == 0 {
		t.Error("expected DroppedMessages to be populated after compaction")
	}
}

func TestHistoryStart_SkipsMultipleSystemMessages(t *testing.T) {
	s := New("main system", 10, 0)
	s.SetSummary("some earlier context")
	s.Add("user", "hello")

	start := s.historyStart()
	if s.Messages[start].Role != "user" {
		t.Errorf("historyStart should point to first user message, got role %q at index %d", s.Messages[start].Role, start)
	}
}

func TestSnapshot_IsACopy(t *testing.T) {
	s := New("sys", 10, 0)
	s.Add("user", "hello")
	snap := s.Snapshot()
	snap[0].Content = "mutated"
	if s.Messages[0].Content == "mutated" {
		t.Error("Snapshot returned a reference, not a copy")
	}
}
