package microlisp

import (
	"strings"
	"testing"
)

func eval2(s string) (string, error) {
	ResetGlobalEnv()
	return SafeEvalString(s)
}

func evalSeq(exprs []string) (string, error) {
	ResetGlobalEnv()
	var result string
	var err error
	for _, expr := range exprs {
		result, err = SafeEvalString(expr)
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func TestBug_FloatSign(t *testing.T) {
	// float-sign was not registered as a builtin
	r1, err := eval2(`(float-sign 1.0)`)
	if err != nil {
		t.Fatalf("float-sign 1.0: unexpected error: %v", err)
	}
	if r1 != "1.0" {
		t.Fatalf("float-sign 1.0: expected 1.0, got %s", r1)
	}
	r2, _ := eval2(`(float-sign -1.0)`)
	if r2 != "-1.0" {
		t.Fatalf("float-sign -1.0: expected -1.0, got %s", r2)
	}
	r3, _ := eval2(`(float-sign -1.0 5.0)`)
	if r3 != "-5.0" {
		t.Fatalf("float-sign -1.0 5.0: expected -5.0, got %s", r3)
	}
	r4, _ := eval2(`(float-sign 3.0 5.0)`)
	if r4 != "5.0" {
		t.Fatalf("float-sign 3.0 5.0: expected 5.0, got %s", r4)
	}
}

func TestBug_NumberpComplex(t *testing.T) {
	// numberp only checked VNum, not VComplex/VRat/VBigInt
	r1, _ := eval2(`(numberp #c(3 4))`)
	if r1 != "#t" {
		t.Fatalf("numberp #c(3 4): expected #t, got %s", r1)
	}
	r2, _ := eval2(`(numberp 42)`)
	if r2 != "#t" {
		t.Fatalf("numberp 42: expected #t, got %s", r2)
	}
	r3, _ := eval2(`(numberp 'foo)`)
	if r3 != "#f" {
		t.Fatalf("numberp 'foo: expected #f, got %s", r3)
	}
}

func TestBug_GetDefault(t *testing.T) {
	// get ignored the default value, always returning nil
	r1, err := eval2(`(get 'foo 'key 'default)`)
	if err != nil {
		t.Fatalf("get with default: unexpected error: %v", err)
	}
	if r1 != "DEFAULT" {
		t.Fatalf("get 'foo 'key 'default: expected DEFAULT, got %s", r1)
	}
	r2, _ := eval2(`(get 'foo 'nonexistent)`)
	if r2 != "NIL" && r2 != "()" {
		t.Fatalf("get no match no default: expected NIL, got %s", r2)
	}
}

func TestBug_SetfRowMajorAref(t *testing.T) {
	// setf row-major-aref had no setter function
	r1, err := eval2(`(let ((arr #(1 2 3 4 5))) (setf (row-major-aref arr 0) 99) (aref arr 0))`)
	if err != nil {
		t.Fatalf("setf row-major-aref: unexpected error: %v", err)
	}
	if r1 != "99" {
		t.Fatalf("setf row-major-aref: expected 99, got %s", r1)
	}
	r2, _ := eval2(`(let ((arr #(1 2 3 4 5))) (setf (row-major-aref arr 2) 77) (aref arr 2))`)
	if r2 != "77" {
		t.Fatalf("setf row-major-aref idx 2: expected 77, got %s", r2)
	}
}

func TestBug_DefparameterUndefined(t *testing.T) {
	// defparameter variable should be accessible immediately
	result, err := evalSeq([]string{
		`(defparameter arr2 (make-array '(2 3) :initial-element 0))`,
		`arr2`,
	})
	if err != nil {
		t.Fatalf("arr2 after defparameter: unexpected error: %v", err)
	}
	if result == "" || result == "NIL" || result == "()" {
		t.Fatalf("arr2 should have a value, got: %s", result)
	}
}

func TestBug_PackageNameFindPackage(t *testing.T) {
	// package-name on find-package result should work
	result, err := eval2(`(package-name (find-package 'cl))`)
	if err != nil {
		t.Fatalf("package-name find-package: unexpected error: %v", err)
	}
	if !strings.Contains(strings.ToUpper(result), "CL") {
		t.Fatalf("package-name: expected CL, got %s", result)
	}
}
