package tools

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type JsonQueryArgs struct {
	JSON string `json:"json"`
	Path string `json:"path"`
}

// JsonQuery extracts a value from a JSON string using a dot-path expression.
// Supported: .key  .key.nested  .array[0]  .array[0].field
// Returns "null" when the path does not exist (not an error).
func JsonQuery(rawJSON, path string) (string, error) {
	var root any
	if err := json.Unmarshal([]byte(rawJSON), &root); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}

	path = strings.TrimPrefix(path, ".")
	if path == "" {
		b, _ := json.Marshal(root)
		return string(b), nil
	}

	result, err := jsonTraverse(root, path)
	if err != nil {
		return "null", nil
	}
	b, err := json.Marshal(result)
	if err != nil {
		return "null", nil
	}
	return string(b), nil
}

func jsonTraverse(v any, path string) (any, error) {
	if path == "" {
		return v, nil
	}

	var segment, rest string
	if idx := strings.IndexByte(path, '.'); idx >= 0 {
		segment, rest = path[:idx], path[idx+1:]
	} else {
		segment = path
	}

	if bOpen := strings.IndexByte(segment, '['); bOpen >= 0 {
		bClose := strings.IndexByte(segment, ']')
		if bClose < 0 {
			return nil, fmt.Errorf("malformed path segment: %s", segment)
		}
		idxVal, err := strconv.Atoi(segment[bOpen+1 : bClose])
		if err != nil {
			return nil, fmt.Errorf("invalid array index: %s", segment[bOpen+1:bClose])
		}

		key := segment[:bOpen]
		if key != "" {
			m, ok := v.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("not an object at %q", key)
			}
			v, ok = m[key]
			if !ok {
				return nil, fmt.Errorf("key %q not found", key)
			}
		}

		arr, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("not an array")
		}
		if idxVal < 0 || idxVal >= len(arr) {
			return nil, fmt.Errorf("index %d out of range (len %d)", idxVal, len(arr))
		}
		return jsonTraverse(arr[idxVal], rest)
	}

	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("not an object at segment %q", segment)
	}
	val, ok := m[segment]
	if !ok {
		return nil, fmt.Errorf("key %q not found", segment)
	}
	return jsonTraverse(val, rest)
}
