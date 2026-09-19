package httpx

import (
	"encoding/json"
	"net/http"
)

type Code string

const (
	CodeInvalidID     Code = "invalid_id"
	CodeNotFound      Code = "not_found"
	CodeInternalError Code = "internal_error"
	CodeMalformedRequest Code = "malformed_request"
	CodeValidationError Code = "validation_error"
	CodeBadRequest    Code = "bad_request"
	CodeUnauthorized  Code = "unauthorized"
	CodeForbidden     Code = "forbidden"
	CodeConflict      Code = "conflict"
	CodeRateLimitExceeded Code = "rate_limit_exceeded"
)

type ErrorEnvolope struct {
	Error ErrorObject `json:"error"`
}

type ErrorObject struct {
	Message string `json:"message"`
	Code    Code `json:"code"`
	Field   string `json:"field,omitempty"`
}

func Error(w http.ResponseWriter, status int, message string, code Code) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorEnvolope{
		Error: ErrorObject{
			Message: message,
			Code:    code,
		},
	})
}

func ValidationError(w http.ResponseWriter, status int, message string, code Code, field string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorEnvolope{
		Error: ErrorObject{
			Message: message,
			Code:    code,
			Field:   field,
		},
	})
}
