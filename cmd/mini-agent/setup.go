package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type providerPreset struct {
	label      string
	provType   string
	baseURL    string
	apiKeyEnv  string
	defaultModel string
}

var presets = []providerPreset{
	{"Groq      (free, fast — llama-3.3-70b)", "openai_compat", "https://api.groq.com/openai/v1", "GROQ_API_KEY", "llama-3.3-70b-versatile"},
	{"OpenAI    (gpt-4o-mini, paid)", "openai_compat", "https://api.openai.com/v1", "OPENAI_API_KEY", "gpt-4o-mini"},
	{"OpenRouter (many models, paid)", "openai_compat", "https://openrouter.ai/api/v1", "OPENROUTER_API_KEY", "anthropic/claude-3-5-haiku"},
	{"Ollama    (local)", "ollama", "http://localhost:11434", "", "qwen2.5-coder:7b"},
	{"Custom    (any OpenAI-compatible endpoint)", "openai_compat", "", "", ""},
}

func runSetup(configPath string) {
	sc := bufio.NewScanner(os.Stdin)

	fmt.Println()
	fmt.Println("  ──────────────────────────────────────────────────")
	fmt.Println("  mini-agent setup")
	fmt.Println("  ──────────────────────────────────────────────────")
	fmt.Println()

	// Choose provider
	fmt.Println("  Choose provider:")
	for i, p := range presets {
		fmt.Printf("    %d) %s\n", i+1, p.label)
	}
	fmt.Println()
	fmt.Print("  > ")
	choice := 0
	sc.Scan()
	fmt.Sscanf(strings.TrimSpace(sc.Text()), "%d", &choice)
	if choice < 1 || choice > len(presets) {
		fmt.Fprintln(os.Stderr, "  invalid choice — aborting")
		os.Exit(1)
	}
	preset := presets[choice-1]

	// Custom endpoint fields
	if preset.baseURL == "" {
		fmt.Print("  Base URL (e.g. https://api.example.com/v1): ")
		sc.Scan()
		preset.baseURL = strings.TrimSpace(sc.Text())
		fmt.Print("  Env var name for API key (e.g. MY_API_KEY): ")
		sc.Scan()
		preset.apiKeyEnv = strings.TrimSpace(sc.Text())
	}

	// Model
	fmt.Println()
	fmt.Println("  ── Step 2: model ─────────────────────────────────")
	var model string
	for {
		fmt.Printf("  Model [%s]: ", preset.defaultModel)
		sc.Scan()
		model = strings.TrimSpace(sc.Text())
		if model == "" {
			model = preset.defaultModel
		}
		// Reject bare single-digit input (user likely still navigating the menu)
		if len(model) == 1 && model[0] >= '1' && model[0] <= '9' {
			fmt.Printf("  %q doesn't look like a model name — enter the model identifier or press Enter for the default\n", model)
			continue
		}
		break
	}

	// API key (cloud only)
	apiKey := ""
	if preset.provType == "openai_compat" && preset.apiKeyEnv != "" {
		// Check if already set
		if existing := os.Getenv(preset.apiKeyEnv); existing != "" {
			fmt.Printf("  %s is already set in env — keeping it\n", preset.apiKeyEnv)
		} else {
			fmt.Printf("  API key (%s): ", preset.apiKeyEnv)
			sc.Scan()
			apiKey = strings.TrimSpace(sc.Text())
		}
	}

	fmt.Println()

	// Write API key to shell profile
	if apiKey != "" {
		profile := shellProfile()
		line := fmt.Sprintf("export %s=%q", preset.apiKeyEnv, apiKey)
		if err := appendToProfile(profile, line, preset.apiKeyEnv); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: could not write to %s: %v\n", profile, err)
			fmt.Printf("  Add this to your shell profile manually:\n    %s\n\n", line)
		} else {
			fmt.Printf("  ✓  %s written to %s\n", preset.apiKeyEnv, profile)
			fmt.Printf("     Run: source %s\n", profile)
			// Also set for the current process so --doctor works immediately
			os.Setenv(preset.apiKeyEnv, apiKey)
		}
	}

	// Build providers YAML block
	var sb strings.Builder
	fmt.Fprintf(&sb, "providers:\n")
	fmt.Fprintf(&sb, "  main:\n")
	fmt.Fprintf(&sb, "    type: %s\n", preset.provType)
	if preset.provType == "openai_compat" {
		fmt.Fprintf(&sb, "    base_url: %q\n", preset.baseURL)
		if preset.apiKeyEnv != "" {
			fmt.Fprintf(&sb, "    api_key_env: %q\n", preset.apiKeyEnv)
		}
	} else {
		fmt.Fprintf(&sb, "    host: %q\n", preset.baseURL)
	}
	fmt.Fprintf(&sb, "    model: %q\n", model)
	fmt.Fprintf(&sb, "    stream: true\n")
	fmt.Fprintf(&sb, "default_provider: main\n")
	fmt.Fprintf(&sb, "\n")
	newBlock := sb.String()

	// Patch config file
	if err := patchConfig(configPath, newBlock); err != nil {
		fmt.Fprintf(os.Stderr, "  error updating config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("  ✓  %s updated\n", configPath)
	fmt.Println()
	fmt.Println("  ──────────────────────────────────────────────────")
	fmt.Println("  Run: mini-agent --doctor")
	fmt.Println("  ──────────────────────────────────────────────────")
	fmt.Println()
}

// patchConfig removes any existing providers/default_provider/fallback_provider
// top-level block from the YAML file, then prepends the new block.
func patchConfig(path, newBlock string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	stripped := stripProviderBlock(string(raw))
	return os.WriteFile(path, []byte(newBlock+stripped), 0o644)
}

// stripProviderBlock removes top-level provider-related keys and their children
// from raw YAML text using a simple line-by-line state machine.
func stripProviderBlock(raw string) string {
	providerKeys := map[string]bool{
		"providers:":              true,
		"default_provider:":       true,
		"fallback_provider:":      true,
		"fallback_after_failures:": true,
	}
	var out []string
	skipping := false
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		isTopLevel := len(line) > 0 && line[0] != ' ' && line[0] != '\t' && line[0] != '#' && line[0] != '\n'
		if isTopLevel {
			key, _, _ := strings.Cut(trimmed, " ")
			if providerKeys[key] {
				skipping = true
				continue
			}
			skipping = false
		}
		if skipping {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// shellProfile returns ~/.zshrc or ~/.bashrc depending on the shell.
func shellProfile() string {
	home, _ := os.UserHomeDir()
	if shell := os.Getenv("SHELL"); strings.Contains(shell, "zsh") {
		return filepath.Join(home, ".zshrc")
	}
	return filepath.Join(home, ".bashrc")
}

// appendToProfile adds the export line to the profile file if not already present.
func appendToProfile(path, line, envKey string) error {
	existing, _ := os.ReadFile(path)
	if strings.Contains(string(existing), envKey) {
		// Replace existing export line
		var lines []string
		for _, l := range strings.Split(string(existing), "\n") {
			if strings.Contains(l, "export "+envKey+"=") {
				lines = append(lines, line)
			} else {
				lines = append(lines, l)
			}
		}
		return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "\n%s\n", line)
	return err
}
