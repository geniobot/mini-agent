package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const maxCmdOutput = 4 * 1024 // 4 KB per stream is plenty for tool results
const cmdTimeout = 30 * time.Second

func RunCommand(command string, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	var stdout, stderr limitBuf
	stdout.limit = maxCmdOutput
	stderr.limit = maxCmdOutput
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	out := strings.TrimSpace(stdout.String())
	if stdout.truncated {
		out += fmt.Sprintf("\n[output truncated at %dKB]", maxCmdOutput/1024)
	}
	errOut := strings.TrimSpace(stderr.String())
	if stderr.truncated {
		errOut += fmt.Sprintf("\n[stderr truncated at %dKB]", maxCmdOutput/1024)
	}
	if err != nil {
		if errOut != "" {
			return strings.TrimSpace(out + "\n" + errOut), err
		}
		return out, err
	}
	if errOut != "" {
		return strings.TrimSpace(out + "\n" + errOut), nil
	}
	return out, nil
}
