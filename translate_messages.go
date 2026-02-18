package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func NormalizeSystemText(system interface{}) string {
	if system == nil {
		return ""
	}
	switch v := system.(type) {
	case string:
		return strings.TrimSpace(v)
	}

	blocks := normalizeContentToBlocks(system)
	texts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			texts = append(texts, strings.TrimSpace(block.Text))
		}
	}
	return strings.Join(texts, "\n")
}

func TranslateMessagesToOpenAI(system interface{}, messages []AnthropicMessage) []OpenAIMessage {
	out := make([]OpenAIMessage, 0, len(messages)+1)
	if systemText := NormalizeSystemText(system); systemText != "" {
		out = append(out, OpenAIMessage{
			Role:    "system",
			Content: systemText,
		})
	}

	for _, message := range messages {
		switch message.Role {
		case "user":
			out = append(out, translateUserMessage(message)...)
		case "assistant":
			out = append(out, translateAssistantMessage(message)...)
		default:
			text := normalizeContentToText(message.Content)
			out = append(out, OpenAIMessage{
				Role:    message.Role,
				Content: text,
			})
		}
	}

	return out
}

func translateUserMessage(message AnthropicMessage) []OpenAIMessage {
	if text, ok := message.Content.(string); ok {
		return []OpenAIMessage{{Role: "user", Content: text}}
	}

	blocks := normalizeContentToBlocks(message.Content)
	textParts := []string{}
	out := []OpenAIMessage{}
	flushText := func() {
		if len(textParts) == 0 {
			return
		}
		out = append(out, OpenAIMessage{
			Role:    "user",
			Content: strings.Join(textParts, "\n"),
		})
		textParts = textParts[:0]
	}
	for _, block := range blocks {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				textParts = append(textParts, block.Text)
			}
		case "tool_result":
			flushText()
			out = append(out, OpenAIMessage{
				Role:       "tool",
				ToolCallID: block.ToolUseID,
				Content:    extractToolResultText(block.Content),
			})
		}
	}
	flushText()
	if len(out) == 0 {
		out = append(out, OpenAIMessage{
			Role:    "user",
			Content: normalizeContentToText(message.Content),
		})
	}
	return out
}

func translateAssistantMessage(message AnthropicMessage) []OpenAIMessage {
	if text, ok := message.Content.(string); ok {
		return []OpenAIMessage{{Role: "assistant", Content: text}}
	}

	blocks := normalizeContentToBlocks(message.Content)
	textParts := []string{}
	toolCalls := []OpenAIToolCall{}
	for i, block := range blocks {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				textParts = append(textParts, block.Text)
			}
		case "tool_use":
			id := block.ID
			if id == "" {
				id = fmt.Sprintf("tool_call_%d_%d", time.Now().UnixNano(), i)
			}
			args := string(block.Input)
			if strings.TrimSpace(args) == "" {
				args = "{}"
			}
			toolCalls = append(toolCalls, OpenAIToolCall{
				ID:   id,
				Type: "function",
				Function: OpenAIToolFunction{
					Name:      block.Name,
					Arguments: args,
				},
			})
		}
	}

	content := strings.Join(textParts, "\n")
	out := OpenAIMessage{
		Role:      "assistant",
		Content:   content,
		ToolCalls: toolCalls,
	}
	return []OpenAIMessage{out}
}

func normalizeContentToText(content interface{}) string {
	if content == nil {
		return ""
	}
	if text, ok := content.(string); ok {
		return text
	}
	blocks := normalizeContentToBlocks(content)
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func normalizeContentToBlocks(content interface{}) []AnthropicContentBlock {
	if content == nil {
		return nil
	}
	switch v := content.(type) {
	case []AnthropicContentBlock:
		return v
	case AnthropicContentBlock:
		return []AnthropicContentBlock{v}
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []AnthropicContentBlock{{Type: "text", Text: v}}
	}

	raw, err := json.Marshal(content)
	if err != nil {
		return nil
	}

	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		return blocks
	}

	var single AnthropicContentBlock
	if err := json.Unmarshal(raw, &single); err == nil && single.Type != "" {
		return []AnthropicContentBlock{single}
	}
	return nil
}

func extractToolResultText(content interface{}) string {
	if content == nil {
		return ""
	}
	if text, ok := content.(string); ok {
		return text
	}
	blocks := normalizeContentToBlocks(content)
	parts := []string{}
	for _, block := range blocks {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}
