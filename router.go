package main

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	routeMarkerPattern     = regexp.MustCompile(`@route:([a-zA-Z0-9_-]+)`)
	modelMarkerPattern     = regexp.MustCompile(`@model:([a-zA-Z0-9._:/-]+)`)
	reasoningMarkerPattern = regexp.MustCompile(`@reasoning:([a-zA-Z0-9_-]+)`)
)

func ResolveProvider(system interface{}, messages []AnthropicMessage, cfg *Config) (string, ProviderConfig, error) {
	if cfg == nil {
		return "", ProviderConfig{}, fmt.Errorf("config is nil")
	}

	routeName := ExtractRouteName(system)
	if routeName == "" {
		routeName = ExtractRouteNameFromMessages(messages)
	}
	if routeName == "" {
		p, ok := cfg.Providers[cfg.DefaultProvider]
		if !ok {
			return "", ProviderConfig{}, fmt.Errorf("default provider %q not found", cfg.DefaultProvider)
		}
		return cfg.DefaultProvider, p, nil
	}

	p, ok := cfg.Providers[routeName]
	if !ok {
		return "", ProviderConfig{}, fmt.Errorf("route provider %q not found", routeName)
	}
	return routeName, p, nil
}

func ResolveProviderAndModel(system interface{}, messages []AnthropicMessage, cfg *Config) (string, ProviderConfig, string, error) {
	name, provider, err := ResolveProvider(system, messages, cfg)
	if err != nil {
		return "", ProviderConfig{}, "", err
	}
	return name, provider, ResolveModelName(system, messages, provider.Model), nil
}

func ResolveModelName(system interface{}, messages []AnthropicMessage, defaultModel string) string {
	model := ExtractModelName(system)
	if model == "" {
		model = ExtractModelNameFromMessages(messages)
	}
	if model == "" {
		return defaultModel
	}
	return model
}

func ResolveReasoningEffort(system interface{}, messages []AnthropicMessage, defaultEffort string) (string, error) {
	effort := NormalizeReasoningEffort(defaultEffort)
	markerEffort := ExtractReasoningEffort(system)
	if markerEffort == "" {
		markerEffort = ExtractReasoningEffortFromMessages(messages)
	}
	if markerEffort == "" {
		if effort == "" {
			return "", nil
		}
		if !IsValidReasoningEffort(effort) {
			return "", fmt.Errorf("invalid default reasoning effort: %s", effort)
		}
		return effort, nil
	}

	normalized := NormalizeReasoningEffort(markerEffort)
	if !IsValidReasoningEffort(normalized) {
		return "", fmt.Errorf("invalid @reasoning value %q (allowed: none/minimal/low/medium/high/xhigh)", markerEffort)
	}
	return normalized, nil
}

func NormalizeReasoningEffort(v string) string {
	return strings.TrimSpace(strings.ToLower(v))
}

func IsValidReasoningEffort(v string) bool {
	switch NormalizeReasoningEffort(v) {
	case "none", "minimal", "low", "medium", "high", "xhigh":
		return true
	default:
		return false
	}
}

func ExtractRouteNameFromMessages(messages []AnthropicMessage) string {
	for _, msg := range messages {
		if name := ExtractRouteName(msg.Content); name != "" {
			return name
		}
	}
	return ""
}

func ExtractModelNameFromMessages(messages []AnthropicMessage) string {
	for _, msg := range messages {
		if model := ExtractModelName(msg.Content); model != "" {
			return model
		}
	}
	return ""
}

func ExtractReasoningEffortFromMessages(messages []AnthropicMessage) string {
	for _, msg := range messages {
		if effort := ExtractReasoningEffort(msg.Content); effort != "" {
			return effort
		}
	}
	return ""
}

func ExtractRouteName(system interface{}) string {
	text := NormalizeSystemText(system)
	m := routeMarkerPattern.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(m[1]))
}

func ExtractModelName(system interface{}) string {
	text := NormalizeSystemText(system)
	m := modelMarkerPattern.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func ExtractReasoningEffort(system interface{}) string {
	text := NormalizeSystemText(system)
	m := reasoningMarkerPattern.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}
