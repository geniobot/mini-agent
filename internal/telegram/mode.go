package telegram

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// AgentHandler is satisfied by agent.Loop — using an interface keeps
// this package free of a direct agent import cycle.
type AgentHandler interface {
	HandleMsg(ctx context.Context, input string) (string, error)
}

// BotMode runs the Telegram polling loop, routing messages to the agent.
type BotMode struct {
	bot            *Bot
	agent          AgentHandler
	allowedChatIDs map[int64]bool // empty means deny-all
}

// Run starts the Telegram bot and blocks until the process is interrupted.
// allowedChatIDs must be non-empty; an empty set denies all incoming messages.
func Run(token string, allowedChatIDs []int64, agent AgentHandler) error {
	if token == "" {
		return fmt.Errorf("telegram bot token is empty — set TELEGRAM_BOT_TOKEN or telegram.bot_token in config")
	}

	allowed := make(map[int64]bool, len(allowedChatIDs))
	for _, id := range allowedChatIDs {
		allowed[id] = true
	}
	if len(allowed) == 0 {
		return fmt.Errorf("telegram.allowed_chat_ids is empty — refusing to start (would accept messages from anyone)")
	}

	bot := NewBot(token)
	mode := &BotMode{bot: bot, agent: agent, allowedChatIDs: allowed}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("  [telegram] bot started — waiting for messages (Ctrl+C to stop)\n\n")
	return mode.poll(ctx)
}

func (m *BotMode) poll(ctx context.Context) error {
	offset := 0
	backoff := time.Second

	for {
		if ctx.Err() != nil {
			fmt.Println("\n  [telegram] shutting down")
			return nil
		}

		updates, err := m.bot.GetUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			fmt.Fprintf(os.Stderr, "  [telegram] getUpdates error: %v — retrying in %s\n", err, backoff)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 60*time.Second)
			continue
		}
		backoff = time.Second // reset on success

		for _, u := range updates {
			offset = u.UpdateID + 1
			if u.Message == nil || u.Message.Text == "" {
				continue
			}
			m.handleUpdate(ctx, u)
		}
	}
}

func (m *BotMode) handleUpdate(ctx context.Context, u Update) {
	msg := u.Message
	chatID := msg.Chat.ID

	if !m.allowedChatIDs[chatID] {
		fmt.Fprintf(os.Stderr, "  [telegram] denied message from chat_id %d (not in allowed_chat_ids)\n", chatID)
		// Send a polite rejection so the user knows the bot is running but they lack access.
		_ = m.bot.SendMessage(ctx, chatID, "Sorry, you are not authorised to use this bot.")
		return
	}

	sender := msg.From.FirstName
	if msg.From.Username != "" {
		sender = "@" + msg.From.Username
	}
	fmt.Printf("  [telegram] %s (chat %d): %s\n", sender, chatID, truncate(msg.Text, 80))

	// Show typing indicator while the agent thinks.
	m.bot.SendTyping(ctx, chatID)

	response, err := m.agent.HandleMsg(ctx, msg.Text)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		fmt.Fprintf(os.Stderr, "  [telegram] agent error: %v\n", err)
		_ = m.bot.SendMessage(ctx, chatID, fmt.Sprintf("Error: %v", err))
		return
	}

	if response == "" {
		response = "(no response)"
	}

	if err := m.bot.SendMessage(ctx, chatID, response); err != nil {
		fmt.Fprintf(os.Stderr, "  [telegram] sendMessage error: %v\n", err)
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
