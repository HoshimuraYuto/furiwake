package main

import "testing"

func testConfig() *Config {
	return &Config{
		DefaultProvider: "anthropic",
		Providers: map[string]ProviderConfig{
			"anthropic": {Type: ProviderTypePassthrough, URL: "https://api.anthropic.com"},
			"openai":    {Type: ProviderTypeOpenAI, URL: "https://api.openai.com", Model: "gpt-5-mini"},
			"codex":     {Type: ProviderTypeChatGPT, URL: "https://chatgpt.com/backend-api/codex/responses", Model: "gpt-5"},
		},
	}
}

func TestExtractRouteName(t *testing.T) {
	got := ExtractRouteName("hello\n@route:codex\nworld")
	if got != "codex" {
		t.Fatalf("expected codex, got %q", got)
	}

	got = ExtractRouteName([]AnthropicContentBlock{
		{Type: "text", Text: "abc"},
		{Type: "text", Text: "@route:openai"},
	})
	if got != "openai" {
		t.Fatalf("expected openai, got %q", got)
	}
}

func TestExtractModelName(t *testing.T) {
	got := ExtractModelName("hello\n@model:gpt-5.3-codex\nworld")
	if got != "gpt-5.3-codex" {
		t.Fatalf("expected gpt-5.3-codex, got %q", got)
	}

	got = ExtractModelName([]AnthropicContentBlock{
		{Type: "text", Text: "abc"},
		{Type: "text", Text: "<!-- @model:qwen2.5-coder:32b -->"},
	})
	if got != "qwen2.5-coder:32b" {
		t.Fatalf("expected qwen2.5-coder:32b, got %q", got)
	}
}

func TestExtractReasoningEffort(t *testing.T) {
	got := ExtractReasoningEffort("hello\n@reasoning:high\nworld")
	if got != "high" {
		t.Fatalf("expected high, got %q", got)
	}

	got = ExtractReasoningEffort([]AnthropicContentBlock{
		{Type: "text", Text: "<!-- @reasoning:xhigh -->"},
	})
	if got != "xhigh" {
		t.Fatalf("expected xhigh, got %q", got)
	}
}

func TestResolveProvider_Default(t *testing.T) {
	name, provider, err := ResolveProvider("no marker", nil, testConfig())
	if err != nil {
		t.Fatalf("ResolveProvider error: %v", err)
	}
	if name != "anthropic" {
		t.Fatalf("unexpected provider name: %s", name)
	}
	if provider.Type != ProviderTypePassthrough {
		t.Fatalf("unexpected type: %s", provider.Type)
	}
}

func TestResolveProvider_ByMarker(t *testing.T) {
	name, provider, err := ResolveProvider("@route:openai", nil, testConfig())
	if err != nil {
		t.Fatalf("ResolveProvider error: %v", err)
	}
	if name != "openai" {
		t.Fatalf("unexpected provider name: %s", name)
	}
	if provider.Type != ProviderTypeOpenAI {
		t.Fatalf("unexpected type: %s", provider.Type)
	}
}

func TestResolveProvider_ByMessageMarker(t *testing.T) {
	messages := []AnthropicMessage{
		{Role: "user", Content: "<!-- @route:codex -->\nPlease implement this"},
	}
	name, provider, err := ResolveProvider("no marker here", messages, testConfig())
	if err != nil {
		t.Fatalf("ResolveProvider error: %v", err)
	}
	if name != "codex" {
		t.Fatalf("expected codex, got %s", name)
	}
	if provider.Type != ProviderTypeChatGPT {
		t.Fatalf("unexpected type: %s", provider.Type)
	}
}

func TestResolveProvider_SystemTakesPrecedence(t *testing.T) {
	messages := []AnthropicMessage{
		{Role: "user", Content: "@route:codex"},
	}
	name, _, err := ResolveProvider("@route:openai", messages, testConfig())
	if err != nil {
		t.Fatalf("ResolveProvider error: %v", err)
	}
	if name != "openai" {
		t.Fatalf("expected system route (openai) to take precedence, got %s", name)
	}
}

func TestResolveProvider_ErrorNotFound(t *testing.T) {
	_, _, err := ResolveProvider("@route:not-found", nil, testConfig())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolveProviderAndModel_DefaultFallback(t *testing.T) {
	name, provider, model, err := ResolveProviderAndModel("@route:openai", nil, testConfig())
	if err != nil {
		t.Fatalf("ResolveProviderAndModel error: %v", err)
	}
	if name != "openai" {
		t.Fatalf("unexpected provider name: %s", name)
	}
	if provider.Type != ProviderTypeOpenAI {
		t.Fatalf("unexpected type: %s", provider.Type)
	}
	if model != "gpt-5-mini" {
		t.Fatalf("expected default model gpt-5-mini, got %s", model)
	}
}

func TestResolveProviderAndModel_BySystemMarker(t *testing.T) {
	_, _, model, err := ResolveProviderAndModel("@route:openai\n@model:gpt-4.1", nil, testConfig())
	if err != nil {
		t.Fatalf("ResolveProviderAndModel error: %v", err)
	}
	if model != "gpt-4.1" {
		t.Fatalf("expected model override gpt-4.1, got %s", model)
	}
}

func TestResolveProviderAndModel_BySystemMarkersInSingleLine(t *testing.T) {
	name, provider, model, err := ResolveProviderAndModel("<!-- @route:codex @model:gpt-5-mini -->", nil, testConfig())
	if err != nil {
		t.Fatalf("ResolveProviderAndModel error: %v", err)
	}
	if name != "codex" {
		t.Fatalf("expected provider codex, got %s", name)
	}
	if provider.Type != ProviderTypeChatGPT {
		t.Fatalf("unexpected provider type: %s", provider.Type)
	}
	if model != "gpt-5-mini" {
		t.Fatalf("expected model override gpt-5-mini, got %s", model)
	}
}

func TestResolveProviderAndModel_ByMessageMarker(t *testing.T) {
	messages := []AnthropicMessage{
		{Role: "user", Content: "<!-- @model:gpt-5-mini -->\nPlease implement this"},
	}
	_, _, model, err := ResolveProviderAndModel("@route:codex", messages, testConfig())
	if err != nil {
		t.Fatalf("ResolveProviderAndModel error: %v", err)
	}
	if model != "gpt-5-mini" {
		t.Fatalf("expected model override gpt-5-mini, got %s", model)
	}
}

func TestResolveProviderAndModel_ByMessageMarkersInSingleLine(t *testing.T) {
	messages := []AnthropicMessage{
		{Role: "user", Content: "<!-- @route:openai @model:gpt-4.1 -->\nPlease implement this"},
	}
	name, provider, model, err := ResolveProviderAndModel("no marker here", messages, testConfig())
	if err != nil {
		t.Fatalf("ResolveProviderAndModel error: %v", err)
	}
	if name != "openai" {
		t.Fatalf("expected provider openai, got %s", name)
	}
	if provider.Type != ProviderTypeOpenAI {
		t.Fatalf("unexpected provider type: %s", provider.Type)
	}
	if model != "gpt-4.1" {
		t.Fatalf("expected model override gpt-4.1, got %s", model)
	}
}

func TestResolveProviderAndModel_SystemModelTakesPrecedence(t *testing.T) {
	messages := []AnthropicMessage{
		{Role: "user", Content: "<!-- @model:gpt-5-mini -->"},
	}
	_, _, model, err := ResolveProviderAndModel("@route:codex\n@model:gpt-5.3-codex", messages, testConfig())
	if err != nil {
		t.Fatalf("ResolveProviderAndModel error: %v", err)
	}
	if model != "gpt-5.3-codex" {
		t.Fatalf("expected system model to take precedence, got %s", model)
	}
}

func TestResolveReasoningEffort_DefaultFallback(t *testing.T) {
	effort, err := ResolveReasoningEffort("no marker", nil, "medium")
	if err != nil {
		t.Fatalf("ResolveReasoningEffort error: %v", err)
	}
	if effort != "medium" {
		t.Fatalf("expected medium, got %s", effort)
	}
}

func TestResolveReasoningEffort_SystemMarker(t *testing.T) {
	effort, err := ResolveReasoningEffort("<!-- @reasoning:high -->", nil, "medium")
	if err != nil {
		t.Fatalf("ResolveReasoningEffort error: %v", err)
	}
	if effort != "high" {
		t.Fatalf("expected high, got %s", effort)
	}
}

func TestResolveReasoningEffort_SystemTakesPrecedenceOverMessage(t *testing.T) {
	messages := []AnthropicMessage{
		{Role: "user", Content: "<!-- @reasoning:low -->"},
	}
	effort, err := ResolveReasoningEffort("<!-- @reasoning:high -->", messages, "medium")
	if err != nil {
		t.Fatalf("ResolveReasoningEffort error: %v", err)
	}
	if effort != "high" {
		t.Fatalf("expected high, got %s", effort)
	}
}

func TestResolveReasoningEffort_InvalidMarker(t *testing.T) {
	_, err := ResolveReasoningEffort("<!-- @reasoning:ultra -->", nil, "medium")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
