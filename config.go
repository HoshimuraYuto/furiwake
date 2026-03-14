package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadConfig(path string) (*Config, error) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{},
		Presets:   map[string]PresetConfig{},
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config %s: %w", path, err)
	}

	if err := yaml.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse yaml config: %w", err)
	}

	if cfg.Listen == "" {
		return nil, fmt.Errorf("listen is required")
	}
	if cfg.SpoofModel == "" {
		return nil, fmt.Errorf("spoof_model is required")
	}
	if cfg.DefaultProvider == "" {
		return nil, fmt.Errorf("default_provider is required")
	}
	if cfg.TimeoutSeconds <= 0 {
		return nil, fmt.Errorf("timeout_seconds is required and must be > 0")
	}

	if len(cfg.Providers) == 0 {
		return nil, fmt.Errorf("providers is required")
	}

	for name, provider := range cfg.Providers {
		p := provider
		p.Type = strings.TrimSpace(strings.ToLower(p.Type))
		p.URL = strings.TrimSpace(p.URL)
		p.Model = strings.TrimSpace(p.Model)
		p.ReasoningEffort = NormalizeReasoningEffort(p.ReasoningEffort)
		p.Auth.Type = strings.TrimSpace(strings.ToLower(p.Auth.Type))

		if p.Type == "" {
			return nil, fmt.Errorf("providers.%s.type is required", name)
		}
		if p.URL == "" {
			return nil, fmt.Errorf("providers.%s.url is required", name)
		}
		switch p.Type {
		case ProviderTypePassthrough, ProviderTypeOpenAI, ProviderTypeChatGPT:
		default:
			return nil, fmt.Errorf("providers.%s.type must be one of passthrough/openai/chatgpt", name)
		}

		if p.Type != ProviderTypePassthrough && p.Model == "" {
			return nil, fmt.Errorf("providers.%s.model is required for type %s", name, p.Type)
		}
		if p.ReasoningEffort != "" && !IsValidReasoningEffort(p.ReasoningEffort) {
			return nil, fmt.Errorf("providers.%s.reasoning_effort must be one of none/minimal/low/medium/high/xhigh", name)
		}
		p.ServiceTier = NormalizeServiceTier(p.ServiceTier)
		if p.ServiceTier != "" && !IsValidServiceTier(p.ServiceTier) {
			return nil, fmt.Errorf("providers.%s.service_tier must be one of priority/flex", name)
		}

		switch p.Auth.Type {
		case "", AuthTypeNone, AuthTypeBearer, AuthTypeCodex:
		default:
			return nil, fmt.Errorf("providers.%s.auth.type must be none/bearer/codex", name)
		}

		cfg.Providers[name] = p
	}

	if _, ok := cfg.Providers[cfg.DefaultProvider]; !ok {
		return nil, fmt.Errorf("default_provider %q is not defined in providers", cfg.DefaultProvider)
	}

	if cfg.Presets == nil {
		cfg.Presets = map[string]PresetConfig{}
	}
	for name, preset := range cfg.Presets {
		if preset.Provider != "" {
			if _, ok := cfg.Providers[preset.Provider]; !ok {
				return nil, fmt.Errorf("presets.%s.provider %q is not defined in providers", name, preset.Provider)
			}
		}
		if preset.ReasoningEffort != "" && !IsValidReasoningEffort(preset.ReasoningEffort) {
			return nil, fmt.Errorf("presets.%s.reasoning_effort must be one of none/minimal/low/medium/high/xhigh", name)
		}
		preset.ServiceTier = NormalizeServiceTier(preset.ServiceTier)
		if preset.ServiceTier != "" && !IsValidServiceTier(preset.ServiceTier) {
			return nil, fmt.Errorf("presets.%s.service_tier must be one of priority/flex", name)
		}
		cfg.Presets[name] = preset
	}

	return cfg, nil
}
