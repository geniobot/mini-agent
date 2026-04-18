package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"mini-agent/internal/config"
	"mini-agent/internal/memory"
	"mini-agent/internal/ollama"
	"mini-agent/internal/tools"
)

type Loop struct {
	cfg      *config.Config
	client   *ollama.Client
	session  *memory.Session
	registry *tools.Registry
}

type fallbackToolRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func New(cfg *config.Config) *Loop {
	return &Loop{
		cfg:      cfg,
		client:   ollama.New(cfg.Ollama.Host),
		session:  memory.New(cfg.Agent.SystemPrompt, cfg.Agent.MaxHistory),
		registry: tools.New(cfg.Tools),
	}
}

func (l *Loop) Run() error {
	fmt.Println("mini-agent v2 ready. Type /exit to quit.")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			return scanner.Err()
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "/exit" || input == "/quit" || input == "/bye" {
			return nil
		}
		if err := l.handle(input); err != nil {
			fmt.Printf("\n[error] %v\n", err)
		}
	}
}

func (l *Loop) handle(input string) error {
	l.session.Add("user", input)
	assistant, err := l.chatOnce()
	if err != nil {
		return err
	}
	if len(assistant.ToolCalls) == 0 && l.registry.Enabled() {
		if tc := parseFallbackToolCall(assistant.Content); len(tc) > 0 {
			fmt.Println("[fallback tool parse]")
			assistant.ToolCalls = tc
			if l.cfg.Tools.UseNativeTools {
				assistant.Content = ""
			}
		}
	}
	if len(assistant.ToolCalls) == 0 {
		l.session.AddMessage(assistant)
		return nil
	}

	fmt.Println("\n[tool phase]")
	
	// Add assistant message. If not using native tools, strip ToolCalls before adding to history.
	historyAssistant := assistant
	if !l.cfg.Tools.UseNativeTools {
		historyAssistant.ToolCalls = nil
	}
	l.session.AddMessage(historyAssistant)

	var toolResults strings.Builder
	for _, tc := range assistant.ToolCalls {
		argsJSON, _ := json.Marshal(tc.Function.Arguments)
		if tc.Function.Name == "run_command" && l.registry.ConfirmRunCommand() {
			if !confirm("Allow run_command: " + l.registry.AllowedCommandPrompt(string(argsJSON)) + " ? [y/N]: ") {
				if l.cfg.Tools.UseNativeTools {
					l.session.AddMessage(memory.Message{Role: "tool", Name: tc.Function.Name, Content: "tool error: user denied command"})
				} else {
					toolResults.WriteString("tool error: user denied command\n")
				}
				continue
			}
		}
		result, err := l.registry.Execute(tc.Function.Name, string(argsJSON))
		if err != nil {
			result = "tool error: " + err.Error()
		}
		fmt.Printf("- %s\n", tc.Function.Name)
		if l.cfg.Tools.UseNativeTools {
			l.session.AddMessage(memory.Message{Role: "tool", Name: tc.Function.Name, Content: result})
		} else {
			toolResults.WriteString(fmt.Sprintf("Tool %s executed. Result:\n%s\n", tc.Function.Name, result))
		}
	}

	if !l.cfg.Tools.UseNativeTools && toolResults.Len() > 0 {
		l.session.AddMessage(memory.Message{Role: "user", Content: toolResults.String()})
	}

	final, err := l.chatOnce()
	if err != nil {
		return err
	}
	l.session.AddMessage(final)
	return nil
}

func (l *Loop) chatOnce() (memory.Message, error) {
	fmt.Print("\nassistant> ")
	var content strings.Builder
	var assistant memory.Message
	var toolCalls []memory.ToolCall
	var reqTools []ollama.ToolSpec
	if l.registry.Enabled() && l.cfg.Tools.UseNativeTools {
		reqTools = l.registry.Specs()
	}

	var printedCount int
	var isToolCall bool

	err := l.client.ChatStream(ollama.ChatRequest{
		Model:     l.cfg.Ollama.Model,
		Messages:  l.session.Snapshot(),
		Stream:    l.cfg.Ollama.Stream,
		Tools:     reqTools,
		Options:   l.cfg.Ollama.Options,
		KeepAlive: l.cfg.Ollama.KeepAlive,
	}, func(chunk ollama.ChatChunk) error {
		if chunk.Message.Content != "" {
			content.WriteString(chunk.Message.Content)
			str := content.String()
			
			if !isToolCall {
				if strings.HasPrefix(str, "```json") || strings.HasPrefix(str, "{") {
					isToolCall = true
				} else if strings.HasPrefix("```json", str) || strings.HasPrefix("{", str) {
					// Wait for more tokens to confirm
				} else {
					fmt.Print(str[printedCount:])
					printedCount = len(str)
				}
			}
		}
		if len(chunk.Message.ToolCalls) > 0 {
			toolCalls = chunk.Message.ToolCalls
		}
		assistant.Role = "assistant"
		assistant.Content = content.String()
		assistant.ToolCalls = toolCalls
		return nil
	})
	fmt.Println()
	return assistant, err
}

func parseFallbackToolCall(content string) []memory.ToolCall {
	clean := strings.TrimSpace(content)
	if clean == "" {
		return nil
	}
	if strings.HasPrefix(clean, "```") {
		lines := strings.Split(clean, "\n")
		if len(lines) >= 3 && strings.HasPrefix(lines[0], "```") && strings.TrimSpace(lines[len(lines)-1]) == "```" {
			clean = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}
	start := strings.Index(clean, "{")
	end := strings.LastIndex(clean, "}")
	if start < 0 || end < start {
		return nil
	}
	candidate := strings.TrimSpace(clean[start : end+1])
	var req fallbackToolRequest
	if err := json.Unmarshal([]byte(candidate), &req); err != nil {
		return nil
	}
	if req.Name == "" || len(req.Arguments) == 0 {
		return nil
	}
	switch req.Name {
	case "read_file", "write_file", "run_command":
		var argsMap map[string]interface{}
		_ = json.Unmarshal(req.Arguments, &argsMap)
		return []memory.ToolCall{{Function: memory.ToolFunction{Name: req.Name, Arguments: argsMap}}}
	default:
		return nil
	}
}

func confirm(prompt string) bool {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	ans := strings.ToLower(strings.TrimSpace(text))
	return ans == "y" || ans == "yes"
}
