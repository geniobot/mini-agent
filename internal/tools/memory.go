package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/geniobot/mini-agent/internal/fileutil"
)

// memoryPathOverride is set by tests to redirect storage to a temp dir.
var memoryPathOverride string

func memoryPath() (string, error) {
	if memoryPathOverride != "" {
		return memoryPathOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".mini-agent", "memory.json"), nil
}

func loadMemory(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("memory file corrupted: %w", err)
	}
	return m, nil
}

func saveMemory(path string, m map[string]string) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteAtomic(path, b, 0o600)
}

// RunMemory executes a memory operation: set, get, delete, or list.
func RunMemory(argsJSON string) (string, error) {
	var args struct {
		Op    string `json:"op"`
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("memory: invalid args: %w", err)
	}

	path, err := memoryPath()
	if err != nil {
		return "", fmt.Errorf("memory: %w", err)
	}

	m, err := loadMemory(path)
	if err != nil {
		return "", err
	}

	switch strings.ToLower(args.Op) {
	case "set":
		if args.Key == "" {
			return "", fmt.Errorf("memory set: key is required")
		}
		m[args.Key] = args.Value
		if err := saveMemory(path, m); err != nil {
			return "", err
		}
		return fmt.Sprintf("memory: set %q", args.Key), nil

	case "get":
		if args.Key == "" {
			return "", fmt.Errorf("memory get: key is required")
		}
		v, ok := m[args.Key]
		if !ok {
			return fmt.Sprintf("memory: key %q not found", args.Key), nil
		}
		return v, nil

	case "delete":
		if args.Key == "" {
			return "", fmt.Errorf("memory delete: key is required")
		}
		if _, ok := m[args.Key]; !ok {
			return fmt.Sprintf("memory: key %q not found", args.Key), nil
		}
		delete(m, args.Key)
		if err := saveMemory(path, m); err != nil {
			return "", err
		}
		return fmt.Sprintf("memory: deleted %q", args.Key), nil

	case "list":
		if len(m) == 0 {
			return "memory: (empty)", nil
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var sb strings.Builder
		for _, k := range keys {
			fmt.Fprintf(&sb, "%s = %s\n", k, m[k])
		}
		return strings.TrimRight(sb.String(), "\n"), nil

	default:
		return "", fmt.Errorf("memory: unknown op %q — use set, get, delete, or list", args.Op)
	}
}
