package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/evanmschultz/valv/internal/domain"
)

type stubExecutor struct {
	request Request
	result  Result
	err     error
	called  bool
}

func (s *stubExecutor) Complete(_ context.Context, req Request) (Result, error) {
	s.called = true
	s.request = req
	return s.result, s.err
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

func TestHandlerServeHTTPWritesChatCompletionResponse(t *testing.T) {
	t.Parallel()

	executor := &stubExecutor{
		result: Result{
			ID:           "chatcmpl-test",
			Model:        "gpt-5.2",
			Content:      "hello from codex",
			FinishReason: "stop",
			Usage: Usage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
			CreatedAt: time.Unix(1700000000, 0).UTC(),
		},
	}
	handler, err := NewHandler(executor, Options{Clock: fixedClock{now: time.Unix(1700000001, 0).UTC()}})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	body := bytes.NewBufferString(`{"model":"gpt-5.2","messages":[{"role":"user","content":"say hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, ChatCompletionsPath, body)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !executor.called {
		t.Fatal("executor was not called")
	}
	if got, want := rr.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}

	var response Response
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response error = %v", err)
	}
	if response.Object != "chat.completion" {
		t.Fatalf("response object = %q, want chat.completion", response.Object)
	}
	if response.ID != "chatcmpl-test" {
		t.Fatalf("response id = %q, want chatcmpl-test", response.ID)
	}
	if response.Model != "gpt-5.2" {
		t.Fatalf("response model = %q, want gpt-5.2", response.Model)
	}
	if got := response.Choices; len(got) != 1 {
		t.Fatalf("choices len = %d, want 1", len(got))
	}
	if got := response.Choices[0].Message.Role; got != RoleAssistant {
		t.Fatalf("choice role = %q, want %q", got, RoleAssistant)
	}
	if got := response.Choices[0].Message.Content; got != "hello from codex" {
		t.Fatalf("choice content = %q, want hello from codex", got)
	}
	if got := response.Choices[0].FinishReason; got != "stop" {
		t.Fatalf("finish reason = %q, want stop", got)
	}
	if got, want := response.Created, int64(1700000000); got != want {
		t.Fatalf("created = %d, want %d", got, want)
	}
	if got := response.Usage.TotalTokens; got != 15 {
		t.Fatalf("usage total tokens = %d, want 15", got)
	}
}

func TestHandlerRejectsStreamingRequests(t *testing.T) {
	t.Parallel()

	handler, err := NewHandler(&stubExecutor{}, Options{})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	body := bytes.NewBufferString(`{"model":"gpt-5.2","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, ChatCompletionsPath, body)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got, want := rr.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}

	var payload ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal error payload = %v", err)
	}
	if payload.Error.Code != "unsupported_feature" {
		t.Fatalf("error code = %q, want unsupported_feature", payload.Error.Code)
	}
}

func TestHandlerRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	handler, err := NewHandler(&stubExecutor{}, Options{})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, ChatCompletionsPath, bytes.NewBufferString(`{`))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got, want := rr.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestHandlerPropagatesExecutorFailure(t *testing.T) {
	t.Parallel()

	handler, err := NewHandler(&stubExecutor{err: errors.New("codex failed")}, Options{})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, ChatCompletionsPath, bytes.NewBufferString(`{"model":"gpt-5.2","messages":[{"role":"user","content":"hello"}]}`))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got, want := rr.Code, http.StatusInternalServerError; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}

	var payload ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal error payload = %v", err)
	}
	if payload.Error.Code != "server_error" {
		t.Fatalf("error code = %q, want server_error", payload.Error.Code)
	}
}

func TestHandlerMapsUnboundProjectToConflict(t *testing.T) {
	t.Parallel()

	handler, err := NewHandler(&stubExecutor{err: domain.ErrUnboundProject}, Options{})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, ChatCompletionsPath, bytes.NewBufferString(`{"model":"gpt-5.2","messages":[{"role":"user","content":"hello"}]}`))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got, want := rr.Code, http.StatusConflict; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}
