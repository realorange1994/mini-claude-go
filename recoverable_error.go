package main

import (
	"fmt"
)

// ─── RecoverableError (MiMo-Code 2) ────────────────────────────────────────
//
// Class-based error that differentiates agent-recoverable errors
// from genuine system faults.
//
// MiMo-Code source: tool/recoverable.ts (35 lines)

// RecoverableError represents an error that the agent can recover from.
type RecoverableError struct {
	Message    string
	Recoverable bool
	ErrorType  string
}

// Error returns the error message.
func (e *RecoverableError) Error() string {
	return e.Message
}

// NewRecoverableError creates a new recoverable error.
func NewRecoverableError(message string) *RecoverableError {
	return &RecoverableError{
		Message:     message,
		Recoverable: true,
		ErrorType:   "recoverable",
	}
}

// NewRecoverableErrorWithType creates a new recoverable error with a type.
func NewRecoverableErrorWithType(message, errorType string) *RecoverableError {
	return &RecoverableError{
		Message:     message,
		Recoverable: true,
		ErrorType:   errorType,
	}
}

// IsRecoverable checks if an error is recoverable.
func IsRecoverable(err error) bool {
	if err == nil {
		return false
	}
	if r, ok := err.(*RecoverableError); ok {
		return r.Recoverable
	}
	return false
}

// FormatRecoverableError formats a recoverable error for display.
func FormatRecoverableError(err *RecoverableError) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("[Recoverable] %s", err.Message)
}
