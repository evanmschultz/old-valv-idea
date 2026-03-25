package openai

import (
	"crypto/rand"
	"encoding/hex"
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
	Model               string    `json:"model"`
	Messages            []Message `json:"messages"`
	Stream              *bool     `json:"stream,omitempty"`
	Temperature         *float64  `json:"temperature,omitempty"`
	TopP                *float64  `json:"top_p,omitempty"`
	N                   *int      `json:"n,omitempty"`
	MaxTokens           *int      `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int      `json:"max_completion_tokens,omitempty"`
	User                string    `json:"user,omitempty"`
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
	if r.Stream != nil && *r.Stream {
		return unsupportedFeature("streaming chat completions are not supported", "stream")
	}
	if r.N != nil && *r.N != 1 {
		return unsupportedFeature("only n=1 is supported", "n")
	}
	if r.Temperature != nil && (*r.Temperature < 0 || *r.Temperature > 2) {
		return invalidRequest("temperature must be between 0 and 2", "temperature")
	}
	if r.TopP != nil && (*r.TopP < 0 || *r.TopP > 1) {
		return invalidRequest("top_p must be between 0 and 1", "top_p")
	}
	if r.MaxTokens != nil && *r.MaxTokens < 1 {
		return invalidRequest("max_tokens must be greater than 0", "max_tokens")
	}
	if r.MaxCompletionTokens != nil && *r.MaxCompletionTokens < 1 {
		return invalidRequest("max_completion_tokens must be greater than 0", "max_completion_tokens")
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
