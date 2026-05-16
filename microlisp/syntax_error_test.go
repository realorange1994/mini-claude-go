package microlisp

import (
	"strings"
	"testing"
	"time"
)

func TestBadSyntaxCloseParen(t *testing.T) {
	ResetGlobalEnv()
	// This should NOT hang — should return syntax error quickly
	limits := ResourceLimits{MaxSteps: 10000, MaxTimeMs: 3000}
	done := make(chan struct{})
	var evalErr error
	go func() {
		_, evalErr = SafeEvalWithLimits("> 3 2)", limits)
		close(done)
	}()
	select {
	case <-done:
		if evalErr == nil {
			t.Error("expected syntax error for unmatched close paren")
		}
		if !strings.Contains(evalErr.Error(), "unmatched") && !strings.Contains(evalErr.Error(), "syntax") {
			t.Fatalf("expected syntax/unmatched error, got: %v", evalErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SafeEvalWithLimits hung on bad syntax input")
	}
}

func TestBadSyntaxExtraCloseParen(t *testing.T) {
	ResetGlobalEnv()
	_, err := SafeEvalWithLimits("(+ 1 2))", DefaultLimits())
	if err == nil {
		t.Fatal("expected error for extra close paren")
	}
	if !strings.Contains(err.Error(), "unmatched") && !strings.Contains(err.Error(), "syntax") {
		t.Fatalf("expected syntax/unmatched error, got: %v", err)
	}
}

func TestBadSyntaxDotOutsideList(t *testing.T) {
	ResetGlobalEnv()
	_, err := SafeEvalWithLimits(". 5", DefaultLimits())
	if err == nil {
		t.Fatal("expected error for dot outside list")
	}
}
