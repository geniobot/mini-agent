package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"mini-agent/internal/agent"
	"mini-agent/internal/config"
	"mini-agent/internal/runlog"
	"mini-agent/internal/session"
	"mini-agent/internal/telegram"
)

func main() {
	configPath   := flag.String("config", "", "path to config file (default: ~/.mini-agent/config.yaml or ./config.yaml)")
	runGoal      := flag.String("run", "", "run a goal non-interactively and exit")
	batchFile    := flag.String("batch", "", "file of goals (one per line) to run and exit; outputs JSON lines")
	parallel     := flag.Int("parallel", 1, "number of goals to run concurrently in --batch mode")
	modelFlag    := flag.String("model", "", "override model from config")
	plain        := flag.Bool("plain", false, "disable colors (also respects NO_COLOR env var)")
	quiet        := flag.Bool("quiet", false, "suppress all decoration; only emit the final answer (implies --plain)")
	fresh        := flag.Bool("fresh", false, "start with empty session, skip loading saved history")
	noSave       := flag.Bool("no-save", false, "do not save session on exit")
	doctor       := flag.Bool("doctor", false, "check config, Ollama connectivity, and model availability then exit")
	setupFlag    := flag.Bool("setup", false, "interactive provider setup wizard")
	telegramMode := flag.Bool("telegram", false, "start Telegram bot mode (requires TELEGRAM_BOT_TOKEN env var)")
	daemonMode   := flag.Bool("daemon", false, "run scheduled goals from the 'schedule:' section in config and exit on SIGTERM")
	noContext    := flag.Bool("no-context", false, "skip loading CONTEXT.md from the working directory")
	systemFlag   := flag.String("system", "", "override the system prompt for this session")
	versionFlag  := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	// Pipe/stdin support: if stdin is not a TTY, read it and combine with any
	// positional args or --run value into a single one-shot prompt.
	if fi, err := os.Stdin.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
		if b, err := io.ReadAll(os.Stdin); err == nil && len(b) > 0 {
			piped := strings.TrimSpace(string(b))
			question := strings.Join(flag.Args(), " ")
			if *runGoal != "" {
				question = *runGoal
			}
			if question != "" {
				*runGoal = piped + "\n\n" + question
			} else {
				*runGoal = piped
			}
			if !*quiet {
				*quiet = true
				*plain = true
			}
		}
	}

	if *versionFlag {
		fmt.Println(agent.Version)
		return
	}

	if *setupFlag {
		runSetup(config.FindConfig(*configPath))
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

	if *systemFlag != "" {
		cfg.Agent.SystemPrompt = *systemFlag
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

	if *daemonMode {
		if err := agent.RunDaemon(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "daemon error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Batch mode: each goal gets its own Loop; no shared session or run log.
	if *batchFile != "" {
		goals, err := agent.ReadGoals(*batchFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "batch: %v\n", err)
			os.Exit(1)
		}
		if len(goals) == 0 {
			fmt.Fprintf(os.Stderr, "batch: no goals found in %s\n", *batchFile)
			os.Exit(1)
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if agent.RunBatch(ctx, cfg, goals, *parallel, os.Stdout) {
			os.Exit(1)
		}
		return
	}

	if *modelFlag != "" {
		if len(cfg.Providers) == 0 {
			cfg.Ollama.Model = *modelFlag
		} else {
			if p, ok := cfg.Providers[cfg.DefaultProvider]; ok {
				p.Model = *modelFlag
				cfg.Providers[cfg.DefaultProvider] = p
			}
		}
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
