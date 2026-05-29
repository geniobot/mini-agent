package session

import "strings"

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
	Name      string                 `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type Session struct {
	SystemPrompt string
	MaxHistory   int
	MaxTokens    int // 0 = disabled; when set, history is trimmed to stay within budget
	Messages     []Message
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

// DropOldest removes n messages from the start of history (after the system prompt).
func (s *Session) DropOldest(n int) {
	start := s.historyStart()
	drop := min(n, len(s.Messages)-start)
	if drop > 0 {
		s.Messages = append(s.Messages[:start], s.Messages[start+drop:]...)
	}
}

func (s *Session) historyStart() int {
	if len(s.Messages) > 0 && s.Messages[0].Role == "system" {
		return 1
	}
	return 0
}

func (s *Session) compact() {
	start := s.historyStart()

	// Message-count trim: keep at most MaxHistory user/assistant pairs.
	if s.MaxHistory > 0 {
		keep := s.MaxHistory * 2
		if len(s.Messages[start:]) > keep {
			s.Messages = append(s.Messages[:start], s.Messages[len(s.Messages)-keep:]...)
		}
	}

	// Token-budget trim: drop oldest pairs until we fit within 65% of MaxTokens,
	// leaving 35% headroom for the model's response.
	if s.MaxTokens > 0 {
		budget := s.MaxTokens * 65 / 100
		for EstimateTokens(s.Messages) > budget && len(s.Messages) > start+2 {
			s.Messages = append(s.Messages[:start], s.Messages[start+2:]...)
		}
	}
}
