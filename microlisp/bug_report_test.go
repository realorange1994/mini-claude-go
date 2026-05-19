package microlisp

import (
	"testing"
)

func TestBugReport1Tailp(t *testing.T) {
	ResetGlobalEnv()
	r, _ := SafeEvalString("(tailp '(c d) '(a b c d))")
	t.Logf("(tailp '(c d) '(a b c d)) => %s", r)
}

func TestBugReport2Ldiff(t *testing.T) {
	ResetGlobalEnv()
	r1, _ := SafeEvalString("(ldiff '(a b c d) '(c d))")
	t.Logf("(ldiff '(a b c d) '(c d)) => %s", r1)
	ResetGlobalEnv()
	r2, _ := SafeEvalString("(ldiff '(a b c d) '(d))")
	t.Logf("(ldiff '(a b c d) '(d)) => %s", r2)
}

func TestBugReport3MemberFromEnd(t *testing.T) {
	ResetGlobalEnv()
	r, _ := SafeEvalString("(member 'b '(a b c b a) :from-end t)")
	t.Logf("(member 'b '(a b c b a) :from-end t) => %s", r)
}

func TestBugReport4Character(t *testing.T) {
	ResetGlobalEnv()
	r, err := SafeEvalString(`(character "ABC")`)
	t.Logf(`(character "ABC") => %v, err=%v`, r, err)
}

func TestBugReport5Schar(t *testing.T) {
	ResetGlobalEnv()
	r, err := SafeEvalString(`(schar "hello" 0)`)
	t.Logf(`(schar "hello" 0) => %v, err=%v`, r, err)
}

func TestBugReport6Reduce(t *testing.T) {
	ResetGlobalEnv()
	r1, err1 := SafeEvalString("(reduce #'+ '())")
	t.Logf("(reduce #'+ '()) => %s, err=%v", r1, err1)
	ResetGlobalEnv()
	r2, err2 := SafeEvalString("(reduce #'+ '() :initial-value 10)")
	t.Logf("(reduce #'+ '() :initial-value 10) => %s, err=%v", r2, err2)
}

func TestBugReport7CoerceIntToString(t *testing.T) {
	ResetGlobalEnv()
	r1, err1 := SafeEvalString("(coerce 123 'string)")
	t.Logf("(coerce 123 'string) => %q, err=%v", r1, err1)
	ResetGlobalEnv()
	r2, err2 := SafeEvalString("(coerce 5 'string)")
	t.Logf("(coerce 5 'string) => %q, err=%v", r2, err2)
}

func TestBugReport8StringCompare(t *testing.T) {
	ResetGlobalEnv()
	r1, _ := SafeEvalString(`(string<= "abd" "abc")`)
	t.Logf(`(string<= "abd" "abc") => %s`, r1)
	ResetGlobalEnv()
	r2, _ := SafeEvalString(`(string>= "abc" "abd")`)
	t.Logf(`(string>= "abc" "abd") => %s`, r2)
	ResetGlobalEnv()
	r3, _ := SafeEvalString(`(string>= "abd" "abc")`)
	t.Logf(`(string>= "abd" "abc") => %s`, r3)
}
