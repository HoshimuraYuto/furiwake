package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJoinURL(t *testing.T) {
	got, err := joinURL("https://api.anthropic.com/base", "/v1/messages", "a=1")
	if err != nil {
		t.Fatalf("joinURL error: %v", err)
	}
	want := "https://api.anthropic.com/base/v1/messages?a=1"
	if got != want {
		t.Fatalf("unexpected url: got=%s want=%s", got, want)
	}
}

func TestCopyHeaders(t *testing.T) {
	src := http.Header{}
	src.Add("X-Test", "a")
	src.Add("X-Test", "b")
	dst := http.Header{}
	dst.Add("X-Test", "old")

	copyHeaders(dst, src)
	values := dst.Values("X-Test")
	if len(values) != 2 || values[0] != "a" || values[1] != "b" {
		t.Fatalf("unexpected header values: %+v", values)
	}
}

func TestProxyPassthrough_AppliesProviderAuth(t *testing.T) {
	t.Setenv("PROVIDER_TOKEN", "provider-secret")

	var seenAuthorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuthorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	s := &Server{
		client: upstream.Client(),
		logger: NewLogger(),
	}
	provider := ProviderConfig{
		Type: ProviderTypePassthrough,
		URL:  upstream.URL,
		Auth: AuthConfig{Type: AuthTypeBearer, TokenEnv: "PROVIDER_TOKEN"},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader([]byte(`{"x":1}`)))
	req.Header.Set("Authorization", "Bearer client-token")

	s.proxyPassthrough(req.Context(), rr, req, "route", "model", "-", provider, []byte(`{"x":1}`))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if seenAuthorization != "Bearer provider-secret" {
		t.Fatalf("unexpected Authorization header: %q", seenAuthorization)
	}
}
