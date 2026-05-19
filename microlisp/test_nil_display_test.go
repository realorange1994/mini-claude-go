package microlisp

import (
	"testing"
)

func TestNilDisplay(t *testing.T) {
	ResetGlobalEnv()
	r, _ := SafeEvalString("nil")
	t.Logf("nil => %s", r)
	ResetGlobalEnv()
	r2, _ := SafeEvalString("'()")
	t.Logf("'() => %s", r2)
	ResetGlobalEnv()
	r3, _ := SafeEvalString("(string<= \"abd\" \"abc\")")
	t.Logf("(string<= \"abd\" \"abc\") => %s", r3)
	ResetGlobalEnv()
	r4, _ := SafeEvalString("(string>= \"abd\" \"abc\")")
	t.Logf("(string>= \"abd\" \"abc\") => %s", r4)
}
