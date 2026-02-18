package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxRetryCount = 5

var retrySleep = time.Sleep

func (s *Server) doProviderRequestWithRetry(
	ctx context.Context,
	method string,
	targetURL string,
	payload []byte,
	incomingHeaders http.Header,
	provider ProviderConfig,
	stream bool,
	routeName string,
	modelName string,
	reasoningEffort string,
) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetryCount; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, method, targetURL, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("failed to create upstream request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		if stream {
			req.Header.Set("Accept", "text/event-stream")
		} else {
			req.Header.Set("Accept", "application/json")
		}
		if incomingHeaders != nil {
			if v := incomingHeaders.Get("x-request-id"); v != "" {
				req.Header.Set("x-request-id", v)
			}
		}

		if err := ApplyProviderAuth(req, provider); err != nil {
			return nil, err
		}

		reqID := strings.TrimSpace(req.Header.Get("x-request-id"))
		if reqID == "" {
			reqID = "-"
		}
		if strings.TrimSpace(routeName) == "" {
			routeName = "-"
		}
		if strings.TrimSpace(modelName) == "" {
			modelName = "-"
		}
		if strings.TrimSpace(reasoningEffort) == "" {
			reasoningEffort = "-"
		}
		s.logger.Infof("[HTTP-OUT] req=%s route=%s model=%s reasoning=%s %s %s", reqID, routeName, modelName, reasoningEffort, req.Method, req.URL.String())
		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
				return nil, err
			}
			if attempt == maxRetryCount {
				return nil, err
			}
			retrySleep(backoffDuration(attempt))
			continue
		}

		if isRetryableStatus(resp.StatusCode) && attempt < maxRetryCount {
			lastErr = fmt.Errorf("upstream returned status %d", resp.StatusCode)
			closeResponseBody(resp)
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			retrySleep(backoffDuration(attempt))
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}

func backoffDuration(attempt int) time.Duration {
	base := 250 * time.Millisecond
	return base * time.Duration(1<<attempt)
}

func ApplyProviderAuth(req *http.Request, provider ProviderConfig) error {
	authType := provider.Auth.Type
	if authType == "" {
		authType = AuthTypeNone
	}

	switch authType {
	case AuthTypeNone:
		return nil
	case AuthTypeBearer:
		tokenEnv := provider.Auth.TokenEnv
		if tokenEnv == "" {
			return fmt.Errorf("bearer auth requires token_env")
		}
		token := strings.TrimSpace(os.Getenv(tokenEnv))
		if token == "" {
			return fmt.Errorf("bearer auth env %s is empty", tokenEnv)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	case AuthTypeCodex:
		creds, err := loadCodexCredentials()
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+creds.Token)
		if creds.AccountID != "" {
			req.Header.Set("Chatgpt-Account-Id", creds.AccountID)
		}
		req.Header.Set("User-Agent", "codex-cli/1.0")
		return nil
	default:
		return fmt.Errorf("unsupported auth type: %s", authType)
	}
}

type codexCredentials struct {
	Token     string
	AccountID string
}

func loadCodexCredentials() (codexCredentials, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return codexCredentials{}, fmt.Errorf("failed to resolve home dir: %w", err)
	}
	path := filepath.Join(homeDir, ".codex", "auth.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return codexCredentials{}, fmt.Errorf("failed to read codex auth file: %w", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(b, &payload); err != nil {
		return codexCredentials{}, fmt.Errorf("invalid codex auth file: %w", err)
	}

	var creds codexCredentials

	// Extract token and account_id from "tokens" object
	if tokens, ok := payload["tokens"].(map[string]interface{}); ok {
		if v, ok := tokens["access_token"].(string); ok && v != "" {
			creds.Token = strings.TrimSpace(v)
		}
		if v, ok := tokens["account_id"].(string); ok && v != "" {
			creds.AccountID = strings.TrimSpace(v)
		}
	}

	// Fallback: search recursively for token
	if creds.Token == "" {
		creds.Token = findTokenRecursive(payload)
	}
	if creds.Token == "" {
		return codexCredentials{}, fmt.Errorf("codex auth file does not contain token")
	}
	return creds, nil
}

func findTokenRecursive(v interface{}) string {
	switch t := v.(type) {
	case map[string]interface{}:
		for _, key := range []string{"access_token", "id_token", "token", "api_key"} {
			if raw, ok := t[key]; ok {
				if str, ok := raw.(string); ok && strings.TrimSpace(str) != "" {
					return strings.TrimSpace(str)
				}
			}
		}
		for _, value := range t {
			if token := findTokenRecursive(value); token != "" {
				return token
			}
		}
	case []interface{}:
		for _, value := range t {
			if token := findTokenRecursive(value); token != "" {
				return token
			}
		}
	case string:
		if strings.Count(t, ".") == 2 && len(t) > 20 {
			return strings.TrimSpace(t)
		}
	}
	return ""
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, nil
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
