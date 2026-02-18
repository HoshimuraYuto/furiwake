package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConvertOpenAINonStreamToAnthropic(t *testing.T) {
	resp := OpenAIChatResponse{
		Model: "gpt-5-mini",
		Choices: []OpenAIChoice{
			{
				Message: OpenAIMessage{
					Role:    "assistant",
					Content: "hello",
					ToolCalls: []OpenAIToolCall{
						{
							ID:   "call_1",
							Type: "function",
							Function: OpenAIToolFunction{
								Name:      "search",
								Arguments: `{"q":"x"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: OpenAIUsage{PromptTokens: 10, CompletionTokens: 20},
	}
	out := convertOpenAINonStreamToAnthropic(resp, "claude-spoof")
	if out.Model != "claude-spoof" {
		t.Fatalf("unexpected model: %s", out.Model)
	}
	if out.StopReason != "tool_use" {
		t.Fatalf("unexpected stop reason: %s", out.StopReason)
	}
	if len(out.Content) != 2 {
		t.Fatalf("unexpected content length: %d", len(out.Content))
	}
	if out.Content[1].Type != "tool_use" || out.Content[1].Name != "search" {
		t.Fatalf("unexpected tool block: %+v", out.Content[1])
	}
}

func TestWriteAnthropicSSEEvent(t *testing.T) {
	var b strings.Builder
	err := writeAnthropicSSEEvent(&b, "message_stop", map[string]interface{}{"type": "message_stop"})
	if err != nil {
		t.Fatalf("writeAnthropicSSEEvent error: %v", err)
	}
	if !strings.Contains(b.String(), "event: message_stop") {
		t.Fatalf("unexpected SSE output: %s", b.String())
	}
	if !strings.Contains(b.String(), `"type":"message_stop"`) {
		t.Fatalf("unexpected payload: %s", b.String())
	}
}

func TestMapFinishReason(t *testing.T) {
	cases := map[string]string{
		"stop":       "end_turn",
		"length":     "max_tokens",
		"tool_calls": "tool_use",
		"other":      "end_turn",
	}
	for in, want := range cases {
		if got := mapFinishReason(in); got != want {
			t.Fatalf("mapFinishReason(%q)=%q want=%q", in, got, want)
		}
	}
}

func TestSafeJSONRawMessage_Valid(t *testing.T) {
	raw := safeJSONRawMessage(`{"ok":true}`)
	var v map[string]bool
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("invalid json after safeJSONRawMessage: %v", err)
	}
	if !v["ok"] {
		t.Fatalf("unexpected payload: %+v", v)
	}
}

func TestConvertOpenAIStreamToAnthropic_WithToolCalls(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"hello"}}]}`,
		"",
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"search"}}]}}]}`,
		"",
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"type":"function","function":{"arguments":"{\"q\":\"go\"}"}}]}}]}`,
		"",
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"completion_tokens":12}}`,
		"",
		`data: [DONE]`,
		"",
	}, "\n")

	rr := httptest.NewRecorder()
	err := convertOpenAIStreamToAnthropic(rr, strings.NewReader(stream), "claude-spoof")
	if err != nil {
		t.Fatalf("convertOpenAIStreamToAnthropic error: %v", err)
	}

	body := rr.Body.String()
	if !strings.Contains(body, `"type":"text_delta"`) || !strings.Contains(body, `"text":"hello"`) {
		t.Fatalf("missing text delta: %s", body)
	}
	if !strings.Contains(body, `"type":"tool_use"`) || !strings.Contains(body, `"id":"call_1"`) || !strings.Contains(body, `"name":"search"`) {
		t.Fatalf("missing tool_use block: %s", body)
	}
	if !strings.Contains(body, `"type":"input_json_delta"`) {
		t.Fatalf("missing input_json_delta: %s", body)
	}
	if !strings.Contains(body, `"stop_reason":"tool_use"`) {
		t.Fatalf("missing tool_use stop reason: %s", body)
	}
}

func TestConvertOpenAIStreamToAnthropic_MultiLineDataEvent(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"hello"}}],`,
		`data: "usage":{"completion_tokens":3}}`,
		"",
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		"",
		`data: [DONE]`,
		"",
	}, "\n")

	rr := httptest.NewRecorder()
	err := convertOpenAIStreamToAnthropic(rr, strings.NewReader(stream), "claude-spoof")
	if err != nil {
		t.Fatalf("convertOpenAIStreamToAnthropic error: %v", err)
	}

	body := rr.Body.String()
	if !strings.Contains(body, `"text":"hello"`) {
		t.Fatalf("missing translated text delta: %s", body)
	}
	if !strings.Contains(body, `"output_tokens":3`) {
		t.Fatalf("missing usage translation: %s", body)
	}
}
