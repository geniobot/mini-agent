package runlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const maxLogBytes = 10 * 1024 * 1024 // 10 MB before rotation

// Entry is one JSON line in the run log.
type Entry struct {
	TS          time.Time `json:"ts"`
	Tool        string    `json:"tool"`
	Args        string    `json:"args,omitempty"`
	ResultBytes int       `json:"result_bytes"`
	OK          bool      `json:"ok"`
	DurationMS  int64     `json:"duration_ms"`
}

// Logger writes JSON-line entries to a log file, rotating at maxLogBytes.
type Logger struct {
	path string
	mu   sync.Mutex
	f    *os.File
	size int64
}

// DefaultPath returns ~/.mini-agent/run.log.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".mini-agent", "run.log"), nil
}

// Open opens (or creates) the log file at path.
func Open(path string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &Logger{path: path, f: f, size: info.Size()}, nil
}

// Log writes one entry. Errors are silently dropped — logging must not break the agent.
func (l *Logger) Log(e Entry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.rotateIfNeeded(); err != nil {
		return
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	b = append(b, '\n')
	n, _ := l.f.Write(b)
	l.size += int64(n)
}

func (l *Logger) rotateIfNeeded() error {
	if l.size < maxLogBytes {
		return nil
	}
	l.f.Close()
	_ = os.Rename(l.path, l.path+".1")
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	l.f = f
	l.size = 0
	return nil
}

// Close flushes and closes the log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil {
		return l.f.Close()
	}
	return nil
}
