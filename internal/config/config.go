package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Ollama OllamaConfig `yaml:"ollama"`
	Agent  AgentConfig  `yaml:"agent"`
	Tools  ToolsConfig  `yaml:"tools"`
}

type OllamaConfig struct {
	Host      string                 `yaml:"host"`
	Model     string                 `yaml:"model"`
	KeepAlive string                 `yaml:"keep_alive"`
	Stream    bool                   `yaml:"stream"`
	Options   map[string]interface{} `yaml:"options"`
}

type AgentConfig struct {
	MaxHistory         int    `yaml:"max_history"`
	SystemPrompt       string `yaml:"system_prompt"`
	StepTimeoutSeconds int    `yaml:"step_timeout_seconds"`
	MaxGoalSteps       int    `yaml:"max_goal_steps"`
}

type ToolsConfig struct {
	Enabled          bool     `yaml:"enabled"`
	UseNativeTools   bool     `yaml:"use_native_tools"`
	EnableReadFile   bool     `yaml:"enable_read_file"`
	EnableWriteFile  bool     `yaml:"enable_write_file"`
	EnableAppendFile bool     `yaml:"enable_append_file"`
	EnableListDir    bool     `yaml:"enable_list_dir"`
	EnableRunCmd      bool     `yaml:"enable_run_command"`
	ConfirmRunCmd     bool     `yaml:"confirm_run_command"`
	ConfirmWriteFile  bool     `yaml:"confirm_write_file"`
	AllowedCommands  []string `yaml:"allowed_commands"`
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
	if !cfg.Tools.EnableReadFile && !cfg.Tools.EnableWriteFile &&
		!cfg.Tools.EnableAppendFile && !cfg.Tools.EnableListDir && !cfg.Tools.EnableRunCmd {
		cfg.Tools.Enabled = false
	}
	return &cfg, nil
}
