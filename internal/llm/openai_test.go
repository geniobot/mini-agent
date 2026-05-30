package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIClient_ChatStream_text(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing auth header")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}`)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":" world"},"finish_reason":null}]}`)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`)
		fmt.Fprintln(w, `data: [DONE]`)
	}))
	defer srv.Close()

	client := NewOpenAI(srv.URL, "test-key", "test-model")
	req := ChatRequest{Model: "test-model", Stream: true}

	var chunks []ChatChunk
	err := client.ChatStream(context.Background(), req, func(c ChatChunk) error {
		chunks = append(chunks, c)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var content strings.Builder
	var done bool
	for _, c := range chunks {
		content.WriteString(c.Message.Content)
		if c.Done {
			done = true
		}
	}
	if content.String() != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", content.String())
	}
	if !done {
		t.Error("expected Done=true in final chunk")
	}
}

func TestOpenAIClient_ChatStream_error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer srv.Close()

	client := NewOpenAI(srv.URL, "bad-key", "model")
	err := client.ChatStream(context.Background(), ChatRequest{}, func(ChatChunk) error { return nil })
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got: %v", err)
	}
}

func TestOpenAIClient_ListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "gpt-4o"},
				{"id": "gpt-4o-mini"},
			},
		})
	}))
	defer srv.Close()

	client := NewOpenAI(srv.URL, "key", "model")
	models, err := client.ListModels()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Errorf("expected 2 models, got %d", len(models))
	}
	if models[0] != "gpt-4o" {
		t.Errorf("expected gpt-4o, got %q", models[0])
	}
}
