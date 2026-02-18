package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type Server struct {
	cfg        *Config
	logger     *Logger
	client     *http.Client
	httpServer *http.Server
}

func NewServer(cfg *Config, logger *Logger) *Server {
	s := &Server{
		cfg:    cfg,
		logger: logger,
		client: &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/v1/messages", s.handleMessages)
	mux.HandleFunc("/v1/messages/count_tokens", s.handleCountTokens)

	s.httpServer = &http.Server{
		Addr:    cfg.Listen,
		Handler: mux,
	}
	return s
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":   true,
		"name": "furiwake",
	})
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<20))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var anthropicReq AnthropicMessageRequest
	if err := json.Unmarshal(body, &anthropicReq); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	providerName, provider, model, err := ResolveProviderAndModel(anthropicReq.System, anthropicReq.Messages, s.cfg)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	reasoningEffort := ""
	reasoningForLog := "-"
	if provider.Type == ProviderTypeChatGPT {
		effort, err := ResolveReasoningEffort(anthropicReq.System, anthropicReq.Messages, provider.ReasoningEffort)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		reasoningEffort = effort
		if effort != "" {
			reasoningForLog = effort
		}
	}

	requestID := strings.TrimSpace(r.Header.Get("x-request-id"))
	if requestID == "" {
		requestID = fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	r.Header.Set("x-request-id", requestID)
	s.logger.Infof("req=%s route=%s type=%s model=%s reasoning=%s stream=%t", requestID, providerName, provider.Type, model, reasoningForLog, anthropicReq.Stream)

	ctx := r.Context()
	switch provider.Type {
	case ProviderTypePassthrough:
		s.proxyPassthrough(ctx, w, r, providerName, model, "-", provider, body)
	case ProviderTypeOpenAI:
		s.proxyOpenAI(ctx, w, providerName, provider, model, "-", anthropicReq, r.Header)
	case ProviderTypeChatGPT:
		s.proxyChatGPT(ctx, w, providerName, provider, model, reasoningEffort, anthropicReq, r.Header)
	default:
		writeJSONError(w, http.StatusBadGateway, "unsupported provider type")
	}
}

func (s *Server) handleCountTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var req CountTokensRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	providerName, provider, err := ResolveProvider(req.System, nil, s.cfg)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if provider.Type == ProviderTypePassthrough {
		s.logger.Debugf("req=%s count_tokens passthrough route=%s", r.Header.Get("x-request-id"), providerName)
		s.proxyPassthrough(r.Context(), w, r, providerName, "-", "-", provider, body)
		return
	}

	tokens := estimateInputTokens(req.System, req.Messages)
	writeJSON(w, http.StatusOK, CountTokensResponse{InputTokens: tokens})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]interface{}{
		"type":    "error",
		"message": message,
	})
}

func relayResponse(w http.ResponseWriter, resp *http.Response) {
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func cloneBody(body []byte) io.ReadCloser {
	return io.NopCloser(bytes.NewReader(body))
}

func isRetryableStatus(status int) bool {
	return status == http.StatusTooManyRequests
}

func closeResponseBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func mapTransportError(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) && netErr.Timeout() {
		return http.StatusGatewayTimeout
	}
	return http.StatusBadGateway
}

func estimateInputTokens(system interface{}, messages []AnthropicMessage) int {
	totalChars := utf8.RuneCountInString(NormalizeSystemText(system))
	for _, msg := range messages {
		totalChars += utf8.RuneCountInString(normalizeContentToText(msg.Content))
	}
	estimated := totalChars / 4
	if estimated < 1 {
		estimated = 1
	}
	return estimated
}
