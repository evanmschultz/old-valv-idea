package openai

import "fmt"

const (
	errorTypeInvalidRequest = "invalid_request_error"
	errorTypeServer         = "server_error"
)

type RequestError struct {
	Status  int    `json:"-"`
	Type    string `json:"type"`
	Message string `json:"message"`
	Param   string `json:"param,omitempty"`
	Code    string `json:"code,omitempty"`
}

func (e RequestError) Error() string {
	switch {
	case e.Param != "" && e.Code != "":
		return fmt.Sprintf("%s: %s (param=%s code=%s)", e.Type, e.Message, e.Param, e.Code)
	case e.Param != "":
		return fmt.Sprintf("%s: %s (param=%s)", e.Type, e.Message, e.Param)
	case e.Code != "":
		return fmt.Sprintf("%s: %s (code=%s)", e.Type, e.Message, e.Code)
	default:
		return fmt.Sprintf("%s: %s", e.Type, e.Message)
	}
}

func invalidRequest(message string, param string) RequestError {
	return RequestError{
		Status:  400,
		Type:    errorTypeInvalidRequest,
		Message: message,
		Param:   param,
		Code:    "invalid_request_error",
	}
}

func unsupportedFeature(message string, param string) RequestError {
	return RequestError{
		Status:  400,
		Type:    errorTypeInvalidRequest,
		Message: message,
		Param:   param,
		Code:    "unsupported_feature",
	}
}

func serverError(message string) RequestError {
	return RequestError{
		Status:  500,
		Type:    errorTypeServer,
		Message: message,
		Code:    "server_error",
	}
}
