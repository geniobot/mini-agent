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
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

type FileWriteArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type FileEditArgs struct {
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

type ListDirArgs struct {
	Path string `json:"path"`
}

type GitArgs struct {
	Subcommand string   `json:"subcommand"`
	Args       []string `json:"args"`
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
			Description: "Read a UTF-8 text file. Optionally pass offset (1-based start line) and limit (max lines) to read part of a large file.",
			Parameters:  schema(map[string]string{"path": "string", "offset": "integer", "limit": "integer"}, []string{"path"}),
		}})
	}
	if r.cfg.EnableWriteFile {
		specs = append(specs, llm.ToolSpec{Type: "function", Function: llm.ToolFunction{
			Name:        "write_file",
			Description: "Write (overwrite) a UTF-8 text file to disk.",
			Parameters:  schema(map[string]string{"path": "string", "content": "string"}, []string{"path", "content"}),
		}})
	}
	if r.cfg.EnableEditFile {
		specs = append(specs, llm.ToolSpec{Type: "function", Function: llm.ToolFunction{
			Name:        "edit_file",
			Description: "Edit an existing file by replacing old_string with new_string. old_string must be unique unless replace_all is true. Use this instead of write_file to change part of a file.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":        map[string]any{"type": "string"},
					"old_string":  map[string]any{"type": "string"},
					"new_string":  map[string]any{"type": "string"},
					"replace_all": map[string]any{"type": "boolean"},
				},
				"required": []string{"path", "old_string", "new_string"},
			},
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
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{"type": "string"},
					"args":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"command"},
			},
		}})
	}
	if r.cfg.EnableWebFetch {
		specs = append(specs, llm.ToolSpec{Type: "function", Function: llm.ToolFunction{
			Name:        "web_fetch",
			Description: "Fetch a URL and return its content as plain text (HTML stripped).",
			Parameters:  schema(map[string]string{"url": "string", "timeout_seconds": "integer"}, []string{"url"}),
		}})
	}
	if r.cfg.EnableSearchFiles {
		specs = append(specs, llm.ToolSpec{Type: "function", Function: llm.ToolFunction{
			Name:        "search_files",
			Description: "Search for a text pattern across all files under a directory (case-insensitive, grep-style output).",
			Parameters:  schema(map[string]string{"pattern": "string", "path": "string", "max_results": "integer"}, []string{"pattern"}),
		}})
	}
	if r.cfg.EnableGit && GitAvailable() {
		specs = append(specs, llm.ToolSpec{Type: "function", Function: llm.ToolFunction{
			Name:        "git",
			Description: "Run a git subcommand (status, log, diff, branch, add, commit, push, pull, etc.) in the current repository.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"subcommand": map[string]any{"type": "string"},
					"args":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"subcommand"},
			},
		}})
	}
	if r.cfg.EnableJsonQuery {
		specs = append(specs, llm.ToolSpec{Type: "function", Function: llm.ToolFunction{
			Name:        "json_query",
			Description: "Extract a value from a JSON string using a dot-path expression (.key, .key.nested, .array[0], .array[0].field). Returns null when path is missing.",
			Parameters:  schema(map[string]string{"json": "string", "path": "string"}, []string{"json", "path"}),
		}})
	}
	if r.cfg.EnableNotify && NotifyAvailable() {
		specs = append(specs, llm.ToolSpec{Type: "function", Function: llm.ToolFunction{
			Name:        "notify",
			Description: "Send a desktop notification to the local machine. Use at the end of a long-running goal to alert the user.",
			Parameters:  schema(map[string]string{"title": "string", "body": "string"}, []string{"body"}),
		}})
	}
	if r.cfg.EnableMemory {
		specs = append(specs, llm.ToolSpec{Type: "function", Function: llm.ToolFunction{
			Name:        "memory",
			Description: "Persist facts across sessions. ops: set (store key+value), get (retrieve by key), delete (remove key), list (show all). Use for information that should survive past the current conversation.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"op":    map[string]any{"type": "string", "enum": []string{"set", "get", "delete", "list"}},
					"key":   map[string]any{"type": "string"},
					"value": map[string]any{"type": "string"},
				},
				"required": []string{"op"},
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
		return ReadFileRange(a.Path, a.Offset, a.Limit)
	case "write_file":
		var a FileWriteArgs
		if err := json.Unmarshal([]byte(arguments), &a); err != nil {
			return "", err
		}
		return WriteFile(a.Path, a.Content)
	case "edit_file":
		var a FileEditArgs
		if err := json.Unmarshal([]byte(arguments), &a); err != nil {
			return "", err
		}
		return EditFile(a.Path, a.OldString, a.NewString, a.ReplaceAll)
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
	case "web_fetch":
		var a struct {
			URL            string `json:"url"`
			TimeoutSeconds int    `json:"timeout_seconds"`
		}
		if err := json.Unmarshal([]byte(arguments), &a); err != nil {
			return "", err
		}
		if a.TimeoutSeconds <= 0 {
			a.TimeoutSeconds = r.cfg.WebFetchTimeoutSecs
		}
		return WebFetch(a.URL, a.TimeoutSeconds)
	case "search_files":
		var a struct {
			Pattern    string `json:"pattern"`
			Path       string `json:"path"`
			MaxResults int    `json:"max_results"`
		}
		if err := json.Unmarshal([]byte(arguments), &a); err != nil {
			return "", err
		}
		if a.Path == "" {
			a.Path = "."
		}
		return SearchFiles(a.Pattern, a.Path, a.MaxResults)
	case "git":
		var a GitArgs
		if err := json.Unmarshal([]byte(arguments), &a); err != nil {
			return "", err
		}
		return RunGit(a.Subcommand, a.Args, "")
	case "json_query":
		var a JsonQueryArgs
		if err := json.Unmarshal([]byte(arguments), &a); err != nil {
			return "", err
		}
		return JsonQuery(a.JSON, a.Path)
	case "notify":
		return RunNotify(arguments)
	case "memory":
		return RunMemory(arguments)
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

func (r *Registry) ConfirmGitWrite() bool {
	return r.cfg.ConfirmGitWrite
}

// GitWriteCheck reports whether a git tool call modifies the repo and needs
// confirmation, plus a human-readable description of the command for the prompt.
func (r *Registry) GitWriteCheck(arguments string) (needsConfirm bool, prompt string) {
	var a GitArgs
	if err := json.Unmarshal([]byte(arguments), &a); err != nil {
		return false, arguments
	}
	desc := strings.TrimSpace("git " + a.Subcommand + " " + strings.Join(a.Args, " "))
	return IsGitWriteSubcommand(a.Subcommand), desc
}

func (r *Registry) allowed(cmd string) bool {
	for _, allowed := range r.cfg.AllowedCommands {
		if strings.TrimSpace(allowed) == cmd {
			return true
		}
	}
	return false
}

func schema(fields map[string]string, required []string) map[string]any {
	props := map[string]any{}
	for k, t := range fields {
		props[k] = map[string]any{"type": t}
	}
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}
