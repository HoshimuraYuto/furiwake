package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestServer() *Server {
	cfg := &Config{
		Listen:          ":0",
		SpoofModel:      "claude-test",
		DefaultProvider: "anthropic",
		Providers: map[string]ProviderConfig{
			"anthropic": {
				Type:  ProviderTypeOpenAI,
				URL:   "http://example.com",
				Model: "gpt-5-mini",
			},
		},
	}
	return NewServer(cfg, NewLogger())
}

func TestHandleHealth(t *testing.T) {
	s := newTestServer()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	s.handleHealth(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleCountTokens(t *testing.T) {
	s := newTestServer()
	reqBody := CountTokensRequest{
		Model:  "claude",
		System: "system text",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "hello world"},
		},
	}
	b, _ := json.Marshal(reqBody)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(b))
	s.handleCountTokens(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var out CountTokensResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if out.InputTokens < 1 {
		t.Fatalf("unexpected token count: %d", out.InputTokens)
	}
}

func TestEstimateInputTokens_MinimumOne(t *testing.T) {
	got := estimateInputTokens(nil, nil)
	if got != 1 {
		t.Fatalf("expected min 1, got %d", got)
	}
}

func TestHandleCountTokens_Passthrough(t *testing.T) {
	var seenPath string
	var seenHeader string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenHeader = r.Header.Get("x-api-key")
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_tokens":321}`))
	}))
	defer upstream.Close()

	cfg := &Config{
		Listen:          ":0",
		SpoofModel:      "claude-test",
		DefaultProvider: "anthropic",
		Providers: map[string]ProviderConfig{
			"anthropic": {
				Type: ProviderTypePassthrough,
				URL:  upstream.URL,
			},
		},
	}
	s := NewServer(cfg, NewLogger())

	reqBody := CountTokensRequest{
		Model:  "claude",
		System: "system",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "hello world"},
		},
	}
	b, _ := json.Marshal(reqBody)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(b))
	req.Header.Set("x-api-key", "test-key")

	s.handleCountTokens(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if seenPath != "/v1/messages/count_tokens" {
		t.Fatalf("unexpected path: %s", seenPath)
	}
	if seenHeader != "test-key" {
		t.Fatalf("header not forwarded, got: %s", seenHeader)
	}
	if rr.Body.String() != "{\"input_tokens\":321}" {
		t.Fatalf("unexpected passthrough body: %q", rr.Body.String())
	}
}

func TestHandleCountTokens_InvalidRoute(t *testing.T) {
	cfg := &Config{
		Listen:          ":0",
		SpoofModel:      "claude-test",
		DefaultProvider: "anthropic",
		Providers: map[string]ProviderConfig{
			"anthropic": {Type: ProviderTypeOpenAI, URL: "http://example.com", Model: "gpt-5-mini"},
		},
	}
	s := NewServer(cfg, NewLogger())

	body := []byte(`{"model":"claude","system":"@route:not-found","messages":[{"role":"user","content":"x"}]}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(body))
	s.handleCountTokens(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}
