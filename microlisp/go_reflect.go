package microlisp

import (
	"fmt"
	"math/big"
	"reflect"
	"strings"
)

// -------- Go nil / Zero-value Predicates --------

// builtinGoIsNil checks if a VGoVal is a Go nil (nil pointer, nil interface, nil slice, etc.)
// Returns T if the underlying Go value is nil, NIL otherwise.
func builtinGoIsNil(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	v := args[0]

	// Lisp nil
	if v.typ == VNil {
		return vbool(true), nil
	}

	// VGoVal: check the underlying Go value
	if v.typ == VGoVal {
		// Try goValReflect first (more accurate)
		if v.goValReflect.IsValid() {
			rv := v.goValReflect
			switch rv.Kind() {
			case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
				return vbool(rv.IsNil()), nil
			default:
				return vbool(false), nil
			}
		}
		// Fallback: use goVal
		rv := reflect.ValueOf(v.goVal)
		if !rv.IsValid() {
			return vbool(true), nil
		}
		switch rv.Kind() {
		case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
			return vbool(rv.IsNil()), nil
		default:
			return vbool(false), nil
		}
	}

	return vbool(false), nil
}

// builtinGoIsZero checks if a value is the zero value for its type.
// Zero values: 0, 0.0, "", false, nil, empty slice/map, zero struct.
func builtinGoIsZero(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(true), nil
	}
	v := args[0]

	if v.typ == VNil {
		return vbool(true), nil
	}

	if v.typ == VGoVal {
		var rv reflect.Value
		if v.goValReflect.IsValid() {
			rv = v.goValReflect
		} else {
			rv = reflect.ValueOf(v.goVal)
		}
		if !rv.IsValid() {
			return vbool(true), nil
		}
		return vbool(reflect.DeepEqual(rv.Interface(), reflect.Zero(rv.Type()).Interface())), nil
	}

	// Lisp numeric zero
	if v.typ == VNum && v.num == 0 {
		return vbool(true), nil
	}

	// Lisp empty string
	if v.typ == VStr && v.str == "" {
		return vbool(true), nil
	}

	// Lisp empty list
	if v.typ == VPair && isNil(v) {
		return vbool(true), nil
	}

	return vbool(false), nil
}

// -------- Go Type Assertion --------

// builtinGoAssertType performs a Go type assertion (v.(Type)).
// Usage:
//   (go:assert-type val "io.Reader")          ; assert, error on failure
//   (go:assert-type val "io.Reader" :default) ; return :default on failure
func builtinGoAssertType(args []*Value) (*Value, error) {
	if len(args) < 2 || args[0].typ != VGoVal || args[1].typ != VStr {
		return nil, fmt.Errorf("go:assert-type: need a Go value and a type string")
	}
	v := args[0]
	typeSpec := args[1].str
	useDefault := len(args) >= 3

	targetType, err := parseGoType(typeSpec)
	if err != nil {
		return nil, fmt.Errorf("go:assert-type: %w", err)
	}

	var rv reflect.Value
	if v.goValReflect.IsValid() {
		rv = v.goValReflect
	} else {
		rv = reflect.ValueOf(v.goVal)
	}
	if !rv.IsValid() {
		if useDefault {
			return args[2], nil
		}
		return nil, fmt.Errorf("go:assert-type: nil value cannot be asserted as %s", typeSpec)
	}

	// If the type matches exactly
	if rv.Type().AssignableTo(targetType) {
		return &Value{
			typ:          VGoVal,
			goVal:        rv.Interface(),
			goValType:    targetType,
			goValReflect: rv,
		}, nil
	}

	// For interface types: check if the value implements the interface
	if targetType.Kind() == reflect.Interface && rv.Type().Implements(targetType) {
		return &Value{
			typ:          VGoVal,
			goVal:        rv.Interface(),
			goValType:    targetType,
			goValReflect: rv,
		}, nil
	}

	// Try dereferencing pointer for exact match
	if rv.Kind() == reflect.Ptr && !rv.IsNil() {
		elem := rv.Elem()
		if elem.Type().AssignableTo(targetType) {
			return &Value{
				typ:          VGoVal,
				goVal:        elem.Interface(),
				goValType:    targetType,
				goValReflect: elem,
			}, nil
		}
		// Pointer implements interface
		if targetType.Kind() == reflect.Interface && elem.Type().Implements(targetType) {
			return &Value{
				typ:          VGoVal,
				goVal:        elem.Interface(),
				goValType:    targetType,
				goValReflect: elem,
			}, nil
		}
	}

	if useDefault {
		return args[2], nil
	}
	return nil, fmt.Errorf("go:assert-type: %s is not assignable to %s", rv.Type(), typeSpec)
}

// builtinGoImplements checks if a Go value implements a named interface.
// Usage: (go:implements val "io.Reader") => T or NIL
func builtinGoImplements(args []*Value) (*Value, error) {
	if len(args) < 2 || args[0].typ != VGoVal || args[1].typ != VStr {
		return nil, fmt.Errorf("go:implements: need a Go value and an interface type string")
	}
	v := args[0]
	typeSpec := args[1].str

	targetType, err := parseGoType(typeSpec)
	if err != nil {
		return nil, fmt.Errorf("go:implements: %w", err)
	}

	if targetType.Kind() != reflect.Interface {
		return nil, fmt.Errorf("go:implements: %s is not an interface type", typeSpec)
	}

	var rv reflect.Value
	if v.goValReflect.IsValid() {
		rv = v.goValReflect
	} else {
		rv = reflect.ValueOf(v.goVal)
	}
	if !rv.IsValid() || rv.IsNil() {
		return vbool(false), nil
	}

	// Check both the value and its pointer (some interfaces are on *T, not T)
	implements := rv.Type().Implements(targetType)
	if !implements && rv.Kind() != reflect.Ptr {
		ptrType := reflect.PtrTo(rv.Type())
		implements = ptrType.Implements(targetType)
	}

	return vbool(implements), nil
}

// -------- Go Reflection / Introspection --------

// builtinGoFieldsOf returns the field list of a struct type.
// Usage: (go:fields-of "crypto/x509.Certificate") or (go:fields-of val)
// Returns: ((Name Type) (Name Type) ...)
func builtinGoFieldsOf(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("go:fields-of: need a type string or Go value")
	}

	var t reflect.Type

	if args[0].typ == VStr {
		var err error
		t, err = parseGoType(args[0].str)
		if err != nil {
			return nil, fmt.Errorf("go:fields-of: %w", err)
		}
	} else if args[0].typ == VGoVal {
		v := args[0]
		if v.goValReflect.IsValid() {
			rv := v.goValReflect
			if rv.Kind() == reflect.Ptr {
				if rv.IsNil() {
					return vnil(), nil
				}
				t = rv.Elem().Type()
			} else {
				t = rv.Type()
			}
		} else {
			t = reflect.TypeOf(v.goVal)
		}
	} else {
		return nil, fmt.Errorf("go:fields-of: need a string type or Go value")
	}

	// Dereference pointer to get underlying struct
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("go:fields-of: %s is not a struct", t)
	}

	var fields *Value
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		fieldInfo := listFromSlice([]*Value{
			vstr(sf.Name),
			vstr(sf.Type.String()),
		})
		fields = cons(fieldInfo, fields)
	}

	// Reverse the list (we built it backwards)
	return reverseList(fields), nil
}

// builtinGoMethodsOf returns the method list of a Go value.
// Usage: (go:methods-of val) => ((Name Signature) ...)
func builtinGoMethodsOf(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VGoVal {
		return nil, fmt.Errorf("go:methods-of: need a Go value")
	}
	v := args[0]

	var rv reflect.Value
	if v.goValReflect.IsValid() {
		rv = v.goValReflect
	} else {
		rv = reflect.ValueOf(v.goVal)
	}
	if !rv.IsValid() {
		return vnil(), nil
	}

	// For non-pointer structs, also check pointer methods
	var methods *Value
	seen := make(map[string]bool)

	// Methods on the value itself
	for i := 0; i < rv.NumMethod(); i++ {
		mt := rv.Type().Method(i)
		if seen[mt.Name] {
			continue
		}
		seen[mt.Name] = true
		sig := methodSignature(mt.Type)
		entry := listFromSlice([]*Value{vstr(mt.Name), vstr(sig)})
		methods = cons(entry, methods)
	}

	// Methods on the pointer type (if not already a pointer)
	if rv.Kind() != reflect.Ptr {
		ptrType := reflect.PtrTo(rv.Type())
		ptrVal := reflect.New(rv.Type())
		for i := 0; i < ptrVal.NumMethod(); i++ {
			mt := ptrType.Method(i)
			if seen[mt.Name] {
				continue
			}
			seen[mt.Name] = true
			sig := methodSignature(mt.Type)
			entry := listFromSlice([]*Value{vstr(mt.Name), vstr(sig)})
			methods = cons(entry, methods)
		}
	}

	return reverseList(methods), nil
}

// builtinGoFuncOf retrieves a method from a Go value and returns it as
// a callable VGoVal (reflect.Value of type reflect.Func).
// Usage:
//   (let ((fn (go:func-of obj "String")))
//     (go:call fn))           ; call the retrieved method
//   (go:func-of obj "Write")  ; get method by name
func builtinGoFuncOf(args []*Value) (*Value, error) {
	if len(args) < 2 || args[0].typ != VGoVal || args[1].typ != VStr {
		return nil, fmt.Errorf("go:func-of: need a Go value and a method name")
	}
	v := args[0]
	methodName := args[1].str

	var rv reflect.Value
	if v.goValReflect.IsValid() {
		rv = v.goValReflect
	} else {
		rv = reflect.ValueOf(v.goVal)
	}
	if !rv.IsValid() {
		return nil, fmt.Errorf("go:func-of: invalid Go value")
	}

	// Try on the value itself first
	method := rv.MethodByName(methodName)
	if method.IsValid() {
		return &Value{
			typ:          VGoVal,
			goVal:        method.Interface(),
			goValType:    method.Type(),
			goValReflect: method,
		}, nil
	}

	// For struct values, also try on the pointer (some methods are on *T)
	if rv.Kind() == reflect.Struct {
		ptr := reflect.New(rv.Type())
		ptr.Elem().Set(rv)
		method = ptr.MethodByName(methodName)
		if method.IsValid() {
			return &Value{
				typ:          VGoVal,
				goVal:        method.Interface(),
				goValType:    method.Type(),
				goValReflect: method,
			}, nil
		}
	}

	return nil, fmt.Errorf("go:func-of: no method %q on %s", methodName, rv.Type())
}

func methodSignature(fnType reflect.Type) string {
	parts := make([]string, 0, fnType.NumIn()-1)
	for i := 1; i < fnType.NumIn(); i++ {
		parts = append(parts, fnType.In(i).String())
	}
	out := ""
	if fnType.NumOut() == 1 {
		out = fnType.Out(0).String()
	} else if fnType.NumOut() > 1 {
		outs := make([]string, fnType.NumOut())
		for i := 0; i < fnType.NumOut(); i++ {
			outs[i] = fnType.Out(i).String()
		}
		out = "(" + strings.Join(outs, ", ") + ")"
	}
	return fmt.Sprintf("(%s) -> %s", strings.Join(parts, ", "), out)
}

func reverseList(l *Value) *Value {
	var result *Value
	for !isNil(l) {
		if l.typ != VPair {
			result = cons(l, result)
			break
		}
		result = cons(l.car, result)
		l = l.cdr
	}
	return result
}

// builtinGoKindOf returns the Go reflect.Kind of a type or value.
// Usage: (go:kind-of "[]int") => "slice"
//        (go:kind-of val)     => "struct"
func builtinGoKindOf(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("go:kind-of: need a type string or Go value")
	}

	var kind reflect.Kind

	if args[0].typ == VStr {
		t, err := parseGoType(args[0].str)
		if err != nil {
			return nil, fmt.Errorf("go:kind-of: %w", err)
		}
		kind = t.Kind()
	} else if args[0].typ == VGoVal {
		v := args[0]
		var rv reflect.Value
		if v.goValReflect.IsValid() {
			rv = v.goValReflect
		} else {
			rv = reflect.ValueOf(v.goVal)
		}
		if !rv.IsValid() {
			return vstr("invalid"), nil
		}
		kind = rv.Kind()
	} else {
		// For Lisp values, return their Lisp type
		return vstr(typeStr(args[0])), nil
	}

	return vstr(strings.ToLower(kind.String())), nil
}

// builtinGoElemOf returns the element type of a container type.
// Usage: (go:elem-of "[]string") => "string"
//        (go:elem-of "map[string]int") => "string" (key)
//        (go:elem-of "*int") => "int"
func builtinGoElemOf(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VStr {
		return nil, fmt.Errorf("go:elem-of: need a type string")
	}

	t, err := parseGoType(args[0].str)
	if err != nil {
		return nil, fmt.Errorf("go:elem-of: %w", err)
	}

	var elemType reflect.Type
	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		elemType = t.Elem()
	case reflect.Chan:
		elemType = t.Elem()
	case reflect.Ptr:
		elemType = t.Elem()
	case reflect.Map:
		// Return (key value) for maps
		keyType := t.Key()
		valType := t.Elem()
		return listFromSlice([]*Value{vstr(keyType.String()), vstr(valType.String())}), nil
	default:
		return nil, fmt.Errorf("go:elem-of: %s has no element type", kindName(t.Kind()))
	}

	return vstr(elemType.String()), nil
}

// builtinGoLenOf returns the length of a Go slice, string, array, map, or channel.
// Usage: (go:len-of val) => integer
func builtinGoLenOf(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VGoVal {
		return nil, fmt.Errorf("go:len-of: need a Go value")
	}
	v := args[0]

	var rv reflect.Value
	if v.goValReflect.IsValid() {
		rv = v.goValReflect
	} else {
		rv = reflect.ValueOf(v.goVal)
	}
	if !rv.IsValid() {
		return nil, fmt.Errorf("go:len-of: invalid Go value")
	}

	switch rv.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.String:
		return vnum(float64(rv.Len())), nil
	case reflect.Chan:
		return vnum(float64(rv.Len())), nil
	case reflect.Ptr:
		if rv.IsNil() {
			return vnum(0), nil
		}
		elem := rv.Elem()
		if elem.Kind() == reflect.Slice || elem.Kind() == reflect.Array || elem.Kind() == reflect.Map || elem.Kind() == reflect.String {
			return vnum(float64(elem.Len())), nil
		}
		return nil, fmt.Errorf("go:len-of: pointer to %s has no length", elem.Kind())
	default:
		return nil, fmt.Errorf("go:len-of: %s has no length", kindName(rv.Kind()))
	}
}

// builtinGoCapOf returns the capacity of a Go slice or channel.
// Usage: (go:cap-of val) => integer
func builtinGoCapOf(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VGoVal {
		return nil, fmt.Errorf("go:cap-of: need a Go value")
	}
	v := args[0]

	var rv reflect.Value
	if v.goValReflect.IsValid() {
		rv = v.goValReflect
	} else {
		rv = reflect.ValueOf(v.goVal)
	}
	if !rv.IsValid() {
		return nil, fmt.Errorf("go:cap-of: invalid Go value")
	}

	switch rv.Kind() {
	case reflect.Slice, reflect.Chan:
		return vnum(float64(rv.Cap())), nil
	case reflect.Ptr:
		if rv.IsNil() {
			return vnum(0), nil
		}
		elem := rv.Elem()
		if elem.Kind() == reflect.Slice || elem.Kind() == reflect.Chan {
			return vnum(float64(elem.Cap())), nil
		}
		return nil, fmt.Errorf("go:cap-of: pointer to %s has no capacity", elem.Kind())
	default:
		return nil, fmt.Errorf("go:cap-of: %s has no capacity", kindName(rv.Kind()))
	}
}

// -------- Go Type Conversion --------

// builtinGoConvert performs a Go type conversion.
// Usage:
//   (go:convert "hello" "[]byte")    => []byte{104, 101, 108, 108, 111}
//   (go:convert 3.14 "int64")        => 3
//   (go:convert val "string")        => string representation
//   (go:convert "[1 2 3]" "[]int64") => []int64{1, 2, 3}
func builtinGoConvert(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("go:convert: need a value and a target type")
	}
	v := args[0]
	typeSpec := args[1].str

	targetType, err := parseGoType(typeSpec)
	if err != nil {
		return nil, fmt.Errorf("go:convert: %w", err)
	}

	converted, err := convertToReflect(v, targetType)
	if err != nil {
		return nil, fmt.Errorf("go:convert: %w", err)
	}

	return &Value{
		typ:          VGoVal,
		goVal:        converted.Interface(),
		goValType:    targetType,
		goValReflect: converted,
	}, nil
}

func convertToReflect(v *Value, t reflect.Type) (reflect.Value, error) {
	// Direct numeric conversions to integer types
	if isNumeric(v) {
		switch t.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val := int64(toNum(v))
			rv := reflect.New(t).Elem()
			rv.SetInt(val)
			return rv, nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			val := uint64(toNum(v))
			rv := reflect.New(t).Elem()
			rv.SetUint(val)
			return rv, nil
		case reflect.Float32, reflect.Float64:
			val := toNum(v)
			rv := reflect.New(t).Elem()
			rv.SetFloat(val)
			return rv, nil
		}
	}

	// string -> []byte / []rune
	if v.typ == VStr && t.Kind() == reflect.Slice {
		elem := t.Elem()
		if elem.Kind() == reflect.Uint8 {
			return reflect.ValueOf([]byte(v.str)), nil
		}
		if elem.Kind() == reflect.Int32 {
			return reflect.ValueOf([]rune(v.str)), nil
		}
	}

	// []byte / []rune -> string
	if t.Kind() == reflect.String {
		var rv reflect.Value
		if v.typ == VGoVal {
			if v.goValReflect.IsValid() {
				rv = v.goValReflect
			} else {
				rv = reflect.ValueOf(v.goVal)
			}
		}
		if rv.IsValid() {
			switch rv.Kind() {
			case reflect.Slice:
				if rv.Type().Elem().Kind() == reflect.Uint8 {
					return reflect.ValueOf(string(rv.Bytes())), nil
				}
				if rv.Type().Elem().Kind() == reflect.Int32 {
					runes := make([]rune, rv.Len())
					for i := 0; i < rv.Len(); i++ {
						runes[i] = rune(rv.Index(i).Int())
					}
					return reflect.ValueOf(string(runes)), nil
				}
			case reflect.Array:
				if rv.Type().Elem().Kind() == reflect.Uint8 {
					// Convert array to string via slice
					slice := rv.Slice(0, rv.Len())
					return reflect.ValueOf(string(slice.Bytes())), nil
				}
			}
		}
		// Fallback: Lisp value to string
		return reflect.ValueOf(ToString(v)), nil
	}

	// VGoVal with compatible type
	if v.typ == VGoVal {
		var rv reflect.Value
		if v.goValReflect.IsValid() {
			rv = v.goValReflect
		} else {
			rv = reflect.ValueOf(v.goVal)
		}
		if rv.IsValid() && rv.Type().AssignableTo(t) {
			return rv, nil
		}
		// Try conversion: int32 -> int64, etc.
		if rv.Type().ConvertibleTo(t) {
			return rv.Convert(t), nil
		}
	}

	// Lisp list -> Go slice
	if isList(v) && (t.Kind() == reflect.Slice || t.Kind() == reflect.Array) {
		elemType := t.Elem()
		var elems []reflect.Value
		for p := v; !isNil(p); p = p.cdr {
			if p.typ != VPair {
				elem, err := convertToReflect(p, elemType)
				if err != nil {
					return reflect.Value{}, fmt.Errorf("list element conversion: %w", err)
				}
				elems = append(elems, elem)
				break
			}
			elem, err := convertToReflect(p.car, elemType)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("list element conversion: %w", err)
			}
			elems = append(elems, elem)
		}

		if t.Kind() == reflect.Slice {
			slice := reflect.MakeSlice(t, len(elems), len(elems))
			for i, e := range elems {
				slice.Index(i).Set(e)
			}
			return slice, nil
		} else {
			arr := reflect.New(t).Elem()
			for i, e := range elems {
				if i < arr.Len() {
					arr.Index(i).Set(e)
				}
			}
			return arr, nil
		}
	}

	// Lisp string -> *big.Int, *big.Float, etc.
	if v.typ == VStr {
		switch t.String() {
		case "*big.Int":
			bi := new(big.Int)
			if _, ok := bi.SetString(v.str, 10); ok {
				return reflect.ValueOf(bi), nil
			}
			return reflect.Value{}, fmt.Errorf("cannot parse %q as big.Int", v.str)
		case "*big.Float":
			bf, _, err := big.ParseFloat(v.str, 10, 128, big.ToNearestEven)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("cannot parse %q as big.Float: %w", v.str, err)
			}
			return reflect.ValueOf(bf), nil
		}
	}

	// General: try lispToReflectSafe
	rv, err := lispToReflectSafe(v, t)
	if err == nil {
		return rv, nil
	}

	return reflect.Value{}, fmt.Errorf("cannot convert %s to %s", typeStr(v), t)
}

// -------- Go Pointer Arithmetic / uintptr --------

// builtinGoUnsafePointer creates a *uintptr from a Go value's address.
// Usage: (go:uintptr val) => numeric address
// Note: This is for inspection/debugging. Go's GC doesn't track uintptr
// as a pointer, so don't store these for long.
func builtinGoUintptr(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VGoVal {
		return nil, fmt.Errorf("go:uintptr: need a Go value")
	}
	v := args[0]

	var rv reflect.Value
	if v.goValReflect.IsValid() {
		rv = v.goValReflect
	} else {
		rv = reflect.ValueOf(v.goVal)
	}
	if !rv.IsValid() {
		return vnum(0), nil
	}

	if rv.Kind() == reflect.Ptr && !rv.IsNil() {
		return vnum(float64(rv.Pointer())), nil
	}
	if rv.CanAddr() {
		return vnum(float64(rv.Addr().Pointer())), nil
	}
	return vnum(0), nil
}

// -------- Go Type Parse helpers exposed --------

// builtinGoTypeParse parses a Go type string and returns metadata.
// Usage: (go:type-parse "map[string][]int") => (:kind "map" :key "string" :elem "[]int")
func builtinGoTypeParse(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VStr {
		return nil, fmt.Errorf("go:type-parse: need a type string")
	}
	t, err := parseGoType(args[0].str)
	if err != nil {
		return nil, fmt.Errorf("go:type-parse: %w", err)
	}

	result := listFromSlice([]*Value{
		cons(vsym("kind"), vstr(kindName(t.Kind()))),
		cons(vsym("string"), vstr(t.String())),
	})

	switch t.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Array, reflect.Chan:
		result = cons(cons(vsym("elem"), vstr(t.Elem().String())), result)
		if t.Kind() == reflect.Array {
			result = cons(cons(vsym("len"), vnum(float64(t.Len()))), result)
		}
		if t.Kind() == reflect.Chan {
			dir := "both"
			switch t.ChanDir() {
			case reflect.SendDir:
				dir = "send"
			case reflect.RecvDir:
				dir = "recv"
			}
			result = cons(cons(vsym("dir"), vstr(dir)), result)
		}
	case reflect.Map:
		result = cons(cons(vsym("key"), vstr(t.Key().String())), result)
		result = cons(cons(vsym("elem"), vstr(t.Elem().String())), result)
	}

	return result, nil
}

func kindName(k reflect.Kind) string {
	switch k {
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "int"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return "uint"
	case reflect.Float32, reflect.Float64:
		return "float"
	case reflect.Complex64, reflect.Complex128:
		return "complex"
	case reflect.String:
		return "string"
	case reflect.Array:
		return "array"
	case reflect.Chan:
		return "chan"
	case reflect.Func:
		return "func"
	case reflect.Interface:
		return "interface"
	case reflect.Map:
		return "map"
	case reflect.Ptr:
		return "ptr"
	case reflect.Slice:
		return "slice"
	case reflect.Struct:
		return "struct"
	default:
		return k.String()
	}
}

