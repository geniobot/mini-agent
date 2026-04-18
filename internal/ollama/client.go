package ollama

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"mini-agent/internal/memory"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

type ToolSpec struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type ChatRequest struct {
	Model     string                 `json:"model"`
	Messages  []memory.Message       `json:"messages"`
	Stream    bool                   `json:"stream"`
	Tools     []ToolSpec             `json:"tools,omitempty"`
	Options   map[string]interface{} `json:"options,omitempty"`
	KeepAlive string                 `json:"keep_alive,omitempty"`
}

type ChatChunk struct {
	Message memory.Message `json:"message"`
	Done    bool           `json:"done"`
	Error   string         `json:"error,omitempty"`
}

func New(baseURL string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTPClient: &http.Client{Timeout: 0}}
}

func (c *Client) ChatStream(req ChatRequest, onChunk func(ChatChunk) error) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequest(http.MethodPost, c.BaseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama http %d: %s", resp.StatusCode, string(b))
	}

	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 1024*64)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var chunk ChatChunk
		if err := json.Unmarshal(line, &chunk); err != nil {
			return err
		}
		if chunk.Error != "" {
			return fmt.Errorf(chunk.Error)
		}
		if err := onChunk(chunk); err != nil {
			return err
		}
		if chunk.Done {
			break
		}
	}
	return scanner.Err()
}
