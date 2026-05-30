package tools

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func requireGit(t *testing.T) {
	t.Helper()
	if !GitAvailable() {
		t.Skip("git not available in PATH")
	}
}

// makeGitRepo creates a temporary git repo with one empty commit and returns its path.
func makeGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test User",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test User",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", dir)
	run("-C", dir, "commit", "--allow-empty", "-m", "initial commit")
	return dir
}

func TestRunGit_status(t *testing.T) {
	requireGit(t)
	dir := makeGitRepo(t)
	out, err := RunGit("status", []string{"--short"}, dir)
	if err != nil {
		t.Fatalf("git status: %v\noutput: %s", err, out)
	}
	// An empty repo after one commit gives either empty output or "nothing to commit"
	// Both are acceptable.
}

func TestRunGit_log(t *testing.T) {
	requireGit(t)
	dir := makeGitRepo(t)
	out, err := RunGit("log", []string{"--oneline"}, dir)
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if !strings.Contains(out, "initial commit") {
		t.Errorf("expected 'initial commit' in log, got: %s", out)
	}
}

func TestRunGit_blockedSubcommand(t *testing.T) {
	_, err := RunGit("reset", nil, "")
	if err == nil {
		t.Error("expected error for blocked subcommand 'reset'")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("error should mention 'blocked', got: %v", err)
	}
}

func TestRunGit_unknownSubcommand(t *testing.T) {
	_, err := RunGit("frobnicate", nil, "")
	if err == nil {
		t.Error("expected error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "allowed list") {
		t.Errorf("error should mention allowed list, got: %v", err)
	}
}

func TestRunGit_blockedFlag(t *testing.T) {
	_, err := RunGit("push", []string{"--force"}, "")
	if err == nil {
		t.Error("expected error for --force flag")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should mention the flag, got: %v", err)
	}
}

func TestRunGit_blockedFlagHard(t *testing.T) {
	_, err := RunGit("checkout", []string{"--hard", "HEAD"}, "")
	if err == nil {
		t.Error("expected error for --hard flag")
	}
}

func TestIsGitWriteSubcommand(t *testing.T) {
	writes := []string{"add", "commit", "push", "pull", "merge"}
	for _, sub := range writes {
		if !IsGitWriteSubcommand(sub) {
			t.Errorf("expected %q to be a write subcommand", sub)
		}
	}
	reads := []string{"status", "log", "diff", "branch", "show"}
	for _, sub := range reads {
		if IsGitWriteSubcommand(sub) {
			t.Errorf("expected %q to NOT be a write subcommand", sub)
		}
	}
}
