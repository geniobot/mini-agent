package tools

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const webFetchMaxBytes = 32 * 1024 // 32 KB — enough for most pages, safe on constrained hardware

// Precompile patterns once at startup.
var (
	reScriptStyle = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)\s*>`)
	reHTMLTag     = regexp.MustCompile(`<[^>]+>`)
	reHorizSpace  = regexp.MustCompile(`[ \t]+`)
	reBlankLines  = regexp.MustCompile(`\n{3,}`)
)

// WebFetch fetches a URL and returns readable plain text.
// HTML tags are stripped, entities decoded, and whitespace normalised.
// Content is truncated at webFetchMaxBytes.
func WebFetch(rawURL string, timeoutSeconds int) (string, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	client := &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second}

	resp, err := client.Get(rawURL) //nolint:noctx — no context from caller; timeout covers it
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, rawURL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, webFetchMaxBytes+1))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	truncated := len(body) > webFetchMaxBytes
	if truncated {
		body = body[:webFetchMaxBytes]
	}

	text := stripHTML(string(body))
	if truncated {
		text += fmt.Sprintf("\n[truncated: content exceeded %d bytes]", webFetchMaxBytes)
	}
	return text, nil
}

// stripHTML converts HTML to readable plain text without external dependencies.
func stripHTML(s string) string {
	s = reScriptStyle.ReplaceAllString(s, " ") // remove <script>/<style> with their content
	s = reHTMLTag.ReplaceAllString(s, " ")     // remove remaining tags
	s = html.UnescapeString(s)                 // decode &amp; &lt; etc.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = reHorizSpace.ReplaceAllString(s, " ")  // collapse spaces/tabs on a line
	s = reBlankLines.ReplaceAllString(s, "\n\n") // collapse 3+ blank lines → 1
	return strings.TrimSpace(s)
}
