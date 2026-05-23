package microlisp

import (
	"reflect"
	"testing"
)

func TestGoMakeSlice(t *testing.T) {
	ResetGlobalEnv()
	result, err := builtinGoMake([]*Value{vstr("[]int"), vnum(3), vnum(10)})
	if err != nil {
		t.Fatalf("go:make slice: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got %v", result.typ)
	}
	rv := result.goValReflect
	if rv.Kind() != reflect.Slice {
		t.Fatalf("expected slice, got %v", rv.Kind())
	}
	if rv.Len() != 3 {
		t.Fatalf("expected len=3, got %d", rv.Len())
	}
	if rv.Cap() != 10 {
		t.Fatalf("expected cap=10, got %d", rv.Cap())
	}
}

func TestGoMakeMap(t *testing.T) {
	ResetGlobalEnv()
	result, err := builtinGoMake([]*Value{vstr("map[string]int")})
	if err != nil {
		t.Fatalf("go:make map: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got %v", result.typ)
	}
	rv := result.goValReflect
	if rv.Kind() != reflect.Map {
		t.Fatalf("expected map, got %v", rv.Kind())
	}
	if rv.IsNil() {
		t.Fatal("map should not be nil")
	}
}

func TestGoMakeChan(t *testing.T) {
	ResetGlobalEnv()
	result, err := builtinGoMake([]*Value{vstr("chan int"), vnum(5)})
	if err != nil {
		t.Fatalf("go:make chan: %v", err)
	}
	rv := result.goValReflect
	if rv.Kind() != reflect.Chan {
		t.Fatalf("expected chan, got %v", rv.Kind())
	}
	if rv.IsNil() {
		t.Fatal("chan should not be nil")
	}
	if rv.Cap() != 5 {
		t.Fatalf("expected cap=5, got %d", rv.Cap())
	}
}

func TestGoMakeByteSlice(t *testing.T) {
	ResetGlobalEnv()
	result, err := builtinGoMake([]*Value{vstr("[]byte"), vnum(0)})
	if err != nil {
		t.Fatalf("go:make []byte: %v", err)
	}
	rv := result.goValReflect
	if rv.Kind() != reflect.Slice {
		t.Fatalf("expected slice, got %v", rv.Kind())
	}
	if rv.Len() != 0 {
		t.Fatalf("expected len=0, got %d", rv.Len())
	}
	if rv.IsNil() {
		t.Fatal("[]byte should not be nil")
	}
}

func TestGoMakePointer(t *testing.T) {
	ResetGlobalEnv()
	result, err := builtinGoMake([]*Value{vstr("*int")})
	if err != nil {
		t.Fatalf("go:make *int: %v", err)
	}
	rv := result.goValReflect
	if rv.Kind() != reflect.Ptr {
		t.Fatalf("expected ptr, got %v", rv.Kind())
	}
	if rv.IsNil() {
		t.Fatal("*int pointer should not be nil")
	}
}

func TestGoMakeNestedTypes(t *testing.T) {
	ResetGlobalEnv()
	result, err := builtinGoMake([]*Value{vstr("[][]int"), vnum(0)})
	if err != nil {
		t.Fatalf("go:make [][]int: %v", err)
	}
	rv := result.goValReflect
	if rv.Kind() != reflect.Slice {
		t.Fatalf("expected slice, got %v", rv.Kind())
	}

	result2, err := builtinGoMake([]*Value{vstr("map[string][]int")})
	if err != nil {
		t.Fatalf("go:make map[string][]int: %v", err)
	}
	rv2 := result2.goValReflect
	if rv2.Kind() != reflect.Map {
		t.Fatalf("expected map, got %v", rv2.Kind())
	}
}

func TestGoMakeStructFallback(t *testing.T) {
	ResetGlobalEnv()
	result, err := builtinGoMake([]*Value{vstr("crypto/x509.Certificate")})
	if err != nil {
		t.Fatalf("go:make struct: %v", err)
	}
	rv := result.goValReflect
	if rv.Kind() != reflect.Ptr {
		t.Fatalf("expected ptr for struct, got %v", rv.Kind())
	}
	if rv.IsNil() {
		t.Fatal("struct pointer should not be nil")
	}
}

func TestParseGoTypeErrors(t *testing.T) {
	_, err := parseGoType("")
	if err == nil {
		t.Fatal("expected error for empty type")
	}
	_, err = parseGoType("foobar")
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
	_, err = parseGoType("map[")
	if err == nil {
		t.Fatal("expected error for malformed map")
	}
}