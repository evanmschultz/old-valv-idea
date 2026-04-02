package openai

import (
	"encoding/json"
	"fmt"
	"io"
)

type ErrorResponse struct {
	Error RequestError `json:"error"`
}

func DecodeRequest(r io.Reader) (Request, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return Request{}, fmt.Errorf("decode chat completions request: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return Request{}, invalidRequest(fmt.Sprintf("decode chat completions request: invalid JSON: %v", err), "body")
	}
	for field := range raw {
		if _, ok := openAIChatCompletionFields[field]; !ok {
			return Request{}, unsupportedFeature(fmt.Sprintf("field %q is not supported by valv api compatibility", field), field)
		}
	}
	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		return Request{}, invalidRequest(fmt.Sprintf("decode chat completions request: %v", err), "body")
	}
	if err := req.Validate(); err != nil {
		return Request{}, err
	}
	return req.Normalize(), nil
}

func EncodeResponse(w io.Writer, response Response) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(response); err != nil {
		return fmt.Errorf("encode chat completions response: %w", err)
	}
	return nil
}

func EncodeError(w io.Writer, err error) error {
	payload := ErrorResponse{Error: errorFrom(err)}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		return fmt.Errorf("encode chat completions error: %w", err)
	}
	return nil
}

func errorFrom(err error) RequestError {
	if err == nil {
		return serverError("unknown error")
	}
	var requestErr RequestError
	if asRequestError(err, &requestErr) {
		return requestErr
	}
	return serverError(err.Error())
}

func asRequestError(err error, target *RequestError) bool {
	if err == nil || target == nil {
		return false
	}
	if requestErr, ok := err.(RequestError); ok {
		*target = requestErr
		return true
	}
	if requestErr, ok := err.(*RequestError); ok {
		*target = *requestErr
		return true
	}
	return false
}
