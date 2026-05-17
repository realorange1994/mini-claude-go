package microlisp

import (
	"testing"
)

func TestHB1_ReturnFromNil(t *testing.T) {
	ResetGlobalEnv()
	result, err := SafeEvalString("(handler-bind ((division-by-zero (lambda (e) (return-from nil 'caught)))) (/ 1 0))")
	if err != nil {
		t.Fatal(err)
	}
	if result != "CAUGHT" {
		t.Fatalf("expected CAUGHT, got %s", result)
	}
}

func TestHB2_NamedBlockReturn(t *testing.T) {
	ResetGlobalEnv()
	result, err := SafeEvalString("(block myblock (handler-bind ((division-by-zero (lambda (e) (return-from myblock 'caught)))) (/ 1 0)))")
	if err != nil {
		t.Fatal(err)
	}
	if result != "CAUGHT" {
		t.Fatalf("expected CAUGHT, got %s", result)
	}
}

func TestHB3_NoReturnFrom(t *testing.T) {
	ResetGlobalEnv()
	_, err := SafeEvalString(`(handler-bind ((division-by-zero (lambda (e) (format t "handled")))) (/ 1 0))`)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	t.Logf("got error: %v", err)
}

func TestHB4_HandlerCaseReturnFrom(t *testing.T) {
	ResetGlobalEnv()
	result, err := SafeEvalString("(handler-case (/ 1 0) (division-by-zero (e) (return-from nil 'caught)))")
	if err != nil {
		t.Fatal(err)
	}
	if result != "CAUGHT" {
		t.Fatalf("expected CAUGHT, got %s", result)
	}
}

func TestHB5_HandlerCaseNormal(t *testing.T) {
	ResetGlobalEnv()
	result, err := SafeEvalString("(handler-case (/ 1 0) (division-by-zero (e) 'handled))")
	if err != nil {
		t.Fatal(err)
	}
	if result != "HANDLED" {
		t.Fatalf("expected HANDLED, got %s", result)
	}
}

func TestHB6_NoErrorClause(t *testing.T) {
	ResetGlobalEnv()
	result, err := SafeEvalString("(handler-case (+ 1 2) (:no-error (vals) (car vals)))")
	if err != nil {
		t.Fatal(err)
	}
	if result != "3" {
		t.Fatalf("expected 3, got %s", result)
	}
}
