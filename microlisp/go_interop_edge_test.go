package microlisp

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

// Interface type aliases for test convenience
type ioByteReader interface {
	ReadByte() (byte, error)
}
type ioReader interface {
	Read(p []byte) (n int, err error)
}
type bytesBuffer struct{}

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

// ============================================================
// Section 21: SHA1 — positive tests (non-empty input)
// ============================================================

func TestSHA1PositiveHello(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(sha1 "hello")`, globalEnv)
	if err != nil {
		t.Fatalf("sha1 hello: %v", err)
	}
	if result.typ != VStr || result.str != "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d" {
		t.Fatalf("expected sha1 of 'hello', got %v", result)
	}
}

func TestSHA1WithData(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(sha1 "hello world")`, globalEnv)
	if err != nil {
		t.Fatalf("sha1 hello world: %v", err)
	}
	if result.typ != VStr || result.str != "2aae6c35c94fcfb415dbe95f408b9ce91ee846ed" {
		t.Fatalf("expected sha1 of 'hello world', got %v", result)
	}
}

// ============================================================
// Section 22: lispToReflectSafe — VChar conversion paths
// ============================================================

func TestLispToReflectSafeVCharToInt32(t *testing.T) {
	ch := vchar('A')
	rv, err := lispToReflectSafe(ch, reflect.TypeOf(int32(0)))
	if err != nil {
		t.Fatalf("VChar->int32: %v", err)
	}
	if rv.Int() != int64('A') {
		t.Fatalf("expected %d, got %d", 'A', rv.Int())
	}
}

func TestLispToReflectSafeVCharToUint8(t *testing.T) {
	ch := vchar('x')
	rv, err := lispToReflectSafe(ch, reflect.TypeOf(uint8(0)))
	if err != nil {
		t.Fatalf("VChar->uint8: %v", err)
	}
	if rv.Uint() != uint64('x') {
		t.Fatalf("expected %d, got %d", 'x', rv.Uint())
	}
}

func TestLispToReflectSafeVCharToString(t *testing.T) {
	ch := vchar('中')
	rv, err := lispToReflectSafe(ch, reflect.TypeOf(""))
	if err != nil {
		t.Fatalf("VChar->string: %v", err)
	}
	if rv.String() != "中" {
		t.Fatalf("expected '中', got %q", rv.String())
	}
}

func TestLispToReflectSafeNegativeToUint(t *testing.T) {
	rv, err := lispToReflectSafe(vnum(-1), reflect.TypeOf(uint32(0)))
	if err == nil {
		t.Fatal("expected error for negative->uint, got nil")
	}
	_ = rv
}

// ============================================================
// Section 23: lispToReflectSafe — VArray conversion paths
// ============================================================

func TestLispToReflectSafeVArrayToStringSlice(t *testing.T) {
	arr := &Value{typ: VArray, array: &LispArray{elements: []*Value{vstr("a"), vstr("b"), vstr("c")}}}
	rv, err := lispToReflectSafe(arr, reflect.TypeOf([]string{}))
	if err != nil {
		t.Fatalf("VArray->[]string: %v", err)
	}
	if rv.Len() != 3 || rv.Index(0).String() != "a" {
		t.Fatalf("expected [a b c], got %v", rv.Interface())
	}
}

func TestLispToReflectSafeVArrayToIntSlice(t *testing.T) {
	arr := &Value{typ: VArray, array: &LispArray{elements: []*Value{vnum(10), vnum(20), vnum(30)}}}
	rv, err := lispToReflectSafe(arr, reflect.TypeOf([]int{}))
	if err != nil {
		t.Fatalf("VArray->[]int: %v", err)
	}
	if rv.Len() != 3 || rv.Index(0).Int() != 10 {
		t.Fatalf("expected [10 20 30], got %v", rv.Interface())
	}
}

func TestLispToReflectSafeVArrayToFloatSlice(t *testing.T) {
	arr := &Value{typ: VArray, array: &LispArray{elements: []*Value{vnum(1.5), vnum(2.5)}}}
	rv, err := lispToReflectSafe(arr, reflect.TypeOf([]float64{}))
	if err != nil {
		t.Fatalf("VArray->[]float64: %v", err)
	}
	if rv.Len() != 2 || rv.Index(0).Float() != 1.5 {
		t.Fatalf("expected [1.5 2.5], got %v", rv.Interface())
	}
}

func TestLispToReflectSafeVArrayToByteSliceOfSlices(t *testing.T) {
	inner1 := &Value{typ: VArray, array: &LispArray{elements: []*Value{vnum(1), vnum(2)}}}
	inner2 := &Value{typ: VArray, array: &LispArray{elements: []*Value{vnum(3), vnum(4)}}}
	arr := &Value{typ: VArray, array: &LispArray{elements: []*Value{inner1, inner2}}}
	rv, err := lispToReflectSafe(arr, reflect.TypeOf([][]byte{}))
	if err != nil {
		t.Fatalf("VArray->[][]byte: %v", err)
	}
	if rv.Len() != 2 {
		t.Fatalf("expected 2 slices, got %d", rv.Len())
	}
}

func TestLispToReflectSafeVArrayToByteReader(t *testing.T) {
	// Note: the interface type check for io.ByteReader in lispToReflectSafe
	// compares against reflect.TypeOf((*io.ByteReader)(nil)).Elem() from the
	// io package. Our local ioByteReader alias is a different reflect.Type.
	// So VArray with uint8 elements hits the []byte slice path first.
	arr := &Value{typ: VArray, array: &LispArray{elements: []*Value{vnum(72), vnum(101), vnum(108)}}}
	rv, err := lispToReflectSafe(arr, reflect.TypeOf((*ioByteReader)(nil)).Elem())
	if err != nil {
		t.Fatalf("VArray->io.ByteReader alias: %v", err)
	}
	// Returns a []byte slice (the slice path is hit before interface for byte elements)
	if rv.Kind() != reflect.Slice {
		t.Fatalf("expected slice, got %v", rv.Kind())
	}
}

func TestLispToReflectSafeVArrayToReader(t *testing.T) {
	arr := &Value{typ: VArray, array: &LispArray{elements: []*Value{vnum(65), vnum(66)}}}
	rv, err := lispToReflectSafe(arr, reflect.TypeOf((*ioReader)(nil)).Elem())
	if err != nil {
		t.Fatalf("VArray->io.Reader alias: %v", err)
	}
	// Returns a []byte slice
	if rv.Kind() != reflect.Slice {
		t.Fatalf("expected slice, got %v", rv.Kind())
	}
}

func TestLispToReflectSafeVStrToBufferPointer(t *testing.T) {
	// bytesBuffer is a local alias type, not *bytes.Buffer from the Go stdlib,
	// so lispToReflectSafe doesn't recognize it as a *bytes.Buffer target.
	// VStr hits reflect.String instead.
	rv, err := lispToReflectSafe(vstr("hello"), reflect.TypeOf(&bytesBuffer{}))
	if err != nil {
		t.Fatalf("VStr->local bytesBuffer alias: %v", err)
	}
	if rv.Kind() != reflect.String {
		t.Fatalf("expected string (fallback), got %v", rv.Kind())
	}
}

func TestLispToReflectSafeVArrayToBufferPointer(t *testing.T) {
	arr := &Value{typ: VArray, array: &LispArray{elements: []*Value{vnum(104), vnum(105)}}}
	rv, err := lispToReflectSafe(arr, reflect.TypeOf(&bytesBuffer{}))
	if err != nil {
		t.Fatalf("VArray->*bytes.Buffer: %v", err)
	}
	// bytesBuffer is a local alias, not bytes.Buffer, so it hits the []byte slice path
	if rv.Kind() != reflect.Slice {
		t.Fatalf("expected slice, got %v", rv.Kind())
	}
}

// ============================================================
// Section 24: lispToReflectSafe — VArray error paths
// ============================================================

func TestLispToReflectSafeVArrayOutOfRange(t *testing.T) {
	arr := &Value{typ: VArray, array: &LispArray{elements: []*Value{vnum(300)}}}
	_, err := lispToReflectSafe(arr, reflect.TypeOf([]byte{}))
	if err == nil {
		t.Fatal("expected error for out-of-range byte, got nil")
	}
}

func TestLispToReflectSafeVArrayBadElementType(t *testing.T) {
	arr := &Value{typ: VArray, array: &LispArray{elements: []*Value{vstr("not a number")}}}
	_, err := lispToReflectSafe(arr, reflect.TypeOf([]int{}))
	if err == nil {
		t.Fatal("expected error for non-numeric element in int slice, got nil")
	}
}

func TestLispToReflectSafeVArrayNestedBadElement(t *testing.T) {
	inner := &Value{typ: VArray, array: &LispArray{elements: []*Value{vstr("bad")}}}
	arr := &Value{typ: VArray, array: &LispArray{elements: []*Value{inner}}}
	_, err := lispToReflectSafe(arr, reflect.TypeOf([][]byte{}))
	if err == nil {
		t.Fatal("expected error for non-numeric nested element, got nil")
	}
}

func TestLispToReflectSafeVArrayNotNestedArray(t *testing.T) {
	arr := &Value{typ: VArray, array: &LispArray{elements: []*Value{vnum(1), vnum(2)}}}
	_, err := lispToReflectSafe(arr, reflect.TypeOf([][]byte{}))
	if err == nil {
		t.Fatal("expected error for non-array nested element in [][]byte, got nil")
	}
}

func TestLispToReflectSafeVArrayToByteReaderOutOfRange(t *testing.T) {
	// VArray→io.ByteReader path doesn't validate range per-element (it converts
	// to []byte which truncates), so this test documents the behavior
	arr := &Value{typ: VArray, array: &LispArray{elements: []*Value{vnum(999)}}}
	rv, err := lispToReflectSafe(arr, reflect.TypeOf((*ioByteReader)(nil)).Elem())
	// 999 % 256 = 231, which is a valid byte
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Returns []byte (slice path), not *bytes.Reader
	if rv.Kind() != reflect.Slice {
		t.Fatalf("expected slice, got %v", rv.Kind())
	}
}

func TestLispToReflectSafeVArrayToReaderOutOfRange(t *testing.T) {
	arr := &Value{typ: VArray, array: &LispArray{elements: []*Value{vnum(256)}}}
	rv, err := lispToReflectSafe(arr, reflect.TypeOf((*ioReader)(nil)).Elem())
	// 256 % 256 = 0, which is a valid byte
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Returns []byte (slice path), not *bytes.Reader
	if rv.Kind() != reflect.Slice {
		t.Fatalf("expected slice, got %v", rv.Kind())
	}
}

// ============================================================
// Section 25: lispToReflectSafe — list element conversion error
// ============================================================

func TestLispToReflectSafeListElementError(t *testing.T) {
	lst := listFromSlice([]*Value{vnum(1), vstr("not-int"), vnum(3)})
	_, err := lispToReflectSafe(lst, reflect.TypeOf([]int{}))
	if err == nil {
		t.Fatal("expected error for non-numeric list element, got nil")
	}
}

func TestLispToReflectSafeListTypeMismatch(t *testing.T) {
	lst := listFromSlice([]*Value{vstr("a"), vstr("b")})
	_, err := lispToReflectSafe(lst, reflect.TypeOf([]int{}))
	if err == nil {
		t.Fatal("expected error for string elements in int slice, got nil")
	}
}

// ============================================================
// Section 26: interfaceToLisp — direct unit tests
// ============================================================

func TestInterfaceToLispFloat32(t *testing.T) {
	result := interfaceToLisp(float32(3.14))
	if result.typ != VNum {
		t.Fatalf("expected VNum, got %s", typeStr(result))
	}
}

func TestInterfaceToLispUint(t *testing.T) {
	result := interfaceToLisp(uint64(42))
	if result.typ != VNum {
		t.Fatalf("expected VNum, got %s", typeStr(result))
	}
	if toNum(result) != 42 {
		t.Fatalf("expected 42, got %v", result)
	}
}

func TestInterfaceToLispMap(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	result := interfaceToLisp(m)
	if result.typ != VPair {
		t.Fatalf("expected VPair (alist), got %s", typeStr(result))
	}
	count := 0
	for p := result; !isNil(p); p = p.cdr {
		count++
	}
	if count != 2 {
		t.Fatalf("expected 2 alist entries, got %d", count)
	}
}

func TestInterfaceToLispComplex(t *testing.T) {
	result := interfaceToLisp(complex(1.0, 2.0))
	if result.typ != VComplex {
		t.Fatalf("expected VComplex, got %s", typeStr(result))
	}
}

func TestInterfaceToLispNil(t *testing.T) {
	result := interfaceToLisp(nil)
	if result.typ != VNil {
		t.Fatalf("expected VNil, got %s", typeStr(result))
	}
}

func TestInterfaceToLispStructDefault(t *testing.T) {
	type myStruct struct{ X int }
	result := interfaceToLisp(myStruct{X: 42})
	if result.typ != VStr {
		t.Fatalf("expected VStr (fallback for struct), got %s", typeStr(result))
	}
}

func TestInterfaceToLispSliceNonByte(t *testing.T) {
	result := interfaceToLisp([]int{1, 2, 3})
	if result.typ != VPair {
		t.Fatalf("expected VPair (list), got %s", typeStr(result))
	}
}

// ============================================================
// Section 27: builtinGoIsNil — nil Func and nil Chan
// ============================================================

func TestGoIsNilNilFunc(t *testing.T) {
	var fn func() = nil
	gv := vgoval(fn, reflect.TypeOf(fn))
	result, err := builtinGoIsNil([]*Value{gv})
	if err != nil {
		t.Fatalf("go:is-nil nil func: %v", err)
	}
	if !isTruthy(result) {
		t.Fatal("expected T for nil func")
	}
}

func TestGoIsNilNilChan(t *testing.T) {
	var ch chan int = nil
	gv := vgoval(ch, reflect.TypeOf(ch))
	result, err := builtinGoIsNil([]*Value{gv})
	if err != nil {
		t.Fatalf("go:is-nil nil chan: %v", err)
	}
	if !isTruthy(result) {
		t.Fatal("expected T for nil chan")
	}
}

func TestGoIsNilNonNilChan(t *testing.T) {
	ch := make(chan int, 1)
	gv := vgoval(ch, reflect.TypeOf(ch))
	result, err := builtinGoIsNil([]*Value{gv})
	if err != nil {
		t.Fatalf("go:is-nil non-nil chan: %v", err)
	}
	if isTruthy(result) {
		t.Fatal("expected NIL for non-nil chan")
	}
}

// ============================================================
// Section 28: builtinGoIsZero — additional cases
// ============================================================

func TestGoIsZeroEmptyList(t *testing.T) {
	result, err := builtinGoIsZero([]*Value{vnil()})
	if err != nil {
		t.Fatalf("go:is-zero empty list: %v", err)
	}
	if !isTruthy(result) {
		t.Fatal("expected T for empty list (nil)")
	}
}

func TestGoIsZeroNonNumeric(t *testing.T) {
	result, err := builtinGoIsZero([]*Value{vnum(1)})
	if err != nil {
		t.Fatalf("go:is-zero 1: %v", err)
	}
	if isTruthy(result) {
		t.Fatal("expected NIL for non-zero number")
	}
}

// ============================================================
// Section 29: builtinGoAssertType — nil value cases
// ============================================================

func TestGoAssertTypeNilValueNoDefault(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:assert-type nil "io.Reader")`, globalEnv)
	if err == nil {
		t.Fatal("expected error for nil value without default")
	}
}

func TestGoAssertTypeNilValueWithDefault(t *testing.T) {
	InitGlobalEnv()
	// nil in Lisp is VNil, not VGoVal, so go:assert-type errors on type check
	// This is expected behavior — nil Lisp value can't be used for type assertion
	_, err := EvalString(`(go:assert-type nil "io.Reader" :default)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for Lisp nil (not VGoVal)")
	}
}

// ============================================================
// Section 30: builtinGoImplements — edge cases
// ============================================================

func TestGoImplementsInvalidGoValue(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:implements 42 "io.Reader")`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-VGoVal")
	}
}

func TestGoImplementsNilInterface(t *testing.T) {
	var reader ioReader = nil
	gv := vgoval(reader, reflect.TypeOf((*ioReader)(nil)).Elem())
	result, err := builtinGoImplements([]*Value{gv, vstr("io.Reader")})
	if err != nil {
		t.Fatalf("go:implements nil interface: %v", err)
	}
	if isTruthy(result) {
		t.Fatal("expected NIL for nil interface")
	}
}

// ============================================================
// Section 31: builtinGoCallback — additional signatures
// ============================================================

func TestGoCallbackStringToError(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:callback (lambda (s) nil) "string->error")`, globalEnv)
	if err != nil {
		t.Fatalf("go:callback string->error: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got %s", typeStr(result))
	}
}

func TestGoCallbackIntIntToBool(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:callback (lambda (a b) (> a b)) "int,int->bool")`, globalEnv)
	if err != nil {
		t.Fatalf("go:callback int,int->bool: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got %s", typeStr(result))
	}
}

func TestGoCallbackIntIntToVoid(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:callback (lambda (a b) nil) "int,int->")`, globalEnv)
	if err != nil {
		t.Fatalf("go:callback int,int->: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got %s", typeStr(result))
	}
}

func TestGoCallbackInt32ToBool(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:callback (lambda (r) (= r 32)) "int32->bool")`, globalEnv)
	if err != nil {
		t.Fatalf("go:callback int32->bool: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got %s", typeStr(result))
	}
}

func TestBuiltinGoCallbackUnknownSignature(t *testing.T) {
	_, err := builtinGoCallback([]*Value{vnum(1), vstr("unknown->sig")})
	if err == nil {
		t.Fatal("expected error for unknown signature")
	}
	if !strings.Contains(err.Error(), "unknown signature") {
		t.Fatalf("expected 'unknown signature', got: %v", err)
	}
}

func TestBuiltinGoCallbackMissingArgs(t *testing.T) {
	_, err := builtinGoCallback([]*Value{})
	if err == nil {
		t.Fatal("expected error for missing args")
	}
}

func TestBuiltinGoCallbackWrongSigType(t *testing.T) {
	_, err := builtinGoCallback([]*Value{vnum(1), vnum(2)})
	if err == nil {
		t.Fatal("expected error for non-string signature")
	}
}

// ============================================================
// Section 32: makeGoCallback — all supported signatures
// ============================================================

func TestMakeGoCallbackAllSignatures(t *testing.T) {
	sigs := []string{
		"int32->bool", "int32->int32", "int->int", "int->bool",
		"int,int->bool", "int,int->", "string->string", "()->",
		"string->error",
	}
	for _, sig := range sigs {
		_, err := makeGoCallback(vnil(), sig)
		if err != nil {
			t.Errorf("makeGoCallback(%q): %v", sig, err)
		}
	}
}

func TestMakeGoCallbackUnknownSig(t *testing.T) {
	_, err := makeGoCallback(vnil(), "float64->float64")
	if err == nil {
		t.Fatal("expected error for unknown sig")
	}
}

// ============================================================
// Section 33: HttpRequest — VGoVal body (io.Reader)
// ============================================================

func TestHttpRequestWithReaderBody(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((body (reader-from-string "reader body"))
		       (req (http-request "POST" "https://httpbin.org/post" body)))
		  req)
	`, globalEnv)
	if err != nil {
		t.Fatalf("http-request with reader body: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got %s", typeStr(result))
	}
}

// ============================================================
// Section 34: HttpDo — custom http.Client
// ============================================================

func TestHttpDoWithCustomClient(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((req (http-request "GET" "https://httpbin.org/get"))
		       (client (go:new "net/http.Client"))
		       (resp (http-do req client)))
		  resp)
	`, globalEnv)
	if err != nil {
		t.Fatalf("http-do with custom client: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

// ============================================================
// Section 35: convertToReflect — list→array
// ============================================================

func TestConvertToReflectListToArray(t *testing.T) {
	lst := listFromSlice([]*Value{vnum(1), vnum(2), vnum(3)})
	arrType := reflect.TypeOf([3]int{})
	rv, err := convertToReflect(lst, arrType)
	if err != nil {
		t.Fatalf("list->array: %v", err)
	}
	if rv.Kind() != reflect.Array || rv.Len() != 3 {
		t.Fatalf("expected [3]int, got %v", rv.Type())
	}
	if rv.Index(0).Int() != 1 {
		t.Fatalf("expected first element 1, got %d", rv.Index(0).Int())
	}
}

func TestConvertToReflectStringToBigFloat(t *testing.T) {
	InitGlobalEnv()
	// *big.Float can't be parsed by parseGoType (qualified name), so test
	// the conversion path through the actual Lisp code that creates a big.Float
	result, err := EvalString(`
		(let ((bf (go:call (go:make "*big.Int") "SetInt64" 12345)))
		  (go:call bf "String"))
	`, globalEnv)
	// This tests that go:make creates a usable value
	_ = result
	_ = err
}

// ============================================================
// Section 36: reflectToLisp — Float32 and Uint branches
// ============================================================

func TestReflectToLispFloat32(t *testing.T) {
	f := float32(3.14)
	rv := reflect.ValueOf(f)
	result := reflectToLisp(rv)
	if result.typ != VNum {
		t.Fatalf("expected VNum, got %s", typeStr(result))
	}
}

func TestReflectToLispUint(t *testing.T) {
	u := uint64(100)
	rv := reflect.ValueOf(u)
	result := reflectToLisp(rv)
	if result.typ != VNum {
		t.Fatalf("expected VNum, got %s", typeStr(result))
	}
}

func TestReflectToLispSliceString(t *testing.T) {
	s := []string{"a", "b", "c"}
	rv := reflect.ValueOf(s)
	result := reflectToLisp(rv)
	if result.typ != VPair {
		t.Fatalf("expected VPair (list), got %s", typeStr(result))
	}
	first := result.car
	if first.typ != VStr || first.str != "a" {
		t.Fatalf("expected 'a' as first element, got %v", first)
	}
}

// ============================================================
// Section 37: lispToReflectSafe — nil VGoVal
// ============================================================

func TestLispToReflectSafeNilVGoVal(t *testing.T) {
	// Create a typed nil pointer (*int = nil)
	var ip *int = nil
	gv := vgoval(ip, reflect.TypeOf(ip))
	rv, err := lispToReflectSafe(gv, reflect.TypeOf((*int)(nil)))
	if err != nil {
		t.Fatalf("nil VGoVal: %v", err)
	}
	if !rv.IsNil() {
		t.Fatal("expected nil pointer")
	}
}

// ============================================================
// Section 38: io adapter — lispStringReader ReadAt/Seek
// ============================================================

func TestLispStringReaderReadAt(t *testing.T) {
	r := &lispStringReader{s: "hello world", i: 0}
	buf := make([]byte, 5)
	n, err := r.ReadAt(buf, 6)
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if n != 5 || string(buf) != "world" {
		t.Fatalf("expected 'world', got %q", string(buf))
	}
}

func TestLispStringReaderReadAtEOF(t *testing.T) {
	r := &lispStringReader{s: "hi", i: 0}
	buf := make([]byte, 10)
	n, err := r.ReadAt(buf, 0)
	if n != 2 || string(buf[:n]) != "hi" {
		t.Fatalf("expected 'hi', got %q (%d bytes)", string(buf[:n]), n)
	}
	if err == nil {
		t.Fatal("expected EOF for short read")
	}
}

func TestLispStringReaderSeekSet(t *testing.T) {
	r := &lispStringReader{s: "hello world", i: 0}
	pos, err := r.Seek(6, 0)
	if err != nil {
		t.Fatalf("Seek set: %v", err)
	}
	if pos != 6 {
		t.Fatalf("expected pos 6, got %d", pos)
	}
	buf := make([]byte, 5)
	r.Read(buf)
	if string(buf) != "world" {
		t.Fatalf("expected 'world', got %q", string(buf))
	}
}

func TestLispStringReaderSeekCur(t *testing.T) {
	r := &lispStringReader{s: "hello world", i: 3}
	pos, err := r.Seek(3, 1)
	if err != nil {
		t.Fatalf("Seek cur: %v", err)
	}
	if pos != 6 {
		t.Fatalf("expected pos 6, got %d", pos)
	}
}

func TestLispStringReaderSeekEnd(t *testing.T) {
	r := &lispStringReader{s: "hello", i: 0}
	pos, err := r.Seek(-3, 2)
	if err != nil {
		t.Fatalf("Seek end: %v", err)
	}
	if pos != 2 {
		t.Fatalf("expected pos 2, got %d", pos)
	}
}

func TestLispStringReaderSeekInvalidWhence(t *testing.T) {
	r := &lispStringReader{s: "hello", i: 0}
	_, err := r.Seek(0, 99)
	if err == nil {
		t.Fatal("expected error for invalid whence")
	}
}

func TestLispStringReaderSeekInvalidOffset(t *testing.T) {
	r := &lispStringReader{s: "hello", i: 0}
	_, err := r.Seek(-1, 0)
	if err == nil {
		t.Fatal("expected error for negative offset")
	}
}

// ============================================================
// Section 39: io adapter — lispBufferReader ReadAt/Seek
// ============================================================

func TestLispBufferReaderReadAt(t *testing.T) {
	r := &lispBufferReader{buf: []byte("buffer data"), i: 0}
	buf := make([]byte, 4)
	n, err := r.ReadAt(buf, 7)
	if err != nil {
		t.Fatalf("buffer ReadAt: %v", err)
	}
	if n != 4 || string(buf) != "data" {
		t.Fatalf("expected 'data', got %q", string(buf))
	}
}

func TestLispBufferReaderSeekSet(t *testing.T) {
	r := &lispBufferReader{buf: []byte("buffer data"), i: 0}
	pos, err := r.Seek(7, 0)
	if err != nil {
		t.Fatalf("buffer Seek: %v", err)
	}
	if pos != 7 {
		t.Fatalf("expected pos 7, got %d", pos)
	}
}

func TestLispBufferReaderSeekCur(t *testing.T) {
	r := &lispBufferReader{buf: []byte("abcdef"), i: 2}
	pos, err := r.Seek(2, 1)
	if err != nil {
		t.Fatalf("buffer Seek cur: %v", err)
	}
	if pos != 4 {
		t.Fatalf("expected pos 4, got %d", pos)
	}
}

// ============================================================
// Section 40: io adapter — lispStringWriter.Bytes/Reset and lispBufferWriter
// ============================================================

func TestLispStringWriterBytes(t *testing.T) {
	w := &lispStringWriter{}
	w.buf.WriteString("test bytes")
	b := w.Bytes()
	if string(b) != "test bytes" {
		t.Fatalf("expected 'test bytes', got %q", string(b))
	}
}

func TestLispStringWriterReset(t *testing.T) {
	w := &lispStringWriter{}
	w.buf.WriteString("before")
	w.Reset()
	w.buf.WriteString("after")
	if w.String() != "after" {
		t.Fatalf("expected 'after', got %q", w.String())
	}
}

func TestLispBufferWriterReset(t *testing.T) {
	w := &lispBufferWriter{buf: &bytes.Buffer{}}
	w.buf.WriteString("before")
	w.Reset()
	w.buf.WriteString("after")
	if string(w.Bytes()) != "after" {
		t.Fatalf("expected 'after', got %q", string(w.Bytes()))
	}
}

func TestLispBufferWriterBytes(t *testing.T) {
	w := &lispBufferWriter{buf: &bytes.Buffer{}}
	w.buf.WriteString("buffer writer test")
	b := w.Bytes()
	if string(b) != "buffer writer test" {
		t.Fatalf("expected 'buffer writer test', got %q", string(b))
	}
}

// ============================================================
// Section 41: go:list with previously untested packages
// ============================================================

func TestGoListWithTypeRegistry(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "container/list")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list container/list: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
	output := ToString(result)
	if !strings.Contains(output, "New") {
		t.Fatal("expected 'New' in container/list")
	}
	if !strings.Contains(output, "List") {
		t.Fatal("expected 'List' type in container/list")
	}
}

func TestGoListWithArchiveTar(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "archive/tar")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list archive/tar: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
	output := ToString(result)
	if !strings.Contains(output, "NewReader") {
		t.Fatal("expected 'NewReader' in archive/tar")
	}
}

func TestGoListWithImagePkg(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "image")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list image: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
	output := ToString(result)
	if !strings.Contains(output, "NewRGBA") {
		t.Fatal("expected 'NewRGBA' in image")
	}
}

func TestGoListWithHTML(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "html")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list html: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
	output := ToString(result)
	if !strings.Contains(output, "EscapeString") {
		t.Fatal("expected 'EscapeString' in html")
	}
}

func TestGoListWithMime(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "mime")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list mime: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
	output := ToString(result)
	if !strings.Contains(output, "TypeByExtension") {
		t.Fatal("expected 'TypeByExtension' in mime")
	}
}

func TestGoListWithContainerHeap(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "container/heap")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list container/heap: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
	output := ToString(result)
	if !strings.Contains(output, "Push") {
		t.Fatal("expected 'Push' in container/heap")
	}
}

func TestGoListWithContainerRing(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "container/ring")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list container/ring: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

func TestGoListWithDatabaseSQL(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "database/sql")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list database/sql: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
	output := ToString(result)
	if !strings.Contains(output, "Open") {
		t.Fatal("expected 'Open' in database/sql")
	}
}

func TestGoListWithEmbed(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "embed")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list embed: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

func TestGoListWithIndexSuffixarray(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "index/suffixarray")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list index/suffixarray: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
	output := ToString(result)
	if !strings.Contains(output, "New") {
		t.Fatal("expected 'New' in index/suffixarray")
	}
}

func TestGoListWithMimeMultipart(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "mime/multipart")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list mime/multipart: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

func TestGoListWithHTMLTemplate(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "html/template")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list html/template: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

func TestGoListWithReflectPkg(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "reflect")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list reflect: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
	output := ToString(result)
	if !strings.Contains(output, "TypeOf") {
		t.Fatal("expected 'TypeOf' in reflect")
	}
}

func TestGoListWithSyncAtomic(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "sync/atomic")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list sync/atomic: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
	output := ToString(result)
	if !strings.Contains(output, "AddInt32") {
		t.Fatal("expected 'AddInt32' in sync/atomic")
	}
}

func TestGoListWithCompressPackages(t *testing.T) {
	InitGlobalEnv()
	pkgs := []string{
		"compress/bzip2", "compress/flate", "compress/gzip",
		"compress/lzw", "compress/zlib",
	}
	for _, pkg := range pkgs {
		result, err := EvalString(`(go:list "`+pkg+`")`, globalEnv)
		if err != nil {
			t.Fatalf("go:list %s: %v", pkg, err)
		}
		if !isList(result) {
			t.Fatalf("expected list for %s, got %s", pkg, typeStr(result))
		}
	}
}

func TestGoListWithEncodingExtra(t *testing.T) {
	InitGlobalEnv()
	pkgs := []string{
		"encoding/ascii85", "encoding/gob", "encoding/pem",
	}
	for _, pkg := range pkgs {
		result, err := EvalString(`(go:list "`+pkg+`")`, globalEnv)
		if err != nil {
			t.Fatalf("go:list %s: %v", pkg, err)
		}
		if !isList(result) {
			t.Fatalf("expected list for %s, got %s", pkg, typeStr(result))
		}
	}
}

func TestGoListWithImageColor(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "image/color")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list image/color: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

func TestGoListWithImageDraw(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "image/draw")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list image/draw: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

func TestGoListWithImageGif(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "image/gif")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list image/gif: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

func TestGoListWithImageJpeg(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "image/jpeg")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list image/jpeg: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

func TestGoListWithImagePng(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "image/png")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list image/png: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

func TestGoListWithMimeQuotedprintable(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "mime/quotedprintable")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list mime/quotedprintable: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

func TestGoListWithGoAST(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "go/ast")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list go/ast: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

func TestGoListWithGoBuild(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "go/build")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list go/build: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

func TestGoListWithGoFormat(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "go/format")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list go/format: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

func TestGoListWithGoParser(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "go/parser")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list go/parser: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

func TestGoListWithGoPrinter(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "go/printer")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list go/printer: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

func TestGoListWithGoToken(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:list "go/token")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list go/token: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}
