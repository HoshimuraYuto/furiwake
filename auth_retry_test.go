package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDoProviderRequestWithRetry_Retry429(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = io.Copy(io.Discard, r.Body)
		if calls < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate_limited"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	s := &Server{
		client: upstream.Client(),
		logger: NewLogger(),
	}
	provider := ProviderConfig{
		Type:  ProviderTypeOpenAI,
		URL:   upstream.URL,
		Model: "gpt-5-mini",
		Auth: AuthConfig{
			Type: AuthTypeNone,
		},
	}

	prevSleep := retrySleep
	retrySleep = func(_ time.Duration) {}
	defer func() { retrySleep = prevSleep }()

	resp, err := s.doProviderRequestWithRetry(
		context.Background(),
		http.MethodPost,
		upstream.URL,
		[]byte(`{"m":"x"}`),
		nil,
		provider,
		false,
		"",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("doProviderRequestWithRetry error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDoProviderRequestWithRetry_DoesNotRetryCanceledContext(t *testing.T) {
	calls := 0
	s := &Server{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			return nil, context.Canceled
		})},
		logger: NewLogger(),
	}
	provider := ProviderConfig{Type: ProviderTypeOpenAI, URL: "http://example.com", Model: "gpt-5-mini", Auth: AuthConfig{Type: AuthTypeNone}}

	sleeps := 0
	prevSleep := retrySleep
	retrySleep = func(_ time.Duration) { sleeps++ }
	defer func() { retrySleep = prevSleep }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.doProviderRequestWithRetry(ctx, http.MethodPost, "http://example.com", []byte(`{"m":"x"}`), nil, provider, false, "", "", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected no upstream calls, got %d", calls)
	}
	if sleeps != 0 {
		t.Fatalf("expected no retries, got %d sleeps", sleeps)
	}
}
