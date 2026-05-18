package microlisp

import (
	"strings"
	"testing"
)

func TestGoCallMultiArgs(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define now (go:import "time.Now"))
		(define t (now))
		(go:call t "AddDate" 1 0 0)
	`, globalEnv)
	if err != nil {
		t.Fatalf("go:call multi-arg failed: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal from AddDate, got %s", typeStr(result))
	}
}

func TestMethodExpression(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define add-date (go:import "time.Time.AddDate"))
		(define now (go:import "time.Now"))
		(define t (now))
		(add-date t 1 0 0)
	`, globalEnv)
	if err != nil {
		t.Fatalf("method expression failed: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal from method expr call, got %s", typeStr(result))
	}
}

func TestGoCallSingleArg(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define new-reader (go:import "strings.NewReader"))
		(define r (new-reader "hello"))
		(go:call r "Len")
	`, globalEnv)
	if err != nil {
		t.Fatalf("go:call single arg failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric from Len, got %s", typeStr(result))
	}
}

func TestGoTypeOfVGoVal(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define now (go:import "time.Now"))
		(go:type-of (now))
	`, globalEnv)
	if err != nil {
		t.Fatalf("go:type-of failed: %v", err)
	}
	if result.typ != VStr || !strings.Contains(result.str, "Time") {
		t.Fatalf("expected 'Time' in type-of result, got: %s", result.str)
	}
}

func TestGoFieldOnFFIReturn(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define generate-key (go:import "crypto/rsa.GenerateKey"))
		(define key (generate-key (go:import "crypto/rand.Reader") 2048))
		(go:type-of key)
	`, globalEnv)
	if err != nil {
		t.Fatalf("go:type-of on FFI return value failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from go:type-of, got %s", typeStr(result))
	}
}
