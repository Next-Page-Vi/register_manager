// Package apperr defines the error type returned by services and rendered by
// the API as RFC 9457 problem details with a stable machine-readable code.
// The frontend translates errors by code; the server never sends prose meant
// for end users.
package apperr

import (
	"errors"
	"fmt"
	"net/http"
)

// Error is a domain error with an HTTP status and a stable code.
type Error struct {
	Status int            `json:"status"`
	Title  string         `json:"title,omitempty"`
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
	// Details carries structured data such as conflicting appointments or
	// field validation errors.
	Details any `json:"details,omitempty"`
}

func (e *Error) Error() string {
	if e.Title != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Title)
	}
	return e.Code
}

// GetStatus implements huma.StatusError.
func (e *Error) GetStatus() int { return e.Status }

// ContentType implements huma.ContentTypeFilter.
func (e *Error) ContentType(ct string) string {
	if ct == "application/json" {
		return "application/problem+json"
	}
	return ct
}

func New(status int, code string) *Error { return &Error{Status: status, Code: code} }

func (e *Error) With(key string, value any) *Error {
	c := *e
	c.Params = make(map[string]any, len(e.Params)+1)
	for k, v := range e.Params {
		c.Params[k] = v
	}
	c.Params[key] = value
	return &c
}

func (e *Error) WithDetails(d any) *Error {
	c := *e
	c.Details = d
	return &c
}

func BadRequest(code string) *Error   { return New(http.StatusBadRequest, code) }
func Unauthorized(code string) *Error { return New(http.StatusUnauthorized, code) }
func Forbidden(code string) *Error    { return New(http.StatusForbidden, code) }
func NotFound(code string) *Error     { return New(http.StatusNotFound, code) }
func Conflict(code string) *Error     { return New(http.StatusConflict, code) }
func Invalid(code string) *Error      { return New(http.StatusUnprocessableEntity, code) }

// Common errors.
var (
	ErrUnauthenticated = Unauthorized("unauthenticated")
	ErrForbidden       = Forbidden("forbidden")
	ErrNotFound        = NotFound("not_found")
	ErrRateLimited     = New(http.StatusTooManyRequests, "rate_limited")
)

// Code returns the code of err if it is an *Error, or "" otherwise.
func Code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// FieldError describes a validation failure on one input field.
type FieldError struct {
	Location string `json:"location"`
	Code     string `json:"code"`
	Message  string `json:"message,omitempty"`
}
