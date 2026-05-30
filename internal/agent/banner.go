package agent

import (
	"fmt"
	"strings"

	"mini-agent/internal/config"
)

// Color variables — set to empty strings by SetPlainMode for script-friendly output.
var (
	ansiCyan   = "\033[96m"
	ansiBlue   = "\033[94m"
	ansiTeal   = "\033[36m"
	ansiGreen  = "\033[92m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[91m"
	ansiDim    = "\033[2m"
	ansiReset  = "\033[0m"
)

// SetPlainMode disables all ANSI color codes.
// Triggered by --plain flag or the NO_COLOR environment variable (https://no-color.org).
func SetPlainMode() {
	ansiCyan   = ""
	ansiBlue   = ""
	ansiTeal   = ""
	ansiGreen  = ""
	ansiYellow = ""
	ansiRed    = ""
	ansiDim    = ""
	ansiReset  = ""
}

var logoMini = [6]string{
	`███╗   ███╗██╗███╗   ██╗██╗`,
	`████╗ ████║██║████╗  ██║██║`,
	`██╔████╔██║██║██╔██╗ ██║██║`,
	`██║╚██╔╝██║██║██║╚██╗██║██║`,
	`██║ ╚═╝ ██║██║██║ ╚████║██║`,
	`╚═╝     ╚═╝╚═╝╚═╝  ╚═══╝╚═╝`,
}

var logoAgent = [6]string{
	` █████╗  ██████╗ ███████╗███╗   ██╗████████╗`,
	`██╔══██╗██╔════╝ ██╔════╝████╗  ██║╚══██╔══╝`,
	`███████║██║  ███╗█████╗  ██╔██╗ ██║   ██║   `,
	`██╔══██║██║   ██║██╔══╝  ██║╚██╗██║   ██║   `,
	`██║  ██║╚██████╔╝███████╗██║ ╚████║   ██║   `,
	`╚═╝  ╚═╝ ╚═════╝ ╚══════╝╚═╝  ╚═══╝   ╚═╝  `,
}

// Version is the current release — used in the banner and --version flag.
const Version = "v2.6.0"

func printBanner(cfg *config.Config) {
	const version = Version
	const sep = "──────────────────────────────────────────────────"

	fmt.Println()
	for _, line := range logoMini {
		fmt.Printf("  %s%s%s\n", ansiCyan, line, ansiReset)
	}
	for _, line := range logoAgent {
		fmt.Printf("  %s%s%s\n", ansiBlue, line, ansiReset)
	}
	fmt.Println()

	tagline := "local · lightweight · fast"
	padding := 44 - len(tagline)
	if padding < 1 {
		padding = 1
	}
	fmt.Printf("  %s%s%s%s%s\n", ansiDim, tagline, strings.Repeat(" ", padding), version, ansiReset)
	fmt.Printf("  %s%s%s\n", ansiDim, sep, ansiReset)

	host := strings.TrimPrefix(cfg.Ollama.Host, "http://")
	fmt.Printf("  %s◆%s  model   %s\n", ansiTeal, ansiReset, cfg.Ollama.Model)
	fmt.Printf("  %s◆%s  host    %s\n", ansiTeal, ansiReset, host)
	fmt.Printf("  %s◆%s  tools   %s\n", ansiTeal, ansiReset, describeTools(cfg))

	fmt.Printf("  %s%s%s\n", ansiDim, sep, ansiReset)
	fmt.Printf("  %sType /exit to quit · /help for commands%s\n\n", ansiDim, ansiReset)
}

func describeTools(cfg *config.Config) string {
	if !cfg.Tools.Enabled {
		return ansiDim + "disabled" + ansiReset
	}
	var parts []string
	if cfg.Tools.EnableReadFile {
		parts = append(parts, "read")
	}
	if cfg.Tools.EnableWriteFile {
		parts = append(parts, "write")
	}
	if cfg.Tools.EnableEditFile {
		parts = append(parts, "edit")
	}
	if cfg.Tools.EnableAppendFile {
		parts = append(parts, "append")
	}
	if cfg.Tools.EnableListDir {
		parts = append(parts, "list_dir")
	}
	if cfg.Tools.EnableRunCmd {
		label := "shell"
		if cfg.Tools.ConfirmRunCmd {
			label += " (confirmed)"
		}
		parts = append(parts, label)
	}
	if cfg.Tools.EnableSearchFiles {
		parts = append(parts, "search")
	}
	if cfg.Tools.EnableWebFetch {
		parts = append(parts, "web")
	}
	if cfg.Tools.EnableGit {
		label := "git"
		if cfg.Tools.ConfirmGitWrite {
			label += " (confirmed)"
		}
		parts = append(parts, label)
	}
	if len(parts) == 0 {
		return ansiDim + "none" + ansiReset
	}
	return strings.Join(parts, " · ")
}
