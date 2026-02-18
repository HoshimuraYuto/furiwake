package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranslateAnthropicToResponses(t *testing.T) {
	req := AnthropicMessageRequest{
		System:    "sys",
		Stream:    true,
		MaxTokens: 123,
		Messages: []AnthropicMessage{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "world"},
		},
	}
	out := translateAnthropicToResponses(req, "gpt-5", "medium")
	if out.Model != "gpt-5" || !out.Stream {
		t.Fatalf("unexpected request: %+v", out)
	}
	if len(out.Input) != 2 { // user + assistant (system is instructions)
		t.Fatalf("unexpected input size: %d", len(out.Input))
	}
	if out.Instructions != "sys" {
		t.Fatalf("unexpected instructions: %q", out.Instructions)
	}
	if out.Store != false {
		t.Fatalf("store should be false")
	}
	if out.Reasoning == nil || out.Reasoning.Summary != "auto" {
		t.Fatalf("unexpected reasoning config: %+v", out.Reasoning)
	}
	if out.Reasoning.Effort != "medium" {
		t.Fatalf("unexpected reasoning effort: %+v", out.Reasoning)
	}
	if out.Input[0].Role != "user" || out.Input[1].Role != "assistant" {
		t.Fatalf("unexpected roles: %+v", out.Input)
	}
}

func TestConvertResponsesJSONToAnthropic(t *testing.T) {
	raw := []byte(`{
	  "output_text":"final answer",
	  "usage":{"input_tokens":11,"output_tokens":22}
	}`)
	out := convertResponsesJSONToAnthropic(raw, "claude-spoof")
	if out.Model != "claude-spoof" {
		t.Fatalf("unexpected model: %s", out.Model)
	}
	if len(out.Content) != 1 || out.Content[0].Text != "final answer" {
		t.Fatalf("unexpected content: %+v", out.Content)
	}
	if out.Usage.InputTokens != 11 || out.Usage.OutputTokens != 22 {
		t.Fatalf("unexpected usage: %+v", out.Usage)
	}
}

func TestExtractResponseText_FromOutputArray(t *testing.T) {
	payload := map[string]interface{}{
		"output": []interface{}{
			map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{"text": "line1"},
					map[string]interface{}{"text": "line2"},
				},
			},
		},
	}
	got := extractResponseText(payload)
	if got != "line1\nline2" {
		t.Fatalf("unexpected text: %q", got)
	}
}

func TestTranslateAnthropicToResponses_WithToolBlocks(t *testing.T) {
	req := AnthropicMessageRequest{
		System: "sys",
		Messages: []AnthropicMessage{
			{
				Role: "assistant",
				Content: []AnthropicContentBlock{
					{Type: "text", Text: "calling"},
					{Type: "tool_use", ID: "tool_1", Name: "search", Input: []byte(`{"q":"go"}`)},
				},
			},
			{
				Role: "user",
				Content: []AnthropicContentBlock{
					{Type: "tool_result", ToolUseID: "tool_1", Content: "done"},
				},
			},
		},
		Tools: []AnthropicTool{
			{Name: "search", InputSchema: []byte(`{"type":"object"}`)},
		},
		ToolChoice: map[string]interface{}{"type": "tool", "name": "search"},
	}
	out := translateAnthropicToResponses(req, "gpt-5", "")
	if len(out.Input) != 3 {
		t.Fatalf("unexpected input size: %d", len(out.Input))
	}
	if out.Input[1].Type != "function_call" || out.Input[2].Type != "function_call_output" {
		t.Fatalf("tool translation mismatch: %+v", out.Input)
	}
	if len(out.Tools) != 1 || out.Tools[0].Name != "search" {
		t.Fatalf("tools mismatch: %+v", out.Tools)
	}
}

func TestConvertResponsesJSONToAnthropic_WithToolUse(t *testing.T) {
	raw := []byte(`{
	  "status":"completed",
	  "output":[
	    {"type":"message","content":[{"text":"answer"}]},
	    {"type":"function_call","call_id":"tool_1","name":"search","arguments":"{\"q\":\"go\"}"}
	  ]
	}`)
	out := convertResponsesJSONToAnthropic(raw, "claude-spoof")
	if len(out.Content) != 2 {
		t.Fatalf("unexpected content blocks: %+v", out.Content)
	}
	if out.Content[1].Type != "tool_use" || out.StopReason != "tool_use" {
		t.Fatalf("expected tool_use stop reason/content: %+v", out)
	}
}

func TestConvertResponsesStreamToAnthropic_ToolUse(t *testing.T) {
	stream := strings.Join([]string{
		"event: response.created",
		`data: {"type":"response.created","response":{"id":"resp_1"}}`,
		"",
		"event: response.output_item.added",
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","call_id":"tool_1","name":"search"}}`,
		"",
		"event: response.function_call_arguments.delta",
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"q\":\"go\"}"}`,
		"",
		"event: response.function_call_arguments.done",
		`data: {"type":"response.function_call_arguments.done","output_index":0}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"output_tokens":7},"output":[{"type":"function_call"}]}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	rr := httptest.NewRecorder()
	err := convertResponsesStreamToAnthropic(rr, strings.NewReader(stream), "claude-spoof", NewLogger())
	if err != nil {
		t.Fatalf("convertResponsesStreamToAnthropic error: %v", err)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"type":"input_json_delta"`) {
		t.Fatalf("expected input_json_delta in stream: %s", body)
	}
	if !strings.Contains(body, `"stop_reason":"tool_use"`) {
		t.Fatalf("expected tool_use stop reason: %s", body)
	}
}

func TestConvertResponsesStreamToAnthropic_MultiLineDataEvent(t *testing.T) {
	stream := strings.Join([]string{
		"event: response.created",
		`data: {"type":"response.created","response":{"id":"resp_1"}}`,
		"",
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","output_index":0,`,
		`data: "content_index":0,"delta":"hello"}`,
		"",
		"event: response.output_text.done",
		`data: {"type":"response.output_text.done","output_index":0,"content_index":0}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"output_tokens":5}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	rr := httptest.NewRecorder()
	err := convertResponsesStreamToAnthropic(rr, strings.NewReader(stream), "claude-spoof", NewLogger())
	if err != nil {
		t.Fatalf("convertResponsesStreamToAnthropic error: %v", err)
	}

	body := rr.Body.String()
	if !strings.Contains(body, `"text":"hello"`) {
		t.Fatalf("expected translated text delta: %s", body)
	}
	if !strings.Contains(body, `"output_tokens":5`) {
		t.Fatalf("expected usage translation: %s", body)
	}
}

func TestCloseOpenResponseBlocks_SortedOrder(t *testing.T) {
	state := &responsesStreamState{
		openBlocks: map[int]bool{
			30: true,
			2:  true,
			25: true,
			1:  true,
			10: true,
			4:  true,
		},
	}

	var b strings.Builder
	if err := closeOpenResponseBlocks(&b, state); err != nil {
		t.Fatalf("closeOpenResponseBlocks error: %v", err)
	}

	got := make([]int, 0, 6)
	for _, line := range strings.Split(b.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &payload); err != nil {
			t.Fatalf("invalid sse payload: %v", err)
		}
		if v, ok := payload["index"].(float64); ok {
			got = append(got, int(v))
		}
	}

	want := []int{1, 2, 4, 10, 25, 30}
	if len(got) != len(want) {
		t.Fatalf("unexpected stop index count: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected stop order: got=%v want=%v", got, want)
		}
	}
}
