package tools

import (
	"bytes"
	"os/exec"
	"strings"
)

func RunCommand(command string, args []string) (string, error) {
	cmd := exec.Command(command, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := strings.TrimSpace(stdout.String())
	errOut := strings.TrimSpace(stderr.String())
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
