// Package httpx provides the API response envelope, typed API errors and
// request binding helpers shared by all modules.
package httpx

import (
	"errors"
	"fmt"
	"net/http"
)

// Business error codes are independent of HTTP status codes. 0 means
// success, 1xxxx are common errors; 2xxxx are reserved for auth and 3xxxx
// for IAM (design doc section 9).
const (
	CodeOK              = 0
	CodeInternal        = 10000
	CodeBadRequest      = 10001
	CodeValidation      = 10002
	CodeNotFound        = 10003
	CodeConflict        = 10004
	CodeTooManyRequests = 10005
	CodeUnavailable     = 10006
	CodePayloadTooLarge = 10007
	CodeOutcomeUnknown  = 10008
)

// Error is an API error with a safe client-facing message. The optional
// cause is only logged, never returned to clients.
type Error struct {
	Status  int
	Code    int
	Message string
	Details any
	cause   error
}

// NewError returns an API error.
func NewError(status, code int, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.cause)
	}
	return e.Message
}

// Unwrap returns the underlying cause.
func (e *Error) Unwrap() error { return e.cause }

// WithDetails returns a copy of e carrying client-facing details.
func (e *Error) WithDetails(details any) *Error {
	clone := *e
	clone.Details = details
	return &clone
}

// WithCause returns a copy of e wrapping an internal cause for logging.
func (e *Error) WithCause(cause error) *Error {
	clone := *e
	clone.cause = cause
	return &clone
}

// FieldError describes one invalid request field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationDetails is the Details payload of a validation error.
type ValidationDetails struct {
	Fields []FieldError `json:"fields"`
}

// BadRequest reports a malformed request.
func BadRequest(message string) *Error {
	return NewError(http.StatusBadRequest, CodeBadRequest, message)
}

// Validation reports semantically invalid fields.
func Validation(fields []FieldError) *Error {
	return NewError(http.StatusUnprocessableEntity, CodeValidation, "参数校验失败").
		WithDetails(ValidationDetails{Fields: fields})
}

// NotFound reports a missing or out-of-scope resource.
func NotFound(message string) *Error {
	return NewError(http.StatusNotFound, CodeNotFound, message)
}

// Conflict reports a version or uniqueness conflict.
func Conflict(message string) *Error {
	return NewError(http.StatusConflict, CodeConflict, message)
}

// TooManyRequests reports rate limiting.
func TooManyRequests() *Error {
	return NewError(http.StatusTooManyRequests, CodeTooManyRequests, "请求过于频繁，请稍后再试")
}

// Unavailable reports a dependency outage.
func Unavailable(message string) *Error {
	return NewError(http.StatusServiceUnavailable, CodeUnavailable, message)
}

// PayloadTooLarge reports a request body over the configured limit.
func PayloadTooLarge() *Error {
	return NewError(http.StatusRequestEntityTooLarge, CodePayloadTooLarge, "请求内容过大")
}

// OutcomeUnknown reports that a commit may or may not have happened, e.g.
// the connection dropped during COMMIT (design doc 6.5.5 B4). Clients must
// re-read the resource instead of treating it as a failure.
func OutcomeUnknown(cause error) *Error {
	return NewError(http.StatusServiceUnavailable, CodeOutcomeUnknown, "操作结果未知，请刷新确认").WithCause(cause)
}

// Internal wraps an unexpected error. Clients only see a generic message.
func Internal(cause error) *Error {
	return NewError(http.StatusInternalServerError, CodeInternal, "服务器内部错误").WithCause(cause)
}

// AsError converts any error into an API error. Errors that are not
// *Error become a generic internal error so that details never leak.
func AsError(err error) *Error {
	if apiErr, ok := errors.AsType[*Error](err); ok {
		return apiErr
	}
	return Internal(err)
}
