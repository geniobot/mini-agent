package session

import "strings"

const summaryMarker = "[context summary]"

type Message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Name      string     `json:"name,omitempty"`
}

type ToolCall struct {
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type Session struct {
	SystemPrompt    string
	MaxHistory      int
	MaxTokens       int // 0 = disabled; when set, history is trimmed to stay within budget
	Messages        []Message
	DroppedMessages []Message // populated by compact(); consumed by the agent loop for summarization
}

func New(systemPrompt string, maxHistory int, maxTokens int) *Session {
	cap := maxHistory*2 + 2
	s := &Session{
		SystemPrompt: systemPrompt,
		MaxHistory:   maxHistory,
		MaxTokens:    maxTokens,
		Messages:     make([]Message, 0, cap),
	}
	if strings.TrimSpace(systemPrompt) != "" {
		s.Messages = append(s.Messages, Message{Role: "system", Content: systemPrompt})
	}
	return s
}

func (s *Session) Add(role, content string) {
	s.Messages = append(s.Messages, Message{Role: role, Content: content})
	s.compact()
}

func (s *Session) AddMessage(msg Message) {
	s.Messages = append(s.Messages, msg)
	s.compact()
}

func (s *Session) Snapshot() []Message {
	out := make([]Message, len(s.Messages))
	copy(out, s.Messages)
	return out
}

// EstimateTokens returns a rough token count for a message slice (~4 chars per token).
func EstimateTokens(msgs []Message) int {
	n := 0
	for _, m := range msgs {
		n += len(m.Content)/4 + 4
		for range m.ToolCalls {
			n += 20
		}
	}
	return n
}

// Len returns the total number of messages including system messages.
func (s *Session) Len() int { return len(s.Messages) }

// TruncateTo cuts the message list to n entries. No-op if already shorter.
// Used by /retry to roll back to the state before the last turn.
func (s *Session) TruncateTo(n int) {
	if n >= 0 && n < len(s.Messages) {
		s.Messages = s.Messages[:n]
	}
}

// DropOldest removes n messages from the start of history (after the system prompt).
func (s *Session) DropOldest(n int) {
	start := s.historyStart()
	drop := min(n, len(s.Messages)-start)
	if drop > 0 {
		s.Messages = append(s.Messages[:start], s.Messages[start+drop:]...)
	}
}

// historyStart returns the index of the first non-system message.
// All leading system messages (main prompt + injected summary) are in the protected zone.
func (s *Session) historyStart() int {
	for i, m := range s.Messages {
		if m.Role != "system" {
			return i
		}
	}
	return len(s.Messages)
}

// SetSummary inserts or replaces a compact-summary system message immediately
// after the main system prompt. The summary survives compaction because it lives
// in the protected zone (before historyStart).
func (s *Session) SetSummary(text string) {
	if text == "" {
		return
	}
	msg := Message{Role: "system", Content: summaryMarker + " " + text}
	for i, m := range s.Messages {
		if m.Role == "system" && strings.HasPrefix(m.Content, summaryMarker) {
			s.Messages[i] = msg
			return
		}
	}
	// No existing summary — insert at position 1 (after main system prompt), or 0 if none.
	insertAt := 0
	if len(s.Messages) > 0 && s.Messages[0].Role == "system" {
		insertAt = 1
	}
	s.Messages = append(s.Messages, Message{})
	copy(s.Messages[insertAt+1:], s.Messages[insertAt:])
	s.Messages[insertAt] = msg
}

func (s *Session) compact() {
	start := s.historyStart()

	// Message-count trim: keep at most MaxHistory user/assistant pairs.
	if s.MaxHistory > 0 {
		keep := s.MaxHistory * 2
		hist := s.Messages[start:]
		if len(hist) > keep {
			dropped := make([]Message, len(hist)-keep)
			copy(dropped, hist[:len(hist)-keep])
			s.DroppedMessages = append(s.DroppedMessages, dropped...)
			s.Messages = append(s.Messages[:start], hist[len(hist)-keep:]...)
		}
	}

	// Token-budget trim: drop oldest pairs until we fit within 65% of MaxTokens,
	// leaving 35% headroom for the model's response.
	if s.MaxTokens > 0 {
		budget := s.MaxTokens * 65 / 100
		for EstimateTokens(s.Messages) > budget && len(s.Messages) > start+2 {
			s.DroppedMessages = append(s.DroppedMessages, s.Messages[start:start+2]...)
			s.Messages = append(s.Messages[:start], s.Messages[start+2:]...)
		}
	}
}
