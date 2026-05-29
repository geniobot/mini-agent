package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"mini-agent/internal/config"
	"mini-agent/internal/llm"
)

type Registry struct {
	cfg config.ToolsConfig
}

type CommandArgs struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type FileReadArgs struct {
	Path string `json:"path"`
}

type FileWriteArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type ListDirArgs struct {
	Path string `json:"path"`
}

func New(cfg config.ToolsConfig) *Registry {
	return &Registry{cfg: cfg}
}

func (r *Registry) Enabled() bool {
	return r.cfg.Enabled
}

func (r *Registry) Specs() []llm.ToolSpec {
	if !r.cfg.Enabled {
		return nil
	}
	var specs []llm.ToolSpec
	if r.cfg.EnableReadFile {
		specs = append(specs, llm.ToolSpec{Type: "function", Function: llm.ToolFunction{
			Name:        "read_file",
			Description: "Read a UTF-8 text file from disk.",
			Parameters:  schema(map[string]string{"path": "string"}, []string{"path"}),
		}})
	}
	if r.cfg.EnableWriteFile {
		specs = append(specs, llm.ToolSpec{Type: "function", Function: llm.ToolFunction{
			Name:        "write_file",
			Description: "Write (overwrite) a UTF-8 text file to disk.",
			Parameters:  schema(map[string]string{"path": "string", "content": "string"}, []string{"path", "content"}),
		}})
	}
	if r.cfg.EnableAppendFile {
		specs = append(specs, llm.ToolSpec{Type: "function", Function: llm.ToolFunction{
			Name:        "append_file",
			Description: "Append text to a file, creating it if it does not exist.",
			Parameters:  schema(map[string]string{"path": "string", "content": "string"}, []string{"path", "content"}),
		}})
	}
	if r.cfg.EnableListDir {
		specs = append(specs, llm.ToolSpec{Type: "function", Function: llm.ToolFunction{
			Name:        "list_dir",
			Description: "List the contents of a directory.",
			Parameters:  schema(map[string]string{"path": "string"}, []string{"path"}),
		}})
	}
	if r.cfg.EnableRunCmd {
		specs = append(specs, llm.ToolSpec{Type: "function", Function: llm.ToolFunction{
			Name:        "run_command",
			Description: "Run an allowed local shell command.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{"type": "string"},
					"args":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				},
				"required": []string{"command"},
			},
		}})
	}
	return specs
}

func (r *Registry) Execute(name, arguments string) (string, error) {
	switch name {
	case "read_file":
		var a FileReadArgs
		if err := json.Unmarshal([]byte(arguments), &a); err != nil {
			return "", err
		}
		return ReadFile(a.Path)
	case "write_file":
		var a FileWriteArgs
		if err := json.Unmarshal([]byte(arguments), &a); err != nil {
			return "", err
		}
		return WriteFile(a.Path, a.Content)
	case "append_file":
		var a FileWriteArgs
		if err := json.Unmarshal([]byte(arguments), &a); err != nil {
			return "", err
		}
		return AppendFile(a.Path, a.Content)
	case "list_dir":
		var a ListDirArgs
		if err := json.Unmarshal([]byte(arguments), &a); err != nil {
			return "", err
		}
		return ListDir(a.Path)
	case "run_command":
		var a CommandArgs
		if err := json.Unmarshal([]byte(arguments), &a); err != nil {
			return "", err
		}
		if !r.allowed(a.Command) {
			return "", fmt.Errorf("command not allowed: %s", a.Command)
		}
		return RunCommand(a.Command, a.Args)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func (r *Registry) AllowedCommandPrompt(arguments string) string {
	var a CommandArgs
	if err := json.Unmarshal([]byte(arguments), &a); err != nil {
		return arguments
	}
	return strings.TrimSpace(a.Command + " " + strings.Join(a.Args, " "))
}

func (r *Registry) ConfirmRunCommand() bool {
	return r.cfg.ConfirmRunCmd
}

func (r *Registry) ConfirmWriteFile() bool {
	return r.cfg.ConfirmWriteFile
}

func (r *Registry) allowed(cmd string) bool {
	for _, allowed := range r.cfg.AllowedCommands {
		if strings.TrimSpace(allowed) == cmd {
			return true
		}
	}
	return false
}

func schema(fields map[string]string, required []string) map[string]interface{} {
	props := map[string]interface{}{}
	for k, t := range fields {
		props[k] = map[string]interface{}{"type": t}
	}
	return map[string]interface{}{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}
