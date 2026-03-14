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
	tierMarkerPattern      = regexp.MustCompile(`@tier:([a-zA-Z0-9_-]+)`)
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

func NormalizeServiceTier(v string) string {
	normalized := strings.TrimSpace(strings.ToLower(v))
	if normalized == "fast" {
		return "priority"
	}
	return normalized
}

func IsValidServiceTier(v string) bool {
	switch NormalizeServiceTier(v) {
	case "priority", "flex":
		return true
	default:
		return false
	}
}

func ExtractServiceTier(system interface{}) string {
	text := NormalizeSystemText(system)
	m := tierMarkerPattern.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func ExtractServiceTierFromMessages(messages []AnthropicMessage) string {
	for _, msg := range messages {
		if tier := ExtractServiceTier(msg.Content); tier != "" {
			return tier
		}
	}
	return ""
}

func ExtractPresetName(system interface{}, presetNames []string) string {
	text := NormalizeSystemText(system)
	for _, name := range presetNames {
		marker := "@" + name
		idx := strings.Index(text, marker)
		if idx < 0 {
			continue
		}
		// Check that the character after the marker is not alphanumeric (word boundary)
		end := idx + len(marker)
		if end >= len(text) {
			return name
		}
		ch := text[end]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '-' || ch == '<' {
			return name
		}
	}
	return ""
}

func ExtractPresetNameFromMessages(messages []AnthropicMessage, presetNames []string) string {
	for _, msg := range messages {
		if name := ExtractPresetName(msg.Content, presetNames); name != "" {
			return name
		}
	}
	return ""
}

// RouteResolution holds the fully resolved routing parameters.
type RouteResolution struct {
	ProviderName    string
	Provider        ProviderConfig
	Model           string
	ReasoningEffort string
	ServiceTier     string
	PresetName      string
}

// ResolveAll performs consolidated resolution of all routing parameters,
// including preset support.
func ResolveAll(system interface{}, messages []AnthropicMessage, cfg *Config) (*RouteResolution, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	// 1. Get list of preset names
	presetNames := make([]string, 0, len(cfg.Presets))
	for name := range cfg.Presets {
		presetNames = append(presetNames, name)
	}

	// 2. Check for preset marker
	presetName := ExtractPresetName(system, presetNames)
	if presetName == "" {
		presetName = ExtractPresetNameFromMessages(messages, presetNames)
	}

	var preset PresetConfig
	hasPreset := false
	if presetName != "" {
		if p, ok := cfg.Presets[presetName]; ok {
			preset = p
			hasPreset = true
		}
	}

	// 3. Resolve provider: explicit @route: > preset's provider > config default_provider
	routeName := ExtractRouteName(system)
	if routeName == "" {
		routeName = ExtractRouteNameFromMessages(messages)
	}
	if routeName == "" && hasPreset && preset.Provider != "" {
		routeName = preset.Provider
	}
	if routeName == "" {
		routeName = cfg.DefaultProvider
	}

	provider, ok := cfg.Providers[routeName]
	if !ok {
		return nil, fmt.Errorf("route provider %q not found", routeName)
	}

	// 4. Resolve model: explicit @model: > preset's model > provider's default model
	model := ExtractModelName(system)
	if model == "" {
		model = ExtractModelNameFromMessages(messages)
	}
	if model == "" && hasPreset && preset.Model != "" {
		model = preset.Model
	}
	if model == "" {
		model = provider.Model
	}

	// 5. Resolve reasoning effort (only for chatgpt type)
	reasoningEffort := ""
	if provider.Type == ProviderTypeChatGPT {
		markerEffort := ExtractReasoningEffort(system)
		if markerEffort == "" {
			markerEffort = ExtractReasoningEffortFromMessages(messages)
		}
		if markerEffort != "" {
			normalized := NormalizeReasoningEffort(markerEffort)
			if !IsValidReasoningEffort(normalized) {
				return nil, fmt.Errorf("invalid @reasoning value %q (allowed: none/minimal/low/medium/high/xhigh)", markerEffort)
			}
			reasoningEffort = normalized
		} else if hasPreset && preset.ReasoningEffort != "" {
			normalized := NormalizeReasoningEffort(preset.ReasoningEffort)
			if !IsValidReasoningEffort(normalized) {
				return nil, fmt.Errorf("invalid preset reasoning effort: %s", preset.ReasoningEffort)
			}
			reasoningEffort = normalized
		} else if provider.ReasoningEffort != "" {
			normalized := NormalizeReasoningEffort(provider.ReasoningEffort)
			if !IsValidReasoningEffort(normalized) {
				return nil, fmt.Errorf("invalid default reasoning effort: %s", provider.ReasoningEffort)
			}
			reasoningEffort = normalized
		}
	}

	serviceTier := ""
	if provider.Type == ProviderTypeChatGPT {
		markerTier := ExtractServiceTier(system)
		if markerTier == "" {
			markerTier = ExtractServiceTierFromMessages(messages)
		}
		if markerTier != "" {
			normalized := NormalizeServiceTier(markerTier)
			if !IsValidServiceTier(normalized) {
				return nil, fmt.Errorf("invalid @tier value %q (allowed: priority/flex)", markerTier)
			}
			serviceTier = normalized
		} else if hasPreset && preset.ServiceTier != "" {
			normalized := NormalizeServiceTier(preset.ServiceTier)
			if !IsValidServiceTier(normalized) {
				return nil, fmt.Errorf("invalid preset service tier: %s", preset.ServiceTier)
			}
			serviceTier = normalized
		} else if provider.ServiceTier != "" {
			normalized := NormalizeServiceTier(provider.ServiceTier)
			if !IsValidServiceTier(normalized) {
				return nil, fmt.Errorf("invalid default service tier: %s", provider.ServiceTier)
			}
			serviceTier = normalized
		}
	}

	return &RouteResolution{
		ProviderName:    routeName,
		Provider:        provider,
		Model:           model,
		ReasoningEffort: reasoningEffort,
		ServiceTier:     serviceTier,
		PresetName:      presetName,
	}, nil
}
