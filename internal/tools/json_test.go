package tools

import (
	"testing"
)

func TestJsonQuery(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		path    string
		want    string
		wantErr bool
	}{
		{
			name: "simple key",
			json: `{"name":"Alice","age":30}`,
			path: ".name",
			want: `"Alice"`,
		},
		{
			name: "integer value",
			json: `{"name":"Alice","age":30}`,
			path: ".age",
			want: `30`,
		},
		{
			name: "nested key",
			json: `{"user":{"name":"Alice","email":"alice@example.com"}}`,
			path: ".user.name",
			want: `"Alice"`,
		},
		{
			name: "array index",
			json: `{"items":["a","b","c"]}`,
			path: ".items[1]",
			want: `"b"`,
		},
		{
			name: "array index field",
			json: `{"users":[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}]}`,
			path: ".users[0].name",
			want: `"Alice"`,
		},
		{
			name: "second array element",
			json: `{"users":[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}]}`,
			path: ".users[1].id",
			want: `2`,
		},
		{
			name: "root array index",
			json: `[10,20,30]`,
			path: ".[1]",
			want: `20`,
		},
		{
			name: "missing key returns null",
			json: `{"name":"Alice"}`,
			path: ".missing",
			want: `null`,
		},
		{
			name: "missing nested key returns null",
			json: `{"user":{"name":"Alice"}}`,
			path: ".user.email",
			want: `null`,
		},
		{
			name: "index out of range returns null",
			json: `{"items":["a"]}`,
			path: ".items[5]",
			want: `null`,
		},
		{
			name: "empty path returns whole doc",
			json: `{"name":"Alice"}`,
			path: ".",
			want: `{"name":"Alice"}`,
		},
		{
			name: "boolean value",
			json: `{"ok":true}`,
			path: ".ok",
			want: `true`,
		},
		{
			name: "null value",
			json: `{"val":null}`,
			path: ".val",
			want: `null`,
		},
		{
			name:    "invalid json returns error",
			json:    `not json`,
			path:    ".key",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := JsonQuery(tt.json, tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("JsonQuery(%q, %q) = %q, want %q", tt.json, tt.path, got, tt.want)
			}
		})
	}
}
