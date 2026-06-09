package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Ollama   OllamaConfig   `yaml:"ollama"`
	Agent    AgentConfig    `yaml:"agent"`
	Tools    ToolsConfig    `yaml:"tools"`
	Telegram TelegramConfig `yaml:"telegram"`
	Schedule []ScheduleEntry `yaml:"schedule"`

	// Multi-provider fields — optional. When absent, the Ollama block is used.
	Providers             map[string]ProviderConfig `yaml:"providers"`
	DefaultProvider       string                    `yaml:"default_provider"`
	FallbackProvider      string                    `yaml:"fallback_provider"`
	FallbackAfterFailures int                       `yaml:"fallback_after_failures"`
}

// ScheduleEntry defines a single cron-triggered goal for --daemon mode.
type ScheduleEntry struct {
	Cron string `yaml:"cron"` // 5-field POSIX cron expression
	Goal string `yaml:"goal"` // goal text passed to the agent
}

type OllamaConfig struct {
	Host      string         `yaml:"host"`
	Model     string         `yaml:"model"`
	KeepAlive string         `yaml:"keep_alive"`
	Stream    bool           `yaml:"stream"`
	Options   map[string]any `yaml:"options"`
}

type AgentConfig struct {
	MaxHistory           int    `yaml:"max_history"`
	SystemPrompt         string `yaml:"system_prompt"`
	SystemPromptWeak     string `yaml:"system_prompt_weak"`
	SystemPromptFrontier string `yaml:"system_prompt_frontier"`
	StepTimeoutSeconds   int    `yaml:"step_timeout_seconds"`
	MaxGoalSteps         int    `yaml:"max_goal_steps"`       // max steps for /run (quick mode)
	GoalMaxSteps         int    `yaml:"goal_max_steps"`       // max steps for /goal (persistent mode); 0 = unlimited
	SummarizeOnCompact   bool   `yaml:"summarize_on_compact"`
	EnableFallbackParser bool   `yaml:"enable_fallback_parser"`
}

type ToolsConfig struct {
	Enabled              bool     `yaml:"enabled"`
	UseNativeTools       bool     `yaml:"use_native_tools"`
	EnableReadFile       bool     `yaml:"enable_read_file"`
	EnableWriteFile      bool     `yaml:"enable_write_file"`
	EnableEditFile       bool     `yaml:"enable_edit_file"`
	EnableAppendFile     bool     `yaml:"enable_append_file"`
	EnableListDir        bool     `yaml:"enable_list_dir"`
	EnableRunCmd         bool     `yaml:"enable_run_command"`
	EnableWebFetch       bool     `yaml:"enable_web_fetch"`
	EnableSearchFiles    bool     `yaml:"enable_search_files"`
	EnableGit            bool     `yaml:"enable_git"`
	EnableJsonQuery      bool     `yaml:"enable_json_query"`
	EnableNotify         bool     `yaml:"enable_notify"`
	EnableMemory         bool     `yaml:"enable_memory"`
	ConfirmRunCmd        bool     `yaml:"confirm_run_command"`
	ConfirmGitWrite      bool     `yaml:"confirm_git_write"`
	ConfirmWriteFile     bool     `yaml:"confirm_write_file"`
	AllowedCommands      []string `yaml:"allowed_commands"`
	WebFetchTimeoutSecs  int      `yaml:"web_fetch_timeout_seconds"`
}

// TelegramConfig holds settings for the optional Telegram bot mode.
// SECURITY: Never put your bot token in config.yaml — use the
// TELEGRAM_BOT_TOKEN environment variable instead.
type TelegramConfig struct {
	Enabled        bool    `yaml:"enabled"`
	BotToken       string  `yaml:"bot_token"`        // prefer TELEGRAM_BOT_TOKEN env var
	AllowedChatIDs []int64 `yaml:"allowed_chat_ids"` // required; empty = deny all
}

// ProviderConfig describes a single LLM backend.
// type "ollama"        — local Ollama instance (host + ollama streaming)
// type "openai_compat" — any OpenAI-compatible /v1/chat/completions endpoint
// type "anthropic"     — Anthropic Claude API (OpenAI-compatible; routed to openai_compat handler)
type ProviderConfig struct {
	Type      string         `yaml:"type"`        // "ollama" | "openai_compat" | "anthropic"
	Host      string         `yaml:"host"`        // ollama: base URL
	BaseURL   string         `yaml:"base_url"`    // openai_compat: base URL
	APIKeyEnv string         `yaml:"api_key_env"` // openai_compat: env var holding the key
	Model     string         `yaml:"model"`
	Stream    bool           `yaml:"stream"`
	KeepAlive string         `yaml:"keep_alive"`
	Options   map[string]any `yaml:"options"`
}

// FindConfig returns the config path to use, checking in priority order:
// 1. An explicit path (from --config flag, if non-empty)
// 2. ./config.yaml (repo / dev usage — only if it already exists)
// 3. ~/.mini-agent/config.yaml (always returned as the user-install fallback;
//    Load will auto-create it with defaults on first run if missing)
func FindConfig(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if _, err := os.Stat("config.yaml"); err == nil {
		return "config.yaml"
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".mini-agent", "config.yaml")
	}
	return "config.yaml"
}

// defaultConfigYAML is written to ~/.mini-agent/config.yaml on first run.
const defaultConfigYAML = `# mini-agent configuration — auto-generated on first run
# Edit this file to change models, enable tools, or add providers.

ollama:
  host: "http://localhost:11434"
  model: "qwen2.5-coder:1.5b"
  keep_alive: "30m"
  stream: true
  options:
    num_ctx: 2048
    num_predict: 512
    num_thread: 4

agent:
  max_history: 8
  max_goal_steps: 10
  step_timeout_seconds: 300
  system_prompt: |
    You are a helpful local assistant. Be concise and direct.
    For file operations output ONLY a JSON object — no explanation, no markdown.
    {"name":"write_file","arguments":{"path":"hello.py","content":"..."}}
    For questions, reply in plain text.

  # Optional: simpler prompt for weak models (1.5B)
  # system_prompt_weak: |
  #   You are a local assistant. Keep responses short.
  #   Output ONLY: {"name":"write_file","arguments":{"path":"hello.py","content":"..."}}

  # Optional: advanced prompt for frontier models (Claude 3.5, GPT-4)
  # system_prompt_frontier: |
  #   You are an expert assistant with advanced reasoning.
  #   Use your full capabilities for complex problems.

  enable_fallback_parser: true

tools:
  enabled: true
  use_native_tools: false
  enable_read_file: true
  enable_write_file: true
  enable_edit_file: true
  enable_append_file: true
  enable_list_dir: true
  enable_run_command: true
  confirm_run_command: true
  confirm_write_file: false
  allowed_commands: ["ls", "cat", "pwd", "echo", "grep", "find", "git", "python3", "python", "go", "node"]
`

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if writeErr := writeDefaultConfig(path); writeErr == nil {
			fmt.Fprintf(os.Stderr, "  [created default config: %s]\n  [run --setup to configure a provider or change the model]\n\n", path)
			b, err = os.ReadFile(path)
		}
	}
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	if cfg.Ollama.Host == "" {
		cfg.Ollama.Host = "http://localhost:11434"
	}
	if cfg.Agent.MaxHistory <= 0 {
		cfg.Agent.MaxHistory = 8
	}
	if cfg.Agent.StepTimeoutSeconds <= 0 {
		cfg.Agent.StepTimeoutSeconds = 300
	}
	if cfg.Agent.MaxGoalSteps <= 0 {
		cfg.Agent.MaxGoalSteps = 10
	}
	// GoalMaxSteps: 0 means unlimited; negative resets to default 50.
	if cfg.Agent.GoalMaxSteps < 0 {
		cfg.Agent.GoalMaxSteps = 50
	}
	if !cfg.Tools.EnableReadFile && !cfg.Tools.EnableWriteFile &&
		!cfg.Tools.EnableEditFile && !cfg.Tools.EnableAppendFile &&
		!cfg.Tools.EnableListDir && !cfg.Tools.EnableRunCmd &&
		!cfg.Tools.EnableWebFetch && !cfg.Tools.EnableSearchFiles &&
		!cfg.Tools.EnableGit && !cfg.Tools.EnableJsonQuery {
		cfg.Tools.Enabled = false
	}
	if cfg.Tools.WebFetchTimeoutSecs <= 0 {
		cfg.Tools.WebFetchTimeoutSecs = 30
	}
	if len(cfg.Providers) > 0 && cfg.FallbackAfterFailures <= 0 {
		cfg.FallbackAfterFailures = 2
	}
	// Bot token must come from the TELEGRAM_BOT_TOKEN environment variable.
	// If a token was set directly in config.yaml, reject it loudly so the
	// user doesn't accidentally commit a credential to version control.
	if cfg.Telegram.BotToken != "" {
		return nil, fmt.Errorf("telegram.bot_token must not be set in config.yaml — " +
			"export TELEGRAM_BOT_TOKEN=<your-token> instead")
	}
	if envToken := os.Getenv("TELEGRAM_BOT_TOKEN"); envToken != "" {
		cfg.Telegram.BotToken = envToken
	}
	return &cfg, nil
}

// Validate checks that required fields are present and values are in range.
// It returns a combined error listing all problems found, not just the first.
func (c *Config) Validate() error {
	var errs []string

	if len(c.Providers) == 0 {
		// Legacy path: validate ollama block as before.
		if c.Ollama.Host == "" {
			errs = append(errs, "ollama.host is required")
		} else if !strings.HasPrefix(c.Ollama.Host, "http://") && !strings.HasPrefix(c.Ollama.Host, "https://") {
			errs = append(errs, "ollama.host must start with http:// or https://")
		}
		if c.Ollama.Model == "" {
			errs = append(errs, "ollama.model is required")
		}
		if nc := intFromOptions(c.Ollama.Options, "num_ctx"); nc != 0 && nc < 64 {
			errs = append(errs, "ollama.options.num_ctx must be >= 64")
		}
		if np := intFromOptions(c.Ollama.Options, "num_predict"); np != 0 && np < 1 {
			errs = append(errs, "ollama.options.num_predict must be >= 1")
		}
	} else {
		// Multi-provider path.
		if c.DefaultProvider == "" {
			errs = append(errs, "default_provider is required when providers: is set")
		} else if _, ok := c.Providers[c.DefaultProvider]; !ok {
			errs = append(errs, fmt.Sprintf("default_provider %q does not name a known provider", c.DefaultProvider))
		} else {
			def := c.Providers[c.DefaultProvider]
			if (def.Type == "openai_compat" || def.Type == "anthropic") && def.APIKeyEnv != "" && os.Getenv(def.APIKeyEnv) == "" {
				errs = append(errs, fmt.Sprintf("default provider %q requires env var %s to be set", c.DefaultProvider, def.APIKeyEnv))
			}
			if def.Type == "openai_compat" && def.BaseURL == "" {
				errs = append(errs, fmt.Sprintf("provider %q: base_url is required for type openai_compat", c.DefaultProvider))
			}
			if def.Model == "" {
				errs = append(errs, fmt.Sprintf("provider %q: model is required", c.DefaultProvider))
			}
		}
		if c.FallbackProvider != "" {
			if _, ok := c.Providers[c.FallbackProvider]; !ok {
				errs = append(errs, fmt.Sprintf("fallback_provider %q does not name a known provider", c.FallbackProvider))
			}
			if c.FallbackProvider == c.DefaultProvider {
				errs = append(errs, "fallback_provider must differ from default_provider")
			}
		}
	}

	if c.Agent.MaxHistory < 1 {
		errs = append(errs, "agent.max_history must be >= 1")
	}
	if c.Agent.MaxGoalSteps < 1 {
		errs = append(errs, "agent.max_goal_steps must be >= 1")
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

func writeDefaultConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultConfigYAML), 0o644)
}

func intFromOptions(opts map[string]any, key string) int {
	if opts == nil {
		return 0
	}
	switch n := opts[key].(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}
