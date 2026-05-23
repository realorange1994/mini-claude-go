package microlisp

import (
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"
)

// -------- Go Struct Access --------
// Provides Lisp-level access to Go struct types registered in GoTypeRegistry.

// builtinGoNew creates a zero-value instance of a registered Go type.
// (go:new "pkg.TypeName") => VGoVal containing the zero value.
func builtinGoNew(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VStr {
		return nil, fmt.Errorf("go:new: need a string like \"crypto/x509.Certificate\"")
	}
	name := args[0].str

	// Parse "pkg.Type" — could be "crypto/x509/Certificate" or "crypto/x509.Certificate"
	// Standard format: "crypto/x509.Certificate" (dot separates package from type name)
	parts := strings.SplitN(name, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("go:new: invalid format %q, use \"package.Type\"", name)
	}
	pkgName, typeName := parts[0], parts[1]

	pkgTypes, ok := GoTypeRegistry[pkgName]
	if !ok {
		return nil, fmt.Errorf("go:new: unknown package %q", pkgName)
	}
	t, ok := pkgTypes[typeName]
	if !ok {
		keys := make([]string, 0, len(pkgTypes))
		for k := range pkgTypes {
			keys = append(keys, k)
		}
		return nil, fmt.Errorf("go:new: unknown type %q in %q. Available: %v", typeName, pkgName, keys)
	}

	// Store the pointer (not the value) so the struct remains settable.
	// reflect.New(t) returns *T. We store the reflect.Value directly in goValReflect
	// so settableness is preserved for go:set-field.
	rv := reflect.New(t)
	v := &Value{
		typ:        VGoVal,
		goVal:      rv.Interface(),
		goValType:  t,
		goValReflect: rv,
	}
	return v, nil
}

// builtinGoField reads a struct field from a VGoVal.
// (go:field obj "FieldName") => field value.
func builtinGoField(args []*Value) (*Value, error) {
	if len(args) < 2 || args[0].typ != VGoVal || args[1].typ != VStr {
		return nil, fmt.Errorf("go:field: need a Go value and a field name")
	}
	obj := args[0]
	fieldName := args[1].str

	// Use goValReflect if available (preserves settableness from reflect.New)
	var rv reflect.Value
	if obj.goValReflect.IsValid() {
		rv = obj.goValReflect
	} else {
		rv = reflect.ValueOf(obj.goVal)
	}
	if !rv.IsValid() {
		return nil, fmt.Errorf("go:field: invalid Go value")
	}

	// Dereference pointer to get the underlying struct
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil, fmt.Errorf("go:field: nil pointer")
		}
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Struct {
		return nil, fmt.Errorf("go:field: value is %s, not a struct", rv.Kind())
	}

	field := rv.FieldByName(fieldName)
	if !field.IsValid() {
		return nil, fmt.Errorf("go:field: no field %q on %s", fieldName, rv.Type())
	}

	// For struct fields from a settable parent, preserve the settable reflect.Value
	// so nested struct fields can be modified via go:set-field.
	// For primitive types, convert to Lisp values directly.
	switch field.Kind() {
	case reflect.Struct, reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map:
		return &Value{
			typ:          VGoVal,
			goVal:        field.Interface(),
			goValType:    field.Type(),
			goValReflect: field,
		}, nil
	default:
		return reflectToLisp(field), nil
	}
}

// builtinGoSetField sets a struct field on a VGoVal.
// (go:set-field obj "FieldName" value) => nil
func builtinGoSetField(args []*Value) (*Value, error) {
	if len(args) < 3 || args[0].typ != VGoVal || args[1].typ != VStr {
		return nil, fmt.Errorf("go:set-field: need a Go value, a field name, and a value")
	}
	obj := args[0]
	fieldName := args[1].str
	lispVal := args[2]

	// Use goValReflect if available (preserves settableness from reflect.New)
	var rv reflect.Value
	if obj.goValReflect.IsValid() {
		rv = obj.goValReflect
	} else {
		rv = reflect.ValueOf(obj.goVal)
	}
	if !rv.IsValid() {
		return nil, fmt.Errorf("go:set-field: invalid Go value")
	}

	// Dereference pointer to get the underlying struct
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil, fmt.Errorf("go:set-field: nil pointer")
		}
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Struct {
		return nil, fmt.Errorf("go:set-field: value is %s, not a struct", rv.Kind())
	}

	field := rv.FieldByName(fieldName)
	if !field.IsValid() {
		return nil, fmt.Errorf("go:set-field: no field %q on %s", fieldName, rv.Type())
	}

	if !field.CanSet() {
		return nil, fmt.Errorf("go:set-field: field %q is not settable", fieldName)
	}

	// Special handling for pointer-to-struct fields
	if field.Kind() == reflect.Ptr && field.Type().Elem().Kind() == reflect.Struct && !isNil(lispVal) {
		elemType := field.Type().Elem()
		newPtr := reflect.New(elemType)
		// Try to set fields if input is VGoVal with struct
		if lispVal.typ == VGoVal {
			srcVal := reflect.ValueOf(lispVal.goVal)
			if srcVal.Kind() == reflect.Ptr {
				srcVal = srcVal.Elem()
			}
			if srcVal.Kind() == reflect.Struct && srcVal.Type().AssignableTo(elemType) {
				newPtr.Elem().Set(srcVal)
			}
			// If struct types don't match exactly, try field-by-field copy
			if srcVal.Kind() == reflect.Struct && !srcVal.Type().AssignableTo(elemType) {
				srcName := srcVal.Type().Name()
				elemName := elemType.Name()
				if srcName == elemName ||
					strings.HasSuffix(srcVal.Type().String(), elemName) ||
					strings.HasSuffix(elemType.String(), srcName) {
					for i := 0; i < srcVal.NumField(); i++ {
						sf := srcVal.Type().Field(i)
						if f, ok := elemType.FieldByName(sf.Name); ok && f.Type.AssignableTo(sf.Type) {
							newPtr.Elem().FieldByName(sf.Name).Set(srcVal.Field(i))
						}
					}
				}
			}
		}
		// For *big.Int: accept numeric input
		if elemType.String() == "big.Int" && isNumeric(lispVal) {
			newPtr.Elem().Set(reflect.ValueOf(*new(big.Int).SetInt64(int64(toNum(lispVal)))))
		}
		field.Set(newPtr)
		return vnil(), nil
	}
	// For nil assignment to pointer fields
	if field.Kind() == reflect.Ptr && isNil(lispVal) {
		field.Set(reflect.Zero(field.Type()))
		return vnil(), nil
	}
	// For type aliases (e.g. x509.KeyUsage is really int):
	// if the underlying type is assignable, create a value of the named type
	if lispVal.typ == VNum && isNumeric(lispVal) {
		switch field.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if field.Type().ConvertibleTo(reflect.TypeOf(int64(0))) {
				val := int64(toNum(lispVal))
				newVal := reflect.New(field.Type()).Elem()
				switch field.Kind() {
				case reflect.Int:    newVal.SetInt(val)
				case reflect.Int8:   newVal.SetInt(val)
				case reflect.Int16:  newVal.SetInt(val)
				case reflect.Int32:  newVal.SetInt(val)
				case reflect.Int64:  newVal.SetInt(val)
				}
				field.Set(newVal)
				return vnil(), nil
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			val := uint64(toNum(lispVal))
			newVal := reflect.New(field.Type()).Elem()
			switch field.Kind() {
			case reflect.Uint:    newVal.SetUint(val)
			case reflect.Uint8:   newVal.SetUint(val)
			case reflect.Uint16:  newVal.SetUint(val)
			case reflect.Uint32:  newVal.SetUint(val)
			case reflect.Uint64:  newVal.SetUint(val)
			}
			field.Set(newVal)
			return vnil(), nil
		}
	}
	// For slices: convert Lisp list to Go slice
	if field.Kind() == reflect.Slice && isList(lispVal) {
		elemType := field.Type().Elem()
		slice := reflect.MakeSlice(field.Type(), 0, 8)
		for p := lispVal; !isNil(p); p = p.cdr {
			elem, err := lispToReflectSafe(p.car, elemType)
			if err != nil {
				return nil, fmt.Errorf("go:set-field: field %q list element: %w", fieldName, err)
			}
			slice = reflect.Append(slice, elem)
		}
		field.Set(slice)
		return vnil(), nil
	}

	reflectVal, err := lispToReflectSafe(lispVal, field.Type())
	if err != nil {
		return nil, fmt.Errorf("go:set-field: field %q: %w", fieldName, err)
	}

	field.Set(reflectVal)
	return vnil(), nil
}

// builtinGoTypeOf returns the Go type name of a VGoVal.
// (go:type-of obj) => type name string or nil.
func builtinGoTypeOf(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("go:type-of: need a value")
	}
	v := args[0]
	if v.typ != VGoVal {
		return nil, fmt.Errorf("go:type-of: expected a Go value, got %s", typeStr(v))
	}
	// Use goValReflect if available (more accurate type info)
	var rv reflect.Value
	if v.goValReflect.IsValid() {
		rv = v.goValReflect
	} else {
		rv = reflect.ValueOf(v.goVal)
	}
	if !rv.IsValid() {
		return vnil(), nil
	}
	typeStr := rv.Type().String()
	return vstr(typeStr), nil
}

// builtinGoCall calls a named method on a Go value.
// (go:call obj "MethodName" args...) => return value(s).
func builtinGoCall(args []*Value) (*Value, error) {
	if len(args) < 2 || args[0].typ != VGoVal || args[1].typ != VStr {
		return nil, fmt.Errorf("go:call: need a Go value, method name, and optional arguments")
	}
	obj := args[0]
	methodName := args[1].str

	var rv reflect.Value
	if obj.goValReflect.IsValid() {
		rv = obj.goValReflect
	} else {
		rv = reflect.ValueOf(obj.goVal)
	}
	if !rv.IsValid() {
		return nil, fmt.Errorf("go:call: invalid Go value")
	}

	// For pointer values, dereference to get the struct/method set
	origRv := rv
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil, fmt.Errorf("go:call: nil pointer")
		}
		rv = rv.Elem()
	}

	method := rv.MethodByName(methodName)
	if !method.IsValid() {
		// Try on the original value (some methods are on pointer receiver)
		method = origRv.MethodByName(methodName)
		if !method.IsValid() {
			return nil, fmt.Errorf("go:call: no method %q on %s", methodName, rv.Type())
		}
	}

	fnType := method.Type()
	numParams := fnType.NumIn()
	isVariadic := fnType.IsVariadic()
	callArgs := make([]reflect.Value, 0, len(args)-2)
	for i, arg := range args[2:] {
		var paramType reflect.Type
		if isVariadic && i >= numParams-1 {
			// Variadic: all extra args get the slice element type
			paramType = fnType.In(numParams - 1).Elem()
		} else if i < numParams {
			paramType = fnType.In(i)
		} else {
			return nil, fmt.Errorf("go:call: method %s takes %d params, got %d", methodName, numParams, len(args)-2)
		}
		rvArg, err := lispToReflectSafe(arg, paramType)
		if err != nil {
			return nil, fmt.Errorf("go:call: method %s arg %d: %w", methodName, i+1, err)
		}
		callArgs = append(callArgs, rvArg)
	}

	// Handle variadic: collapse trailing args into a slice
	if isVariadic && len(args)-2 > numParams-1 {
		fixedCount := numParams - 1
		variadicType := fnType.In(fixedCount)
		slice := reflect.MakeSlice(variadicType, len(callArgs)-fixedCount, len(callArgs)-fixedCount)
		for j := fixedCount; j < len(callArgs); j++ {
			slice.Index(j - fixedCount).Set(callArgs[j])
		}
		callArgs = append(callArgs[:fixedCount], slice)
	} else if isVariadic && len(args)-2 == numParams-1 {
		// No variadic args, append zero slice
		callArgs = append(callArgs, reflect.Zero(fnType.In(numParams-1)))
	}

	results := method.Call(callArgs)
	if len(results) == 0 {
		return vnil(), nil
	}
	if len(results) == 1 {
		return reflectToLisp(results[0]), nil
	}
	lispResults := make([]*Value, len(results))
	for i, r := range results {
		lispResults[i] = reflectToLisp(r)
	}
	return listFromSlice(lispResults), nil
}

// -------- Go Type Parsing and make --------
// parseGoType parses a Go type string like "[]int", "map[string]int", "chan T", "*T", or "pkg.Type".
func parseGoType(spec string) (reflect.Type, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("empty type specifier")
	}

	// Built-in types
	switch spec {
	case "bool":
		return reflect.TypeOf(true), nil
	case "int":
		return reflect.TypeOf(int(0)), nil
	case "int8":
		return reflect.TypeOf(int8(0)), nil
	case "int16":
		return reflect.TypeOf(int16(0)), nil
	case "int32":
		return reflect.TypeOf(int32(0)), nil
	case "int64":
		return reflect.TypeOf(int64(0)), nil
	case "uint":
		return reflect.TypeOf(uint(0)), nil
	case "uint8":
		return reflect.TypeOf(uint8(0)), nil
	case "uint16":
		return reflect.TypeOf(uint16(0)), nil
	case "uint32":
		return reflect.TypeOf(uint32(0)), nil
	case "uint64":
		return reflect.TypeOf(uint64(0)), nil
	case "uintptr":
		return reflect.TypeOf(uintptr(0)), nil
	case "float32":
		return reflect.TypeOf(float32(0)), nil
	case "float64":
		return reflect.TypeOf(float64(0)), nil
	case "complex64":
		return reflect.TypeOf(complex64(0)), nil
	case "complex128":
		return reflect.TypeOf(complex128(0)), nil
	case "string":
		return reflect.TypeOf(""), nil
	case "byte":
		return reflect.TypeOf(byte(0)), nil
	case "rune":
		return reflect.TypeOf(rune(0)), nil
	}

	// *T — pointer
	if strings.HasPrefix(spec, "*") {
		elem, err := parseGoType(spec[1:])
		if err != nil {
			return nil, fmt.Errorf("invalid pointer element type: %w", err)
		}
		return reflect.PtrTo(elem), nil
	}

	// []T — slice
	if strings.HasPrefix(spec, "[]") {
		elem, err := parseGoType(spec[2:])
		if err != nil {
			return nil, fmt.Errorf("invalid slice element type: %w", err)
		}
		return reflect.SliceOf(elem), nil
	}

	// chan<- T — send-only channel
	if strings.HasPrefix(spec, "chan<- ") {
		elem, err := parseGoType(spec[7:])
		if err != nil {
			return nil, fmt.Errorf("invalid chan<- element type: %w", err)
		}
		return reflect.ChanOf(reflect.SendDir, elem), nil
	}

	// <-chan T — receive-only channel
	if strings.HasPrefix(spec, "<-chan ") {
		elem, err := parseGoType(spec[7:])
		if err != nil {
			return nil, fmt.Errorf("invalid <-chan element type: %w", err)
		}
		return reflect.ChanOf(reflect.RecvDir, elem), nil
	}

	// chan T — bidirectional channel
	if strings.HasPrefix(spec, "chan ") {
		elem, err := parseGoType(spec[5:])
		if err != nil {
			return nil, fmt.Errorf("invalid chan element type: %w", err)
		}
		return reflect.ChanOf(reflect.BothDir, elem), nil
	}

	// map[K]V — map
	if strings.HasPrefix(spec, "map[") {
		// Find matching ']' for key
		depth := 0
		keyEnd := -1
		for i := 4; i < len(spec); i++ {
			switch spec[i] {
			case '[':
				depth++
			case ']':
				if depth == 0 {
					keyEnd = i
					break
				}
				depth--
			}
		}
		if keyEnd < 0 {
			return nil, fmt.Errorf("invalid map type: missing ']'")
		}
		keySpec := spec[4:keyEnd]
		valSpec := spec[keyEnd+1:]
		keyType, err := parseGoType(keySpec)
		if err != nil {
			return nil, fmt.Errorf("invalid map key type: %w", err)
		}
		valType, err := parseGoType(valSpec)
		if err != nil {
			return nil, fmt.Errorf("invalid map value type: %w", err)
		}
		return reflect.MapOf(keyType, valType), nil
	}

	// array[T] — fixed-size array (not common in FFI but parseable)
	if strings.HasPrefix(spec, "[") && !strings.HasPrefix(spec, "[]") {
		// find ']' for array size
		closeIdx := strings.Index(spec, "]")
		if closeIdx < 2 {
			return nil, fmt.Errorf("invalid array type")
		}
		sizeStr := spec[1:closeIdx]
		elemSpec := spec[closeIdx+1:]
		elemType, err := parseGoType(elemSpec)
		if err != nil {
			return nil, fmt.Errorf("invalid array element type: %w", err)
		}
		n, err := strconv.Atoi(sizeStr)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("invalid array size: %s", sizeStr)
		}
		return reflect.ArrayOf(n, elemType), nil
	}

	// pkg.Type — look up in GoTypeRegistry
	parts := strings.SplitN(spec, ".", 2)
	if len(parts) == 2 {
		pkgName, typeName := parts[0], parts[1]
		if pkgTypes, ok := GoTypeRegistry[pkgName]; ok {
			if t, ok := pkgTypes[typeName]; ok {
				return t, nil
			}
		}
	}

	return nil, fmt.Errorf("unknown type: %q", spec)
}

// builtinGoMake creates a properly initialized Go value.
// For slices: (go:make "[]int" size &optional capacity)
// For maps:   (go:make "map[string]int")
// For chans:  (go:make "chan int" buffer-size)
// For others: behaves like go:new (creates zero-value)
func builtinGoMake(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VStr {
		return nil, fmt.Errorf("go:make: need a string type specifier")
	}
	name := args[0].str

	t, err := parseGoType(name)
	if err != nil {
		return nil, fmt.Errorf("go:make: %v", err)
	}

	var rv reflect.Value

	switch t.Kind() {
	case reflect.Slice:
		size, cap := 0, 8
		if len(args) >= 2 {
			if !isNumeric(args[1]) {
				return nil, fmt.Errorf("go:make: slice size must be numeric")
			}
			size = int(toNum(args[1]))
			if size < 0 {
				return nil, fmt.Errorf("go:make: negative slice size")
			}
		}
		if len(args) >= 3 {
			if !isNumeric(args[2]) {
				return nil, fmt.Errorf("go:make: slice capacity must be numeric")
			}
			cap = int(toNum(args[2]))
			if cap < size {
				cap = size
			}
		}
		rv = reflect.MakeSlice(t, size, cap)

	case reflect.Map:
		rv = reflect.MakeMap(t)

	case reflect.Chan:
		buf := 0
		if len(args) >= 2 {
			if !isNumeric(args[1]) {
				return nil, fmt.Errorf("go:make: channel buffer must be numeric")
			}
			buf = int(toNum(args[1]))
			if buf < 0 {
				buf = 0
			}
		}
		rv = reflect.MakeChan(t, buf)

	default:
		// Structs, interfaces, named types: same as go:new
		rv = reflect.New(t)
	}

	return &Value{
		typ:          VGoVal,
		goVal:        rv.Interface(),
		goValType:    t,
		goValReflect: rv,
	}, nil
}
