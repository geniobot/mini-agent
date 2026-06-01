package agent

import (
	"context"
	"fmt"
	"strings"

	"mini-agent/internal/llm"
	"mini-agent/internal/session"
)

const summarizeMaxMsgChars = 600 // per-message content cap when building the transcript

// summarizeDropped asks the active LLM to produce a short summary of the dropped
// messages so the agent can preserve key facts across compaction boundaries.
// Works with any provider (Ollama or OpenAI-compat).
func summarizeDropped(ctx context.Context, client llm.Client, model string, dropped []session.Message) (string, error) {
	var transcript strings.Builder
	for _, m := range dropped {
		if m.Role == "system" {
			continue
		}
		content := m.Content
		if len(content) > summarizeMaxMsgChars {
			content = content[:summarizeMaxMsgChars] + "…"
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		fmt.Fprintf(&transcript, "%s: %s\n\n", m.Role, content)
	}
	if transcript.Len() == 0 {
		return "", nil
	}

	prompt := "Summarize the following conversation excerpt in 2-3 sentences. " +
		"Preserve key facts, file names, goals, and decisions. Omit pleasantries.\n\n" +
		transcript.String()

	req := llm.ChatRequest{
		Model:  model,
		Stream: true,
		Messages: []session.Message{
			{Role: "user", Content: prompt},
		},
	}

	var result strings.Builder
	err := client.ChatStream(ctx, req, func(chunk llm.ChatChunk) error {
		result.WriteString(chunk.Message.Content)
		return nil
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.String()), nil
}
