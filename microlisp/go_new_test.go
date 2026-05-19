package microlisp

import (
	"testing"
)

// TestGoNewPkixNameConsistency verifies that go:new consistently
// returns VGoVal for registered types.
func TestGoNewPkixNameConsistency(t *testing.T) {
	InitGlobalEnv()
	for i := 0; i < 10; i++ {
		result, err := EvalString(`(go:new "crypto/x509/pkix.Name")`, globalEnv)
		if err != nil {
			t.Fatalf("iteration %d: go:new pkix.Name failed: %v", i, err)
		}
		if result.typ != VGoVal {
			t.Fatalf("iteration %d: expected VGoVal, got %s", i, typeStr(result))
		}
	}
}

// TestGoNewPkixNameNotOverwrittenByDefine verifies that defining a Lisp
// variable named "name" does not interfere with go:new "crypto/x509/pkix.Name".
func TestGoNewPkixNameNotOverwrittenByDefine(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(define name (go:new "crypto/x509/pkix.Name"))`, globalEnv)
	if err != nil {
		t.Fatalf("define name failed: %v", err)
	}
	result, err := EvalString(`(go:new "crypto/x509/pkix.Name")`, globalEnv)
	if err != nil {
		t.Fatalf("go:new after define failed: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got %s", typeStr(result))
	}
}

// TestGoNewPkixNameWithDefine verifies go:new works after defining "name"
// as a string or function.
func TestGoNewPkixNameWithDefine(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(define name "hello")`, globalEnv)
	if err != nil {
		t.Fatalf("define name string failed: %v", err)
	}
	result, err := EvalString(`(go:new "crypto/x509/pkix.Name")`, globalEnv)
	if err != nil {
		t.Fatalf("go:new after define failed: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got %s", typeStr(result))
	}
}

// TestGoFieldReturnsSettableValue verifies that go:field returns a
// settable VGoVal when the parent struct is settable (from go:new).
// This is critical for nested struct field modification.
func TestGoFieldReturnsSettableValue(t *testing.T) {
	InitGlobalEnv()

	result, err := EvalString(`
		(define cert (go:new "crypto/x509.Certificate"))
		(define subject (go:field cert "Subject"))
		(go:set-field subject "CommonName" "test")
		(go:field subject "CommonName")
	`, globalEnv)
	if err != nil {
		t.Fatalf("nested struct field set failed: %v", err)
	}
	if result.typ != VStr || result.str != "test" {
		t.Fatalf("expected 'test', got: %v", result)
	}
}
