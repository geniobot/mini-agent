package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

const notifyTimeout = 5 * time.Second

// NotifyAvailable returns true if a desktop notification binary is present.
func NotifyAvailable() bool {
	_, err := notifyBinary()
	return err == nil
}

func notifyBinary() (string, error) {
	switch runtime.GOOS {
	case "linux":
		if path, err := exec.LookPath("notify-send"); err == nil {
			return path, nil
		}
		return "", fmt.Errorf("notify-send not found — install libnotify-bin")
	case "darwin":
		// osascript ships with macOS; no install needed
		if path, err := exec.LookPath("osascript"); err == nil {
			return path, nil
		}
		return "", fmt.Errorf("osascript not found")
	default:
		return "", fmt.Errorf("desktop notifications not supported on %s", runtime.GOOS)
	}
}

// Notify sends a desktop notification with the given title and body.
// Returns nil (not an error) when the notification binary is absent — the
// caller should treat a missing binary as a soft skip, not a failure.
func Notify(title, body string) error {
	bin, err := notifyBinary()
	if err != nil {
		return nil // silently no-op when binary absent
	}

	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
	defer cancel()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.CommandContext(ctx, bin, title, body)
	case "darwin":
		script := fmt.Sprintf(`display notification %q with title %q`, body, title)
		cmd = exec.CommandContext(ctx, bin, "-e", script)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("notify failed: %w — %s", err, string(out))
	}
	return nil
}

// RunNotify parses the JSON args string and sends a desktop notification.
func RunNotify(argsJSON string) (string, error) {
	var args struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("notify: invalid args: %w", err)
	}
	if args.Title == "" {
		args.Title = "mini-agent"
	}
	if args.Body == "" {
		return "", fmt.Errorf("notify: body is required")
	}
	if err := Notify(args.Title, args.Body); err != nil {
		return "", err
	}
	return fmt.Sprintf("notification sent: %q — %q", args.Title, args.Body), nil
}
