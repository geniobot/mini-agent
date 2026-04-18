package memory

import "strings"

type Message struct {
	Role      string      `json:"role"`
	Content   string      `json:"content,omitempty"`
	ToolCalls []ToolCall  `json:"tool_calls,omitempty"`
	Name      string      `json:"name,omitempty"`
}

type ToolCall struct {
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type Session struct {
	SystemPrompt string
	MaxHistory   int
	Messages     []Message
}

func New(systemPrompt string, maxHistory int) *Session {
	s := &Session{SystemPrompt: systemPrompt, MaxHistory: maxHistory}
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

func (s *Session) compact() {
	if s.MaxHistory <= 0 {
		return
	}
	start := 0
	if len(s.Messages) > 0 && s.Messages[0].Role == "system" {
		start = 1
	}
	keep := s.MaxHistory * 2
	if len(s.Messages[start:]) <= keep {
		return
	}
	s.Messages = append(s.Messages[:start], s.Messages[len(s.Messages)-keep:]...)
}
