package agent

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"mini-agent/internal/config"
	"mini-agent/internal/llm"
)

// RunDoctor checks config validity, provider connectivity, and model availability.
// It prints a human-readable report and exits cleanly — no error is returned.
func RunDoctor(cfg *config.Config) {
	const sep = "──────────────────────────────────────────────────"
	fmt.Printf("\n%s%s%s\n", ansiDim, sep, ansiReset)
	fmt.Printf("  %smini-agent doctor%s\n", ansiTeal, ansiReset)
	fmt.Printf("%s%s%s\n\n", ansiDim, sep, ansiReset)

	ok := true

	if err := cfg.Validate(); err != nil {
		fmt.Printf("  %s✗%s  config    %v\n", ansiRed, ansiReset, err)
		ok = false
	} else {
		fmt.Printf("  %s✓%s  config    OK\n", ansiGreen, ansiReset)
	}

	if len(cfg.Providers) == 0 {
		// Legacy: check Ollama only.
		host := strings.TrimPrefix(cfg.Ollama.Host, "http://")
		host = strings.TrimPrefix(host, "https://")
		if err := llm.Ping(cfg.Ollama.Host); err != nil {
			fmt.Printf("  %s✗%s  ollama    not reachable at %s\n", ansiRed, ansiReset, host)
			fmt.Printf("           is Ollama running? try: ollama serve\n")
			ok = false
		} else {
			fmt.Printf("  %s✓%s  ollama    reachable at %s\n", ansiGreen, ansiReset, host)
			var client llm.Client = llm.NewOllama(cfg.Ollama.Host)
			if ml, ok2 := client.(llm.ModelLister); ok2 {
				models, err := ml.ListModels()
				if err != nil {
					fmt.Printf("  %s?%s  models    could not list: %v\n", ansiYellow, ansiReset, err)
				} else {
					fmt.Printf("  %s✓%s  models    %d available\n", ansiGreen, ansiReset, len(models))
					if slices.Contains(models, cfg.Ollama.Model) {
						fmt.Printf("  %s✓%s  model     %q ready\n", ansiGreen, ansiReset, cfg.Ollama.Model)
					} else {
						fmt.Printf("  %s✗%s  model     %q not found — run: ollama pull %s\n",
							ansiRed, ansiReset, cfg.Ollama.Model, cfg.Ollama.Model)
						ok = false
					}
				}
			}
		}
	} else {
		// Multi-provider: check each named provider.
		for name, p := range cfg.Providers {
			switch p.Type {
			case "ollama":
				if err := llm.Ping(p.Host); err != nil {
					fmt.Printf("  %s✗%s  %-12s not reachable at %s\n", ansiRed, ansiReset, name, p.Host)
					ok = false
				} else {
					fmt.Printf("  %s✓%s  %-12s ollama OK at %s — model: %s\n", ansiGreen, ansiReset, name, p.Host, p.Model)
				}
			case "openai_compat":
				apiKey := os.Getenv(p.APIKeyEnv)
				if apiKey == "" {
					fmt.Printf("  %s✗%s  %-12s env var %s is not set\n", ansiRed, ansiReset, name, p.APIKeyEnv)
					if name == cfg.DefaultProvider {
						ok = false
					}
				} else {
					fmt.Printf("  %s✓%s  %-12s api key set (%s) — model: %s\n", ansiGreen, ansiReset, name, p.APIKeyEnv, p.Model)
				}
			}
		}
	}

	fmt.Printf("\n%s%s%s\n", ansiDim, sep, ansiReset)
	if ok {
		fmt.Printf("  %s✓  all checks passed — ready to run%s\n", ansiGreen, ansiReset)
	} else {
		fmt.Printf("  %s✗  fix the issues above before running mini-agent%s\n", ansiRed, ansiReset)
	}
	fmt.Printf("%s%s%s\n\n", ansiDim, sep, ansiReset)
}
