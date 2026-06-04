package tools

import (
	"testing"
)

func TestRunNotify_MissingBody(t *testing.T) {
	_, err := RunNotify(`{"title":"test"}`)
	if err == nil {
		t.Fatal("expected error for missing body, got nil")
	}
}

func TestRunNotify_InvalidJSON(t *testing.T) {
	_, err := RunNotify(`not json`)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestRunNotify_DefaultTitle(t *testing.T) {
	// When notify binary is absent this should succeed silently (no-op).
	// When it is present it sends a real notification — acceptable in CI.
	_, err := RunNotify(`{"body":"hello from test"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNotifyAvailable_DoesNotPanic(t *testing.T) {
	// Just verify the function doesn't panic regardless of environment.
	_ = NotifyAvailable()
}
