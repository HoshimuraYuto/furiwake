package main

import "encoding/json"

func TranslateAnthropicToOpenAI(req AnthropicMessageRequest, model string) OpenAIChatRequest {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	out := OpenAIChatRequest{
		Model:     model,
		Messages:  TranslateMessagesToOpenAI(req.System, req.Messages),
		Stream:    req.Stream,
		MaxTokens: maxTokens,
	}
	if req.Stream {
		out.StreamOptions = &OpenAIStreamOptions{IncludeUsage: true}
	}

	if len(req.Tools) > 0 {
		out.Tools = translateTools(req.Tools)
	}
	if req.ToolChoice != nil {
		out.ToolChoice = translateToolChoice(req.ToolChoice)
	}
	return out
}

func translateTools(tools []AnthropicTool) []OpenAITool {
	out := make([]OpenAITool, 0, len(tools))
	for _, tool := range tools {
		parameters := tool.InputSchema
		if len(parameters) == 0 {
			parameters = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out = append(out, OpenAITool{
			Type: "function",
			Function: OpenAIFunctionDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  parameters,
			},
		})
	}
	return out
}

func translateToolChoice(v interface{}) interface{} {
	m, ok := v.(map[string]interface{})
	if !ok {
		return v
	}
	t, _ := m["type"].(string)
	switch t {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "none":
		return "none"
	case "tool":
		name, _ := m["name"].(string)
		return map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name": name,
			},
		}
	default:
		return v
	}
}
