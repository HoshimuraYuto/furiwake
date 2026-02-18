package main

import (
	"encoding/json"
	"testing"
)

func TestNormalizeSystemText(t *testing.T) {
	got := NormalizeSystemText("  hello  ")
	if got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}

	got = NormalizeSystemText([]AnthropicContentBlock{
		{Type: "text", Text: "first"},
		{Type: "text", Text: "second"},
	})
	if got != "first\nsecond" {
		t.Fatalf("unexpected system text: %q", got)
	}
}

func TestTranslateMessagesToOpenAI_WithToolUseAndResult(t *testing.T) {
	messages := []AnthropicMessage{
		{
			Role: "assistant",
			Content: []AnthropicContentBlock{
				{Type: "text", Text: "calling tool"},
				{Type: "tool_use", ID: "tool_1", Name: "search", Input: json.RawMessage(`{"q":"golang"}`)},
			},
		},
		{
			Role: "user",
			Content: []AnthropicContentBlock{
				{Type: "tool_result", ToolUseID: "tool_1", Content: "result-body"},
			},
		},
	}

	out := TranslateMessagesToOpenAI("@route:openai", messages)
	if len(out) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(out))
	}
	if out[0].Role != "system" {
		t.Fatalf("expected system at index0, got %s", out[0].Role)
	}
	if out[1].Role != "assistant" || len(out[1].ToolCalls) != 1 {
		t.Fatalf("assistant message/tool_calls translation failed: %+v", out[1])
	}
	if out[2].Role != "tool" || out[2].ToolCallID != "tool_1" {
		t.Fatalf("tool result translation failed: %+v", out[2])
	}
}

func TestNormalizeContentToBlocks_FromMap(t *testing.T) {
	in := []map[string]interface{}{
		{"type": "text", "text": "x"},
	}
	blocks := normalizeContentToBlocks(in)
	if len(blocks) != 1 || blocks[0].Text != "x" {
		t.Fatalf("unexpected blocks: %+v", blocks)
	}
}

func TestTranslateMessagesToOpenAI_UserToolResultOrder(t *testing.T) {
	messages := []AnthropicMessage{
		{
			Role: "user",
			Content: []AnthropicContentBlock{
				{Type: "text", Text: "before"},
				{Type: "tool_result", ToolUseID: "tool_1", Content: "tool output"},
				{Type: "text", Text: "after"},
			},
		},
	}

	out := TranslateMessagesToOpenAI(nil, messages)
	if len(out) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(out))
	}
	if out[0].Role != "user" || out[0].Content != "before" {
		t.Fatalf("unexpected first message: %+v", out[0])
	}
	if out[1].Role != "tool" || out[1].ToolCallID != "tool_1" || out[1].Content != "tool output" {
		t.Fatalf("unexpected second message: %+v", out[1])
	}
	if out[2].Role != "user" || out[2].Content != "after" {
		t.Fatalf("unexpected third message: %+v", out[2])
	}
}
