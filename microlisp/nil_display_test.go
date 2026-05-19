package microlisp

import (
	"testing"
)

func TestNilDisplay(t *testing.T) {
	// nil and '() both represent the empty list in this implementation.
	// The display representation is "()" rather than "NIL".

	ResetGlobalEnv()
	r, err := SafeEvalString("nil")
	if err != nil {
		t.Fatalf("evaluating nil failed: %v", err)
	}
	if r != "()" {
		t.Errorf("expected nil to display as (), got %s", r)
	}

	ResetGlobalEnv()
	r2, err := SafeEvalString("'()")
	if err != nil {
		t.Fatalf("evaluating '() failed: %v", err)
	}
	if r2 != "()" {
		t.Errorf("expected '() to display as (), got %s", r2)
	}

	// "abd" > "abc", so string<= should return false (empty list)
	ResetGlobalEnv()
	r3, err := SafeEvalString(`(string<= "abd" "abc")`)
	if err != nil {
		t.Fatalf("evaluating string<= failed: %v", err)
	}
	if r3 != "()" {
		t.Errorf("expected (string<= \"abd\" \"abc\") to be () (false), got %s", r3)
	}

	// "abd" > "abc", so string>= should return true (non-empty)
	ResetGlobalEnv()
	r4, err := SafeEvalString(`(string>= "abd" "abc")`)
	if err != nil {
		t.Fatalf("evaluating string>= failed: %v", err)
	}
	if r4 == "()" {
		t.Errorf("expected (string>= \"abd\" \"abc\") to be truthy, got %s", r4)
	}
}
