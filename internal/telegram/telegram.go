// Package telegram provides a minimal Telegram Bot API client.
// No external dependencies — uses only stdlib net/http and encoding/json.
//
// SECURITY NOTE: Never put your bot token in config.yaml or commit it to
// version control. Set the TELEGRAM_BOT_TOKEN environment variable instead.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	apiBase       = "https://api.telegram.org/bot"
	maxMsgLen     = 4096 // Telegram's hard message length limit
	pollTimeoutS  = 30   // long-polling window in seconds
)

// Bot is a minimal Telegram Bot API client.
type Bot struct {
	token  string
	client *http.Client
}

// Update is one item from getUpdates.
type Update struct {
	UpdateID int      `json:"update_id"`
	Message  *Message `json:"message"`
}

// Message is a Telegram message.
type Message struct {
	MessageID int    `json:"message_id"`
	From      *User  `json:"from"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
}

// Chat identifies the conversation.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"` // "private", "group", "supergroup", "channel"
}

// User is the sender.
type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username,omitempty"`
}

type apiResp struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description,omitempty"`
}

// NewBot creates a Bot with the given token.
func NewBot(token string) *Bot {
	return &Bot{
		token: token,
		client: &http.Client{
			Timeout: time.Duration(pollTimeoutS+5) * time.Second,
		},
	}
}

// GetUpdates polls for new messages. offset should be last_update_id + 1.
func (b *Bot) GetUpdates(ctx context.Context, offset int) ([]Update, error) {
	raw, err := b.call(ctx, "getUpdates", map[string]any{
		"offset":  offset,
		"timeout": pollTimeoutS,
	})
	if err != nil {
		return nil, err
	}
	var updates []Update
	return updates, json.Unmarshal(raw, &updates)
}

// SendMessage sends plain-text to chatID, splitting if needed.
func (b *Bot) SendMessage(ctx context.Context, chatID int64, text string) error {
	for _, chunk := range splitMessage(text) {
		if _, err := b.call(ctx, "sendMessage", map[string]any{
			"chat_id": chatID,
			"text":    chunk,
		}); err != nil {
			return err
		}
	}
	return nil
}

// SendTyping sends the "typing" action indicator.
func (b *Bot) SendTyping(ctx context.Context, chatID int64) {
	_, _ = b.call(ctx, "sendChatAction", map[string]any{
		"chat_id": chatID,
		"action":  "typing",
	})
}

func (b *Bot) call(ctx context.Context, method string, payload any) (json.RawMessage, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	url := apiBase + b.token + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result apiResp
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if !result.OK {
		return nil, fmt.Errorf("telegram API error: %s", result.Description)
	}
	return result.Result, nil
}

// splitMessage splits text into Telegram-legal chunks of at most maxMsgLen chars,
// preferring to break at newlines.
func splitMessage(text string) []string {
	if len(text) <= maxMsgLen {
		return []string{text}
	}
	var chunks []string
	for len(text) > maxMsgLen {
		cut := maxMsgLen
		if i := strings.LastIndex(text[:cut], "\n"); i > 0 {
			cut = i + 1
		}
		chunks = append(chunks, text[:cut])
		text = text[cut:]
	}
	if text != "" {
		chunks = append(chunks, text)
	}
	return chunks
}
