package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Server) proxyChatGPT(
	ctx context.Context,
	w http.ResponseWriter,
	routeName string,
	provider ProviderConfig,
	model string,
	reasoningEffort string,
	anthropicReq AnthropicMessageRequest,
	incomingHeaders http.Header,
) {
	// Codex requires stream:true for all requests. Force it regardless of the
	// original caller's preference and handle the non-streaming case by
	// collecting the SSE stream internally.
	req := translateAnthropicToResponses(anthropicReq, model, reasoningEffort)
	req.Stream = true
	payload, err := json.Marshal(req)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to encode upstream request")
		return
	}

	// DEBUG: log outgoing request to Codex
	s.logger.Debugf("[CODEX-REQ] payload=%s", truncateForLog(string(payload), 2000))

	resp, err := s.doProviderRequestWithRetry(
		ctx,
		http.MethodPost,
		provider.URL,
		payload,
		incomingHeaders,
		provider,
		true, // always stream toward Codex
		routeName,
		req.Model,
		req.Reasoning.Effort,
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
		if err := convertResponsesStreamToAnthropic(w, resp.Body, s.cfg.SpoofModel, s.logger); err != nil {
			s.logger.Errorf("responses stream translation failed: %v", err)
		}
		return
	}

	// Non-streaming caller: collect the SSE stream and build a JSON response
	// from the response.completed event.
	raw, err := collectResponsesStreamAsJSON(resp.Body, s.logger)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to collect upstream stream: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, convertResponsesJSONToAnthropic(raw, s.cfg.SpoofModel))
}

func translateAnthropicToResponses(req AnthropicMessageRequest, model string, reasoningEffort string) ChatGPTResponsesRequest {
	out := ChatGPTResponsesRequest{
		Model:             model,
		Instructions:      NormalizeSystemText(req.System),
		Input:             translateAnthropicMessagesToResponsesInput(req.Messages),
		ParallelToolCalls: false,
		Reasoning:         &ReasoningConfig{Summary: "auto"},
		Store:             false,
		Stream:            req.Stream,
		Include:           []string{"reasoning.encrypted_content"},
	}
	if normalized := NormalizeReasoningEffort(reasoningEffort); normalized != "" {
		out.Reasoning.Effort = normalized
	}

	if len(req.Tools) > 0 {
		out.Tools = translateAnthropicToolsToResponses(req.Tools)
	}
	if req.ToolChoice != nil {
		out.ToolChoice = translateToolChoiceToResponses(req.ToolChoice)
	}
	return out
}

func translateAnthropicMessagesToResponsesInput(messages []AnthropicMessage) []ResponsesInputItem {
	out := make([]ResponsesInputItem, 0, len(messages))
	for _, message := range messages {
		role := message.Role
		if role != "user" && role != "assistant" {
			continue
		}

		if text, ok := message.Content.(string); ok {
			if strings.TrimSpace(text) == "" {
				continue
			}
			out = append(out, ResponsesInputItem{
				Type:    "message",
				Role:    role,
				Content: text,
			})
			continue
		}

		blocks := normalizeContentToBlocks(message.Content)
		textParts := make([]string, 0, len(blocks))
		flushText := func() {
			if len(textParts) == 0 {
				return
			}
			out = append(out, ResponsesInputItem{
				Type:    "message",
				Role:    role,
				Content: strings.Join(textParts, "\n"),
			})
			textParts = textParts[:0]
		}

		for i, block := range blocks {
			switch block.Type {
			case "text":
				if strings.TrimSpace(block.Text) != "" {
					textParts = append(textParts, block.Text)
				}
			case "tool_use":
				flushText()
				callID := strings.TrimSpace(block.ID)
				if callID == "" {
					callID = fmt.Sprintf("toolu_%d_%d", time.Now().UnixNano(), i)
				}
				out = append(out, ResponsesInputItem{
					Type:      "function_call",
					ID:        "fc_" + callID,
					CallID:    callID,
					Name:      block.Name,
					Arguments: string(safeJSONRawMessage(string(block.Input))),
				})
			case "tool_result":
				flushText()
				if strings.TrimSpace(block.ToolUseID) == "" {
					continue
				}
				outputText := extractToolResultText(block.Content)
				if outputText == "" {
					outputText = "(empty)"
				}
				out = append(out, ResponsesInputItem{
					Type:   "function_call_output",
					CallID: block.ToolUseID,
					Output: outputText,
				})
			}
		}
		flushText()
	}
	return out
}

func translateAnthropicToolsToResponses(tools []AnthropicTool) []ResponsesTool {
	out := make([]ResponsesTool, 0, len(tools))
	for _, tool := range tools {
		parameters := tool.InputSchema
		if len(parameters) == 0 {
			parameters = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out = append(out, ResponsesTool{
			Type:        "function",
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  parameters,
		})
	}
	return out
}

func translateToolChoiceToResponses(v interface{}) interface{} {
	m, ok := v.(map[string]interface{})
	if !ok {
		return "auto"
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
			"name": name,
		}
	default:
		return "auto"
	}
}

type responsesStreamState struct {
	messageID     string
	blockIndex    int
	textBlocks    map[string]int
	toolBlocks    map[int]int
	openBlocks    map[int]bool
	stopReason    string
	outputTokens  int
	messageOpened bool
}

func convertResponsesStreamToAnthropic(w http.ResponseWriter, src io.Reader, spoofModel string, logger *Logger) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming unsupported by response writer")
	}

	state := &responsesStreamState{
		messageID:    fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		blockIndex:   -1,
		textBlocks:   map[string]int{},
		toolBlocks:   map[int]int{},
		openBlocks:   map[int]bool{},
		stopReason:   "end_turn",
		outputTokens: 0,
	}

	sendMessageStart := func() error {
		if state.messageOpened {
			return nil
		}
		state.messageOpened = true
		return writeAnthropicSSEEvent(w, "message_start", map[string]interface{}{
			"type": "message_start",
			"message": map[string]interface{}{
				"id":            state.messageID,
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
		})
	}

	if err := readSSEEvents(src, func(eventName, data string) error {
		data = strings.TrimSpace(data)
		if data == "" {
			return nil
		}
		if data == "[DONE]" {
			return errSSEStreamDone
		}

		// DEBUG: log raw SSE event from Codex
		logger.Debugf("[CODEX-SSE] event=%s data=%s", eventName, truncateForLog(data, 500))

		var event map[string]interface{}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			logger.Warnf("[CODEX-SSE] JSON parse error: %v", err)
			return nil
		}

		if eventType, _ := event["type"].(string); eventType == "response.created" {
			if err := applyResponsesEventToAnthropic(w, event, eventName, state); err != nil {
				return err
			}
			if err := sendMessageStart(); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		}

		if err := sendMessageStart(); err != nil {
			return err
		}
		if err := applyResponsesEventToAnthropic(w, event, eventName, state); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}); err != nil {
		return err
	}

	logger.Debugf("[CODEX-SSE] stream ended, stopReason=%s outputTokens=%d openBlocks=%d messageOpened=%t",
		state.stopReason, state.outputTokens, len(state.openBlocks), state.messageOpened)

	if !state.messageOpened {
		if err := sendMessageStart(); err != nil {
			return err
		}
	}
	if err := closeOpenResponseBlocks(w, state); err != nil {
		return err
	}
	if err := writeAnthropicSSEEvent(w, "message_delta", map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   state.stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]int{
			"output_tokens": state.outputTokens,
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

func applyResponsesEventToAnthropic(
	w io.Writer,
	event map[string]interface{},
	currentEvent string,
	state *responsesStreamState,
) error {
	_ = currentEvent // type field is source of truth; currentEvent is retained for compatibility.
	t, _ := event["type"].(string)
	if t == "" {
		return nil
	}

	switch t {
	case "response.created":
		if resp, ok := event["response"].(map[string]interface{}); ok {
			if id, ok := resp["id"].(string); ok && id != "" {
				state.messageID = id
			}
		}
		return nil
	case "response.content_part.added":
		part, _ := event["part"].(map[string]interface{})
		partType, _ := part["type"].(string)
		if partType != "output_text" {
			return nil
		}
		state.blockIndex++
		idx := state.blockIndex
		key := responseTextKey(event)
		state.textBlocks[key] = idx
		state.openBlocks[idx] = true
		return writeAnthropicSSEEvent(w, "content_block_start", map[string]interface{}{
			"type":  "content_block_start",
			"index": idx,
			"content_block": map[string]interface{}{
				"type": "text",
				"text": "",
			},
		})
	case "response.output_text.delta":
		key := responseTextKey(event)
		idx, ok := state.textBlocks[key]
		if !ok {
			state.blockIndex++
			idx = state.blockIndex
			state.textBlocks[key] = idx
			state.openBlocks[idx] = true
			if err := writeAnthropicSSEEvent(w, "content_block_start", map[string]interface{}{
				"type":  "content_block_start",
				"index": idx,
				"content_block": map[string]interface{}{
					"type": "text",
					"text": "",
				},
			}); err != nil {
				return err
			}
		}
		delta, _ := event["delta"].(string)
		if delta == "" {
			return nil
		}
		return writeAnthropicSSEEvent(w, "content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": idx,
			"delta": map[string]interface{}{
				"type": "text_delta",
				"text": delta,
			},
		})
	case "response.output_text.done":
		key := responseTextKey(event)
		idx, ok := state.textBlocks[key]
		if !ok {
			return nil
		}
		delete(state.openBlocks, idx)
		return writeAnthropicSSEEvent(w, "content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": idx,
		})
	case "response.output_item.added":
		item, _ := event["item"].(map[string]interface{})
		itemType, _ := item["type"].(string)
		if itemType != "function_call" {
			return nil
		}
		outputIndex := responseOutputIndex(event)
		state.blockIndex++
		idx := state.blockIndex
		state.toolBlocks[outputIndex] = idx
		state.openBlocks[idx] = true
		callID, _ := item["call_id"].(string)
		name, _ := item["name"].(string)
		if callID == "" {
			callID = fmt.Sprintf("toolu_%d", time.Now().UnixNano())
		}
		return writeAnthropicSSEEvent(w, "content_block_start", map[string]interface{}{
			"type":  "content_block_start",
			"index": idx,
			"content_block": map[string]interface{}{
				"type":  "tool_use",
				"id":    callID,
				"name":  name,
				"input": map[string]interface{}{},
			},
		})
	case "response.function_call_arguments.delta":
		outputIndex := responseOutputIndex(event)
		idx, ok := state.toolBlocks[outputIndex]
		if !ok {
			state.blockIndex++
			idx = state.blockIndex
			state.toolBlocks[outputIndex] = idx
			state.openBlocks[idx] = true
			if err := writeAnthropicSSEEvent(w, "content_block_start", map[string]interface{}{
				"type":  "content_block_start",
				"index": idx,
				"content_block": map[string]interface{}{
					"type":  "tool_use",
					"id":    fmt.Sprintf("toolu_%d", time.Now().UnixNano()),
					"name":  "unknown",
					"input": map[string]interface{}{},
				},
			}); err != nil {
				return err
			}
		}
		delta, _ := event["delta"].(string)
		if delta == "" {
			return nil
		}
		return writeAnthropicSSEEvent(w, "content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": idx,
			"delta": map[string]interface{}{
				"type":         "input_json_delta",
				"partial_json": delta,
			},
		})
	case "response.function_call_arguments.done":
		outputIndex := responseOutputIndex(event)
		idx, ok := state.toolBlocks[outputIndex]
		if !ok {
			return nil
		}
		delete(state.openBlocks, idx)
		return writeAnthropicSSEEvent(w, "content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": idx,
		})
	case "response.completed":
		updateResponseCompletionState(event, state)
		return nil
	default:
		return nil
	}
}

func closeOpenResponseBlocks(w io.Writer, state *responsesStreamState) error {
	indexes := make([]int, 0, len(state.openBlocks))
	for idx := range state.openBlocks {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)

	for _, idx := range indexes {
		if err := writeAnthropicSSEEvent(w, "content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": idx,
		}); err != nil {
			return err
		}
	}
	state.openBlocks = map[int]bool{}
	return nil
}

func responseOutputIndex(event map[string]interface{}) int {
	if v, ok := event["output_index"].(float64); ok {
		return int(v)
	}
	return -1
}

func responseTextKey(event map[string]interface{}) string {
	outputIdx := responseOutputIndex(event)
	contentIdx := -1
	if v, ok := event["content_index"].(float64); ok {
		contentIdx = int(v)
	}
	return strconv.Itoa(outputIdx) + ":" + strconv.Itoa(contentIdx)
}

func updateResponseCompletionState(event map[string]interface{}, state *responsesStreamState) {
	resp, ok := event["response"].(map[string]interface{})
	if !ok {
		return
	}

	if usage, ok := resp["usage"].(map[string]interface{}); ok {
		if v, ok := usage["output_tokens"].(float64); ok {
			state.outputTokens = int(v)
		}
	}

	state.stopReason = "end_turn"
	if status, _ := resp["status"].(string); status == "incomplete" {
		if details, ok := resp["status_details"].(map[string]interface{}); ok {
			if reason, _ := details["reason"].(string); reason == "max_output_tokens" {
				state.stopReason = "max_tokens"
			}
		}
	}
	if output, ok := resp["output"].([]interface{}); ok {
		for _, item := range output {
			entry, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if itemType, _ := entry["type"].(string); itemType == "function_call" {
				state.stopReason = "tool_use"
				break
			}
		}
	}
}

func convertResponsesJSONToAnthropic(raw []byte, spoofModel string) AnthropicMessageResponse {
	out := AnthropicMessageResponse{
		ID:    fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		Type:  "message",
		Role:  "assistant",
		Model: spoofModel,
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		out.Content = []AnthropicContentBlock{{
			Type: "text",
			Text: string(raw),
		}}
		return out
	}

	text := extractResponseText(payload)
	if strings.TrimSpace(text) != "" {
		out.Content = append(out.Content, AnthropicContentBlock{
			Type: "text",
			Text: text,
		})
	}
	out.Content = append(out.Content, extractResponseToolUses(payload)...)

	if usage, ok := payload["usage"].(map[string]interface{}); ok {
		if v, ok := usage["input_tokens"].(float64); ok {
			out.Usage.InputTokens = int(v)
		}
		if v, ok := usage["output_tokens"].(float64); ok {
			out.Usage.OutputTokens = int(v)
		}
	}

	out.StopReason = determineResponsesStopReason(payload, out.Content)
	return out
}

func determineResponsesStopReason(payload map[string]interface{}, content []AnthropicContentBlock) string {
	for _, block := range content {
		if block.Type == "tool_use" {
			return "tool_use"
		}
	}
	if status, _ := payload["status"].(string); status == "incomplete" {
		if details, ok := payload["status_details"].(map[string]interface{}); ok {
			if reason, _ := details["reason"].(string); reason == "max_output_tokens" {
				return "max_tokens"
			}
		}
	}
	return "end_turn"
}

func extractResponseText(payload map[string]interface{}) string {
	if v, ok := payload["output_text"].(string); ok && strings.TrimSpace(v) != "" {
		return v
	}

	output, ok := payload["output"].([]interface{})
	if !ok {
		return ""
	}
	parts := []string{}
	for _, item := range output {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		itemType, _ := entry["type"].(string)
		if itemType != "" && itemType != "message" {
			continue
		}
		content, ok := entry["content"].([]interface{})
		if !ok {
			continue
		}
		for _, c := range content {
			block, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			if text, _ := block["text"].(string); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func extractResponseToolUses(payload map[string]interface{}) []AnthropicContentBlock {
	output, ok := payload["output"].([]interface{})
	if !ok {
		return nil
	}
	out := []AnthropicContentBlock{}
	for _, item := range output {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		itemType, _ := entry["type"].(string)
		if itemType != "function_call" {
			continue
		}
		callID, _ := entry["call_id"].(string)
		name, _ := entry["name"].(string)
		args, _ := entry["arguments"].(string)
		if callID == "" {
			callID = fmt.Sprintf("toolu_%d", time.Now().UnixNano())
		}
		out = append(out, AnthropicContentBlock{
			Type:  "tool_use",
			ID:    callID,
			Name:  name,
			Input: safeJSONRawMessage(args),
		})
	}
	return out
}

// collectResponsesStreamAsJSON reads a Codex SSE stream and returns the
// response JSON extracted from the response.completed event. This is used
// when the original caller requested a non-streaming response, but Codex
// requires stream:true on all requests.
func collectResponsesStreamAsJSON(src io.Reader, logger *Logger) ([]byte, error) {
	var responseJSON []byte
	err := readSSEEvents(src, func(_, data string) error {
		data = strings.TrimSpace(data)
		if data == "" || data == "[DONE]" {
			return nil
		}
		logger.Debugf("[CODEX-SSE-COLLECT] data=%s", truncateForLog(data, 500))
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil
		}
		if t, _ := event["type"].(string); t == "response.completed" {
			if resp, ok := event["response"]; ok {
				b, err := json.Marshal(resp)
				if err == nil {
					responseJSON = b
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if responseJSON == nil {
		return nil, fmt.Errorf("no response.completed event received from Codex stream")
	}
	return responseJSON, nil
}
