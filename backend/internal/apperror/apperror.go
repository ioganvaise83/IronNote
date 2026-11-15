// Package apperror defines the single error vocabulary of the API.
//
// Handlers never write raw errors: they return this type, and the response layer
// turns it into {"error":{"code","message","details"}}. That keeps status codes
// consistent and prevents database internals from leaking to the client.
package apperror

import (
	"errors"
	"fmt"
	"net/http"
)

type Code string

const (
	CodeValidation   Code = "validation_error"
	CodeNotFound     Code = "not_found"
	CodeConflict     Code = "conflict"
	CodeRuleViolated Code = "rule_violation"
	CodeInternal     Code = "internal_error"
)

type Error struct {
	Code    Code
	Message string
	// Details carries field-level validation messages, never SQL text.
	Details map[string]string
	// Err is the wrapped cause; it is logged, never serialised.
	Err error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) Status() int {
	switch e.Code {
	case CodeValidation:
		return http.StatusBadRequest
	case CodeNotFound:
		return http.StatusNotFound
	case CodeConflict:
		return http.StatusConflict
	case CodeRuleViolated:
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

func Validation(message string, details map[string]string) *Error {
	return &Error{Code: CodeValidation, Message: message, Details: details}
}

func NotFound(what string) *Error {
	return &Error{Code: CodeNotFound, Message: what + " не найден"}
}

func Conflict(message string) *Error {
	return &Error{Code: CodeConflict, Message: message}
}

func Rule(message string) *Error {
	return &Error{Code: CodeRuleViolated, Message: message}
}

func Internal(err error) *Error {
	return &Error{Code: CodeInternal, Message: "Внутренняя ошибка сервера", Err: err}
}

// As extracts an *Error from a wrapped chain.
func As(err error) (*Error, bool) {
	var target *Error
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}
