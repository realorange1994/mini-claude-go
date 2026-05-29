// Package session provides session working directory validation.
// Aligned to pi's session-cwd.ts.
package session

import (
	"fmt"
	"os"
)

// SessionCwdIssue describes a missing session working directory.
type SessionCwdIssue struct {
	SessionFile string
	SessionCwd  string
	FallbackCwd string
}

// GetMissingSessionCwdIssue checks whether the session's stored CWD exists.
// Returns nil if the CWD exists or there is no session file.
func GetMissingSessionCwdIssue(sessionFile, sessionCwd, fallbackCwd string) *SessionCwdIssue {
	if sessionFile == "" {
		return nil
	}

	if sessionCwd == "" {
		return nil
	}

	// Check if the session's CWD still exists
	if _, err := os.Stat(sessionCwd); err == nil {
		return nil
	}

	return &SessionCwdIssue{
		SessionFile: sessionFile,
		SessionCwd:  sessionCwd,
		FallbackCwd: fallbackCwd,
	}
}

// FormatMissingSessionCwdError formats an error message for a missing CWD.
func FormatMissingSessionCwdError(issue *SessionCwdIssue) string {
	sessionFileInfo := ""
	if issue.SessionFile != "" {
		sessionFileInfo = fmt.Sprintf("\nSession file: %s", issue.SessionFile)
	}
	return fmt.Sprintf("Stored session working directory does not exist: %s%s\nCurrent working directory: %s",
		issue.SessionCwd, sessionFileInfo, issue.FallbackCwd)
}

// FormatMissingSessionCwdPrompt formats a prompt for the user to decide.
func FormatMissingSessionCwdPrompt(issue *SessionCwdIssue) string {
	return fmt.Sprintf("cwd from session file does not exist\n%s\n\ncontinue in current cwd\n%s",
		issue.SessionCwd, issue.FallbackCwd)
}

// MissingSessionCwdError is an error when the session's stored CWD doesn't exist.
type MissingSessionCwdError struct {
	Issue *SessionCwdIssue
}

func (e *MissingSessionCwdError) Error() string {
	return FormatMissingSessionCwdError(e.Issue)
}

// AssertSessionCwdExists throws if the session's stored CWD doesn't exist.
func AssertSessionCwdExists(sessionFile, sessionCwd, fallbackCwd string) error {
	issue := GetMissingSessionCwdIssue(sessionFile, sessionCwd, fallbackCwd)
	if issue != nil {
		return &MissingSessionCwdError{Issue: issue}
	}
	return nil
}