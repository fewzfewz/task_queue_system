package errors

import (
	"errors"
	"fmt"
)

// AppError represents a domain-specific error with an identifying code.
type AppError struct {
	Code    string
	Message string
	Err     error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Common error codes
const (
	CodeNotFound       = "NOT_FOUND"
	CodeInvalidArgument = "INVALID_ARGUMENT"
	CodeInternal       = "INTERNAL_ERROR"
	CodeUnauthorized   = "UNAUTHORIZED"
	CodeQueueFull      = "QUEUE_FULL"
)

// NewNotFound returns a 404-style error.
func NewNotFound(resource string, id string) *AppError {
	return &AppError{
		Code:    CodeNotFound,
		Message: fmt.Sprintf("%s not found: %s", resource, id),
	}
}

// NewInvalidArgument returns a 400-style error.
func NewInvalidArgument(msg string) *AppError {
	return &AppError{
		Code:    CodeInvalidArgument,
		Message: msg,
	}
}

// NewInternal returns a 500-style error.
func NewInternal(msg string, err error) *AppError {
	return &AppError{
		Code:    CodeInternal,
		Message: msg,
		Err:     err,
	}
}

// NewQueueFull returns a backpressure-related error.
func NewQueueFull() *AppError {
	return &AppError{
		Code:    CodeQueueFull,
		Message: "queue overloaded; please try again later",
	}
}


// IsCode checks if an error (or any error in its chain) is an AppError with the given code.
func IsCode(err error, code string) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code == code
	}
	return false
}
