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
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"mini-agent/internal/config"
	"mini-agent/internal/llm"
	"mini-agent/internal/runlog"
	"mini-agent/internal/session"
	"mini-agent/internal/tools"
)

type Loop struct {
	cfg             *config.Config
	client          llm.Client            // current active client (swapped during goal escalation)
	defaultClient   llm.Client            // original client — restored after each goal
	fallbackClient  llm.Client            // nil when no fallback is configured
	activeProvider  config.ProviderConfig // mirrors which client is active
	defaultProvider config.ProviderConfig
	session         *session.Session
	registry        *tools.Registry
	maxCtx          int
	savePath        string         // empty = no session persistence
	quiet           bool           // suppress decoration; only emit clean output
	logger          *runlog.Logger // nil = no run log
	goal             *GoalState // non-nil while a /goal is active or paused
	contextFile      string     // set when CONTEXT.md is loaded; printed in startup banner
	lastPromptTokens int        // previous prompt's token count; drop signals compaction
}

type fallbackToolRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func buildClient(p config.ProviderConfig) llm.Client {
	switch p.Type {
	case "openai_compat":
		return llm.NewOpenAI(p.BaseURL, os.Getenv(p.APIKeyEnv), p.Model)
	default: // "ollama"
		return llm.NewOllama(p.Host)
	}
}

func New(cfg *config.Config) *Loop {
	var defProvider config.ProviderConfig
	if len(cfg.Providers) == 0 {
		// Backwards-compat: synthesize a ProviderConfig from the ollama block.
		defProvider = config.ProviderConfig{
			Type:      "ollama",
			Host:      cfg.Ollama.Host,
			Model:     cfg.Ollama.Model,
			Stream:    cfg.Ollama.Stream,
			KeepAlive: cfg.Ollama.KeepAlive,
			Options:   cfg.Ollama.Options,
		}
	} else {
		defProvider = cfg.Providers[cfg.DefaultProvider]
	}

	defClient := buildClient(defProvider)
	var fbClient llm.Client
	if cfg.FallbackProvider != "" {
		fbProvider := cfg.Providers[cfg.FallbackProvider]
		fbClient = buildClient(fbProvider)
	}

	ctx := numCtx(defProvider.Options)
	if ctx == 0 && defProvider.Type == "openai_compat" {
		ctx = 8192 // sensible default for cloud providers that don't use num_ctx
	}
	return &Loop{
		cfg:             cfg,
		client:          defClient,
		defaultClient:   defClient,
		fallbackClient:  fbClient,
		activeProvider:  defProvider,
		defaultProvider: defProvider,
		session:         session.New(cfg.Agent.SystemPrompt, cfg.Agent.MaxHistory, ctx),
		registry:        tools.New(cfg.Tools),
		maxCtx:          ctx,
	}
}

func (l *Loop) SetSavePath(p string)      { l.savePath = p }
func (l *Loop) SetQuiet(q bool)           { l.quiet = q }
func (l *Loop) SetLogger(lg *runlog.Logger) { l.logger = lg }

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
		if l.contextFile != "" {
			fmt.Printf("  %s◆%s  context  %s\n\n", ansiTeal, ansiReset, l.contextFile)
		}
		l.showGoalBanner()
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
			continue
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
		case input == "/history" || strings.HasPrefix(input, "/history "):
			n := 6
			if rest := strings.TrimSpace(strings.TrimPrefix(input, "/history")); rest != "" {
				if v, err := strconv.Atoi(rest); err == nil && v > 0 {
					n = v
				}
			}
			l.printHistory(n)
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
		case input == "/models":
			l.printModels()
			continue
		case input == "/model":
			fmt.Printf("  current model: %s\n", l.activeProvider.Model)
			continue
		case strings.HasPrefix(input, "/model "):
			name := strings.TrimSpace(strings.TrimPrefix(input, "/model "))
			l.activeProvider.Model = name
			l.defaultProvider.Model = name
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
		if goal, ok := strings.CutPrefix(input, "/run "); ok {
			err = l.runGoal(ctx, strings.TrimSpace(goal))
		} else if input == "/goal" || strings.HasPrefix(input, "/goal ") {
			// /goal handles its own Ctrl+C internally (pauses rather than interrupts).
			err = l.handleGoalCommand(ctx, input)
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
	goalPart := ""
	if l.goal != nil {
		switch l.goal.Status {
		case GoalActive:
			goalPart = fmt.Sprintf("%s[◎ goal: %d steps]%s ", ansiCyan, l.goal.Steps, ansiReset)
		case GoalPaused:
			goalPart = fmt.Sprintf("%s[⏸ goal paused]%s ", ansiYellow, ansiReset)
		}
	}
	if l.maxCtx > 0 {
		tokens := session.EstimateTokens(l.session.Snapshot())
		compactedNotice := ""
		if l.lastPromptTokens > 0 && tokens < l.lastPromptTokens {
			compactedNotice = fmt.Sprintf("%s(compacted) %s", ansiDim, ansiReset)
		}
		l.lastPromptTokens = tokens
		pct := float64(tokens) / float64(l.maxCtx)
		color := ansiDim
		switch {
		case pct >= 0.90:
			color = ansiRed
		case pct >= 0.75:
			color = ansiYellow
		}
		fmt.Printf("\n%s%s%s[%d/%d tok]%s > ", compactedNotice, goalPart, color, tokens, l.maxCtx, ansiReset)
	} else {
		fmt.Printf("\n%s> ", goalPart)
	}
}

func (l *Loop) printHelp() {
	fmt.Printf("\n%sCommands:%s\n", ansiTeal, ansiReset)
	cmds := [][2]string{
		{"/goal <objective>", "set a persistent goal and start working toward it"},
		{"/goal", "show current goal status"},
		{"/goal resume", "continue a paused goal"},
		{"/goal clear", "stop and discard the current goal"},
		{"/run <goal>", "run a quick one-shot goal (max 10 steps)"},
		{"/models", "list all models available in Ollama"},
		{"/load <file>", "inject a file into conversation context"},
		{"/history [N]", "show last N messages from session (default 6)"},
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
	providerName := l.cfg.DefaultProvider
	if providerName == "" {
		providerName = "ollama"
	}
	fmt.Printf("  %s◆%s  provider  %s (%s)\n", ansiTeal, ansiReset, providerName, l.activeProvider.Type)
	fmt.Printf("  %s◆%s  model     %s\n", ansiTeal, ansiReset, l.activeProvider.Model)
	var endpoint string
	if l.activeProvider.Type == "ollama" {
		endpoint = strings.TrimPrefix(l.activeProvider.Host, "http://")
		endpoint = strings.TrimPrefix(endpoint, "https://")
	} else {
		endpoint = l.activeProvider.BaseURL
	}
	fmt.Printf("  %s◆%s  endpoint  %s\n", ansiTeal, ansiReset, endpoint)
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
	l.printf("%s[unloading %s...]%s\n", ansiDim, l.activeProvider.Model, ansiReset)
	if err := u.Unload(l.activeProvider.Model); err != nil {
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

// pingOllama checks Ollama connectivity. Skipped for non-ollama providers.
// In display mode it prints a status line.
func (l *Loop) pingOllama(display bool) error {
	if l.activeProvider.Type != "ollama" && l.activeProvider.Type != "" {
		return nil
	}
	host := strings.TrimPrefix(l.activeProvider.Host, "http://")
	if err := llm.Ping(l.activeProvider.Host); err != nil {
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
			found := slices.Contains(models, l.activeProvider.Model)
			suffix = fmt.Sprintf("%s (%d models available)", host, len(models))
			if !found {
				suffix += fmt.Sprintf(" \033[33m— model %q not found, run: ollama pull %s\033[0m",
					l.activeProvider.Model, l.activeProvider.Model)
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
		if strings.TrimSpace(assistant.Content) == "" {
			l.printf("%s(empty response — model produced no output; try a simpler prompt or /model <larger-model>)%s\n", ansiDim, ansiReset)
		}
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
		if tc.Function.Name == "git" && l.registry.ConfirmGitWrite() {
			if needsConfirm, prompt := l.registry.GitWriteCheck(string(argsJSON)); needsConfirm {
				if !confirm(fmt.Sprintf("Allow %q? [y/N]: ", prompt)) {
					if l.cfg.Tools.UseNativeTools {
						l.session.AddMessage(session.Message{Role: "tool", Name: tc.Function.Name, Content: "tool error: user denied git command"})
					} else {
						toolResults.WriteString("tool error: user denied git command\n")
					}
					continue
				}
			}
		}
		toolStart := time.Now()
		result, err := l.registry.Execute(tc.Function.Name, string(argsJSON))
		if err != nil {
			result = "tool error: " + err.Error()
		}
		l.logTool(tc, result, err, time.Since(toolStart))
		if summary := toolSummary(tc); summary != "" {
			l.printf("  %s[%s]%s %s → %s\n", ansiTeal, tc.Function.Name, ansiReset, summary, truncStr(strings.TrimSpace(result), 80))
		} else {
			l.printf("  %s[%s]%s %s\n", ansiTeal, tc.Function.Name, ansiReset, truncStr(strings.TrimSpace(result), 80))
		}
		if l.cfg.Tools.UseNativeTools {
			l.session.AddMessage(session.Message{Role: "tool", Name: tc.Function.Name, Content: result})
		} else {
			fmt.Fprintf(&toolResults, "Tool %s executed. Result:\n%s\n", tc.Function.Name, result)
		}
	}

	if !l.cfg.Tools.UseNativeTools && toolResults.Len() > 0 {
		l.session.AddMessage(session.Message{Role: "user", Content: toolResults.String()})
	}

	final, err := l.chatOnce(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(final.Content) == "" {
		l.printf("%s(no follow-up from model after tool execution)%s\n", ansiDim, ansiReset)
	}
	l.session.AddMessage(final)
	if l.quiet {
		fmt.Println(final.Content)
	}
	return nil
}

// chatOnce sends the current session, retrying on context overflow and rate limits.
func (l *Loop) chatOnce(ctx context.Context) (session.Message, error) {
	msg, err := l.chatOnceWith(ctx, l.session.Snapshot())
	if err != nil && isContextOverflow(err) {
		l.printf("\n%s[context overflow — trimming 2 messages and retrying]%s\n", ansiDim, ansiReset)
		l.session.DropOldest(2)
		msg, err = l.chatOnceWith(ctx, l.session.Snapshot())
	}
	// Retry on rate limit (429): parse suggested delay from error message and wait.
	for retries := 0; retries < 3 && err != nil && isRateLimit(err); retries++ {
		delay := parseRetryAfter(err.Error())
		l.printf("%s[rate limited — retrying in %.0fs]%s\n", ansiDim, delay.Seconds(), ansiReset)
		select {
		case <-ctx.Done():
			return msg, ctx.Err()
		case <-time.After(delay):
		}
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
		Model:     l.activeProvider.Model,
		Messages:  msgs,
		Stream:    l.activeProvider.Stream,
		Tools:     reqTools,
		Options:   l.activeProvider.Options,
		KeepAlive: l.activeProvider.KeepAlive,
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
	// Strip a leading code fence (```json ... ```) if present.
	if strings.HasPrefix(clean, "```") {
		lines := strings.Split(clean, "\n")
		if len(lines) >= 3 && strings.HasPrefix(lines[0], "```") && strings.TrimSpace(lines[len(lines)-1]) == "```" {
			clean = strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
		}
	}
	// When the model writes prose before a JSON block (e.g. "I will run ...\n```json\n{...}"),
	// find the first embedded code fence and extract its contents.
	if !strings.HasPrefix(clean, "{") {
		if i := strings.Index(clean, "```"); i >= 0 {
			rest := clean[i:]
			lines := strings.Split(rest, "\n")
			if len(lines) >= 3 {
				end := -1
				for j := len(lines) - 1; j > 0; j-- {
					if strings.TrimSpace(lines[j]) == "```" {
						end = j
						break
					}
				}
				if end > 1 {
					clean = strings.TrimSpace(strings.Join(lines[1:end], "\n"))
				}
			}
		}
	}
	if !strings.HasPrefix(clean, "{") {
		return nil
	}
	end := strings.LastIndex(clean, "}")
	if end < 0 {
		return nil
	}
	candidate := strings.TrimSpace(clean[:end+1])
	var req fallbackToolRequest
	if err := json.Unmarshal([]byte(candidate), &req); err != nil {
		return nil
	}
	if req.Name == "" || len(req.Arguments) == 0 {
		return nil
	}
	switch req.Name {
	case "read_file", "write_file", "edit_file", "append_file", "list_dir", "run_command", "web_fetch", "search_files", "git":
		var argsMap map[string]any
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
func (l *Loop) printf(format string, args ...any) {
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

func isRateLimit(err error) bool {
	return err != nil && strings.Contains(err.Error(), "http 429")
}

// parseRetryAfter extracts "Please try again in Xs" from a 429 error body.
// Falls back to 5 seconds if the delay cannot be parsed. Capped at 30 seconds.
func parseRetryAfter(msg string) time.Duration {
	const fallback = 5 * time.Second
	// Look for "try again in <number>s" or "try again in <number>.<frac>s"
	idx := strings.Index(msg, "try again in ")
	if idx < 0 {
		return fallback
	}
	rest := msg[idx+len("try again in "):]
	var secs float64
	if _, err := fmt.Sscanf(rest, "%f", &secs); err != nil || secs <= 0 {
		return fallback
	}
	d := time.Duration(secs*1000) * time.Millisecond
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

// toolSummary returns a human-readable description of a tool call's key arguments.
func toolSummary(tc session.ToolCall) string {
	args := tc.Function.Arguments
	switch tc.Function.Name {
	case "read_file", "list_dir":
		if p, ok := args["path"].(string); ok {
			return p
		}
	case "write_file", "append_file":
		if p, ok := args["path"].(string); ok {
			if c, ok := args["content"].(string); ok {
				return fmt.Sprintf("%s (%d bytes)", p, len(c))
			}
			return p
		}
	case "edit_file":
		if p, ok := args["path"].(string); ok {
			return p
		}
	case "run_command":
		if cmd, ok := args["command"].(string); ok {
			if rawArgs, ok := args["args"].([]any); ok && len(rawArgs) > 0 {
				parts := []string{cmd}
				for _, a := range rawArgs {
					parts = append(parts, fmt.Sprint(a))
				}
				return strings.Join(parts, " ")
			}
			return cmd
		}
	case "web_fetch":
		if u, ok := args["url"].(string); ok {
			return u
		}
	case "search_files":
		if p, ok := args["pattern"].(string); ok {
			return fmt.Sprintf("%q", p)
		}
	case "git":
		if sub, ok := args["subcommand"].(string); ok {
			parts := []string{sub}
			if rawArgs, ok := args["args"].([]any); ok {
				for _, a := range rawArgs {
					parts = append(parts, fmt.Sprint(a))
				}
			}
			return strings.Join(parts, " ")
		}
	}
	return ""
}

func (l *Loop) printHistory(n int) {
	msgs := l.session.Snapshot()
	var chat []session.Message
	for _, m := range msgs {
		if m.Role != "system" {
			chat = append(chat, m)
		}
	}
	if len(chat) == 0 {
		fmt.Printf("%s[no history]%s\n", ansiDim, ansiReset)
		return
	}
	if n > len(chat) {
		n = len(chat)
	}
	recent := chat[len(chat)-n:]
	fmt.Printf("\n%s[last %d messages]%s\n", ansiDim, len(recent), ansiReset)
	for _, m := range recent {
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		if len(content) > 200 {
			content = content[:200] + "…"
		}
		color := ansiDim
		switch m.Role {
		case "assistant":
			color = ansiTeal
		case "user":
			color = ansiReset
		}
		fmt.Printf("  %s%s:%s %s\n", color, m.Role, ansiReset, content)
	}
	fmt.Println()
}

// HandleMsg processes a single user message and returns the assistant response text.
// It is used by non-interactive frontends such as the Telegram bot.
// Quiet mode is engaged internally so streaming is suppressed; the response
// is read from the session after handle() returns.
func (l *Loop) HandleMsg(ctx context.Context, input string) (string, error) {
	before := len(l.session.Messages)
	prevQuiet := l.quiet
	l.quiet = true
	err := l.handle(ctx, input)
	l.quiet = prevQuiet
	if err != nil {
		return "", err
	}
	msgs := l.session.Snapshot()
	for i := len(msgs) - 1; i >= before; i-- {
		if msgs[i].Role == "assistant" && msgs[i].Content != "" {
			return msgs[i].Content, nil
		}
	}
	return "", nil
}

// InjectContextFile reads CONTEXT.md (or .mini-agent/CONTEXT.md) from the working
// directory and injects it into the session as a user/assistant pair so the model
// always has project context without the user needing to /load it manually.
func (l *Loop) InjectContextFile() {
	for _, p := range []string{"CONTEXT.md", ".mini-agent/CONTEXT.md"} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		text := strings.TrimSpace(string(b))
		if text == "" {
			continue
		}
		if len(text) > 4096 {
			text = text[:4096] + "\n[truncated: CONTEXT.md exceeds 4 KB]"
		}
		l.session.Add("user", "Project context (CONTEXT.md):\n\n"+text)
		l.session.Add("assistant", "Understood, I have the project context.")
		tokens := session.EstimateTokens([]session.Message{{Content: text}})
		l.contextFile = fmt.Sprintf("%s (~%d tokens)", p, tokens)
		return
	}
}

// printModels lists all models available in the configured Ollama instance.
func (l *Loop) printModels() {
	ml, ok := l.client.(llm.ModelLister)
	if !ok {
		l.printf("%s[model listing not supported by this backend]%s\n", ansiDim, ansiReset)
		return
	}
	models, err := ml.ListModels()
	if err != nil {
		fmt.Printf("[error] %v\n", err)
		return
	}
	if len(models) == 0 {
		l.printf("%s[no models found — run: ollama pull <model>]%s\n", ansiDim, ansiReset)
		return
	}
	fmt.Printf("\n%s[%d models available]%s\n", ansiDim, len(models), ansiReset)
	for _, m := range models {
		if m == l.activeProvider.Model {
			fmt.Printf("  %s●%s %s %s(active)%s\n", ansiTeal, ansiReset, m, ansiDim, ansiReset)
		} else {
			fmt.Printf("  %s·%s %s\n", ansiDim, ansiReset, m)
		}
	}
	fmt.Println()
}

// showGoalBanner checks for a persisted goal and prints a notice if one is paused.
func (l *Loop) showGoalBanner() {
	statePath, err := GoalStatePath()
	if err != nil {
		return
	}
	g, err := loadGoalState(statePath)
	if err != nil || g == nil {
		return
	}
	l.goal = g
	switch g.Status {
	case GoalPaused:
		fmt.Printf("  %s[⏸ goal paused: %q — %d steps — /goal resume to continue]%s\n\n",
			ansiYellow, g.Objective, g.Steps, ansiReset)
	case GoalActive:
		// Marked active but not running — treat as paused (process was killed mid-run).
		g.Status = GoalPaused
		saveGoalState(statePath, g)
		fmt.Printf("  %s[⏸ goal paused: %q — %d steps — /goal resume to continue]%s\n\n",
			ansiYellow, g.Objective, g.Steps, ansiReset)
	}
}

// logTool writes one entry to the run log if a logger is configured.
func (l *Loop) logTool(tc session.ToolCall, result string, execErr error, dur time.Duration) {
	if l.logger == nil {
		return
	}
	l.logger.Log(runlog.Entry{
		TS:          time.Now().UTC(),
		Tool:        tc.Function.Name,
		Args:        toolSummary(tc),
		ResultBytes: len(result),
		OK:          execErr == nil,
		DurationMS:  dur.Milliseconds(),
	})
}

// recordGoalResult appends a summary of a completed or stopped goal to the session
// so it persists across restarts and is visible in /history.
func (l *Loop) recordGoalResult(goal, status, detail string) {
	var content string
	if detail != "" {
		content = fmt.Sprintf("[goal %s: %q — %s]", status, goal, detail)
	} else {
		content = fmt.Sprintf("[goal %s: %q]", status, goal)
	}
	l.session.Add("assistant", content)
}

func numCtx(opts map[string]any) int {
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
