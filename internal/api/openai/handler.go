package openai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/evanmschultz/valv/internal/domain"
)

type Executor interface {
	Complete(context.Context, Request) (Result, error)
}

type Streamer interface {
	Stream(context.Context, Request, func(string) error) error
}

type Clock interface {
	Now() time.Time
}

type Options struct {
	Clock  Clock
	Logger *log.Logger
}

type Handler struct {
	executor Executor
	clock    Clock
	logger   *log.Logger
}

func NewHandler(executor Executor, options Options) (*Handler, error) {
	if executor == nil {
		return nil, fmt.Errorf("new openai handler: executor is required")
	}
	clock := options.Clock
	if clock == nil {
		clock = systemClock{}
	}
	return &Handler{
		executor: executor,
		clock:    clock,
		logger:   options.Logger,
	}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != ChatCompletionsPath {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, invalidRequest("method not allowed", ""))
		return
	}

	req, err := DecodeRequest(r.Body)
	if err != nil {
		h.debug("rejecting chat completion request", "error", err)
		writeError(w, statusFromError(err), err)
		return
	}

	result, err := h.executor.Complete(r.Context(), req)
	if err != nil {
		h.debug("chat completion executor failed", "error", err)
		writeError(w, statusFromExecutorError(err), err)
		return
	}

	response := result.Response(h.clock.Now())
	if result.Model != "" {
		response.Model = strings.TrimSpace(result.Model)
	}
	if response.ID == "" {
		id, idErr := NewID("chatcmpl-")
		if idErr != nil {
			h.debug("failed to generate completion id", "error", idErr)
			writeError(w, http.StatusInternalServerError, idErr)
			return
		}
		response.ID = id
	}
	if req.HasStreaming() {
		if streamer, ok := h.executor.(Streamer); ok {
			h.writeExecutorStream(w, r, req, streamer, response.ID)
			return
		}
		writeStream(w, response)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) writeExecutorStream(w http.ResponseWriter, r *http.Request, req Request, streamer Streamer, responseID string) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = "unknown"
	}
	created := h.clock.Now().UTC().Unix()
	flusher, _ := w.(http.Flusher)
	started := false

	writeStart := func() {
		if started {
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		started = true
	}
	writeContent := func(content string) error {
		writeStart()
		writeSSEChunk(w, streamResponse{
			ID:      responseID,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   model,
			Choices: []streamChoice{{
				Index: 0,
				Delta: streamDelta{
					Role:    "assistant",
					Content: content,
				},
				Logprobs:     nil,
				FinishReason: nil,
			}},
			Usage: nil,
		})
		if flusher != nil {
			flusher.Flush()
		}
		return nil
	}

	if err := streamer.Stream(r.Context(), req, writeContent); err != nil {
		if started {
			h.debug("chat completion streaming executor failed after stream start", "error", err)
			return
		}
		h.debug("chat completion streaming executor failed", "error", err)
		writeError(w, statusFromExecutorError(err), err)
		return
	}

	writeStart()
	finish := "stop"
	writeSSEChunk(w, streamResponse{
		ID:      responseID,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []streamChoice{{
			Index:        0,
			Delta:        streamDelta{},
			Logprobs:     nil,
			FinishReason: &finish,
		}},
		Usage: nil,
	})
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
	if flusher != nil {
		flusher.Flush()
	}
}

type streamDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type streamChoice struct {
	Index        int         `json:"index"`
	Delta        streamDelta `json:"delta"`
	Logprobs     any         `json:"logprobs"`
	FinishReason *string     `json:"finish_reason"`
}

type streamResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []streamChoice `json:"choices"`
	Usage   *Usage         `json:"usage"`
}

func writeStream(w http.ResponseWriter, response Response) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	content := ""
	if len(response.Choices) > 0 {
		content = response.Choices[0].Message.Content
	}

	streamUsage := (*Usage)(nil)
	first := streamResponse{
		ID:      response.ID,
		Object:  "chat.completion.chunk",
		Created: response.Created,
		Model:   response.Model,
		Choices: []streamChoice{{
			Index: 0,
			Delta: streamDelta{
				Role:    "assistant",
				Content: content,
			},
			Logprobs:     nil,
			FinishReason: nil,
		}},
		Usage: streamUsage,
	}
	writeSSEChunk(w, first)

	finish := "stop"
	final := streamResponse{
		ID:      response.ID,
		Object:  "chat.completion.chunk",
		Created: response.Created,
		Model:   response.Model,
		Choices: []streamChoice{{
			Index:        0,
			Delta:        streamDelta{},
			Logprobs:     nil,
			FinishReason: &finish,
		}},
		Usage: streamUsage,
	}
	writeSSEChunk(w, final)
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
}

func writeSSEChunk(w http.ResponseWriter, chunk streamResponse) {
	var buf bytes.Buffer
	enc := newJSONEncoder(&buf)
	if err := enc.Encode(chunk); err != nil {
		return
	}
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(buf.Bytes())
	_, _ = w.Write([]byte("\n"))
}

func (h *Handler) debug(msg string, keyvals ...any) {
	if h == nil || h.logger == nil {
		return
	}
	h.logger.Debug(msg, keyvals...)
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now().UTC()
}

func statusFromError(err error) int {
	var requestErr RequestError
	if asRequestError(err, &requestErr) && requestErr.Status != 0 {
		return requestErr.Status
	}
	if errors.Is(err, context.Canceled) {
		return 499
	}
	if errors.Is(err, domain.ErrUnboundProject) {
		return http.StatusConflict
	}
	return http.StatusBadRequest
}

func statusFromExecutorError(err error) int {
	var requestErr RequestError
	if asRequestError(err, &requestErr) && requestErr.Status != 0 {
		return requestErr.Status
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, domain.ErrUnboundProject) {
		return statusFromError(err)
	}
	return http.StatusInternalServerError
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = EncodeError(w, err)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := jsonEncoder{w: w}
	_ = enc.Encode(value)
}

type jsonEncoder struct {
	w http.ResponseWriter
}

func (e jsonEncoder) Encode(value any) error {
	enc := newJSONEncoder(e.w)
	return enc.Encode(value)
}
