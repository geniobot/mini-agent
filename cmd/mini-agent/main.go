package main

import (
	"flag"
	"fmt"
	"os"

	"mini-agent/internal/agent"
	"mini-agent/internal/config"
	"mini-agent/internal/runlog"
	"mini-agent/internal/session"
	"mini-agent/internal/telegram"
)

func main() {
	configPath := flag.String("config", "", "path to config file (default: ~/.mini-agent/config.yaml or ./config.yaml)")
	runGoal := flag.String("run", "", "run a goal non-interactively and exit")
	modelFlag := flag.String("model", "", "override model from config")
	plain := flag.Bool("plain", false, "disable colors (also respects NO_COLOR env var)")
	quiet := flag.Bool("quiet", false, "suppress all decoration; only emit the final answer (implies --plain)")
	fresh := flag.Bool("fresh", false, "start with empty session, skip loading saved history")
	noSave := flag.Bool("no-save", false, "do not save session on exit")
	doctor       := flag.Bool("doctor", false, "check config, Ollama connectivity, and model availability then exit")
	telegramMode := flag.Bool("telegram", false, "start Telegram bot mode (requires TELEGRAM_BOT_TOKEN env var)")
	noContext    := flag.Bool("no-context", false, "skip loading CONTEXT.md from the working directory")
	versionFlag  := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println(agent.Version)
		return
	}

	if *quiet || *plain || os.Getenv("NO_COLOR") != "" {
		agent.SetPlainMode()
	}

	cfg, err := config.Load(config.FindConfig(*configPath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	if *doctor {
		agent.RunDoctor(cfg)
		return
	}

	if *telegramMode {
		loop := agent.New(cfg)
		if err := telegram.Run(cfg.Telegram.BotToken, cfg.Telegram.AllowedChatIDs, loop); err != nil {
			fmt.Fprintf(os.Stderr, "telegram error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *modelFlag != "" {
		cfg.Ollama.Model = *modelFlag
	}

	loop := agent.New(cfg)

	if *quiet {
		loop.SetQuiet(true)
	}

	// Run log — open once, close on exit. Errors are non-fatal.
	if logPath, err := runlog.DefaultPath(); err == nil {
		if lg, err := runlog.Open(logPath); err == nil {
			loop.SetLogger(lg)
			defer lg.Close()
		}
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

	// Inject CONTEXT.md if present in the working directory.
	if !*noContext {
		loop.InjectContextFile()
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
