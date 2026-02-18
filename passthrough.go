package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) proxyPassthrough(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	routeName string,
	modelName string,
	reasoningEffort string,
	provider ProviderConfig,
	body []byte,
) {
	targetURL, err := joinURL(provider.URL, r.URL.Path, r.URL.RawQuery)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}

	req, err := http.NewRequestWithContext(ctx, r.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create relay request")
		return
	}

	copyHeaders(req.Header, r.Header)
	req.Header.Del("Host")
	if err := ApplyProviderAuth(req, provider); err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
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
		writeJSONError(w, mapTransportError(err), fmt.Sprintf("relay failed: %v", err))
		return
	}
	defer resp.Body.Close()

	relayResponse(w, resp)
}

func joinURL(baseURL, path, rawQuery string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid provider url: %w", err)
	}

	u.Path = strings.TrimRight(u.Path, "/") + path
	u.RawQuery = rawQuery
	return u.String(), nil
}

func copyHeaders(dst, src http.Header) {
	for k, values := range src {
		dst.Del(k)
		for _, v := range values {
			dst.Add(k, v)
		}
	}
}
