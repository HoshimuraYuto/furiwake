package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

func (s *Server) proxyOpenAI(
	ctx context.Context,
	w http.ResponseWriter,
	routeName string,
	provider ProviderConfig,
	model string,
	reasoningEffort string,
	anthropicReq AnthropicMessageRequest,
	incomingHeaders http.Header,
) {
	openAIReq := TranslateAnthropicToOpenAI(anthropicReq, model)
	payload, err := json.Marshal(openAIReq)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to encode upstream request")
		return
	}

	resp, err := s.doProviderRequestWithRetry(
		ctx,
		http.MethodPost,
		provider.URL,
		payload,
		incomingHeaders,
		provider,
		anthropicReq.Stream,
		routeName,
		openAIReq.Model,
		reasoningEffort,
	)
	if err != nil {
		writeJSONError(w, mapTransportError(err), err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		writeJSON(w, resp.StatusCode, map[string]interface{}{
			"type":    "error",
			"message": string(raw),
		})
		return
	}

	if anthropicReq.Stream {
		if err := convertOpenAIStreamToAnthropic(w, resp.Body, s.cfg.SpoofModel); err != nil {
			s.logger.Errorf("openai stream translation failed: %v", err)
		}
		return
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to read upstream response")
		return
	}

	var openAIResp OpenAIChatResponse
	if err := json.Unmarshal(raw, &openAIResp); err != nil {
		writeJSONError(w, http.StatusBadGateway, "invalid upstream JSON response")
		return
	}

	writeJSON(w, http.StatusOK, convertOpenAINonStreamToAnthropic(openAIResp, s.cfg.SpoofModel))
}

type trackedToolCall struct {
	anthropicIndex int
	id             string
	name           string
}

func convertOpenAIStreamToAnthropic(w http.ResponseWriter, src io.Reader, spoofModel string) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming unsupported by response writer")
	}

	messageID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	if err := writeAnthropicSSEEvent(w, "message_start", map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            messageID,
			"type":          "message",
			"role":          "assistant",
			"model":         spoofModel,
			"content":       []interface{}{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]int{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		},
	}); err != nil {
		return err
	}
	flusher.Flush()

	nextBlockIndex := 0
	activeTextIndex := -1
	activeToolCalls := map[int]*trackedToolCall{}
	openBlocks := map[int]bool{}
	stopReason := "end_turn"
	outputTokens := 0

	closeBlock := func(index int) error {
		if !openBlocks[index] {
			return nil
		}
		delete(openBlocks, index)
		return writeAnthropicSSEEvent(w, "content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": index,
		})
	}

	if err := readSSEEvents(src, func(_ string, data string) error {
		data = strings.TrimSpace(data)
		if data == "" {
			return nil
		}
		if data == "[DONE]" {
			return errSSEStreamDone
		}

		var chunk OpenAIChatStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return nil
		}
		if chunk.Usage.CompletionTokens > 0 {
			outputTokens = chunk.Usage.CompletionTokens
		}
		if len(chunk.Choices) == 0 {
			return nil
		}

		choice := chunk.Choices[0]

		if choice.Delta.Content != "" {
			if activeTextIndex < 0 {
				activeTextIndex = nextBlockIndex
				nextBlockIndex++
				openBlocks[activeTextIndex] = true
				if err := writeAnthropicSSEEvent(w, "content_block_start", map[string]interface{}{
					"type":  "content_block_start",
					"index": activeTextIndex,
					"content_block": map[string]interface{}{
						"type": "text",
						"text": "",
					},
				}); err != nil {
					return err
				}
			}
			if err := writeAnthropicSSEEvent(w, "content_block_delta", map[string]interface{}{
				"type":  "content_block_delta",
				"index": activeTextIndex,
				"delta": map[string]interface{}{
					"type": "text_delta",
					"text": choice.Delta.Content,
				},
			}); err != nil {
				return err
			}
			flusher.Flush()
		}

		for _, tc := range choice.Delta.ToolCalls {
			toolIdx := tc.Index
			tracked := activeToolCalls[toolIdx]

			if tc.ID != "" {
				if activeTextIndex >= 0 {
					if err := closeBlock(activeTextIndex); err != nil {
						return err
					}
					activeTextIndex = -1
				}
				tracked = &trackedToolCall{
					anthropicIndex: nextBlockIndex,
					id:             tc.ID,
					name:           tc.Function.Name,
				}
				nextBlockIndex++
				activeToolCalls[toolIdx] = tracked
				openBlocks[tracked.anthropicIndex] = true
				if err := writeAnthropicSSEEvent(w, "content_block_start", map[string]interface{}{
					"type":  "content_block_start",
					"index": tracked.anthropicIndex,
					"content_block": map[string]interface{}{
						"type":  "tool_use",
						"id":    tracked.id,
						"name":  tracked.name,
						"input": map[string]interface{}{},
					},
				}); err != nil {
					return err
				}
			}

			if tracked == nil {
				tracked = &trackedToolCall{
					anthropicIndex: nextBlockIndex,
					id:             fmt.Sprintf("toolu_%d", time.Now().UnixNano()),
					name:           tc.Function.Name,
				}
				nextBlockIndex++
				activeToolCalls[toolIdx] = tracked
				openBlocks[tracked.anthropicIndex] = true
				if err := writeAnthropicSSEEvent(w, "content_block_start", map[string]interface{}{
					"type":  "content_block_start",
					"index": tracked.anthropicIndex,
					"content_block": map[string]interface{}{
						"type":  "tool_use",
						"id":    tracked.id,
						"name":  tracked.name,
						"input": map[string]interface{}{},
					},
				}); err != nil {
					return err
				}
			}

			if tc.Function.Arguments != "" {
				if err := writeAnthropicSSEEvent(w, "content_block_delta", map[string]interface{}{
					"type":  "content_block_delta",
					"index": tracked.anthropicIndex,
					"delta": map[string]interface{}{
						"type":         "input_json_delta",
						"partial_json": tc.Function.Arguments,
					},
				}); err != nil {
					return err
				}
			}
			flusher.Flush()
		}

		if choice.FinishReason != "" {
			stopReason = mapFinishReason(choice.FinishReason)
		}
		return nil
	}); err != nil {
		return err
	}

	if activeTextIndex >= 0 {
		if err := closeBlock(activeTextIndex); err != nil {
			return err
		}
	}
	toolBlockIndexes := make([]int, 0, len(activeToolCalls))
	for _, tc := range activeToolCalls {
		toolBlockIndexes = append(toolBlockIndexes, tc.anthropicIndex)
	}
	sort.Ints(toolBlockIndexes)
	for _, idx := range toolBlockIndexes {
		if err := closeBlock(idx); err != nil {
			return err
		}
	}

	if err := writeAnthropicSSEEvent(w, "message_delta", map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]int{
			"output_tokens": outputTokens,
		},
	}); err != nil {
		return err
	}
	if err := writeAnthropicSSEEvent(w, "message_stop", map[string]interface{}{
		"type": "message_stop",
	}); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func convertOpenAINonStreamToAnthropic(resp OpenAIChatResponse, spoofModel string) AnthropicMessageResponse {
	out := AnthropicMessageResponse{
		ID:    fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		Type:  "message",
		Role:  "assistant",
		Model: spoofModel,
		Usage: AnthropicUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}

	if len(resp.Choices) == 0 {
		return out
	}

	message := resp.Choices[0].Message
	if strings.TrimSpace(message.Content) != "" {
		out.Content = append(out.Content, AnthropicContentBlock{
			Type: "text",
			Text: message.Content,
		})
	}
	for _, tc := range message.ToolCalls {
		input := safeJSONRawMessage(tc.Function.Arguments)
		out.Content = append(out.Content, AnthropicContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}
	out.StopReason = mapFinishReason(resp.Choices[0].FinishReason)
	return out
}

func safeJSONRawMessage(raw string) json.RawMessage {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return json.RawMessage(`{}`)
	}
	if !json.Valid([]byte(raw)) {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(raw)
}

func writeAnthropicSSEEvent(w io.Writer, event string, payload interface{}) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", string(b)); err != nil {
		return err
	}
	return nil
}

func mapFinishReason(v string) string {
	switch v {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	default:
		return "end_turn"
	}
}
