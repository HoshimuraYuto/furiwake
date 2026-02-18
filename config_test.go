package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "furiwake.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	return path
}

func TestLoadConfig_Success(t *testing.T) {
	path := writeTempConfig(t, `
listen: ":9999"
spoof_model: "claude-test"
default_provider: "anthropic"
timeout_seconds: 120
providers:
  anthropic:
    type: passthrough
    url: "https://api.anthropic.com"
    auth:
      type: none
  openai:
    type: openai
    url: "https://api.openai.com/v1/chat/completions"
    model: "gpt-5-mini"
    reasoning_effort: "HIGH"
    auth:
      type: bearer
      token_env: OPENAI_API_KEY
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.Listen != ":9999" {
		t.Fatalf("unexpected listen: %s", cfg.Listen)
	}
	if cfg.SpoofModel != "claude-test" {
		t.Fatalf("unexpected spoof model: %s", cfg.SpoofModel)
	}
	if cfg.DefaultProvider != "anthropic" {
		t.Fatalf("unexpected default provider: %s", cfg.DefaultProvider)
	}
	if got := cfg.Providers["openai"].Auth.Type; got != AuthTypeBearer {
		t.Fatalf("unexpected auth type normalization: %s", got)
	}
	if got := cfg.Providers["openai"].ReasoningEffort; got != "high" {
		t.Fatalf("unexpected reasoning_effort normalization: %s", got)
	}
}

func TestLoadConfig_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "missing listen",
			yaml: `
spoof_model: "claude-test"
default_provider: "anthropic"
timeout_seconds: 300
providers:
  anthropic:
    type: passthrough
    url: "https://api.anthropic.com"
`,
			wantErr: "listen is required",
		},
		{
			name: "missing spoof_model",
			yaml: `
listen: ":9999"
default_provider: "anthropic"
timeout_seconds: 300
providers:
  anthropic:
    type: passthrough
    url: "https://api.anthropic.com"
`,
			wantErr: "spoof_model is required",
		},
		{
			name: "missing default_provider",
			yaml: `
listen: ":9999"
spoof_model: "claude-test"
timeout_seconds: 300
providers:
  anthropic:
    type: passthrough
    url: "https://api.anthropic.com"
`,
			wantErr: "default_provider is required",
		},
		{
			name: "missing timeout_seconds",
			yaml: `
listen: ":9999"
spoof_model: "claude-test"
default_provider: "anthropic"
providers:
  anthropic:
    type: passthrough
    url: "https://api.anthropic.com"
`,
			wantErr: "timeout_seconds is required and must be > 0",
		},
		{
			name: "all missing",
			yaml: `
providers:
  anthropic:
    type: passthrough
    url: "https://api.anthropic.com"
`,
			wantErr: "listen is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempConfig(t, tt.yaml)
			_, err := LoadConfig(path)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestLoadConfig_InvalidProviderType(t *testing.T) {
	path := writeTempConfig(t, `
listen: ":9999"
spoof_model: "claude-test"
default_provider: bad
timeout_seconds: 300
providers:
  bad:
    type: unknown
    url: "https://example.com"
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "must be one of passthrough/openai/chatgpt") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfig_InvalidReasoningEffort(t *testing.T) {
	path := writeTempConfig(t, `
listen: ":9999"
spoof_model: "claude-test"
default_provider: codex
timeout_seconds: 300
providers:
  codex:
    type: chatgpt
    url: "https://chatgpt.com/backend-api/codex/responses"
    model: "gpt-5-codex"
    reasoning_effort: "ultra"
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "reasoning_effort") {
		t.Fatalf("unexpected error: %v", err)
	}
}
