package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/geniobot/mini-agent/internal/session"
)

// OpenAIClient implements Client and ModelLister against any OpenAI-compatible
// /v1/chat/completions endpoint (OpenRouter, Groq, LM Studio, Deepseek, etc.).
type OpenAIClient struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

func NewOpenAI(baseURL, apiKey, model string) *OpenAIClient {
	base := strings.TrimRight(baseURL, "/")
	base = strings.TrimSuffix(base, "/v1") // normalise: client appends /v1/chat/completions
	return &OpenAIClient{
		BaseURL:    base,
		APIKey:     apiKey,
		Model:      model,
		HTTPClient: &http.Client{Timeout: 0},
	}
}

type openAIRequest struct {
	Model       string            `json:"model"`
	Messages    []session.Message `json:"messages"`
	Stream      bool              `json:"stream"`
	Tools       []ToolSpec        `json:"tools,omitempty"`
	Temperature *float64          `json:"temperature,omitempty"`
	MaxTokens   *int              `json:"max_tokens,omitempty"`
}

type openAIChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *OpenAIClient) ChatStream(ctx context.Context, req ChatRequest, onChunk func(ChatChunk) error) error {
	oaReq := openAIRequest{
		Model:    req.Model,
		Messages: req.Messages,
		Stream:   req.Stream,
		Tools:    req.Tools,
	}
	if t, ok := floatFromOptions(req.Options, "temperature"); ok {
		oaReq.Temperature = &t
	}
	if n, ok := intFromOptions(req.Options, "num_predict"); ok {
		oaReq.MaxTokens = &n
	}

	body, err := json.Marshal(oaReq)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("openai http %d: %s", resp.StatusCode, string(b))
	}

	// tool_calls[index] → accumulated arguments string
	type tcEntry struct{ id, name, args string }
	toolArgsBuf := map[int]tcEntry{}

	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "data: [DONE]" {
			// Assemble any accumulated tool calls into the final chunk.
			var tcs []session.ToolCall
			for _, tc := range toolArgsBuf {
				var args map[string]any
				if tc.args != "" {
					_ = json.Unmarshal([]byte(tc.args), &args)
				}
				tcs = append(tcs, session.ToolCall{
					Function: session.ToolFunction{Name: tc.name, Arguments: args},
				})
			}
			msg := session.Message{Role: "assistant"}
			if len(tcs) > 0 {
				msg.ToolCalls = tcs
			}
			return onChunk(ChatChunk{Message: msg, Done: true})
		}
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var chunk openAIChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("openai sse parse: %w", err)
		}
		if chunk.Error != nil {
			return fmt.Errorf("openai error: %s", chunk.Error.Message)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		// Stream text content.
		if delta.Content != "" {
			if err := onChunk(ChatChunk{Message: session.Message{Role: "assistant", Content: delta.Content}}); err != nil {
				return err
			}
		}

		// Accumulate tool call argument fragments.
		for _, tc := range delta.ToolCalls {
			entry := toolArgsBuf[tc.Index]
			if tc.ID != "" {
				entry.id = tc.ID
			}
			if tc.Function.Name != "" {
				entry.name = tc.Function.Name
			}
			entry.args += tc.Function.Arguments
			toolArgsBuf[tc.Index] = entry
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	// Flush any accumulated tool calls even if [DONE] was absent.
	var tcs []session.ToolCall
	for _, tc := range toolArgsBuf {
		var args map[string]any
		if tc.args != "" {
			_ = json.Unmarshal([]byte(tc.args), &args)
		}
		tcs = append(tcs, session.ToolCall{
			Function: session.ToolFunction{Name: tc.name, Arguments: args},
		})
	}
	msg := session.Message{Role: "assistant"}
	if len(tcs) > 0 {
		msg.ToolCalls = tcs
	}
	return onChunk(ChatChunk{Message: msg, Done: true})
}

// ListModels fetches the model list from /v1/models.
func (c *OpenAIClient) ListModels() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	names := make([]string, len(payload.Data))
	for i, m := range payload.Data {
		names[i] = m.ID
	}
	return names, nil
}

func floatFromOptions(opts map[string]any, key string) (float64, bool) {
	if opts == nil {
		return 0, false
	}
	switch v := opts[key].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	}
	return 0, false
}

func intFromOptions(opts map[string]any, key string) (int, bool) {
	if opts == nil {
		return 0, false
	}
	switch v := opts[key].(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	}
	return 0, false
}
