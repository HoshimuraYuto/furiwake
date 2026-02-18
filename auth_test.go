package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestApplyProviderAuth_Bearer(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "abc123")
	req, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	err := ApplyProviderAuth(req, ProviderConfig{
		Auth: AuthConfig{Type: AuthTypeBearer, TokenEnv: "OPENAI_API_KEY"},
	})
	if err != nil {
		t.Fatalf("ApplyProviderAuth error: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer abc123" {
		t.Fatalf("unexpected auth header: %s", got)
	}
}

func TestApplyProviderAuth_BearerMissingEnv(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	err := ApplyProviderAuth(req, ProviderConfig{
		Auth: AuthConfig{Type: AuthTypeBearer, TokenEnv: "MISSING_KEY"},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFindTokenRecursive(t *testing.T) {
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.signature"
	v := map[string]interface{}{
		"a": []interface{}{
			map[string]interface{}{
				"nested": map[string]interface{}{
					"access_token": token,
				},
			},
		},
	}
	got := findTokenRecursive(v)
	if got != token {
		t.Fatalf("unexpected token: %q", got)
	}
}

func TestSafeJSONRawMessage(t *testing.T) {
	if got := string(safeJSONRawMessage("")); got != "{}" {
		t.Fatalf("expected {}, got %s", got)
	}
	if got := string(safeJSONRawMessage("not-json")); got != "{}" {
		t.Fatalf("expected {}, got %s", got)
	}
	okJSON := `{"a":1}`
	if got := string(safeJSONRawMessage(okJSON)); !strings.Contains(got, `"a":1`) {
		t.Fatalf("expected original json, got %s", got)
	}
}
