package main

import (
	"fmt"
	"strings"
)

// ─── MiMo Gateway Error Handling (MiMo-Code 5) ─────────────────────────────
//
// Provider-specific error handling for MiMo model gateway.
// Maps non-standard error codes to human-readable messages.
//
// MiMo-Code source: provider/error.ts (48-70 lines)

// Gateway error codes
const (
	GatewayCodeContentModeration = 421
	GatewayCodeRiskControl       = 441
)

// FriendlyGatewayCodes maps error codes to human-readable messages.
var FriendlyGatewayCodes = map[int]string{
	GatewayCodeContentModeration: "Request blocked by content moderation",
	GatewayCodeRiskControl:       "Request blocked by risk control",
}

// GatewayError represents a MiMo gateway error.
type GatewayError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Param   string `json:"param,omitempty"`
}

// Error returns the error message.
func (e *GatewayError) Error() string {
	if e.Param != "" && e.Param != e.Message {
		return fmt.Sprintf("%s: %s", e.Message, e.Param)
	}
	return e.Message
}

// IsGatewayError checks if an error is a gateway error.
func IsGatewayError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*GatewayError)
	return ok
}

// FormatGatewayError formats a gateway error for display.
func FormatGatewayError(err *GatewayError) string {
	if err == nil {
		return ""
	}

	// Check for friendly message
	if friendly, ok := FriendlyGatewayCodes[err.Code]; ok {
		msg := friendly
		if err.Param != "" && err.Param != err.Message {
			msg += ": " + err.Param
		}
		return msg
	}

	// Check for HTML/proxy error pages
	if strings.Contains(err.Message, "<html") || strings.Contains(err.Message, "<!DOCTYPE") {
		return fmt.Sprintf("Gateway returned HTML error (code %d). Try running: opencode auth login", err.Code)
	}

	return err.Error()
}

// NewGatewayError creates a new gateway error.
func NewGatewayError(code int, message string, param string) *GatewayError {
	return &GatewayError{
		Code:    code,
		Message: message,
		Param:   param,
	}
}
