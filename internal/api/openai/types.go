package openai

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const ChatCompletionsPath = "/v1/chat/completions"

type Role string

const (
	RoleSystem    Role = "system"
	RoleDeveloper Role = "developer"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleFunction  Role = "function"
)

type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

type Request struct {
	Model               string          `json:"model"`
	Messages            []Message       `json:"messages"`
	Stream              *bool           `json:"stream,omitempty"`
	Temperature         *float64        `json:"temperature,omitempty"`
	TopP                *float64        `json:"top_p,omitempty"`
	N                   *int            `json:"n,omitempty"`
	MaxTokens           *int            `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int            `json:"max_completion_tokens,omitempty"`
	User                string          `json:"user,omitempty"`
	PresencePenalty     *float64        `json:"presence_penalty,omitempty"`
	FrequencyPenalty    *float64        `json:"frequency_penalty,omitempty"`
	LogitBias           json.RawMessage `json:"logit_bias,omitempty"`
	Stop                json.RawMessage `json:"stop,omitempty"`
	Tools               json.RawMessage `json:"tools,omitempty"`
	ToolChoice          json.RawMessage `json:"tool_choice,omitempty"`
	Functions           json.RawMessage `json:"functions,omitempty"`
	ResponseFormat      json.RawMessage `json:"response_format,omitempty"`
	Seed                *int            `json:"seed,omitempty"`
	TopLogprobs         *int            `json:"top_logprobs,omitempty"`
	Logprobs            *bool           `json:"logprobs,omitempty"`
	ParallelToolCalls   *bool           `json:"parallel_tool_calls,omitempty"`
	FunctionCall        json.RawMessage `json:"function_call,omitempty"`
	Store               *bool           `json:"store,omitempty"`
	Metadata            json.RawMessage `json:"metadata,omitempty"`
	ReasoningEffort     string          `json:"reasoning_effort,omitempty"`
	Prediction          json.RawMessage `json:"prediction,omitempty"`
	ServiceTier         string          `json:"service_tier,omitempty"`
	StreamOptions       json.RawMessage `json:"stream_options,omitempty"`
	Modalities          json.RawMessage `json:"modalities,omitempty"`
}

var openAIChatCompletionFields = map[string]struct{}{
	"model":                 {},
	"messages":              {},
	"stream":                {},
	"temperature":           {},
	"top_p":                 {},
	"n":                     {},
	"max_tokens":            {},
	"max_completion_tokens": {},
	"user":                  {},
	"presence_penalty":      {},
	"frequency_penalty":     {},
	"logit_bias":            {},
	"stop":                  {},
	"tools":                 {},
	"tool_choice":           {},
	"functions":             {},
	"response_format":       {},
	"seed":                  {},
	"top_logprobs":          {},
	"logprobs":              {},
	"parallel_tool_calls":   {},
	"function_call":         {},
	"store":                 {},
	"metadata":              {},
	"reasoning_effort":      {},
	"prediction":            {},
	"service_tier":          {},
	"stream_options":        {},
	"modalities":            {},
}

type Result struct {
	ID                string
	Model             string
	Content           string
	FinishReason      string
	Usage             Usage
	CreatedAt         time.Time
	SystemFingerprint string
}

type Response struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	Choices           []Choice `json:"choices"`
	Usage             Usage    `json:"usage"`
	SystemFingerprint *string  `json:"system_fingerprint,omitempty"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (r Request) Validate() error {
	if strings.TrimSpace(r.Model) == "" {
		return invalidRequest("model is required", "model")
	}
	if len(r.Messages) == 0 {
		return invalidRequest("messages must contain at least one entry", "messages")
	}
	if r.N != nil && *r.N != 1 {
		return unsupportedFeature("only n=1 is supported", "n")
	}
	if r.Temperature != nil {
		return unsupportedFeature("temperature is not supported for api compatibility", "temperature")
	}
	if r.TopP != nil {
		return unsupportedFeature("top_p is not supported for api compatibility", "top_p")
	}
	if r.MaxTokens != nil {
		return unsupportedFeature("max_tokens is not supported for api compatibility", "max_tokens")
	}
	if r.MaxCompletionTokens != nil {
		return unsupportedFeature("max_completion_tokens is not supported for api compatibility", "max_completion_tokens")
	}
	if r.PresencePenalty != nil {
		return unsupportedFeature("presence_penalty is not supported for api compatibility", "presence_penalty")
	}
	if r.FrequencyPenalty != nil {
		return unsupportedFeature("frequency_penalty is not supported for api compatibility", "frequency_penalty")
	}
	if len(r.LogitBias) > 0 {
		return unsupportedFeature("logit_bias is not supported for api compatibility", "logit_bias")
	}
	if len(r.Stop) > 0 {
		return unsupportedFeature("stop is not supported for api compatibility", "stop")
	}
	if len(r.Tools) > 0 {
		return unsupportedFeature("tools is not supported for api compatibility", "tools")
	}
	if len(r.ToolChoice) > 0 {
		return unsupportedFeature("tool_choice is not supported for api compatibility", "tool_choice")
	}
	if len(r.Functions) > 0 {
		return unsupportedFeature("functions is not supported for api compatibility", "functions")
	}
	if len(r.ResponseFormat) > 0 {
		return unsupportedFeature("response_format is not supported for api compatibility", "response_format")
	}
	if r.Seed != nil {
		return unsupportedFeature("seed is not supported for api compatibility", "seed")
	}
	if r.TopLogprobs != nil {
		return unsupportedFeature("top_logprobs is not supported for api compatibility", "top_logprobs")
	}
	if r.Logprobs != nil {
		return unsupportedFeature("logprobs is not supported for api compatibility", "logprobs")
	}
	if r.ParallelToolCalls != nil {
		return unsupportedFeature("parallel_tool_calls is not supported for api compatibility", "parallel_tool_calls")
	}
	if len(r.FunctionCall) > 0 {
		return unsupportedFeature("function_call is not supported for api compatibility", "function_call")
	}
	if r.ReasoningEffort != "" {
		return unsupportedFeature("reasoning_effort is not supported for api compatibility", "reasoning_effort")
	}
	if len(r.Prediction) > 0 {
		return unsupportedFeature("prediction is not supported for api compatibility", "prediction")
	}
	if r.ServiceTier != "" {
		return unsupportedFeature("service_tier is not supported for api compatibility", "service_tier")
	}
	if len(r.StreamOptions) > 0 {
		return unsupportedFeature("stream_options is not supported for api compatibility", "stream_options")
	}
	if len(r.Modalities) > 0 {
		return unsupportedFeature("modalities is not supported for api compatibility", "modalities")
	}
	for i, message := range r.Messages {
		if err := message.Validate(i); err != nil {
			return err
		}
	}
	return nil
}

func (m Message) Validate(index int) error {
	switch m.Role {
	case RoleSystem, RoleDeveloper, RoleUser, RoleAssistant, RoleTool, RoleFunction:
	default:
		return invalidRequest(fmt.Sprintf("messages[%d].role must be a supported chat role", index), fmt.Sprintf("messages[%d].role", index))
	}
	if strings.TrimSpace(m.Content) == "" {
		return invalidRequest(fmt.Sprintf("messages[%d].content is required", index), fmt.Sprintf("messages[%d].content", index))
	}
	return nil
}

func (r Request) Normalize() Request {
	next := r
	next.Model = strings.TrimSpace(next.Model)
	next.User = strings.TrimSpace(next.User)
	for i := range next.Messages {
		next.Messages[i].Role = Role(strings.TrimSpace(string(next.Messages[i].Role)))
		next.Messages[i].Name = strings.TrimSpace(next.Messages[i].Name)
	}
	return next
}

func (r Request) HasStreaming() bool {
	return r.Stream != nil && *r.Stream
}

func (r Request) WantsMultipleChoices() bool {
	return r.N != nil && *r.N > 1
}

func NewID(prefix string) (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return prefix + hex.EncodeToString(raw[:]), nil
}

func (r Result) Response(now time.Time) Response {
	model := strings.TrimSpace(r.Model)
	if model == "" {
		model = "unknown"
	}
	id := strings.TrimSpace(r.ID)
	if id == "" {
		id = "chatcmpl-" + strings.Repeat("0", 24)
	}
	fingerprint := strings.TrimSpace(r.SystemFingerprint)
	var fingerprintPtr *string
	if fingerprint != "" {
		fingerprintPtr = &fingerprint
	}
	finishReason := strings.TrimSpace(r.FinishReason)
	if finishReason == "" {
		finishReason = "stop"
	}
	created := r.CreatedAt
	if created.IsZero() {
		created = now
	}
	return Response{
		ID:      id,
		Object:  "chat.completion",
		Created: created.UTC().Unix(),
		Model:   model,
		Choices: []Choice{{
			Index: 0,
			Message: Message{
				Role:    RoleAssistant,
				Content: r.Content,
			},
			FinishReason: finishReason,
		}},
		Usage:             r.Usage,
		SystemFingerprint: fingerprintPtr,
	}
}
