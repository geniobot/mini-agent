package agent

import (
	"fmt"
	"os"
)

const bashCompletion = `# mini-agent bash completion
# Install:
#   mini-agent --completion bash >> ~/.bash_completion
#   source ~/.bash_completion
# Or system-wide:
#   mini-agent --completion bash | sudo tee /etc/bash_completion.d/mini-agent

_mini_agent_completions() {
    local cur="${COMP_WORDS[COMP_CWORD]}"
    local prev="${COMP_WORDS[COMP_CWORD-1]}"

    # Flag arguments that expect a value
    case "$prev" in
        --config|--run|--batch|--model|--system|--completion)
            return ;;
        --parallel)
            COMPREPLY=($(compgen -W "1 2 4 8" -- "$cur"))
            return ;;
    esac

    # Flags
    local flags="--config --run --batch --parallel --model --plain --quiet
        --fresh --no-save --doctor --setup --telegram --daemon --no-context
        --system --version --completion"

    COMPREPLY=($(compgen -W "$flags" -- "$cur"))
}

complete -F _mini_agent_completions mini-agent
`

const zshCompletion = `#compdef mini-agent
# mini-agent zsh completion
# Install:
#   mkdir -p ~/.zsh/completions
#   mini-agent --completion zsh > ~/.zsh/completions/_mini-agent
#   echo 'fpath=(~/.zsh/completions $fpath)' >> ~/.zshrc
#   echo 'autoload -Uz compinit && compinit' >> ~/.zshrc
#   source ~/.zshrc

_mini_agent() {
    _arguments \
        '--config[path to config file]:file:_files' \
        '--run[run a goal non-interactively and exit]:goal' \
        '--batch[file of goals to run in batch mode]:file:_files' \
        '--parallel[number of concurrent batch goals]:number' \
        '--model[override model from config]:model' \
        '--system[override system prompt for this session]:prompt' \
        '--plain[disable colors]' \
        '--quiet[suppress decoration; only emit final answer]' \
        '--fresh[start with empty session]' \
        '--no-save[do not save session on exit]' \
        '--no-context[skip loading CONTEXT.md]' \
        '--doctor[validate config and Ollama connectivity then exit]' \
        '--setup[interactive provider setup wizard]' \
        '--telegram[start Telegram bot mode]' \
        '--daemon[run scheduled goals from config and exit on SIGTERM]' \
        '--version[print version and exit]' \
        '--completion[print shell completion script]:shell:(bash zsh)'
}

compdef _mini_agent mini-agent
`

// PrintCompletion writes a shell completion script for the given shell to stdout.
// Supported shells: bash, zsh.
func PrintCompletion(shell string) {
	switch shell {
	case "bash":
		fmt.Print(bashCompletion)
	case "zsh":
		fmt.Print(zshCompletion)
	default:
		fmt.Fprintf(os.Stderr, "unsupported shell %q — use bash or zsh\n", shell)
		os.Exit(1)
	}
}
