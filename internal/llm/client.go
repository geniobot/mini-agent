package llm

import (
	"context"

	"github.com/geniobot/mini-agent/internal/session"
)

type ToolSpec struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ChatRequest struct {
	Model     string                 `json:"model"`
	Messages  []session.Message      `json:"messages"`
	Stream    bool                   `json:"stream"`
	Tools     []ToolSpec             `json:"tools,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
	KeepAlive string                 `json:"keep_alive,omitempty"`
}

type ChatChunk struct {
	Message session.Message `json:"message"`
	Done    bool            `json:"done"`
	Error   string          `json:"error,omitempty"`
}

// Client is the minimal interface every LLM backend must implement.
type Client interface {
	ChatStream(ctx context.Context, req ChatRequest, onChunk func(ChatChunk) error) error
}

// ModelLister is an optional capability for listing available models.
// Checked via type assertion — not required by all backends.
type ModelLister interface {
	ListModels() ([]string, error)
}

// Unloader is an optional capability for evicting a model from RAM.
// Checked via type assertion — not required by all backends.
type Unloader interface {
	Unload(model string) error
}
