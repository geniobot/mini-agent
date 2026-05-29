package main

import (
	"flag"
	"fmt"
	"os"

	"mini-agent/internal/agent"
	"mini-agent/internal/config"
	"mini-agent/internal/session"
)

func main() {
	configPath := flag.String("config", "", "path to config file (default: ~/.mini-agent/config.yaml or ./config.yaml)")
	runGoal := flag.String("run", "", "run a goal non-interactively and exit")
	modelFlag := flag.String("model", "", "override model from config")
	plain := flag.Bool("plain", false, "disable colors (also respects NO_COLOR env var)")
	quiet := flag.Bool("quiet", false, "suppress all decoration; only emit the final answer (implies --plain)")
	fresh := flag.Bool("fresh", false, "start with empty session, skip loading saved history")
	noSave := flag.Bool("no-save", false, "do not save session on exit")
	flag.Parse()

	if *quiet || *plain || os.Getenv("NO_COLOR") != "" {
		agent.SetPlainMode()
	}

	cfg, err := config.Load(config.FindConfig(*configPath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	if *modelFlag != "" {
		cfg.Ollama.Model = *modelFlag
	}

	loop := agent.New(cfg)

	if *quiet {
		loop.SetQuiet(true)
	}

	// Session persistence: load history unless --fresh; save on exit unless --no-save or --run.
	savePath, pathErr := session.DefaultPath()
	if pathErr == nil {
		if !*fresh {
			if n, err := loop.LoadSession(savePath); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not load session: %v\n", err)
			} else if n > 0 && !*quiet {
				fmt.Printf("  \033[2m[session restored: %d messages]\033[0m\n", n)
			}
		}
		if !*noSave && *runGoal == "" {
			loop.SetSavePath(savePath)
		}
	}

	if *runGoal != "" {
		if err := loop.RunGoal(*runGoal); err != nil {
			fmt.Fprintf(os.Stderr, "goal error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := loop.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "runtime error: %v\n", err)
		os.Exit(1)
	}
}
