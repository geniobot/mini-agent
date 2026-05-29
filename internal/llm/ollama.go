package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OllamaClient implements Client, ModelLister, and Unloader against a local Ollama instance.
type OllamaClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewOllama(baseURL string) *OllamaClient {
	return &OllamaClient{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: 0}, // no global timeout; use per-request contexts
	}
}

func (c *OllamaClient) ChatStream(ctx context.Context, req ChatRequest, onChunk func(ChatChunk) error) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/chat", bytes.NewReader(body))
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
	buf := make([]byte, 0, 64*1024)
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
			return fmt.Errorf("%s", chunk.Error)
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

// ListModels returns the names of all models available in this Ollama instance.
func (c *OllamaClient) ListModels() ([]string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(c.BaseURL + "/api/tags")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	names := make([]string, len(payload.Models))
	for i, m := range payload.Models {
		names[i] = m.Name
	}
	return names, nil
}

// Unload evicts the named model from Ollama's RAM immediately.
func (c *OllamaClient) Unload(model string) error {
	payload := fmt.Sprintf(`{"model":%q,"keep_alive":0}`, model)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(c.BaseURL+"/api/generate", "application/json", strings.NewReader(payload))
	if err != nil {
		return err
	}
	io.ReadAll(resp.Body) //nolint
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("ollama http %d", resp.StatusCode)
	}
	return nil
}

// Ping checks whether an Ollama instance is reachable. Fast (5 s timeout).
func Ping(baseURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(strings.TrimRight(baseURL, "/") + "/api/tags")
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("ollama returned HTTP %d", resp.StatusCode)
	}
	return nil
}
