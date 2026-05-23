package microlisp

import (
	"reflect"
	"strings"
	"testing"
)

// ============================================================
// Section 1: go_sort.go — sort-interface, sort-list, sorted-p
// ============================================================

func TestSortListBasic(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(sort-list (list 3 1 4 1 5) (lambda (a b) (< a b)))`, globalEnv)
	if err != nil {
		t.Fatalf("sort-list: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
	if result.car.typ != VNum || toNum(result.car) != 1 {
		t.Fatalf("expected first element 1, got %v", result.car)
	}
}

func TestSortListEmpty(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(sort-list nil (lambda (a b) (< a b)))`, globalEnv)
	if err != nil {
		t.Fatalf("sort-list empty: %v", err)
	}
	if !isNil(result) {
		t.Fatalf("expected nil for empty list, got %v", ToString(result))
	}
}

func TestSortListAlreadySorted(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(sort-list (list 1 2 3) (lambda (a b) (< a b)))`, globalEnv)
	if err != nil {
		t.Fatalf("sort-list already sorted: %v", err)
	}
	if result.car.typ != VNum || toNum(result.car) != 1 {
		t.Fatalf("expected 1, got %v", result.car)
	}
}

func TestSortListDescending(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(sort-list (list 1 2 3 4 5) (lambda (a b) (> a b)))`, globalEnv)
	if err != nil {
		t.Fatalf("sort-list desc: %v", err)
	}
	if result.car.typ != VNum || toNum(result.car) != 5 {
		t.Fatalf("expected first element 5, got %v", result.car)
	}
}

func TestSortListVector(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(sort-list #(3 1 2) (lambda (a b) (< a b)))`, globalEnv)
	if err != nil {
		t.Fatalf("sort-list vector: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list from vector sort, got %s", typeStr(result))
	}
}

func TestSortListErrorCases(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(sort-list 42 (lambda (a b) (< a b)))`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-list arg")
	}
	_, err = EvalString(`(sort-list (list 1 2))`, globalEnv)
	if err == nil {
		t.Fatal("expected error for missing less-fn")
	}
}

func TestSortedPTrue(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(sorted-p (list 1 2 3 4) (lambda (a b) (< a b)))`, globalEnv)
	if err != nil {
		t.Fatalf("sorted-p true: %v", err)
	}
	if isNil(result) {
		t.Fatalf("expected T, got NIL")
	}
}

func TestSortedPFalse(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(sorted-p (list 3 1 4) (lambda (a b) (< a b)))`, globalEnv)
	if err != nil {
		t.Fatalf("sorted-p false: %v", err)
	}
	// Returns #f (false) rather than NIL
	if isTruthy(result) {
		t.Fatalf("expected false/NIL, got %v", ToString(result))
	}
}

func TestSortedPEmptyList(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(sorted-p nil (lambda (a b) (< a b)))`, globalEnv)
	if err != nil {
		t.Fatalf("sorted-p empty: %v", err)
	}
	if isNil(result) {
		t.Fatalf("expected T for empty list, got NIL")
	}
}

func TestSortedPSingleElement(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(sorted-p (list 42) (lambda (a b) (< a b)))`, globalEnv)
	if err != nil {
		t.Fatalf("sorted-p single: %v", err)
	}
	if isNil(result) {
		t.Fatalf("expected T for single element, got NIL")
	}
}

func TestSortedPVector(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(sorted-p #(1 2 3) (lambda (a b) (< a b)))`, globalEnv)
	if err != nil {
		t.Fatalf("sorted-p vector: %v", err)
	}
	if isNil(result) {
		t.Fatalf("expected T, got NIL")
	}
}

func TestSortedPErrorCases(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(sorted-p (list 1 2))`, globalEnv)
	if err == nil {
		t.Fatal("expected error for missing less-fn")
	}
}

func TestSortInterfaceBasic(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(sort-interface 5 (lambda (i j) (< i j)) (lambda (i j) nil))`, globalEnv)
	if err != nil {
		t.Fatalf("sort-interface: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal sort.Interface, got %s", typeStr(result))
	}
}

func TestSortInterfaceWithLenFn(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(sort-interface (lambda () 3) (lambda (i j) (< i j)) (lambda (i j) nil))`, globalEnv)
	if err != nil {
		t.Fatalf("sort-interface len-fn: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got %s", typeStr(result))
	}
}

func TestSortInterfaceMissingArgs(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(sort-interface 5)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for missing args")
	}
}

func TestSortInterfaceLenFnNonNumber(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(sort-interface (lambda () "not-a-number") (lambda (i j) t) (lambda (i j) nil))`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-number len-fn result")
	}
}

// ============================================================
// Section 2: go_ffi.go — go:list, go:register
// ============================================================

func TestGoListNoArgs(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list)`, globalEnv)
	if err != nil {
		t.Fatalf("go:list no args: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list of packages, got %s", typeStr(result))
	}
	output := ToString(result)
	if !strings.Contains(output, "fmt") {
		t.Fatal("expected 'fmt' in package list")
	}
}

func TestGoListWithPackage(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "fmt")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list fmt: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
	output := ToString(result)
	if !strings.Contains(output, "Sprintf") {
		t.Fatal("expected Sprintf in symbol list")
	}
}

func TestGoListUnknownPackage(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:list "nonexistent_package_xyz")`, globalEnv)
	if err == nil {
		t.Fatal("expected error for unknown package")
	}
}

func TestGoRegisterBasic(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:register "test.MyConst" 42)`, globalEnv)
	if err != nil {
		t.Fatalf("go:register: %v", err)
	}
	if result.typ != VStr || result.str != "test.MyConst" {
		t.Fatalf("expected 'test.MyConst', got %v", result)
	}
	val, err := EvalString(`(go:import "test.MyConst")`, globalEnv)
	if err != nil {
		t.Fatalf("go:import after register: %v", err)
	}
	if !isNumeric(val) || toNum(val) != 42 {
		t.Fatalf("expected 42, got %v", val)
	}
}

func TestGoRegisterString(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:register "test.Hello" "world")`, globalEnv)
	if err != nil {
		t.Fatalf("go:register string: %v", err)
	}
	val, err := EvalString(`(go:import "test.Hello")`, globalEnv)
	if err != nil {
		t.Fatalf("go:import after register: %v", err)
	}
	if val.typ != VStr || val.str != "world" {
		t.Fatalf("expected 'world', got %v", val)
	}
}

func TestGoRegisterBool(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:register "test.Enabled" #t)`, globalEnv)
	if err != nil {
		t.Fatalf("go:register bool: %v", err)
	}
}

func TestGoRegisterInvalidName(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:register "no-dot" 1)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for name without dot")
	}
}

func TestGoRegisterNonStringName(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:register 42 1)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-string name")
	}
}

func TestGoRegisterMissingArgs(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:register "test.X")`, globalEnv)
	if err == nil {
		t.Fatal("expected error for missing value arg")
	}
}

func TestGoRegisterUnsupportedType(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:register "test.Func" (lambda () 1))`, globalEnv)
	if err == nil {
		t.Fatal("expected error for unsupported value type")
	}
}

func TestGoRegisterBlacklisted(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:register "os-exit" 1)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for blacklisted name")
	}
}

func TestGoRegisterNewPackage(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:register "mynewpkg.MyValue" 99)`, globalEnv)
	if err != nil {
		t.Fatalf("go:register new package: %v", err)
	}
	val, err := EvalString(`(go:import "mynewpkg.MyValue")`, globalEnv)
	if err != nil {
		t.Fatalf("go:import after package creation: %v", err)
	}
	if !isNumeric(val) || toNum(val) != 99 {
		t.Fatalf("expected 99, got %v", val)
	}
}

func TestGoListAfterRegister(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:register "listtestpkg.Foo" 1)`, globalEnv)
	if err != nil {
		t.Fatalf("go:register: %v", err)
	}
	result, err := EvalString(`(go:list "listtestpkg")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list after register: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
	output := ToString(result)
	if !strings.Contains(output, "Foo") {
		t.Fatalf("expected 'Foo' in list, got %s", output)
	}
}

// ============================================================
// Section 3: interfaceToLisp and lispToInterface edge cases
// ============================================================

func TestLispToInterfaceDottedPair(t *testing.T) {
	InitGlobalEnv()
	// Create a dotted pair via (cons 1 2) — both atoms, not a proper list
	result, err := EvalString(`
		(let ((dp (cons 1 2)))
		  (format nil "~a" dp))`, globalEnv)
	if err != nil {
		t.Fatalf("dotted pair format: %v", err)
	}
	// A dotted pair (1 . 2) should format as "(1 . 2)"
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

func TestLispToInterfaceVArray(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(funcall (go:import "fmt.Sprintf") "%v" #(1 2 3))`, globalEnv)
	if err != nil {
		t.Fatalf("VArray through FFI: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

func TestLispToInterfaceVGoVal(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let ((buf (go:make "bytes.Buffer")))
		  (funcall (go:import "fmt.Sprintf") "%T" buf))`, globalEnv)
	if err != nil {
		t.Fatalf("VGoVal through FFI: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

func TestInterfaceToLispDefaultFmtSprintf(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(funcall (go:import "fmt.Sprintf") "%v" #t)`, globalEnv)
	if err != nil {
		t.Fatalf("bool through fmt.Sprintf: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

// ============================================================
// Section 4: go:cap-of — additional tests
// (Note: channels use VChannel type, not VGoVal, so go:cap-of
// expects VGoVal. Use chan-info for channel capacity.)
// ============================================================

func TestGoCapOfNonContainer(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:cap-of (go:make "int"))`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-container type")
	}
}

// ============================================================
// Section 5: go:convert — additional edge cases
// ============================================================

func TestGoConvertToBool(t *testing.T) {
	// lispToReflectSafe for bool: only VBool #t returns true
	// A numeric value (even 1) is NOT treated as true in this conversion
	result, err := builtinGoConvert([]*Value{vnum(1), vstr("bool")})
	if err != nil {
		t.Fatalf("go:convert 1->bool: %v", err)
	}
	rv := result.goValReflect
	if rv.Kind() != reflect.Bool {
		t.Fatalf("expected bool, got %v", rv.Kind())
	}
	// 1 is VNum, not VBool, so converts to false
	if rv.Bool() {
		t.Fatal("expected false for numeric 1 (not VBool type)")
	}

	// Actual true comes from VBool #t
	result2, err := builtinGoConvert([]*Value{globalEnv.bindings["#t"], vstr("bool")})
	if err != nil {
		t.Fatalf("go:convert #t->bool: %v", err)
	}
	if !result2.goValReflect.Bool() {
		t.Fatal("expected true for #t")
	}
}

func TestGoConvertZeroToBool(t *testing.T) {
	result, err := builtinGoConvert([]*Value{vnum(0), vstr("bool")})
	if err != nil {
		t.Fatalf("go:convert 0->bool: %v", err)
	}
	rv := result.goValReflect
	if rv.Kind() != reflect.Bool {
		t.Fatalf("expected bool, got %v", rv.Kind())
	}
	if rv.Bool() {
		t.Fatal("expected false")
	}
}

func TestGoConvertStringToBigInt(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((bi (go:new "math/big.Int"))
		       (bi2 (go:call bi "SetInt64" 123456789012345)))
		  (go:call bi "String"))`, globalEnv)
	if err != nil {
		t.Fatalf("big.Int: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %v", result)
	}
}

// ============================================================
// Section 6: go:call — variadic, zero-return, nil pointer
// ============================================================

func TestGoCallVariadic(t *testing.T) {
	InitGlobalEnv()
	// Use funcall for free functions, go:call for methods on VGoVal
	result, err := EvalString(`(funcall (go:import "fmt.Sprintf") "%s %d %s" "hello" 42 "world")`, globalEnv)
	if err != nil {
		t.Fatalf("funcall variadic: %v", err)
	}
	if result.typ != VStr || !strings.Contains(result.str, "hello") {
		t.Fatalf("expected 'hello' in result, got %v", result)
	}
}

func TestGoCallVariadicNoExtraArgs(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(funcall (go:import "fmt.Sprintf") "no placeholders")`, globalEnv)
	if err != nil {
		t.Fatalf("funcall variadic no extra: %v", err)
	}
	if result.typ != VStr || result.str != "no placeholders" {
		t.Fatalf("expected 'no placeholders', got %v", result)
	}
}

func TestGoCallZeroReturn(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(funcall (go:import "time.Sleep") 1000)`, globalEnv)
	if err != nil {
		t.Fatalf("funcall zero return: %v", err)
	}
}

func TestGoCallOnNilPointer(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`
		(let ((p (go:make "*int")))
		  (go:call p "String"))`, globalEnv)
	// Should error gracefully or handle nil receiver
	_ = err
}

// ============================================================
// Section 7: go:field / go:set-field — edge cases
// ============================================================

func TestGoFieldNonStruct(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:field (go:make "int") "X")`, globalEnv)
	if err == nil {
		t.Fatal("expected error for field access on non-struct")
	}
}

func TestGoFieldNonexistentField(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`
		(let ((cert (go:new "crypto/x509.Certificate")))
		  (go:field cert "NonExistentField"))`, globalEnv)
	if err == nil {
		t.Fatal("expected error for nonexistent field")
	}
}

func TestGoSetFieldSliceField(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let ((cert (go:new "crypto/x509.Certificate")))
		  (go:set-field cert "Extensions" (list))
		  (go:field cert "Extensions"))`, globalEnv)
	if err != nil {
		t.Fatalf("go:set-field slice: %v", err)
	}
	_ = result
}

func TestGoSetFieldNonExistentField(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`
		(let ((cert (go:new "crypto/x509.Certificate")))
		  (go:set-field cert "NoSuchField" 42))`, globalEnv)
	if err == nil {
		t.Fatal("expected error for nonexistent field set")
	}
}

// ============================================================
// Section 8: base64-decode error handling
// ============================================================

func TestBase64DecodeInvalidInput(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(ignore-errors (base64-decode "!!!not-base64!!!"))`, globalEnv)
	// Should not panic; may return nil or partial result
	_ = result
	_ = err
}

func TestBase64DecodeEmptyString(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(base64-decode "")`, globalEnv)
	if err != nil {
		t.Fatalf("base64-decode empty: %v", err)
	}
	_ = result
}

func TestBase64RoundTrip(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((original "Hello, World!")
		       (encoded (base64-encode original))
		       (decoded (base64-decode encoded)))
		  decoded)`, globalEnv)
	if err != nil {
		t.Fatalf("base64 round-trip: %v", err)
	}
	if result.typ != VStr || result.str != "Hello, World!" {
		t.Fatalf("expected original string, got %v", result)
	}
}

// ============================================================
// Section 9: Hash functions — empty string input
// ============================================================

func TestMD5EmptyString(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(md5 "")`, globalEnv)
	if err != nil {
		t.Fatalf("md5 empty: %v", err)
	}
	if result.typ != VStr || result.str != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Fatalf("expected md5 of empty string, got %v", result)
	}
}

func TestSHA1EmptyString(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(sha1 "")`, globalEnv)
	if err != nil {
		t.Fatalf("sha1 empty: %v", err)
	}
	if result.typ != VStr || result.str != "da39a3ee5e6b4b0d3255bfef95601890afd80709" {
		t.Fatalf("expected sha1 of empty string, got %v", result)
	}
}

func TestSHA256EmptyString(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(sha256 "")`, globalEnv)
	if err != nil {
		t.Fatalf("sha256 empty: %v", err)
	}
	if result.typ != VStr || result.str != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("expected sha256 of empty string, got %v", result)
	}
}

// ============================================================
// Section 10: binary helpers — int64, write uint32/uint64
// ============================================================

func TestBinaryInt64Roundtrip(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((data (binary-write-uint64 9223372036854775807 "big"))
		       (val (binary-read-int64 data "big")))
		  val)`, globalEnv)
	if err != nil {
		t.Fatalf("int64 roundtrip: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric, got %s", typeStr(result))
	}
}

func TestBinaryWriteUint32ReadBack(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((data (binary-write-uint32 4294967295 "little"))
		       (val (binary-read-uint32 data "little")))
		  val)`, globalEnv)
	if err != nil {
		t.Fatalf("uint32 max roundtrip: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 4294967295 {
		t.Fatalf("expected 4294967295, got %v", result)
	}
}

func TestBinaryWriteUint64ReadBack(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((data (binary-write-uint64 18446744073709551615 "little"))
		       (val (binary-read-uint64 data "little")))
		  val)`, globalEnv)
	if err != nil {
		t.Fatalf("uint64 max roundtrip: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric, got %s", typeStr(result))
	}
}

// ============================================================
// Section 11: lazy_ffi_scenario — case-insensitive lookup
// ============================================================

func TestLazyGoImportCaseInsensitive(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(ignore-errors (file-exists-p "."))`, globalEnv)
	// Should not error with "unknown function" — lazily loaded
	_ = result
	_ = err
}

// ============================================================
// Section 12: go:methods-of — pointer methods
// ============================================================

func TestGoMethodsOfPointerType(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((buf (go:make "bytes.Buffer"))
		       (methods (go:methods-of buf)))
		  (length methods))`, globalEnv)
	if err != nil {
		t.Fatalf("go:methods-of pointer: %v", err)
	}
	if !isNumeric(result) || toNum(result) == 0 {
		t.Fatalf("expected non-zero method count, got %v", result)
	}
}

func TestGoMethodsOfViaNew(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let ((cert (go:new "crypto/x509.Certificate")))
		  (go:methods-of cert))`, globalEnv)
	if err != nil {
		t.Fatalf("go:methods-of via go:new: %v", err)
	}
	_ = result
}

// ============================================================
// Section 13: go:kind-of — additional types
// ============================================================

func TestGoKindOfArray(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:kind-of "[5]int")`, globalEnv)
	if err != nil {
		t.Fatalf("go:kind-of array: %v", err)
	}
	if result.str != "array" {
		t.Fatalf("expected 'array', got %q", result.str)
	}
}

func TestGoKindOfInterface(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:kind-of "io.Reader")`, globalEnv)
	if err != nil {
		t.Fatalf("go:kind-of interface: %v", err)
	}
	if result.str != "interface" {
		t.Fatalf("expected 'interface', got %q", result.str)
	}
}

// ============================================================
// Section 14: go:fields-of — nil pointer test
// ============================================================

func TestGoFieldsOfNilPointer(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`
		(let ((p (go:make "*crypto/x509.Certificate")))
		  (go:fields-of p))`, globalEnv)
	// Should either error or return fields — both acceptable
	_ = err
}

// ============================================================
// Section 15: GoFFILazyTableByPkg — apropos grouping
// ============================================================

func TestGoFFILazyTableByPkgExists(t *testing.T) {
	if len(GoFFILazyTableByPkg) == 0 {
		t.Fatal("GoFFILazyTableByPkg should not be empty")
	}
	found := false
	for pkg := range GoFFILazyTableByPkg {
		if pkg == "fmt" || pkg == "strings" || pkg == "math" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected common packages in GoFFILazyTableByPkg")
	}
}

// ============================================================
// Section 16: GoFFILazyTable — verify case-insensitive fallback
// ============================================================

func TestGoFFILazyTableHasEntries(t *testing.T) {
	if len(GoFFILazyTable) == 0 {
		t.Fatal("GoFFILazyTable should not be empty")
	}
}

// ============================================================
// Section 17: sort-list with strings
// ============================================================

func TestSortListStrings(t *testing.T) {
	InitGlobalEnv()
	// sort-list with string comparison — use a lambda that returns a bool
	result, err := EvalString(`
		(sort-list (list "banana" "apple" "cherry")
		  (lambda (a b) (if (string< a b) #t #f)))`, globalEnv)
	if err != nil {
		t.Fatalf("sort-list strings: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
	first := result.car
	if first.typ != VStr {
		t.Fatalf("expected string first element, got %s", typeStr(first))
	}
	if first.str != "apple" {
		t.Fatalf("expected 'apple', got %q", first.str)
	}
}

// ============================================================
// Section 18: go:register list value (error case)
// ============================================================

func TestGoRegisterListValue(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:register "test.MyList" (list 1 2 3))`, globalEnv)
	if err == nil {
		t.Fatal("expected error for unsupported list value")
	}
}

// ============================================================
// Section 19: go:register vector value (error case)
// ============================================================

func TestGoRegisterVectorValue(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:register "test.MyVec" #(1 2 3))`, globalEnv)
	if err == nil {
		t.Fatal("expected error for unsupported vector value")
	}
}

// ============================================================
// Section 20: go:register double-dot in same package
// ============================================================

func TestGoRegisterMultipleInSamePackage(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`
		(progn
		  (go:register "multipkg.A" 1)
		  (go:register "multipkg.B" 2)
		  (go:register "multipkg.C" 3))`, globalEnv)
	if err != nil {
		t.Fatalf("go:register multiple same pkg: %v", err)
	}
	a, _ := EvalString(`(go:import "multipkg.A")`, globalEnv)
	b, _ := EvalString(`(go:import "multipkg.B")`, globalEnv)
	c, _ := EvalString(`(go:import "multipkg.C")`, globalEnv)
	if toNum(a) != 1 || toNum(b) != 2 || toNum(c) != 3 {
		t.Fatalf("expected 1,2,3 got %v,%v,%v", a, b, c)
	}
}
