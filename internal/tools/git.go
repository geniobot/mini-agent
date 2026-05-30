package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	gitTimeout    = 30 * time.Second
	gitMaxOutput  = 8 * 1024 // 8 KB — enough for typical git output
)

// Read-only subcommands that never modify the repository.
var gitReadOnly = map[string]bool{
	"status": true, "log": true, "diff": true, "show": true,
	"branch": true, "fetch": true, "blame": true, "shortlog": true,
	"describe": true, "remote": true, "ls-files": true, "ls-remote": true,
	"grep": true, "rev-parse": true, "rev-list": true, "cat-file": true,
	"for-each-ref": true, "stash": true, // stash without args lists stash
}

// Write subcommands that modify the repository — require confirmation.
var gitWrite = map[string]bool{
	"add": true, "commit": true, "push": true, "pull": true, "clone": true,
	"merge": true, "cherry-pick": true, "revert": true, "rm": true,
	"mv": true, "checkout": true, "switch": true, "restore": true,
	"init": true, "tag": true, "apply": true,
}

// Blocked regardless of confirmation — too destructive or interactive.
var gitBlocked = map[string]bool{
	"reset": true, "rebase": true, "filter-branch": true, "clean": true,
	"gc": true, "bisect": true, "update-ref": true, "replace": true,
	"filter-repo": true,
}

// Flags that could cause irreversible data loss — always blocked.
var gitBlockedFlags = []string{
	"--force", "-f", "--force-with-lease", "--force-if-includes",
	"--hard", "--no-verify",
}

// GitAvailable returns true if the git binary is present in PATH.
func GitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// IsGitWriteSubcommand returns true when sub requires user confirmation.
func IsGitWriteSubcommand(sub string) bool {
	return gitWrite[strings.ToLower(sub)]
}

// RunGit executes a git subcommand in the given directory.
// dir may be empty to use the process working directory.
func RunGit(subcommand string, args []string, dir string) (string, error) {
	sub := strings.ToLower(subcommand)

	if gitBlocked[sub] {
		return "", fmt.Errorf("git %q is blocked — use it manually in your terminal", subcommand)
	}
	if !gitReadOnly[sub] && !gitWrite[sub] {
		return "", fmt.Errorf("git subcommand %q is not in the allowed list", subcommand)
	}
	for _, arg := range args {
		for _, bad := range gitBlockedFlags {
			if arg == bad {
				return "", fmt.Errorf("git flag %q is blocked for safety", arg)
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	cmdArgs := append([]string{subcommand}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	if dir != "" {
		cmd.Dir = dir
	}

	var stdout, stderr limitBuf
	stdout.limit = gitMaxOutput
	stderr.limit = gitMaxOutput
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	out := strings.TrimSpace(stdout.String())
	if stdout.truncated {
		out += fmt.Sprintf("\n[output truncated at %dKB]", gitMaxOutput/1024)
	}
	errOut := strings.TrimSpace(stderr.String())

	if err != nil {
		if errOut != "" {
			return strings.TrimSpace(out+"\n"+errOut), err
		}
		return out, err
	}
	if errOut != "" {
		return strings.TrimSpace(out+"\n"+errOut), nil
	}
	return out, nil
}
