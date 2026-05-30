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
}

type OllamaConfig struct {
	Host      string         `yaml:"host"`
	Model     string         `yaml:"model"`
	KeepAlive string         `yaml:"keep_alive"`
	Stream    bool           `yaml:"stream"`
	Options   map[string]any `yaml:"options"`
}

type AgentConfig struct {
	MaxHistory         int    `yaml:"max_history"`
	SystemPrompt       string `yaml:"system_prompt"`
	StepTimeoutSeconds int    `yaml:"step_timeout_seconds"`
	MaxGoalSteps       int    `yaml:"max_goal_steps"`       // max steps for /run (quick mode)
	GoalMaxSteps       int    `yaml:"goal_max_steps"`       // max steps for /goal (persistent mode); 0 = unlimited
	SummarizeOnCompact bool   `yaml:"summarize_on_compact"`
}

type ToolsConfig struct {
	Enabled              bool     `yaml:"enabled"`
	UseNativeTools       bool     `yaml:"use_native_tools"`
	EnableReadFile       bool     `yaml:"enable_read_file"`
	EnableWriteFile      bool     `yaml:"enable_write_file"`
	EnableAppendFile     bool     `yaml:"enable_append_file"`
	EnableListDir        bool     `yaml:"enable_list_dir"`
	EnableRunCmd         bool     `yaml:"enable_run_command"`
	EnableWebFetch       bool     `yaml:"enable_web_fetch"`
	EnableSearchFiles    bool     `yaml:"enable_search_files"`
	ConfirmRunCmd        bool     `yaml:"confirm_run_command"`
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

// FindConfig returns the config path to use, checking in priority order:
// 1. An explicit path (from --config flag, if non-empty)
// 2. ~/.mini-agent/config.yaml
// 3. ./config.yaml (current directory fallback)
func FindConfig(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, ".mini-agent", "config.yaml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "config.yaml"
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
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
		!cfg.Tools.EnableAppendFile && !cfg.Tools.EnableListDir &&
		!cfg.Tools.EnableRunCmd && !cfg.Tools.EnableWebFetch &&
		!cfg.Tools.EnableSearchFiles {
		cfg.Tools.Enabled = false
	}
	if cfg.Tools.WebFetchTimeoutSecs <= 0 {
		cfg.Tools.WebFetchTimeoutSecs = 30
	}
	// Bot token from environment variable takes precedence over config file.
	// This keeps credentials out of version-controlled files.
	if envToken := os.Getenv("TELEGRAM_BOT_TOKEN"); envToken != "" {
		cfg.Telegram.BotToken = envToken
	}
	return &cfg, nil
}

// Validate checks that required fields are present and values are in range.
// It returns a combined error listing all problems found, not just the first.
func (c *Config) Validate() error {
	var errs []string

	if c.Ollama.Host == "" {
		errs = append(errs, "ollama.host is required")
	} else if !strings.HasPrefix(c.Ollama.Host, "http://") && !strings.HasPrefix(c.Ollama.Host, "https://") {
		errs = append(errs, "ollama.host must start with http:// or https://")
	}
	if c.Ollama.Model == "" {
		errs = append(errs, "ollama.model is required")
	}
	if c.Agent.MaxHistory < 1 {
		errs = append(errs, "agent.max_history must be >= 1")
	}
	if c.Agent.MaxGoalSteps < 1 {
		errs = append(errs, "agent.max_goal_steps must be >= 1")
	}
	if nc := intFromOptions(c.Ollama.Options, "num_ctx"); nc != 0 && nc < 64 {
		errs = append(errs, "ollama.options.num_ctx must be >= 64")
	}
	if np := intFromOptions(c.Ollama.Options, "num_predict"); np != 0 && np < 1 {
		errs = append(errs, "ollama.options.num_predict must be >= 1")
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
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
