package main

import (
	"encoding/json"
	"testing"
)

func TestTranslateAnthropicToOpenAI(t *testing.T) {
	req := AnthropicMessageRequest{
		System:    "sys",
		Stream:    true,
		MaxTokens: 123,
		Messages: []AnthropicMessage{
			{Role: "user", Content: "hello"},
		},
		Tools: []AnthropicTool{
			{
				Name:        "search",
				Description: "search docs",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`),
			},
		},
		ToolChoice: map[string]interface{}{"type": "any"},
	}
	out := TranslateAnthropicToOpenAI(req, "gpt-5-mini")
	if out.Model != "gpt-5-mini" {
		t.Fatalf("unexpected model: %s", out.Model)
	}
	if !out.Stream || out.MaxTokens != 123 {
		t.Fatalf("stream/max_tokens mismatch: %+v", out)
	}
	if len(out.Messages) != 2 { // system + user
		t.Fatalf("unexpected messages count: %d", len(out.Messages))
	}
	if len(out.Tools) != 1 || out.Tools[0].Function.Name != "search" {
		t.Fatalf("tools translation mismatch: %+v", out.Tools)
	}
	if out.ToolChoice != "required" {
		t.Fatalf("tool_choice any should map to required: %#v", out.ToolChoice)
	}
}

func TestTranslateToolChoice_Tool(t *testing.T) {
	got := translateToolChoice(map[string]interface{}{
		"type": "tool",
		"name": "search",
	})
	m, ok := got.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected type: %T", got)
	}
	if m["type"] != "function" {
		t.Fatalf("unexpected type field: %#v", m["type"])
	}
}
