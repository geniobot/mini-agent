package session

import (
	"fmt"
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

func TestSnapshot_IsACopy(t *testing.T) {
	s := New("sys", 10, 0)
	s.Add("user", "hello")
	snap := s.Snapshot()
	snap[0].Content = "mutated"
	if s.Messages[0].Content == "mutated" {
		t.Error("Snapshot returned a reference, not a copy")
	}
}
