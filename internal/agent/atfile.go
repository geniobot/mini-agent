package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// atFileRe matches @path tokens: @main.go, @src/util.py, @~/notes.txt
var atFileRe = regexp.MustCompile(`@([\w./~\-]+)`)

// expandAtFiles scans input for @path tokens, reads each file that exists,
// and prepends their contents before the original message. Tokens for files
// that cannot be read are left unchanged so the model still sees the reference.
func expandAtFiles(input string) string {
	matches := atFileRe.FindAllStringSubmatch(input, -1)
	if len(matches) == 0 {
		return input
	}

	var preamble strings.Builder
	for _, m := range matches {
		rawPath := m[1]
		path := rawPath
		if strings.HasPrefix(path, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				path = filepath.Join(home, path[2:])
			}
		}
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		fmt.Fprintf(&preamble, "=== %s ===\n%s\n\n", rawPath, strings.TrimRight(string(content), "\n"))
	}
	if preamble.Len() == 0 {
		return input
	}
	return preamble.String() + input
}
