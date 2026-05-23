package microlisp

import (
	"strings"
	"testing"
)

func TestBug1_DefunFirstCall(t *testing.T) {
	ResetGlobalEnv()
	_, err := SafeEvalString("(defun test-defun () 123)")
	if err != nil {
		t.Fatalf("defun failed: %v", err)
	}
	result, err := SafeEvalString("(test-defun)")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if result != "123" {
		t.Fatalf("expected 123, got %s", result)
	}
}

func TestBug2_SetfMultiPair(t *testing.T) {
	ResetGlobalEnv()
	SafeEvalString("(defvar ht2 (make-hash-table))")
	SafeEvalString("(setf (gethash 'a ht2) 1 (gethash 'b ht2) 2)")
	r, err := SafeEvalString("(gethash 'a ht2)")
	if err != nil {
		t.Fatalf("gethash a failed: %v", err)
	}
	// gethash returns (value found-p), we just need the value part to be 1
	if !strings.Contains(r, "1") {
		t.Fatalf("expected gethash 'a to return value 1, got %s", r)
	}
}

func TestBug3_DefvarFirstRef(t *testing.T) {
	ResetGlobalEnv()
	SafeEvalString("(defvar *ht7* (make-hash-table))")
	_, err := SafeEvalString("(setf (gethash 'a *ht7*) 1)")
	if err != nil {
		t.Fatalf("setf on defvar var failed: %v", err)
	}
}

func TestBug6_BoundpReturns(t *testing.T) {
	ResetGlobalEnv()
	SafeEvalString("(defvar *ht9*)")
	r, err := SafeEvalString("(boundp '*ht9*)")
	if err != nil {
		t.Fatalf("boundp failed: %v", err)
	}
	// In CL, boundp returns T or NIL, not #f
	if r == "#f" {
		t.Fatalf("boundp should return T or NIL, not #f (Scheme style)")
	}
}

func TestBug7_DefvarReturnValue(t *testing.T) {
	ResetGlobalEnv()
	r, err := SafeEvalString("(defvar *ht8* (make-hash-table))")
	if err != nil {
		t.Fatalf("defvar failed: %v", err)
	}
	t.Logf("defvar returned: %s", r)
}

func TestBug9_MacroexpandQuote(t *testing.T) {
	ResetGlobalEnv()
	r, err := SafeEvalString("(macroexpand '(if t 1 2))")
	if err != nil {
		t.Fatalf("macroexpand failed: %v", err)
	}
	t.Logf("macroexpand result: %s", r)
}

func TestBug4_HashTableCount(t *testing.T) {
	ResetGlobalEnv()
	SafeEvalString("(defvar ht4 (make-hash-table))")
	SafeEvalString("(setf (gethash 'a ht4) 1 (gethash 'b ht4) 2 (gethash 'c ht4) 3)")
	r, err := SafeEvalString("(hash-table-count ht4)")
	if err != nil {
		t.Fatalf("hash-table-count failed: %v", err)
	}
	if r != "3" {
		t.Fatalf("expected count 3, got %s", r)
	}
}

func TestBug5_DefmacroSideEffects(t *testing.T) {
	ResetGlobalEnv()
	// First, verify that princ output is captured at all
	out, r, err := SafeEvalStringCapture(`(princ "hello")`)
	if err != nil {
		t.Fatalf("princ failed: %v", err)
	}
	t.Logf("princ captured stdout: %q, result: %s", out, r)
	if !strings.Contains(out, "hello") {
		t.Fatalf("princ output not captured: got %q", out)
	}

	// Now test macro side effects
	SafeEvalString(`(defmacro foo () (princ "expanding") '(+ 1 2))`)

	// Check macroexpand - should expand and execute side effects
	out2, r2, err := SafeEvalStringCapture(`(macroexpand '(foo))`)
	if err != nil {
		t.Fatalf("macroexpand failed: %v", err)
	}
	t.Logf("macroexpand captured stdout: %q, result: %s", out2, r2)

	// Check that the expansion produces (+ 1 2)
	if r2 != "(+ 1 2)" {
		t.Fatalf("expected expansion (+ 1 2), got %s", r2)
	}
	// In CL, side effects in the macro body SHOULD happen during expansion.
	if !strings.Contains(out2, "expanding") {
		t.Fatalf("expected stdout to contain 'expanding' from macro side effect, got: %q", out2)
	}
}

// --- New bugs ---

func TestBug_LetrecImplicitLambda(t *testing.T) {
	ResetGlobalEnv()
	r, err := SafeEvalString(`(letrec ((sq (x) (* x x))) (sq 5))`)
	if err != nil {
		t.Fatalf("letrec implicit lambda failed: %v", err)
	}
	if r != "25" {
		t.Fatalf("expected 25, got %s", r)
	}
}

func TestBug_TagbodyReturnValue(t *testing.T) {
	ResetGlobalEnv()
	// In CL, integers are valid go tags, so `42` is a tag, not a value.
	// Use an actual expression to test return value tracking.
	r, err := SafeEvalString(`(tagbody :a (go :b) :b (+ 20 22))`)
	if err != nil {
		t.Fatalf("tagbody failed: %v", err)
	}
	if r != "42" {
		t.Fatalf("expected 42, got %s", r)
	}
}

func TestBug_TagbodyReturnFrom(t *testing.T) {
	ResetGlobalEnv()
	// tagbody inside block: return-from should work
	r, err := SafeEvalString(`(block nil (tagbody :a (return-from nil 3)))`)
	if err != nil {
		t.Fatalf("tagbody return-from failed: %v", err)
	}
	if r != "3" {
		t.Fatalf("expected 3, got %s", r)
	}
}

func TestBug_LabelsErrorMessage(t *testing.T) {
	ResetGlobalEnv()
	_, err := SafeEvalString(`(labels (x) x)`)
	if err == nil {
		t.Fatal("expected error for malformed labels")
	}
	// Should mention "labels" not "flet"
	if strings.Contains(strings.ToLower(err.Error()), "flet") {
		t.Fatalf("error message says 'flet' instead of 'labels': %v", err)
	}
}
