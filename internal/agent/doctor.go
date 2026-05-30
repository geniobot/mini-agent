package agent

import (
	"fmt"
	"slices"
	"strings"

	"mini-agent/internal/config"
	"mini-agent/internal/llm"
)

// RunDoctor checks config validity, Ollama connectivity, and model availability.
// It prints a human-readable report and exits cleanly — no error is returned.
func RunDoctor(cfg *config.Config) {
	const sep = "──────────────────────────────────────────────────"
	fmt.Printf("\n%s%s%s\n", ansiDim, sep, ansiReset)
	fmt.Printf("  %smini-agent doctor%s\n", ansiTeal, ansiReset)
	fmt.Printf("%s%s%s\n\n", ansiDim, sep, ansiReset)

	ok := true

	// Config validation.
	if err := cfg.Validate(); err != nil {
		fmt.Printf("  %s✗%s  config    %v\n", ansiRed, ansiReset, err)
		ok = false
	} else {
		fmt.Printf("  %s✓%s  config    OK\n", ansiGreen, ansiReset)
	}

	// Ollama connectivity.
	host := strings.TrimPrefix(cfg.Ollama.Host, "http://")
	host = strings.TrimPrefix(host, "https://")
	if err := llm.Ping(cfg.Ollama.Host); err != nil {
		fmt.Printf("  %s✗%s  ollama    not reachable at %s\n", ansiRed, ansiReset, host)
		fmt.Printf("           is Ollama running? try: ollama serve\n")
		ok = false
	} else {
		fmt.Printf("  %s✓%s  ollama    reachable at %s\n", ansiGreen, ansiReset, host)

		// Model availability — assign to interface so the type assertion works.
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

	fmt.Printf("\n%s%s%s\n", ansiDim, sep, ansiReset)
	if ok {
		fmt.Printf("  %s✓  all checks passed — ready to run%s\n", ansiGreen, ansiReset)
	} else {
		fmt.Printf("  %s✗  fix the issues above before running mini-agent%s\n", ansiRed, ansiReset)
	}
	fmt.Printf("%s%s%s\n\n", ansiDim, sep, ansiReset)
}
