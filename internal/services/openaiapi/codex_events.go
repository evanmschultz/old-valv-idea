package openaiapi

import (
	"bytes"
	"encoding/json"
	"strings"

	openai "github.com/evanmschultz/valv/internal/api/openai"
)

type codexEventStream struct {
	buffer      bytes.Buffer
	onAssistant func(string) error
	providerErr error
	sawContent  bool
}

func newCodexEventStream(onAssistant func(string) error) *codexEventStream {
	return &codexEventStream{onAssistant: onAssistant}
}

func (s *codexEventStream) Write(p []byte) (int, error) {
	for _, b := range p {
		if b == '\n' {
			if err := s.processLine(s.buffer.String()); err != nil {
				return 0, err
			}
			s.buffer.Reset()
			continue
		}
		s.buffer.WriteByte(b)
	}
	return len(p), nil
}

func (s *codexEventStream) Flush() error {
	if s.buffer.Len() == 0 {
		return nil
	}
	line := s.buffer.String()
	s.buffer.Reset()
	return s.processLine(line)
}

func (s *codexEventStream) processLine(line string) error {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	if content, ok := extractAssistantContent(line); ok && strings.TrimSpace(content) != "" {
		s.sawContent = true
		if s.onAssistant != nil {
			return s.onAssistant(content)
		}
	}
	if s.providerErr == nil {
		if reqErr, ok := parseProviderRequestError(line); ok {
			s.providerErr = reqErr
		}
	}
	return nil
}

func extractProviderRequestError(err error) (error, bool) {
	if err == nil {
		return nil, false
	}
	return parseProviderRequestError(err.Error())
}

func parseProviderRequestError(text string) (error, bool) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.Index(line, "{")
		if idx < 0 {
			continue
		}
		if reqErr, ok := parseProviderRequestErrorJSON(line[idx:]); ok {
			return reqErr, true
		}
	}
	return nil, false
}

type codexProviderEnvelope struct {
	Type    string               `json:"type"`
	Role    string               `json:"role"`
	Content string               `json:"content"`
	Status  int                  `json:"status"`
	Message string               `json:"message"`
	Error   *codexProviderDetail `json:"error"`
	Item    *codexProviderItem   `json:"item"`
}

type codexProviderDetail struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Param   string `json:"param"`
}

type codexProviderItem struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content string `json:"content"`
}

func parseProviderRequestErrorJSON(raw string) (error, bool) {
	var envelope codexProviderEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil, false
	}
	if envelope.Error != nil && strings.TrimSpace(envelope.Error.Message) != "" {
		return openai.RequestError{
			Status:  statusOrDefault(envelope.Status, 400),
			Type:    defaultString(envelope.Error.Type, "invalid_request_error"),
			Message: strings.TrimSpace(envelope.Error.Message),
			Param:   strings.TrimSpace(envelope.Error.Param),
			Code:    strings.TrimSpace(envelope.Error.Code),
		}, true
	}
	if nested := strings.TrimSpace(envelope.Message); nested != "" {
		if reqErr, ok := parseProviderRequestErrorJSON(nested); ok {
			return reqErr, true
		}
	}
	return nil, false
}

func extractAssistantContent(raw string) (string, bool) {
	var envelope codexProviderEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return "", false
	}
	switch {
	case envelope.Type == "message" && strings.EqualFold(strings.TrimSpace(envelope.Role), "assistant"):
		return strings.TrimSpace(envelope.Content), true
	case envelope.Item != nil && envelope.Item.Type == "message" && strings.EqualFold(strings.TrimSpace(envelope.Item.Role), "assistant"):
		return strings.TrimSpace(envelope.Item.Content), true
	default:
		return "", false
	}
}

func statusOrDefault(status, fallback int) int {
	if status > 0 {
		return status
	}
	return fallback
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return fallback
}
