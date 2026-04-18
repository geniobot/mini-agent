package config

import (
	"os"

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
	MaxHistory   int    `yaml:"max_history"`
	SystemPrompt string `yaml:"system_prompt"`
}

type ToolsConfig struct {
	Enabled         bool     `yaml:"enabled"`
	UseNativeTools  bool     `yaml:"use_native_tools"`
	EnableReadFile  bool     `yaml:"enable_read_file"`
	EnableWriteFile bool     `yaml:"enable_write_file"`
	EnableRunCmd    bool     `yaml:"enable_run_command"`
	ConfirmRunCmd   bool     `yaml:"confirm_run_command"`
	AllowedCommands []string `yaml:"allowed_commands"`
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
	if !cfg.Tools.EnableReadFile && !cfg.Tools.EnableWriteFile && !cfg.Tools.EnableRunCmd {
		cfg.Tools.Enabled = false
	}
	return &cfg, nil
}
