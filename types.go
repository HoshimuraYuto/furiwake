package main

import "encoding/json"

const (
	ProviderTypePassthrough = "passthrough"
	ProviderTypeOpenAI      = "openai"
	ProviderTypeChatGPT     = "chatgpt"

	AuthTypeNone   = "none"
	AuthTypeBearer = "bearer"
	AuthTypeCodex  = "codex"
)

type Config struct {
	Listen          string                    `yaml:"listen"`
	SpoofModel      string                    `yaml:"spoof_model"`
	DefaultProvider string                    `yaml:"default_provider"`
	TimeoutSeconds  int                       `yaml:"timeout_seconds"`
	Providers       map[string]ProviderConfig `yaml:"providers"`
}

type ProviderConfig struct {
	Type            string     `yaml:"type"`
	URL             string     `yaml:"url"`
	Model           string     `yaml:"model"`
	ReasoningEffort string     `yaml:"reasoning_effort"`
	Auth            AuthConfig `yaml:"auth"`
}

type AuthConfig struct {
	Type     string `yaml:"type"`
	TokenEnv string `yaml:"token_env"`
}

type AnthropicMessageRequest struct {
	Model      string             `json:"model"`
	MaxTokens  int                `json:"max_tokens,omitempty"`
	System     interface{}        `json:"system,omitempty"`
	Messages   []AnthropicMessage `json:"messages"`
	Stream     bool               `json:"stream,omitempty"`
	Tools      []AnthropicTool    `json:"tools,omitempty"`
	ToolChoice interface{}        `json:"tool_choice,omitempty"`
}

type AnthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type AnthropicContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   interface{}     `json:"content,omitempty"`
}

type AnthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type AnthropicMessageResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"`
	Role         string                  `json:"role"`
	Content      []AnthropicContentBlock `json:"content"`
	Model        string                  `json:"model"`
	StopReason   string                  `json:"stop_reason,omitempty"`
	StopSequence string                  `json:"stop_sequence,omitempty"`
	Usage        AnthropicUsage          `json:"usage"`
}

type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type CountTokensRequest struct {
	Model    string             `json:"model"`
	System   interface{}        `json:"system,omitempty"`
	Messages []AnthropicMessage `json:"messages"`
}

type CountTokensResponse struct {
	InputTokens int `json:"input_tokens"`
}

type OpenAIChatRequest struct {
	Model         string               `json:"model"`
	Messages      []OpenAIMessage      `json:"messages"`
	Stream        bool                 `json:"stream,omitempty"`
	StreamOptions *OpenAIStreamOptions `json:"stream_options,omitempty"`
	MaxTokens     int                  `json:"max_completion_tokens,omitempty"`
	Tools         []OpenAITool         `json:"tools,omitempty"`
	ToolChoice    interface{}          `json:"tool_choice,omitempty"`
}

type OpenAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

type OpenAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
}

type OpenAITool struct {
	Type     string                   `json:"type"`
	Function OpenAIFunctionDefinition `json:"function"`
}

type OpenAIFunctionDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type OpenAIToolCall struct {
	ID       string             `json:"id"`
	Index    int                `json:"index,omitempty"`
	Type     string             `json:"type"`
	Function OpenAIToolFunction `json:"function"`
}

type OpenAIToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAIChatResponse struct {
	ID      string         `json:"id"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   OpenAIUsage    `json:"usage"`
}

type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type OpenAIChatStreamChunk struct {
	ID      string               `json:"id"`
	Model   string               `json:"model"`
	Choices []OpenAIStreamChoice `json:"choices"`
	Usage   OpenAIUsage          `json:"usage,omitempty"`
}

type OpenAIStreamChoice struct {
	Index        int         `json:"index"`
	Delta        OpenAIDelta `json:"delta"`
	FinishReason string      `json:"finish_reason"`
}

type OpenAIDelta struct {
	Role      string           `json:"role,omitempty"`
	Content   string           `json:"content,omitempty"`
	ToolCalls []OpenAIToolCall `json:"tool_calls,omitempty"`
}

type ChatGPTResponsesRequest struct {
	Model             string               `json:"model"`
	Instructions      string               `json:"instructions,omitempty"`
	Input             []ResponsesInputItem `json:"input"`
	Tools             []ResponsesTool      `json:"tools,omitempty"`
	ToolChoice        interface{}          `json:"tool_choice,omitempty"`
	ParallelToolCalls bool                 `json:"parallel_tool_calls"`
	Reasoning         *ReasoningConfig     `json:"reasoning,omitempty"`
	Store             bool                 `json:"store"`
	Stream            bool                 `json:"stream,omitempty"`
	Include           []string             `json:"include,omitempty"`
}

type ReasoningConfig struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary"`
}

type ResponsesInputItem struct {
	Type      string `json:"type"`
	Role      string `json:"role,omitempty"`
	Content   string `json:"content,omitempty"`
	ID        string `json:"id,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    string `json:"output,omitempty"`
}

type ResponsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}
