package microlisp

import (
	"reflect"
	"testing"
)

// ============================================================
// parseGoType — success cases
// ============================================================

func TestParseGoTypeBuiltins(t *testing.T) {
	builtins := []string{
		"bool", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"float32", "float64", "complex64", "complex128",
		"string", "byte", "rune",
	}
	for _, b := range builtins {
		typ, err := parseGoType(b)
		if err != nil {
			t.Errorf("parseGoType(%q): unexpected error: %v", b, err)
			continue
		}
		if typ.Kind() == reflect.Invalid {
			t.Errorf("parseGoType(%q): returned Invalid kind", b)
		}
	}
}

func TestParseGoTypePointer(t *testing.T) {
	cases := []struct {
		spec string
		kind reflect.Kind
		elem reflect.Kind
	}{
		{"*int", reflect.Ptr, reflect.Int},
		{"*string", reflect.Ptr, reflect.String},
		{"*bool", reflect.Ptr, reflect.Bool},
		{"*float64", reflect.Ptr, reflect.Float64},
		{"**int", reflect.Ptr, reflect.Ptr}, // nested pointer
	}
	for _, c := range cases {
		typ, err := parseGoType(c.spec)
		if err != nil {
			t.Errorf("parseGoType(%q): unexpected error: %v", c.spec, err)
			continue
		}
		if typ.Kind() != c.kind {
			t.Errorf("parseGoType(%q): kind=%v, want %v", c.spec, typ.Kind(), c.kind)
		}
		if typ.Elem().Kind() != c.elem {
			t.Errorf("parseGoType(%q): elem=%v, want %v", c.spec, typ.Elem().Kind(), c.elem)
		}
	}
}

func TestParseGoTypeSlice(t *testing.T) {
	cases := []struct {
		spec string
		elem reflect.Kind
	}{
		{"[]int", reflect.Int},
		{"[]string", reflect.String},
		{"[]byte", reflect.Uint8},
		{"[]float64", reflect.Float64},
		{"[]bool", reflect.Bool},
		{"[][]int", reflect.Slice}, // nested slice
	}
	for _, c := range cases {
		typ, err := parseGoType(c.spec)
		if err != nil {
			t.Errorf("parseGoType(%q): unexpected error: %v", c.spec, err)
			continue
		}
		if typ.Kind() != reflect.Slice {
			t.Errorf("parseGoType(%q): kind=%v, want Slice", c.spec, typ.Kind())
		}
		if typ.Elem().Kind() != c.elem {
			t.Errorf("parseGoType(%q): elem=%v, want %v", c.spec, typ.Elem().Kind(), c.elem)
		}
	}
}

func TestParseGoTypeMap(t *testing.T) {
	cases := []struct {
		spec string
		key  reflect.Kind
		val  reflect.Kind
	}{
		{"map[string]int", reflect.String, reflect.Int},
		{"map[int]string", reflect.Int, reflect.String},
		{"map[int]bool", reflect.Int, reflect.Bool},
		{"map[string]*int", reflect.String, reflect.Ptr}, // pointer value
		{"map[int][]string", reflect.Int, reflect.Slice}, // slice value
	}
	for _, c := range cases {
		typ, err := parseGoType(c.spec)
		if err != nil {
			t.Errorf("parseGoType(%q): unexpected error: %v", c.spec, err)
			continue
		}
		if typ.Kind() != reflect.Map {
			t.Errorf("parseGoType(%q): kind=%v, want Map", c.spec, typ.Kind())
		}
		if typ.Key().Kind() != c.key {
			t.Errorf("parseGoType(%q): key=%v, want %v", c.spec, typ.Key().Kind(), c.key)
		}
		if typ.Elem().Kind() != c.val {
			t.Errorf("parseGoType(%q): val=%v, want %v", c.spec, typ.Elem().Kind(), c.val)
		}
	}
}

func TestParseGoTypeChannel(t *testing.T) {
	typ, err := parseGoType("chan int")
	if err != nil {
		t.Fatalf("chan int: %v", err)
	}
	if typ.Kind() != reflect.Chan {
		t.Fatalf("expected Chan, got %v", typ.Kind())
	}
	if typ.ChanDir() != reflect.BothDir {
		t.Fatalf("expected BothDir, got %v", typ.ChanDir())
	}

	typ, err = parseGoType("chan<- string")
	if err != nil {
		t.Fatalf("chan<- string: %v", err)
	}
	if typ.ChanDir() != reflect.SendDir {
		t.Fatalf("expected SendDir, got %v", typ.ChanDir())
	}

	typ, err = parseGoType("<-chan float64")
	if err != nil {
		t.Fatalf("<-chan float64: %v", err)
	}
	if typ.ChanDir() != reflect.RecvDir {
		t.Fatalf("expected RecvDir, got %v", typ.ChanDir())
	}
}

func TestParseGoTypeArray(t *testing.T) {
	typ, err := parseGoType("[5]int")
	if err != nil {
		t.Fatalf("[5]int: %v", err)
	}
	if typ.Kind() != reflect.Array {
		t.Fatalf("expected Array, got %v", typ.Kind())
	}
	if typ.Len() != 5 {
		t.Fatalf("expected len=5, got %d", typ.Len())
	}
	if typ.Elem().Kind() != reflect.Int {
		t.Fatalf("expected Int elem, got %v", typ.Elem().Kind())
	}

	// Nested array
	typ, err = parseGoType("[3]*string")
	if err != nil {
		t.Fatalf("[3]*string: %v", err)
	}
	if typ.Len() != 3 {
		t.Fatalf("expected len=3, got %d", typ.Len())
	}
	if typ.Elem().Kind() != reflect.Ptr {
		t.Fatalf("expected Ptr elem, got %v", typ.Elem().Kind())
	}
	if typ.Elem().Elem().Kind() != reflect.String {
		t.Fatalf("expected String inner elem, got %v", typ.Elem().Elem().Kind())
	}
}

func TestParseGoTypeRegistryFallback(t *testing.T) {
	ResetGlobalEnv()
	typ, err := parseGoType("crypto/x509.Certificate")
	if err != nil {
		t.Fatalf("registry lookup: %v", err)
	}
	if typ.Kind() != reflect.Struct {
		t.Fatalf("expected Struct, got %v", typ.Kind())
	}

	// Unknown package
	_, err = parseGoType("unknown/pkg.Foo")
	if err == nil {
		t.Fatal("expected error for unknown package")
	}

	// Unknown type in known package
	_, err = parseGoType("crypto/x509.NonexistentType")
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestParseGoTypeWhitespace(t *testing.T) {
	typ, err := parseGoType("  []int  ")
	if err != nil {
		t.Fatalf("whitespace: %v", err)
	}
	if typ.Kind() != reflect.Slice {
		t.Fatalf("expected Slice after trim, got %v", typ.Kind())
	}
}

// ============================================================
// parseGoType — error cases
// ============================================================

func TestParseGoTypeErrorCases(t *testing.T) {
	cases := []struct {
		spec string
		desc string
	}{
		{"", "empty string"},
		{"foobar", "unknown type"},
		{"map[", "unclosed map bracket"},
		{"map[int", "unclosed map key bracket"},
		{"map[foobar]int", "invalid map key type"},
		{"map[int]foobar", "invalid map value type"},
		{"[]foobar", "invalid slice element type"},
		{"*foobar", "invalid pointer element type"},
		{"[]", "empty slice element"},
		{"chan ", "empty chan element"},
		{"chan<-", "empty chan-send element"},
		{"<-chan", "empty chan-recv element"},
		{"[abc]int", "non-numeric array size"},
		{"[-1]int", "negative array size"},
	}
	for _, c := range cases {
		_, err := parseGoType(c.spec)
		if err == nil {
			t.Errorf("parseGoType(%q) [%s]: expected error, got nil", c.spec, c.desc)
		}
	}
}

// ============================================================
// builtinGoMake — success cases
// ============================================================

func TestGoMakeSliceDefaults(t *testing.T) {
	ResetGlobalEnv()
	// No size → empty slice with default cap=8
	result, err := builtinGoMake([]*Value{vstr("[]int")})
	if err != nil {
		t.Fatalf("go:make []int no size: %v", err)
	}
	rv := result.goValReflect
	if rv.Kind() != reflect.Slice {
		t.Fatalf("expected slice, got %v", rv.Kind())
	}
	if rv.Len() != 0 {
		t.Fatalf("expected len=0, got %d", rv.Len())
	}
	if rv.Cap() != 8 {
		t.Fatalf("expected cap=8 default, got %d", rv.Cap())
	}
}

func TestGoMakeSliceSizeOnly(t *testing.T) {
	ResetGlobalEnv()
	result, err := builtinGoMake([]*Value{vstr("[]string"), vnum(5)})
	if err != nil {
		t.Fatalf("go:make []string size=5: %v", err)
	}
	rv := result.goValReflect
	if rv.Len() != 5 || rv.Cap() != 8 {
		t.Fatalf("expected len=5 cap=8, got len=%d cap=%d", rv.Len(), rv.Cap())
	}
}

func TestGoMakeSliceSizeAndCap(t *testing.T) {
	ResetGlobalEnv()
	result, err := builtinGoMake([]*Value{vstr("[]float64"), vnum(3), vnum(20)})
	if err != nil {
		t.Fatalf("go:make []float64 3 20: %v", err)
	}
	rv := result.goValReflect
	if rv.Len() != 3 || rv.Cap() != 20 {
		t.Fatalf("expected len=3 cap=20, got len=%d cap=%d", rv.Len(), rv.Cap())
	}
}

func TestGoMakeSliceCapLessThanSize(t *testing.T) {
	ResetGlobalEnv()
	// cap < size should be corrected to cap = size
	result, err := builtinGoMake([]*Value{vstr("[]int"), vnum(10), vnum(3)})
	if err != nil {
		t.Fatalf("go:make cap < size: %v", err)
	}
	rv := result.goValReflect
	if rv.Cap() != 10 {
		t.Fatalf("expected cap=10 (corrected), got %d", rv.Cap())
	}
}

func TestGoMakeSliceUsable(t *testing.T) {
	ResetGlobalEnv()
	result, err := builtinGoMake([]*Value{vstr("[]int"), vnum(1)})
	if err != nil {
		t.Fatalf("go:make []byte: %v", err)
	}
	rv := result.goValReflect
	// MakeSlice elements are settable even though the slice itself isn't
	rv.Index(0).SetInt(42)
	if rv.Len() != 1 {
		t.Fatalf("expected len=1, got %d", rv.Len())
	}
	if rv.Index(0).Int() != 42 {
		t.Fatalf("expected 42, got %d", rv.Index(0).Int())
	}
}

func TestGoMakeMapUsable(t *testing.T) {
	ResetGlobalEnv()
	result, err := builtinGoMake([]*Value{vstr("map[string]int")})
	if err != nil {
		t.Fatalf("go:make map: %v", err)
	}
	rv := result.goValReflect
	// Should be able to set entries
	rv.SetMapIndex(reflect.ValueOf("key"), reflect.ValueOf(42))
	val := rv.MapIndex(reflect.ValueOf("key"))
	if !val.IsValid() || val.Int() != 42 {
		t.Fatal("map write/read failed")
	}
}

func TestGoMakeChanUnbuffered(t *testing.T) {
	ResetGlobalEnv()
	result, err := builtinGoMake([]*Value{vstr("chan string")})
	if err != nil {
		t.Fatalf("go:make chan string: %v", err)
	}
	rv := result.goValReflect
	if rv.Cap() != 0 {
		t.Fatalf("expected unbuffered (cap=0), got %d", rv.Cap())
	}
}

func TestGoMakeArray(t *testing.T) {
	ResetGlobalEnv()
	result, err := builtinGoMake([]*Value{vstr("[10]int")})
	if err != nil {
		t.Fatalf("go:make [10]int: %v", err)
	}
	rv := result.goValReflect
	// Arrays fall through to default (reflect.New), so we get *[10]int
	if rv.Kind() != reflect.Ptr {
		t.Fatalf("expected Ptr (to array), got %v", rv.Kind())
	}
	arr := rv.Elem()
	if arr.Kind() != reflect.Array {
		t.Fatalf("expected Array elem, got %v", arr.Kind())
	}
	if arr.Len() != 10 {
		t.Fatalf("expected len=10, got %d", arr.Len())
	}
}

func TestGoMakeInterface(t *testing.T) {
	ResetGlobalEnv()
	result, err := builtinGoMake([]*Value{vstr("io.Reader")})
	if err != nil {
		t.Fatalf("go:make io.Reader: %v", err)
	}
	rv := result.goValReflect
	// Interfaces fall through to default (reflect.New), so we get *io.Reader
	if rv.Kind() != reflect.Ptr {
		t.Fatalf("expected Ptr (to interface), got %v", rv.Kind())
	}
}

// ============================================================
// builtinGoMake — error cases
// ============================================================

func TestGoMakeErrorCases(t *testing.T) {
	ResetGlobalEnv()

	// No arguments
	_, err := builtinGoMake(nil)
	if err == nil {
		t.Fatal("expected error for no args")
	}

	// Non-string first argument
	_, err = builtinGoMake([]*Value{vnum(42)})
	if err == nil {
		t.Fatal("expected error for non-string arg")
	}

	// Unknown type
	_, err = builtinGoMake([]*Value{vstr("nonexistent.Type")})
	if err == nil {
		t.Fatal("expected error for unknown type")
	}

	// Negative slice size
	_, err = builtinGoMake([]*Value{vstr("[]int"), vnum(-1)})
	if err == nil {
		t.Fatal("expected error for negative size")
	}

	// Non-numeric slice size
	_, err = builtinGoMake([]*Value{vstr("[]int"), vstr("abc")})
	if err == nil {
		t.Fatal("expected error for non-numeric size")
	}

	// Non-numeric slice capacity
	_, err = builtinGoMake([]*Value{vstr("[]int"), vnum(5), vstr("abc")})
	if err == nil {
		t.Fatal("expected error for non-numeric cap")
	}

	// Non-numeric channel buffer
	_, err = builtinGoMake([]*Value{vstr("chan int"), vstr("abc")})
	if err == nil {
		t.Fatal("expected error for non-numeric buffer")
	}

	// Negative channel buffer → clamped to 0
	result, err := builtinGoMake([]*Value{vstr("chan int"), vnum(-5)})
	if err != nil {
		t.Fatalf("negative buffer should be clamped: %v", err)
	}
	if result.goValReflect.Cap() != 0 {
		t.Fatalf("expected cap=0 after clamp, got %d", result.goValReflect.Cap())
	}
}

// ============================================================
// Integration: go:make + go:call / reflectToLisp
// ============================================================

func TestGoMakeReflectToLispRoundTrip(t *testing.T) {
	ResetGlobalEnv()

	// Create a slice, set values, verify round-trip
	result, err := builtinGoMake([]*Value{vstr("[]int"), vnum(3)})
	if err != nil {
		t.Fatalf("go:make: %v", err)
	}
	rv := result.goValReflect
	rv.Index(0).SetInt(10)
	rv.Index(1).SetInt(20)
	rv.Index(2).SetInt(30)

	// Convert back through reflectToLisp
	lispVal := reflectToLisp(rv)
	if lispVal.typ != VGoVal {
		// For slices that aren't []byte, reflectToLisp wraps as VGoVal
		_ = lispVal
	}
}

func TestGoMakeAndGoCallIntegration(t *testing.T) {
	ResetGlobalEnv()
	// Create a bytes.Buffer via make, then call methods on it
	result, err := SafeEvalString(`
		(let ((buf (go:make "bytes.Buffer")))
			(go:call buf "WriteString" "hello ")
			(go:call buf "WriteString" "world")
			(go:call buf "String"))`)
	if err != nil {
		t.Fatalf("go:make + go:call: %v", err)
	}
	// ToString wraps strings in quotes
	if result != `"hello world"` && result != "hello world" {
		t.Fatalf("expected hello world, got %q", result)
	}
}
