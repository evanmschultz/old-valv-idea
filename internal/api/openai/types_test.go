package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRequestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request Request
		wantErr bool
	}{
		{
			name: "valid",
			request: Request{
				Model: "gpt-5.2",
				Messages: []Message{{
					Role:    RoleUser,
					Content: "hello",
				}},
			},
		},
		{
			name: "missing model",
			request: Request{
				Messages: []Message{{Role: RoleUser, Content: "hello"}},
			},
			wantErr: true,
		},
		{
			name: "missing messages",
			request: Request{
				Model: "gpt-5.2",
			},
			wantErr: true,
		},
		{
			name: "empty message content",
			request: Request{
				Model: "gpt-5.2",
				Messages: []Message{{
					Role:    RoleUser,
					Content: "   ",
				}},
			},
			wantErr: true,
		},
		{
			name: "streaming supported",
			request: Request{
				Model:  "gpt-5.2",
				Stream: boolPtr(true),
				Messages: []Message{{
					Role:    RoleUser,
					Content: "hello",
				}},
			},
			wantErr: false,
		},
		{
			name: "multiple choices unsupported",
			request: Request{
				Model: "gpt-5.2",
				N:     intPtr(2),
				Messages: []Message{{
					Role:    RoleUser,
					Content: "hello",
				}},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.request.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("Validate() error = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestDecodeRequest(t *testing.T) {
	t.Parallel()

	req, err := DecodeRequest(strings.NewReader(`{"model":"gpt-5.2","messages":[{"role":"user","content":"hello"}]}`))
	if err != nil {
		t.Fatalf("DecodeRequest() error = %v", err)
	}
	if req.Model != "gpt-5.2" {
		t.Fatalf("DecodeRequest().Model = %q, want gpt-5.2", req.Model)
	}
	if got := req.Messages[0].Role; got != RoleUser {
		t.Fatalf("DecodeRequest().Messages[0].Role = %q, want %q", got, RoleUser)
	}
}

func TestDecodeRequestRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	if _, err := DecodeRequest(strings.NewReader(`{`)); err == nil {
		t.Fatal("DecodeRequest() error = nil, want parse error")
	}
}

func TestEncodeResponse(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := EncodeResponse(&buf, Result{
		ID:      "chatcmpl-test",
		Model:   "gpt-5.2",
		Content: "hello",
		Usage: Usage{
			PromptTokens:     1,
			CompletionTokens: 2,
			TotalTokens:      3,
		},
		CreatedAt: time.Unix(1700000000, 0).UTC(),
	}.Response(time.Unix(1700000001, 0).UTC()))
	if err != nil {
		t.Fatalf("EncodeResponse() error = %v", err)
	}

	var response Response
	if err := json.Unmarshal(buf.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response error = %v", err)
	}
	if response.Object != "chat.completion" {
		t.Fatalf("response object = %q, want chat.completion", response.Object)
	}
}

func TestEncodeError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := EncodeError(&buf, RequestError{
		Status:  400,
		Type:    errorTypeInvalidRequest,
		Message: "bad request",
		Param:   "model",
		Code:    "invalid_request_error",
	})
	if err != nil {
		t.Fatalf("EncodeError() error = %v", err)
	}

	var payload ErrorResponse
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal payload error = %v", err)
	}
	if payload.Error.Param != "model" {
		t.Fatalf("payload error param = %q, want model", payload.Error.Param)
	}
}

func TestErrorHelpers(t *testing.T) {
	t.Parallel()

	got := errorFrom(errors.New("boom"))
	if got.Code != "server_error" {
		t.Fatalf("errorFrom().Code = %q, want server_error", got.Code)
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func intPtr(v int) *int {
	return &v
}
