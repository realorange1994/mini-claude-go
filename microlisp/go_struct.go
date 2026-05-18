package microlisp

import (
	"fmt"
	"math/big"
	"reflect"
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

	return reflectToLisp(field), nil
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

	// Special handling for pointer-to-struct and pointer-to-int (like *big.Int)
	if field.Kind() == reflect.Ptr && field.Type().Elem().Kind() == reflect.Struct && !isNil(lispVal) {
		// For *big.Int, *x509.Certificate, etc.: create new instance
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
		}
		// For numeric input to *big.Int
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
	callArgs := make([]reflect.Value, 0, len(args)-2)
	for i, arg := range args[2:] {
		paramIdx := i // method receiver is index 0, so first arg is index 1
		paramType := fnType.In(paramIdx + 1)
		rvArg, err := lispToReflectSafe(arg, paramType)
		if err != nil {
			return nil, fmt.Errorf("go:call: method %s arg %d: %w", methodName, i+1, err)
		}
		callArgs = append(callArgs, rvArg)
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
