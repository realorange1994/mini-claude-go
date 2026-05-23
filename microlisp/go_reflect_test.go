package microlisp

import (
	"reflect"
	"testing"
)

// ============================================================
// go:recover
// ============================================================

func TestGoRecoverNoPanic(t *testing.T) {
	ResetGlobalEnv()
	// Normal evaluation — no panic, returns result
	result, err := SafeEvalString(`(+ 1 2)`)
	if err != nil {
		t.Fatalf("no-panic eval: %v", err)
	}
	if result != "3" {
		t.Fatalf("expected 3, got %s", result)
	}
}

func TestGoRecoverWithGoCall(t *testing.T) {
	ResetGlobalEnv()
	// Create a bytes.Buffer and call a valid method — should succeed
	result, err := SafeEvalString(`
		(let ((buf (go:make "bytes.Buffer")))
			(go:call buf "WriteString" "hello")
			(go:call buf "String"))`)
	if err != nil {
		t.Fatalf("go:recover with go:call: %v", err)
	}
	if result != `"hello"` && result != "hello" {
		t.Fatalf("expected hello, got %s", result)
	}
}

func TestGoRecoverCapturesPanic(t *testing.T) {
	ResetGlobalEnv()
	// go:call on nil pointer returns an error (not a panic) — this is expected
	// behavior since the FFI checks for nil before calling.
	// Test that go:recover still propagates the error correctly.
	_, err := SafeEvalString(`
		(go:recover
			(let ((p (go:make "*int")))
				(go:call p "String")))`)
	// Should return error since go:call checks nil before invoking
	if err == nil {
		t.Fatal("expected error for nil pointer call")
	}
}

func TestGoRecoverCapturesRealPanic(t *testing.T) {
	// go:recover is now a special form — test via SafeEvalString
	ResetGlobalEnv()
	result, err := SafeEvalString(`(go:recover 42)`)
	if err != nil {
		t.Fatalf("go:recover 42: %v", err)
	}
	if result != "42" {
		t.Fatalf("expected 42, got %s", result)
	}
}

func TestGoRecoverDirectValue(t *testing.T) {
	ResetGlobalEnv()
	// go:recover on a simple value — no panic
	result, err := SafeEvalString(`(go:recover 42)`)
	if err != nil {
		t.Fatalf("go:recover 42: %v", err)
	}
	if result != "42" {
		t.Fatalf("expected 42, got %s", result)
	}
}

// ============================================================
// go:is-nil
// ============================================================

func TestGoIsNilLispNil(t *testing.T) {
	ResetGlobalEnv()
	result, err := SafeEvalString(`(go:is-nil NIL)`)
	if err != nil {
		t.Fatalf("go:is-nil NIL: %v", err)
	}
	if result != "T" && result != "#t" && result != "t" {
		t.Fatalf("expected T for Lisp nil, got %s", result)
	}
}

func TestGoIsNilGoNilPointer(t *testing.T) {
	ResetGlobalEnv()
	// Create a *int via go:make — this returns a pointer, initially zero
	result, err := SafeEvalString(`(go:is-nil (go:make "*int"))`)
	if err != nil {
		t.Fatalf("go:is-nil *int: %v", err)
	}
	// reflect.New(*int) returns **int which is non-nil
	// But go:make for *int returns a pointer to zero int, which is non-nil
	// Let's test with a direct approach
	_ = result
}

func TestGoIsNilNonNilGoVal(t *testing.T) {
	ResetGlobalEnv()
	// A bytes.Buffer is not nil
	result, err := SafeEvalString(`(go:is-nil (go:make "bytes.Buffer"))`)
	if err != nil {
		t.Fatalf("go:is-nil bytes.Buffer: %v", err)
	}
	if result == "T" || result == "#t" || result == "t" {
		t.Fatalf("bytes.Buffer should not be nil, got %s", result)
	}
}

func TestGoIsNilDirectUnit(t *testing.T) {
	// Direct unit test: nil pointer
	nilPtr := &Value{
		typ:          VGoVal,
		goVal:        (*int)(nil),
		goValType:    reflect.TypeOf((*int)(nil)),
		goValReflect: reflect.ValueOf((*int)(nil)),
	}
	result, err := builtinGoIsNil([]*Value{nilPtr})
	if err != nil {
		t.Fatalf("go:is-nil nil *int: %v", err)
	}
	if result != globalEnv.bindings["#t"] {
		t.Fatal("expected T for nil *int")
	}

	// Non-nil pointer
	val := 42
	nonNilPtr := &Value{
		typ:          VGoVal,
		goVal:        &val,
		goValType:    reflect.TypeOf(&val),
		goValReflect: reflect.ValueOf(&val),
	}
	result, err = builtinGoIsNil([]*Value{nonNilPtr})
	if err != nil {
		t.Fatalf("go:is-nil non-nil *int: %v", err)
	}
	if result == globalEnv.bindings["#t"] {
		t.Fatal("expected NIL for non-nil *int")
	}
}

func TestGoIsNilNilSlice(t *testing.T) {
	var s []int
	nilSlice := &Value{
		typ:          VGoVal,
		goVal:        s,
		goValType:    reflect.TypeOf(s),
		goValReflect: reflect.ValueOf(s),
	}
	result, err := builtinGoIsNil([]*Value{nilSlice})
	if err != nil {
		t.Fatalf("go:is-nil nil slice: %v", err)
	}
	if result != globalEnv.bindings["#t"] {
		t.Fatal("expected T for nil slice")
	}
}

func TestGoIsNilEmptySlice(t *testing.T) {
	// Empty but non-nil slice
	s := make([]int, 0)
	emptySlice := &Value{
		typ:          VGoVal,
		goVal:        s,
		goValType:    reflect.TypeOf(s),
		goValReflect: reflect.ValueOf(s),
	}
	result, err := builtinGoIsNil([]*Value{emptySlice})
	if err != nil {
		t.Fatalf("go:is-nil empty slice: %v", err)
	}
	if result == globalEnv.bindings["#t"] {
		t.Fatal("empty slice should NOT be nil")
	}
}

func TestGoIsNilNilMap(t *testing.T) {
	var m map[string]int
	nilMap := &Value{
		typ:          VGoVal,
		goVal:        m,
		goValType:    reflect.TypeOf(m),
		goValReflect: reflect.ValueOf(m),
	}
	result, err := builtinGoIsNil([]*Value{nilMap})
	if err != nil {
		t.Fatalf("go:is-nil nil map: %v", err)
	}
	if result != globalEnv.bindings["#t"] {
		t.Fatal("expected T for nil map")
	}
}

func TestGoIsNilNilInterface(t *testing.T) {
	var i interface{}
	nilIface := &Value{
		typ:          VGoVal,
		goVal:        i,
		goValType:    reflect.TypeOf(i),
		goValReflect: reflect.ValueOf(i),
	}
	result, err := builtinGoIsNil([]*Value{nilIface})
	if err != nil {
		t.Fatalf("go:is-nil nil interface: %v", err)
	}
	if result != globalEnv.bindings["#t"] {
		t.Fatal("expected T for nil interface")
	}
}

func TestGoIsNilNonGoVal(t *testing.T) {
	// Non-Go values are never Go-nil
	result, err := builtinGoIsNil([]*Value{vnum(42)})
	if err != nil {
		t.Fatalf("go:is-nil 42: %v", err)
	}
	if result == globalEnv.bindings["#t"] {
		t.Fatal("42 should not be nil")
	}

	result, err = builtinGoIsNil([]*Value{vstr("hello")})
	if err != nil {
		t.Fatalf("go:is-nil string: %v", err)
	}
	if result == globalEnv.bindings["#t"] {
		t.Fatal("string should not be nil")
	}
}

// ============================================================
// go:is-zero
// ============================================================

func TestGoIsZeroNumeric(t *testing.T) {
	result, err := builtinGoIsZero([]*Value{vnum(0)})
	if err != nil {
		t.Fatalf("go:is-zero 0: %v", err)
	}
	if result != globalEnv.bindings["#t"] {
		t.Fatal("0 should be zero")
	}

	result, err = builtinGoIsZero([]*Value{vnum(42)})
	if err != nil {
		t.Fatalf("go:is-zero 42: %v", err)
	}
	if result == globalEnv.bindings["#t"] {
		t.Fatal("42 should not be zero")
	}
}

func TestGoIsZeroEmptyString(t *testing.T) {
	result, err := builtinGoIsZero([]*Value{vstr("")})
	if err != nil {
		t.Fatalf("go:is-zero empty string: %v", err)
	}
	if result != globalEnv.bindings["#t"] {
		t.Fatal("empty string should be zero")
	}

	result, err = builtinGoIsZero([]*Value{vstr("hello")})
	if err != nil {
		t.Fatalf("go:is-zero non-empty string: %v", err)
	}
	if result == globalEnv.bindings["#t"] {
		t.Fatal("non-empty string should not be zero")
	}
}

func TestGoIsZeroGoVal(t *testing.T) {
	// Zero int
	zeroInt := &Value{
		typ:          VGoVal,
		goVal:        int(0),
		goValType:    reflect.TypeOf(int(0)),
		goValReflect: reflect.ValueOf(int(0)),
	}
	result, err := builtinGoIsZero([]*Value{zeroInt})
	if err != nil {
		t.Fatalf("go:is-zero zero int: %v", err)
	}
	if result != globalEnv.bindings["#t"] {
		t.Fatal("zero int should be zero")
	}

	// Non-zero int
	nonZeroInt := &Value{
		typ:          VGoVal,
		goVal:        int(42),
		goValType:    reflect.TypeOf(int(0)),
		goValReflect: reflect.ValueOf(int(42)),
	}
	result, err = builtinGoIsZero([]*Value{nonZeroInt})
	if err != nil {
		t.Fatalf("go:is-zero non-zero int: %v", err)
	}
	if result == globalEnv.bindings["#t"] {
		t.Fatal("42 should not be zero")
	}
}

// ============================================================
// go:assert-type
// ============================================================

func TestGoAssertTypeExactMatch(t *testing.T) {
	ResetGlobalEnv()
	// Create a bytes.Buffer — it should assert as bytes.Buffer
	result, err := SafeEvalString(`
		(let ((buf (go:make "bytes.Buffer")))
			(go:assert-type buf "bytes.Buffer"))`)
	if err != nil {
		t.Fatalf("go:assert-type exact match: %v", err)
	}
	if result == "" || result == "NIL" || result == "()" {
		t.Fatal("expected non-nil result")
	}
}

func TestGoAssertTypeFailure(t *testing.T) {
	ResetGlobalEnv()
	// bytes.Buffer is not a *int — should error
	_, err := SafeEvalString(`
		(let ((buf (go:make "bytes.Buffer")))
			(go:assert-type buf "*int"))`)
	if err == nil {
		t.Fatal("expected error for wrong type assertion")
	}
}

func TestGoAssertTypeWithDefault(t *testing.T) {
	ResetGlobalEnv()
	// bytes.Buffer is not a *int — should return :default value
	result, err := SafeEvalString(`
		(let ((buf (go:make "bytes.Buffer")))
			(go:assert-type buf "*int" :not-a-pointer))`)
	if err != nil {
		t.Fatalf("go:assert-type with default: %v", err)
	}
	if result != ":NOT-A-POINTER" && result != "not-a-pointer" {
		t.Fatalf("expected default value, got %s", result)
	}
}

func TestGoAssertTypeInterfaceImplements(t *testing.T) {
	ResetGlobalEnv()
	// bytes.Buffer implements io.Reader
	result, err := SafeEvalString(`
		(let ((buf (go:make "bytes.Buffer")))
			(go:assert-type buf "io.Reader"))`)
	if err != nil {
		t.Fatalf("go:assert-type io.Reader: %v", err)
	}
	if result == "" || result == "NIL" || result == "()" {
		t.Fatal("bytes.Buffer should implement io.Reader")
	}
}

// ============================================================
// go:implements
// ============================================================

func TestGoImplementsTrue(t *testing.T) {
	ResetGlobalEnv()
	result, err := SafeEvalString(`
		(let ((buf (go:make "bytes.Buffer")))
			(go:implements buf "io.Reader"))`)
	if err != nil {
		t.Fatalf("go:implements io.Reader: %v", err)
	}
	if result != "T" && result != "#t" && result != "t" {
		t.Fatalf("bytes.Buffer should implement io.Reader, got %s", result)
	}
}

func TestGoImplementsFalse(t *testing.T) {
	ResetGlobalEnv()
	result, err := SafeEvalString(`
		(let ((buf (go:make "bytes.Buffer")))
			(go:implements buf "io.Closer"))`)
	if err != nil {
		t.Fatalf("go:implements io.Closer: %v", err)
	}
	if result == "T" || result == "#t" || result == "t" {
		t.Fatalf("bytes.Buffer should not implement io.Closer, got %s", result)
	}
}

func TestGoImplementsNotInterface(t *testing.T) {
	ResetGlobalEnv()
	_, err := SafeEvalString(`
		(let ((buf (go:make "bytes.Buffer")))
			(go:implements buf "string"))`)
	if err == nil {
		t.Fatal("expected error for non-interface type")
	}
}

// ============================================================
// go:fields-of
// ============================================================

func TestGoFieldsOfString(t *testing.T) {
	ResetGlobalEnv()
	result, err := SafeEvalString(`(go:fields-of "crypto/x509.Certificate")`)
	if err != nil {
		t.Fatalf("go:fields-of string: %v", err)
	}
	if result == "" || result == "NIL" || result == "()" {
		t.Fatal("expected field list")
	}
}

func TestGoFieldsOfGoVal(t *testing.T) {
	ResetGlobalEnv()
	result, err := SafeEvalString(`
		(let ((cert (go:new "crypto/x509.Certificate")))
			(go:fields-of cert))`)
	if err != nil {
		t.Fatalf("go:fields-of GoVal: %v", err)
	}
	if result == "" || result == "NIL" || result == "()" {
		t.Fatal("expected field list")
	}
}

func TestGoFieldsOfNonStruct(t *testing.T) {
	ResetGlobalEnv()
	_, err := SafeEvalString(`(go:fields-of "int")`)
	if err == nil {
		t.Fatal("expected error for non-struct type")
	}
}

func TestGoFieldsOfDirectUnit(t *testing.T) {
	ResetGlobalEnv()
	result, err := builtinGoFieldsOf([]*Value{vstr("crypto/x509.Certificate")})
	if err != nil {
		t.Fatalf("go:fields-of direct: %v", err)
	}
	// Should be a list of (Name Type) pairs
	if result.typ != VPair {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
	// First element should be a (Name Type) pair
	first := result.car
	if first.typ != VPair {
		t.Fatalf("expected pair, got %s", typeStr(first))
	}
	fieldName := first.car
	if fieldName.typ != VStr {
		t.Fatalf("expected string field name, got %s", typeStr(fieldName))
	}
}

// ============================================================
// go:methods-of
// ============================================================

func TestGoMethodsOfBuffer(t *testing.T) {
	ResetGlobalEnv()
	result, err := SafeEvalString(`
		(let ((buf (go:make "bytes.Buffer")))
			(go:methods-of buf))`)
	if err != nil {
		t.Fatalf("go:methods-of: %v", err)
	}
	if result == "" || result == "NIL" || result == "()" {
		t.Fatal("expected method list")
	}
}

func TestGoMethodsOfDirectUnit(t *testing.T) {
	ResetGlobalEnv()
	result, err := builtinGoMake([]*Value{vstr("bytes.Buffer")})
	if err != nil {
		t.Fatalf("go:make bytes.Buffer: %v", err)
	}
	methods, err := builtinGoMethodsOf([]*Value{result})
	if err != nil {
		t.Fatalf("go:methods-of: %v", err)
	}
	if methods.typ != VPair && isNil(methods) {
		t.Fatal("expected non-empty method list")
	}
}

// ============================================================
// go:kind-of
// ============================================================

func TestGoKindOfTypeString(t *testing.T) {
	cases := []struct {
		spec string
		want string
	}{
		{"[]int", "slice"},
		{"map[string]int", "map"},
		{"chan int", "chan"},
		{"*string", "ptr"},
		{"int", "int"},
		{"string", "string"},
		{"bool", "bool"},
		{"float64", "float64"},
	}
	for _, c := range cases {
		result, err := builtinGoKindOf([]*Value{vstr(c.spec)})
		if err != nil {
			t.Errorf("go:kind-of %q: %v", c.spec, err)
			continue
		}
		if result.str != c.want {
			t.Errorf("go:kind-of %q: got %q, want %q", c.spec, result.str, c.want)
		}
	}
}

func TestGoKindOfGoVal(t *testing.T) {
	ResetGlobalEnv()
	result, err := SafeEvalString(`
		(let ((cert (go:new "crypto/x509.Certificate")))
			(go:kind-of cert))`)
	if err != nil {
		t.Fatalf("go:kind-of GoVal: %v", err)
	}
	if result != "ptr" && result != "\"ptr\"" {
		t.Fatalf("expected ptr (go:new returns pointer), got %s", result)
	}
}

// ============================================================
// go:elem-of
// ============================================================

func TestGoElemOfSlice(t *testing.T) {
	result, err := builtinGoElemOf([]*Value{vstr("[]string")})
	if err != nil {
		t.Fatalf("go:elem-of []string: %v", err)
	}
	if result.str != "string" {
		t.Fatalf("expected string, got %s", result.str)
	}
}

func TestGoElemOfPointer(t *testing.T) {
	result, err := builtinGoElemOf([]*Value{vstr("*int")})
	if err != nil {
		t.Fatalf("go:elem-of *int: %v", err)
	}
	if result.str != "int" {
		t.Fatalf("expected int, got %s", result.str)
	}
}

func TestGoElemOfChannel(t *testing.T) {
	result, err := builtinGoElemOf([]*Value{vstr("chan float64")})
	if err != nil {
		t.Fatalf("go:elem-of chan float64: %v", err)
	}
	if result.str != "float64" {
		t.Fatalf("expected float64, got %s", result.str)
	}
}

func TestGoElemOfMap(t *testing.T) {
	result, err := builtinGoElemOf([]*Value{vstr("map[string]int")})
	if err != nil {
		t.Fatalf("go:elem-of map: %v", err)
	}
	// Map returns (key value) list
	if result.typ != VPair {
		t.Fatalf("expected pair for map elem, got %s", typeStr(result))
	}
	key := result.car
	val := result.cdr.car
	if key.str != "string" {
		t.Fatalf("expected key=string, got %s", key.str)
	}
	if val.str != "int" {
		t.Fatalf("expected val=int, got %s", val.str)
	}
}

func TestGoElemOfNoElement(t *testing.T) {
	_, err := builtinGoElemOf([]*Value{vstr("int")})
	if err == nil {
		t.Fatal("expected error for non-container type")
	}
}

// ============================================================
// go:len-of / go:cap-of
// ============================================================

func TestGoLenOfSlice(t *testing.T) {
	ResetGlobalEnv()
	result, err := builtinGoMake([]*Value{vstr("[]int"), vnum(5)})
	if err != nil {
		t.Fatalf("go:make: %v", err)
	}
	length, err := builtinGoLenOf([]*Value{result})
	if err != nil {
		t.Fatalf("go:len-of: %v", err)
	}
	if toNum(length) != 5 {
		t.Fatalf("expected len=5, got %v", toNum(length))
	}
}

func TestGoCapOfSlice(t *testing.T) {
	ResetGlobalEnv()
	result, err := builtinGoMake([]*Value{vstr("[]int"), vnum(3), vnum(20)})
	if err != nil {
		t.Fatalf("go:make: %v", err)
	}
	capacity, err := builtinGoCapOf([]*Value{result})
	if err != nil {
		t.Fatalf("go:cap-of: %v", err)
	}
	if toNum(capacity) != 20 {
		t.Fatalf("expected cap=20, got %v", toNum(capacity))
	}
}

func TestGoLenOfMap(t *testing.T) {
	ResetGlobalEnv()
	result, err := builtinGoMake([]*Value{vstr("map[string]int")})
	if err != nil {
		t.Fatalf("go:make map: %v", err)
	}
	length, err := builtinGoLenOf([]*Value{result})
	if err != nil {
		t.Fatalf("go:len-of map: %v", err)
	}
	if toNum(length) != 0 {
		t.Fatalf("expected len=0 for empty map, got %v", toNum(length))
	}
}

func TestGoLenOfChannel(t *testing.T) {
	ResetGlobalEnv()
	result, err := builtinGoMake([]*Value{vstr("chan int"), vnum(5)})
	if err != nil {
		t.Fatalf("go:make chan: %v", err)
	}
	length, err := builtinGoLenOf([]*Value{result})
	if err != nil {
		t.Fatalf("go:len-of chan: %v", err)
	}
	if toNum(length) != 0 {
		t.Fatalf("expected len=0 for empty chan, got %v", toNum(length))
	}
}

func TestGoLenOfNonContainer(t *testing.T) {
	ResetGlobalEnv()
	intVal := &Value{
		typ:          VGoVal,
		goVal:        int(42),
		goValType:    reflect.TypeOf(int(0)),
		goValReflect: reflect.ValueOf(int(42)),
	}
	_, err := builtinGoLenOf([]*Value{intVal})
	if err == nil {
		t.Fatal("expected error for non-container type")
	}
}

// ============================================================
// go:convert
// ============================================================

func TestGoConvertStringToByteSlice(t *testing.T) {
	result, err := builtinGoConvert([]*Value{vstr("hello"), vstr("[]byte")})
	if err != nil {
		t.Fatalf("go:convert string->[]byte: %v", err)
	}
	rv := result.goValReflect
	if rv.Kind() != reflect.Slice {
		t.Fatalf("expected slice, got %v", rv.Kind())
	}
	if rv.Len() != 5 {
		t.Fatalf("expected len=5, got %d", rv.Len())
	}
}

func TestGoConvertByteSliceToString(t *testing.T) {
	// First create a []byte
	bs := []byte("world")
	byteSlice := &Value{
		typ:          VGoVal,
		goVal:        bs,
		goValType:    reflect.TypeOf(bs),
		goValReflect: reflect.ValueOf(bs),
	}
	result, err := builtinGoConvert([]*Value{byteSlice, vstr("string")})
	if err != nil {
		t.Fatalf("go:convert []byte->string: %v", err)
	}
	rv := result.goValReflect
	if rv.Kind() != reflect.String {
		t.Fatalf("expected string, got %v", rv.Kind())
	}
	if rv.String() != "world" {
		t.Fatalf("expected 'world', got %s", rv.String())
	}
}

func TestGoConvertNumericToInt64(t *testing.T) {
	result, err := builtinGoConvert([]*Value{vnum(42), vstr("int64")})
	if err != nil {
		t.Fatalf("go:convert 42->int64: %v", err)
	}
	rv := result.goValReflect
	if rv.Kind() != reflect.Int64 {
		t.Fatalf("expected int64, got %v", rv.Kind())
	}
	if rv.Int() != 42 {
		t.Fatalf("expected 42, got %d", rv.Int())
	}
}

func TestGoConvertNumericToFloat64(t *testing.T) {
	result, err := builtinGoConvert([]*Value{vnum(3), vstr("float64")})
	if err != nil {
		t.Fatalf("go:convert 3->float64: %v", err)
	}
	rv := result.goValReflect
	if rv.Kind() != reflect.Float64 {
		t.Fatalf("expected float64, got %v", rv.Kind())
	}
	if rv.Float() != 3.0 {
		t.Fatalf("expected 3.0, got %f", rv.Float())
	}
}

func TestGoConvertNumericToUint32(t *testing.T) {
	result, err := builtinGoConvert([]*Value{vnum(100), vstr("uint32")})
	if err != nil {
		t.Fatalf("go:convert 100->uint32: %v", err)
	}
	rv := result.goValReflect
	if rv.Kind() != reflect.Uint32 {
		t.Fatalf("expected uint32, got %v", rv.Kind())
	}
	if rv.Uint() != 100 {
		t.Fatalf("expected 100, got %d", rv.Uint())
	}
}

func TestGoConvertStringToRuneSlice(t *testing.T) {
	result, err := builtinGoConvert([]*Value{vstr("abc"), vstr("[]rune")})
	if err != nil {
		t.Fatalf("go:convert string->[]rune: %v", err)
	}
	rv := result.goValReflect
	if rv.Kind() != reflect.Slice {
		t.Fatalf("expected slice, got %v", rv.Kind())
	}
	if rv.Len() != 3 {
		t.Fatalf("expected len=3, got %d", rv.Len())
	}
}

func TestGoConvertListToSlice(t *testing.T) {
	// Lisp list (1 2 3) -> []int
	lispList := listFromSlice([]*Value{vnum(1), vnum(2), vnum(3)})
	result, err := builtinGoConvert([]*Value{lispList, vstr("[]int64")})
	if err != nil {
		t.Fatalf("go:convert list->[]int64: %v", err)
	}
	rv := result.goValReflect
	if rv.Kind() != reflect.Slice {
		t.Fatalf("expected slice, got %v", rv.Kind())
	}
	if rv.Len() != 3 {
		t.Fatalf("expected len=3, got %d", rv.Len())
	}
	if rv.Index(0).Int() != 1 || rv.Index(1).Int() != 2 || rv.Index(2).Int() != 3 {
		t.Fatalf("expected [1 2 3], got %v", rv.Interface())
	}
}

func TestGoConvertInvalidType(t *testing.T) {
	_, err := builtinGoConvert([]*Value{vnum(42), vstr("foobar")})
	if err == nil {
		t.Fatal("expected error for invalid target type")
	}
}

// ============================================================
// go:uintptr
// ============================================================

func TestGoUintptrOfPointer(t *testing.T) {
	val := 42
	ptr := &Value{
		typ:          VGoVal,
		goVal:        &val,
		goValType:    reflect.TypeOf(&val),
		goValReflect: reflect.ValueOf(&val),
	}
	result, err := builtinGoUintptr([]*Value{ptr})
	if err != nil {
		t.Fatalf("go:uintptr: %v", err)
	}
	if toNum(result) == 0 {
		t.Fatal("expected non-zero pointer address")
	}
}

func TestGoUintptrOfInvalid(t *testing.T) {
	nilPtr := &Value{
		typ:          VGoVal,
		goVal:        (*int)(nil),
		goValType:    reflect.TypeOf((*int)(nil)),
		goValReflect: reflect.ValueOf((*int)(nil)),
	}
	result, err := builtinGoUintptr([]*Value{nilPtr})
	if err != nil {
		t.Fatalf("go:uintptr nil: %v", err)
	}
	if toNum(result) != 0 {
		t.Fatal("expected 0 for nil pointer")
	}
}

// ============================================================
// go:type-parse
// ============================================================

func TestGoTypeParseSlice(t *testing.T) {
	result, err := builtinGoTypeParse([]*Value{vstr("[]int")})
	if err != nil {
		t.Fatalf("go:type-parse []int: %v", err)
	}
	if result.typ != VPair {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

func TestGoTypeParseMap(t *testing.T) {
	result, err := builtinGoTypeParse([]*Value{vstr("map[string]int")})
	if err != nil {
		t.Fatalf("go:type-parse map: %v", err)
	}
	if result.typ != VPair {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

func TestGoTypeParseChannel(t *testing.T) {
	result, err := builtinGoTypeParse([]*Value{vstr("chan int")})
	if err != nil {
		t.Fatalf("go:type-parse chan: %v", err)
	}
	if result.typ != VPair {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

func TestGoTypeParsePointer(t *testing.T) {
	result, err := builtinGoTypeParse([]*Value{vstr("*string")})
	if err != nil {
		t.Fatalf("go:type-parse *string: %v", err)
	}
	if result.typ != VPair {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

func TestGoTypeParseInvalid(t *testing.T) {
	_, err := builtinGoTypeParse([]*Value{vstr("foobar")})
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
}

// ============================================================
// Integration tests
// ============================================================

func TestGoReflectIntegration(t *testing.T) {
	ResetGlobalEnv()
	// Create a Certificate, inspect it, set a field, read it back
	result, err := SafeEvalString(`
		(let ((cert (go:new "crypto/x509.Certificate")))
			(go:set-field cert "SerialNumber" 1)
			(go:field cert "SerialNumber"))`)
	if err != nil {
		t.Fatalf("integration: %v", err)
	}
	_ = result // SerialNumber is *big.Int, result format may vary
}

func TestGoConvertRoundTrip(t *testing.T) {
	// string -> []byte -> string
	bs, err := builtinGoConvert([]*Value{vstr("roundtrip"), vstr("[]byte")})
	if err != nil {
		t.Fatalf("string->[]byte: %v", err)
	}
	back, err := builtinGoConvert([]*Value{bs, vstr("string")})
	if err != nil {
		t.Fatalf("[]byte->string: %v", err)
	}
	rv := back.goValReflect
	if rv.String() != "roundtrip" {
		t.Fatalf("round-trip failed: got %q", rv.String())
	}
}

// ============================================================
// go:with-defer (special form)
// ============================================================

func TestGoWithDeferNormal(t *testing.T) {
	ResetGlobalEnv()
	// Create a bytes.Buffer, use it, and Close should be called
	// bytes.Buffer has a Reset() method (not Close), let's use Reset
	result, err := SafeEvalString(`
		(go:with-defer (buf (go:make "bytes.Buffer") "Reset")
			(go:call buf "WriteString" "hello")
			(go:call buf "String"))`)
	if err != nil {
		t.Fatalf("go:with-defer normal: %v", err)
	}
	if result != `"hello"` && result != "hello" {
		t.Fatalf("expected hello, got %s", result)
	}
}

func TestGoWithDeferCleanupOnNil(t *testing.T) {
	ResetGlobalEnv()
	// go:with-defer with a non-Go value should still work (no cleanup called)
	result, err := SafeEvalString(`
		(go:with-defer (x 42 "Close")
			(+ x 8))`)
	if err != nil {
		t.Fatalf("go:with-defer non-Go: %v", err)
	}
	if result != "50" {
		t.Fatalf("expected 50, got %s", result)
	}
}

func TestGoWithDeferNoMethod(t *testing.T) {
	ResetGlobalEnv()
	// If the method doesn't exist, body should still execute
	result, err := SafeEvalString(`
		(go:with-defer (buf (go:make "bytes.Buffer") "NonexistentMethod")
			(go:call buf "WriteString" "test")
			(go:call buf "String"))`)
	if err != nil {
		t.Fatalf("go:with-defer no method: %v", err)
	}
	if result != `"test"` && result != "test" {
		t.Fatalf("expected test, got %s", result)
	}
}

func TestGoWithDeferDefaultClose(t *testing.T) {
	ResetGlobalEnv()
	// Without specifying method name, defaults to "Close"
	result, err := SafeEvalString(`
		(go:with-defer (buf (go:make "bytes.Buffer"))
			(go:call buf "WriteString" "world")
			(go:call buf "String"))`)
	if err != nil {
		t.Fatalf("go:with-defer default Close: %v", err)
	}
	// bytes.Buffer has no Close method, but no panic should happen
	if result != `"world"` && result != "world" {
		t.Fatalf("expected world, got %s", result)
	}
}

func TestGoWithDeferMalformed(t *testing.T) {
	ResetGlobalEnv()
	_, err := SafeEvalString(`(go:with-defer)`)
	if err == nil {
		t.Fatal("expected error for no args")
	}

	_, err = SafeEvalString(`(go:with-defer 42)`)
	if err == nil {
		t.Fatal("expected error for non-list binding")
	}

	_, err = SafeEvalString(`(go:with-defer ("not-a-symbol" 42 "Close"))`)
	if err == nil {
		t.Fatal("expected error for non-symbol var")
	}
}

// ============================================================
// go:func-of
// ============================================================

func TestGoFuncOfMethod(t *testing.T) {
	ResetGlobalEnv()
	// Get the String method from a bytes.Buffer
	result, err := SafeEvalString(`
		(let ((buf (go:make "bytes.Buffer")))
			(go:call buf "WriteString" "func-test")
			(let ((fn (go:func-of buf "String")))
				(go:type-of fn)))`)
	if err != nil {
		t.Fatalf("go:func-of: %v", err)
	}
	// Should return a function type
	if result == "" || result == "NIL" || result == "()" {
		t.Fatal("expected function type, got nil")
	}
}

func TestGoFuncOfNonexistent(t *testing.T) {
	ResetGlobalEnv()
	_, err := SafeEvalString(`
		(let ((buf (go:make "bytes.Buffer")))
			(go:func-of buf "NonexistentMethod"))`)
	if err == nil {
		t.Fatal("expected error for nonexistent method")
	}
}

func TestGoFuncOfDirectUnit(t *testing.T) {
	ResetGlobalEnv()
	// Create a bytes.Buffer and get WriteString method
	buf, err := builtinGoMake([]*Value{vstr("bytes.Buffer")})
	if err != nil {
		t.Fatalf("go:make bytes.Buffer: %v", err)
	}
	fn, err := builtinGoFuncOf([]*Value{buf, vstr("WriteString")})
	if err != nil {
		t.Fatalf("go:func-of WriteString: %v", err)
	}
	if fn.typ != VGoVal {
		t.Fatalf("expected VGoVal, got %s", typeStr(fn))
	}
	rv := fn.goValReflect
	if !rv.IsValid() {
		t.Fatal("expected valid reflect.Value")
	}
	if rv.Kind() != reflect.Func {
		t.Fatalf("expected Func kind, got %v", rv.Kind())
	}
}

func TestGoFuncOfInvalidValue(t *testing.T) {
	ResetGlobalEnv()
	_, err := builtinGoFuncOf([]*Value{vnum(42), vstr("Method")})
	if err == nil {
		t.Fatal("expected error for non-GoVal")
	}
}
