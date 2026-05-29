package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"mini-agent/internal/config"
	"mini-agent/internal/llm"
	"mini-agent/internal/session"
	"mini-agent/internal/tools"
)

type Loop struct {
	cfg      *config.Config
	client   llm.Client
	session  *session.Session
	registry *tools.Registry
	maxCtx   int
	savePath string // empty = no session persistence
	quiet    bool   // suppress decoration; only emit clean output
}

type fallbackToolRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func New(cfg *config.Config) *Loop {
	ctx := numCtx(cfg.Ollama.Options)
	return &Loop{
		cfg:      cfg,
		client:   llm.NewOllama(cfg.Ollama.Host),
		session:  session.New(cfg.Agent.SystemPrompt, cfg.Agent.MaxHistory, ctx),
		registry: tools.New(cfg.Tools),
		maxCtx:   ctx,
	}
}

func (l *Loop) SetSavePath(p string) { l.savePath = p }
func (l *Loop) SetQuiet(q bool)      { l.quiet = q }

// LoadSession restores messages from path into the current session.
func (l *Loop) LoadSession(path string) (int, error) {
	msgs, err := session.Load(path)
	if err != nil || len(msgs) == 0 {
		return 0, err
	}
	for _, m := range msgs {
		l.session.AddMessage(m)
	}
	return len(msgs), nil
}

// RunGoal is the public entry point for non-interactive goal execution (--run flag).
func (l *Loop) RunGoal(goal string) error {
	if err := l.pingOllama(false); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return l.runGoal(ctx, goal)
}

func (l *Loop) Run() error {
	if !l.quiet {
		printBanner(l.cfg)
		l.pingOllama(true) //nolint:errcheck — error printed inside; we continue to REPL
	}

	// Persist session on clean exit.
	if l.savePath != "" {
		defer func() {
			if err := session.Save(l.savePath, l.session.Snapshot()); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not save session: %v\n", err)
			}
		}()
	}

	scanner := bufio.NewScanner(os.Stdin)
	for {
		if !l.quiet {
			l.printPrompt()
		}
		if !scanner.Scan() {
			return scanner.Err()
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Commands that don't involve the LLM are handled without a context.
		switch {
		case input == "/exit" || input == "/quit" || input == "/bye":
			return nil
		case input == "/clear":
			l.session = session.New(l.cfg.Agent.SystemPrompt, l.cfg.Agent.MaxHistory, l.maxCtx)
			l.printf("%s[session cleared]%s\n", ansiDim, ansiReset)
		case input == "/forget" || strings.HasPrefix(input, "/forget "):
			n := 2
			if rest := strings.TrimSpace(strings.TrimPrefix(input, "/forget")); rest != "" {
				if v, err := strconv.Atoi(rest); err == nil && v > 0 {
					n = v
				}
			}
			l.session.DropOldest(n)
			l.printf("%s[dropped %d messages from history]%s\n", ansiDim, n, ansiReset)
			continue
		case input == "/help":
			l.printHelp()
			continue
		case input == "/status":
			l.printStatus()
			continue
		case input == "/unload":
			l.unloadModel()
			continue
		case input == "/model":
			fmt.Printf("  current model: %s\n", l.cfg.Ollama.Model)
			continue
		case strings.HasPrefix(input, "/model "):
			name := strings.TrimSpace(strings.TrimPrefix(input, "/model "))
			l.cfg.Ollama.Model = name
			l.printf("%s[model → %s]%s\n", ansiTeal, name, ansiReset)
			continue
		case strings.HasPrefix(input, "/load "):
			path := strings.TrimSpace(strings.TrimPrefix(input, "/load "))
			l.loadFileIntoContext(path)
			continue
		}

		// LLM operations run under a signal-cancellable context.
		// Ctrl+C cancels the current generation and returns to the prompt.
		// A second Ctrl+C (while not in a generation) uses the default handler and exits.
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		var err error
		if strings.HasPrefix(input, "/run ") {
			goal := strings.TrimSpace(strings.TrimPrefix(input, "/run "))
			err = l.runGoal(ctx, goal)
		} else {
			err = l.handle(ctx, input)
		}
		stop()

		if err != nil {
			if errors.Is(err, context.Canceled) {
				l.printf("\n%s[interrupted — Ctrl+C again to exit]%s\n", ansiDim, ansiReset)
			} else {
				fmt.Printf("\n[error] %v\n", err)
			}
		}
	}
}

func (l *Loop) printPrompt() {
	if l.maxCtx > 0 {
		tokens := session.EstimateTokens(l.session.Snapshot())
		fmt.Printf("\n%s[%d/%d tok]%s > ", ansiDim, tokens, l.maxCtx, ansiReset)
	} else {
		fmt.Print("\n> ")
	}
}

func (l *Loop) printHelp() {
	fmt.Printf("\n%sCommands:%s\n", ansiTeal, ansiReset)
	cmds := [][2]string{
		{"/run <goal>", "run a goal autonomously (multi-step)"},
		{"/load <file>", "inject a file into conversation context"},
		{"/model [name]", "show or switch the active model"},
		{"/unload", "evict model from RAM to free memory"},
		{"/status", "show session and model status"},
		{"/forget [N]", "drop last N messages from history (default 2)"},
		{"/clear", "reset entire conversation history"},
		{"/help", "show this message"},
		{"/exit", "quit"},
	}
	for _, c := range cmds {
		fmt.Printf("  %s%-16s%s %s\n", ansiDim, c[0], ansiReset, c[1])
	}
	fmt.Println()
}

func (l *Loop) printStatus() {
	const sep = "──────────────────────────────────────────────────"
	fmt.Printf("\n%s%s%s\n", ansiDim, sep, ansiReset)
	fmt.Printf("  %s◆%s  model     %s\n", ansiTeal, ansiReset, l.cfg.Ollama.Model)
	host := strings.TrimPrefix(l.cfg.Ollama.Host, "http://")
	fmt.Printf("  %s◆%s  host      %s\n", ansiTeal, ansiReset, host)
	tokens := session.EstimateTokens(l.session.Snapshot())
	fmt.Printf("  %s◆%s  tokens    %d / %d\n", ansiTeal, ansiReset, tokens, l.maxCtx)
	fmt.Printf("  %s◆%s  history   %d messages\n", ansiTeal, ansiReset, len(l.session.Messages))
	fmt.Printf("  %s◆%s  timeout   %ds / step\n", ansiTeal, ansiReset, l.cfg.Agent.StepTimeoutSeconds)
	fmt.Printf("  %s◆%s  max steps %d / goal\n", ansiTeal, ansiReset, l.cfg.Agent.MaxGoalSteps)
	if l.savePath != "" {
		fmt.Printf("  %s◆%s  session   %s\n", ansiTeal, ansiReset, l.savePath)
	}
	fmt.Printf("%s%s%s\n\n", ansiDim, sep, ansiReset)
}

func (l *Loop) unloadModel() {
	u, ok := l.client.(llm.Unloader)
	if !ok {
		l.printf("%s[unload not supported by this backend]%s\n", ansiDim, ansiReset)
		return
	}
	l.printf("%s[unloading %s...]%s\n", ansiDim, l.cfg.Ollama.Model, ansiReset)
	if err := u.Unload(l.cfg.Ollama.Model); err != nil {
		fmt.Printf("[error] %v\n", err)
		return
	}
	l.printf("%s[model unloaded — RAM freed]%s\n", ansiGreen, ansiReset)
}

// loadFileIntoContext reads a file and injects it into the session as a user/assistant pair.
func (l *Loop) loadFileIntoContext(path string) {
	content, err := tools.ReadFile(filepath.Clean(path))
	if err != nil {
		fmt.Printf("[error] %v\n", err)
		return
	}
	l.session.Add("user", fmt.Sprintf("File context: %s\n\n%s", filepath.Base(path), content))
	l.session.Add("assistant", fmt.Sprintf("I have %s in context.", filepath.Base(path)))
	tokens := session.EstimateTokens(l.session.Snapshot())
	l.printf("%s[loaded %s — %d/%d tok]%s\n", ansiDim, path, tokens, l.maxCtx, ansiReset)
}

// pingOllama checks Ollama connectivity. In display mode it prints a status line.
func (l *Loop) pingOllama(display bool) error {
	host := strings.TrimPrefix(l.cfg.Ollama.Host, "http://")
	if err := llm.Ping(l.cfg.Ollama.Host); err != nil {
		msg := fmt.Sprintf("ollama not reachable at %s — is it running?", host)
		if display {
			fmt.Printf("  \033[91m✗\033[0m  %s\n\n", msg)
			return err
		}
		return fmt.Errorf("%s", msg)
	}
	if !display {
		return nil
	}
	suffix := host
	if ml, ok := l.client.(llm.ModelLister); ok {
		if models, err := ml.ListModels(); err == nil {
			found := false
			for _, m := range models {
				if m == l.cfg.Ollama.Model {
					found = true
					break
				}
			}
			suffix = fmt.Sprintf("%s (%d models available)", host, len(models))
			if !found {
				suffix += fmt.Sprintf(" \033[33m— model %q not found, run: ollama pull %s\033[0m",
					l.cfg.Ollama.Model, l.cfg.Ollama.Model)
			}
		}
	}
	fmt.Printf("  %s◆%s  ollama   %s✓%s %s\n\n", ansiTeal, ansiReset, ansiGreen, ansiReset, suffix)
	return nil
}

func (l *Loop) handle(ctx context.Context, input string) error {
	l.session.Add("user", input)
	assistant, err := l.chatOnce(ctx)
	if err != nil {
		return err
	}
	if len(assistant.ToolCalls) == 0 && l.registry.Enabled() {
		if tc := parseFallbackToolCall(assistant.Content); len(tc) > 0 {
			l.printf("[fallback tool parse]\n")
			assistant.ToolCalls = tc
			if l.cfg.Tools.UseNativeTools {
				assistant.Content = ""
			}
		}
	}
	if len(assistant.ToolCalls) == 0 {
		l.session.AddMessage(assistant)
		// In quiet mode the content wasn't streamed; print it now.
		if l.quiet {
			fmt.Println(assistant.Content)
		}
		return nil
	}

	l.printf("\n[tool phase]\n")

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
					l.session.AddMessage(session.Message{Role: "tool", Name: tc.Function.Name, Content: "tool error: user denied command"})
				} else {
					toolResults.WriteString("tool error: user denied command\n")
				}
				continue
			}
		}
		if tc.Function.Name == "write_file" && l.registry.ConfirmWriteFile() {
			if path, ok := tc.Function.Arguments["path"].(string); ok {
				if _, err := os.Stat(filepath.Clean(path)); err == nil {
					if !confirm(fmt.Sprintf("Overwrite existing file %q? [y/N]: ", path)) {
						if l.cfg.Tools.UseNativeTools {
							l.session.AddMessage(session.Message{Role: "tool", Name: tc.Function.Name, Content: "tool error: user denied overwrite"})
						} else {
							toolResults.WriteString("tool error: user denied overwrite\n")
						}
						continue
					}
				}
			}
		}
		result, err := l.registry.Execute(tc.Function.Name, string(argsJSON))
		if err != nil {
			result = "tool error: " + err.Error()
		}
		l.printf("- %s\n", tc.Function.Name)
		if l.cfg.Tools.UseNativeTools {
			l.session.AddMessage(session.Message{Role: "tool", Name: tc.Function.Name, Content: result})
		} else {
			toolResults.WriteString(fmt.Sprintf("Tool %s executed. Result:\n%s\n", tc.Function.Name, result))
		}
	}

	if !l.cfg.Tools.UseNativeTools && toolResults.Len() > 0 {
		l.session.AddMessage(session.Message{Role: "user", Content: toolResults.String()})
	}

	final, err := l.chatOnce(ctx)
	if err != nil {
		return err
	}
	l.session.AddMessage(final)
	if l.quiet {
		fmt.Println(final.Content)
	}
	return nil
}

// chatOnce sends the current session, retrying once on context overflow.
func (l *Loop) chatOnce(ctx context.Context) (session.Message, error) {
	msg, err := l.chatOnceWith(ctx, l.session.Snapshot())
	if err != nil && isContextOverflow(err) {
		l.printf("\n%s[context overflow — trimming 2 messages and retrying]%s\n", ansiDim, ansiReset)
		l.session.DropOldest(2)
		msg, err = l.chatOnceWith(ctx, l.session.Snapshot())
	}
	return msg, err
}

// chatOnceWith streams a response for the given message list, respecting ctx.
func (l *Loop) chatOnceWith(ctx context.Context, msgs []session.Message) (session.Message, error) {
	if !l.quiet {
		fmt.Print("\nassistant> ")
	}
	var content strings.Builder
	content.Grow(512)
	var assistant session.Message
	var toolCalls []session.ToolCall
	var reqTools []llm.ToolSpec
	if l.registry.Enabled() && l.cfg.Tools.UseNativeTools {
		reqTools = l.registry.Specs()
	}

	var printedCount int
	var isToolCall bool

	err := l.client.ChatStream(ctx, llm.ChatRequest{
		Model:     l.cfg.Ollama.Model,
		Messages:  msgs,
		Stream:    l.cfg.Ollama.Stream,
		Tools:     reqTools,
		Options:   l.cfg.Ollama.Options,
		KeepAlive: l.cfg.Ollama.KeepAlive,
	}, func(chunk llm.ChatChunk) error {
		if chunk.Message.Content != "" {
			content.WriteString(chunk.Message.Content)
			str := content.String()
			if !isToolCall && !l.quiet {
				if strings.HasPrefix(str, "```json") || strings.HasPrefix(str, "{") {
					isToolCall = true
				} else if strings.HasPrefix("```json", str) {
					// wait for more tokens — accumulating a possible tool-call fence
				} else {
					fmt.Print(str[printedCount:])
					printedCount = len(str)
				}
			} else if !l.quiet && isToolCall {
				// already suppressing
			}
			if l.quiet {
				isToolCall = strings.HasPrefix(strings.TrimSpace(str), "{") ||
					strings.HasPrefix(strings.TrimSpace(str), "```json")
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
	if !l.quiet {
		fmt.Println()
	}
	return assistant, err
}

func parseFallbackToolCall(content string) []session.ToolCall {
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
	case "read_file", "write_file", "append_file", "list_dir", "run_command":
		var argsMap map[string]interface{}
		_ = json.Unmarshal(req.Arguments, &argsMap)
		return []session.ToolCall{{Function: session.ToolFunction{Name: req.Name, Arguments: argsMap}}}
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

// printf is a quiet-aware print: suppressed when l.quiet is true.
func (l *Loop) printf(format string, args ...interface{}) {
	if !l.quiet {
		fmt.Printf(format, args...)
	}
}

func isContextOverflow(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "context") &&
		(strings.Contains(s, "exceed") || strings.Contains(s, "too long") || strings.Contains(s, "overflow"))
}

func numCtx(opts map[string]interface{}) int {
	if opts == nil {
		return 0
	}
	v, ok := opts["num_ctx"]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}
