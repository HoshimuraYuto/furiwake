package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleMessages_OpenAINonStream(t *testing.T) {
	var seenModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req OpenAIChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		seenModel = req.Model
		_ = json.NewEncoder(w).Encode(OpenAIChatResponse{
			ID:    "chatcmpl_1",
			Model: "gpt-5-mini",
			Choices: []OpenAIChoice{
				{
					Index: 0,
					Message: OpenAIMessage{
						Role:    "assistant",
						Content: "hello from openai",
					},
					FinishReason: "stop",
				},
			},
			Usage: OpenAIUsage{PromptTokens: 5, CompletionTokens: 7},
		})
	}))
	defer upstream.Close()

	cfg := &Config{
		Listen:          ":0",
		SpoofModel:      "claude-spoof",
		DefaultProvider: "openai",
		Providers: map[string]ProviderConfig{
			"openai": {Type: ProviderTypeOpenAI, URL: upstream.URL, Model: "gpt-5-mini"},
		},
	}
	s := NewServer(cfg, NewLogger())
	s.client = upstream.Client()

	in := AnthropicMessageRequest{
		Model: "claude",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "hi"},
		},
		Stream: false,
	}
	body, _ := json.Marshal(in)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	s.handleMessages(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if seenModel != "gpt-5-mini" {
		t.Fatalf("unexpected upstream model: %s", seenModel)
	}
	var out AnthropicMessageResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if len(out.Content) == 0 || out.Content[0].Text != "hello from openai" {
		t.Fatalf("unexpected anthropic response: %+v", out)
	}
}

func TestHandleMessages_OpenAINonStream_ModelOverride(t *testing.T) {
	var seenModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req OpenAIChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		seenModel = req.Model
		_ = json.NewEncoder(w).Encode(OpenAIChatResponse{
			ID:    "chatcmpl_1",
			Model: req.Model,
			Choices: []OpenAIChoice{
				{
					Index: 0,
					Message: OpenAIMessage{
						Role:    "assistant",
						Content: "hello from openai",
					},
					FinishReason: "stop",
				},
			},
			Usage: OpenAIUsage{PromptTokens: 5, CompletionTokens: 7},
		})
	}))
	defer upstream.Close()

	cfg := &Config{
		Listen:          ":0",
		SpoofModel:      "claude-spoof",
		DefaultProvider: "openai",
		Providers: map[string]ProviderConfig{
			"openai": {Type: ProviderTypeOpenAI, URL: upstream.URL, Model: "gpt-5-mini"},
		},
	}
	s := NewServer(cfg, NewLogger())
	s.client = upstream.Client()

	in := AnthropicMessageRequest{
		Model:  "claude",
		System: "<!-- @route:openai @model:gpt-4.1 -->",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "hi"},
		},
		Stream: false,
	}
	body, _ := json.Marshal(in)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	s.handleMessages(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if seenModel != "gpt-4.1" {
		t.Fatalf("expected model override gpt-4.1, got %s", seenModel)
	}
}

func TestHandleMessages_OpenAIStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()

	cfg := &Config{
		Listen:          ":0",
		SpoofModel:      "claude-spoof",
		DefaultProvider: "openai",
		Providers: map[string]ProviderConfig{
			"openai": {Type: ProviderTypeOpenAI, URL: upstream.URL, Model: "gpt-5-mini"},
		},
	}
	s := NewServer(cfg, NewLogger())
	s.client = upstream.Client()

	in := AnthropicMessageRequest{
		Model: "claude",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "hi"},
		},
		Stream: true,
	}
	body, _ := json.Marshal(in)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	s.handleMessages(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	out := rr.Body.String()
	if !strings.Contains(out, "event: message_start") || !strings.Contains(out, "event: message_stop") {
		t.Fatalf("unexpected stream response: %s", out)
	}
}

func TestHandleMessages_ChatGPTNonStream(t *testing.T) {
	var seenModel string
	var seenReasoningEffort string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatGPTResponsesRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		seenModel = req.Model
		if req.Reasoning != nil {
			seenReasoningEffort = req.Reasoning.Effort
		}
		// Codex requires stream:true; respond with SSE including response.completed
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: response.completed\n")
		_, _ = io.WriteString(w, `data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","output_text":"hello from responses","output":[],"usage":{"input_tokens":2,"output_tokens":3}}}`+"\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()

	cfg := &Config{
		Listen:          ":0",
		SpoofModel:      "claude-spoof",
		DefaultProvider: "codex",
		Providers: map[string]ProviderConfig{
			"codex": {Type: ProviderTypeChatGPT, URL: upstream.URL, Model: "gpt-5-codex", ReasoningEffort: "medium"},
		},
	}
	s := NewServer(cfg, NewLogger())
	s.client = upstream.Client()

	in := AnthropicMessageRequest{
		Model: "claude",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "hi"},
		},
		Stream: false,
	}
	body, _ := json.Marshal(in)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	s.handleMessages(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var out AnthropicMessageResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if len(out.Content) == 0 || out.Content[0].Text != "hello from responses" {
		t.Fatalf("unexpected anthropic response: %+v", out)
	}
	if seenModel != "gpt-5-codex" {
		t.Fatalf("unexpected model: %s", seenModel)
	}
	if seenReasoningEffort != "medium" {
		t.Fatalf("expected default reasoning effort medium, got %s", seenReasoningEffort)
	}
}

func TestHandleMessages_ChatGPTNonStream_ModelOverride(t *testing.T) {
	var seenModel string
	var seenReasoningEffort string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatGPTResponsesRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		seenModel = req.Model
		if req.Reasoning != nil {
			seenReasoningEffort = req.Reasoning.Effort
		}
		// Codex requires stream:true; respond with SSE including response.completed
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: response.completed\n")
		_, _ = io.WriteString(w, `data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","output_text":"hello from responses","output":[],"usage":{"input_tokens":2,"output_tokens":3}}}`+"\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()

	cfg := &Config{
		Listen:          ":0",
		SpoofModel:      "claude-spoof",
		DefaultProvider: "codex",
		Providers: map[string]ProviderConfig{
			"codex": {Type: ProviderTypeChatGPT, URL: upstream.URL, Model: "gpt-5-codex", ReasoningEffort: "medium"},
		},
	}
	s := NewServer(cfg, NewLogger())
	s.client = upstream.Client()

	in := AnthropicMessageRequest{
		Model:  "claude",
		System: "<!-- @route:codex @model:gpt-5-mini @reasoning:high -->",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "hi"},
		},
		Stream: false,
	}
	body, _ := json.Marshal(in)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	s.handleMessages(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if seenModel != "gpt-5-mini" {
		t.Fatalf("expected model override gpt-5-mini, got %s", seenModel)
	}
	if seenReasoningEffort != "high" {
		t.Fatalf("expected reasoning override high, got %s", seenReasoningEffort)
	}
}

func TestHandleMessages_ChatGPTNonStream_InvalidReasoning(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called for invalid reasoning marker")
	}))
	defer upstream.Close()

	cfg := &Config{
		Listen:          ":0",
		SpoofModel:      "claude-spoof",
		DefaultProvider: "codex",
		Providers: map[string]ProviderConfig{
			"codex": {Type: ProviderTypeChatGPT, URL: upstream.URL, Model: "gpt-5-codex", ReasoningEffort: "medium"},
		},
	}
	s := NewServer(cfg, NewLogger())
	s.client = upstream.Client()

	in := AnthropicMessageRequest{
		Model:  "claude",
		System: "<!-- @route:codex @reasoning:ultra -->",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "hi"},
		},
		Stream: false,
	}
	body, _ := json.Marshal(in)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	s.handleMessages(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}
