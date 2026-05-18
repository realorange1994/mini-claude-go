package microlisp

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// -------- Go FFI Call Mechanism --------
// These functions provide the Lisp-level interface to GoPackageRegistry.

// builtinGoImport implements (go:import "package.FuncName").
// Returns a callable Lisp function that wraps the Go function.
func builtinGoImport(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VStr {
		return nil, fmt.Errorf("go:import: need a string like \"math.Sin\" or \"fmt.Printf\"")
	}
	name := args[0].str

	// Parse "package.Func" format
	parts := strings.SplitN(name, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("go:import: invalid format \"%s\", use \"package.Func\"", name)
	}
	pkgName, symName := parts[0], parts[1]

	pkg, ok := GoPackageRegistry[pkgName]
	if !ok {
		// List available packages
		pkgs := make([]string, 0, len(GoPackageRegistry))
		for k := range GoPackageRegistry {
			pkgs = append(pkgs, k)
		}
		sort.Strings(pkgs)
		return nil, fmt.Errorf("go:import: unknown package \"%s\". Available packages: %v", pkgName, pkgs)
	}

	fnVal, ok := pkg[symName]
	if !ok {
		// List available symbols in package
		syms := make([]string, 0, len(pkg))
		for k := range pkg {
			syms = append(syms, k)
		}
		sort.Strings(syms)
		return nil, fmt.Errorf("go:import: unknown symbol \"%s\" in package \"%s\". Available: %v", symName, pkgName, syms)
	}

	if fnVal.Kind() != reflect.Func {
		return reflectToLisp(fnVal), nil
	}

	// Wrap as a VPrim that calls the Go function via reflect
	wrapper := &goFunc{fn: fnVal, name: name}
	return &Value{
		typ:  VPrim,
		fn:   makeGoPrim(wrapper),
		name: name,
	}, nil
}

// goFunc wraps a reflected Go function
type goFunc struct {
	fn   reflect.Value
	name string
}

// makeGoPrim creates a NativeFunc that calls the wrapped Go function.
func makeGoPrim(wf *goFunc) NativeFunc {
	return func(args []*Value) (*Value, error) {
		fnType := wf.fn.Type()
		numIn := fnType.NumIn()
		isVariadic := fnType.IsVariadic()

		callArgs := make([]reflect.Value, 0, len(args))
		for i, arg := range args {
			var paramType reflect.Type
			if isVariadic && i >= numIn-1 {
				paramType = fnType.In(numIn - 1).Elem()
			} else if i < numIn {
				paramType = fnType.In(i)
			} else {
				return nil, fmt.Errorf("go:import %s: too many arguments", wf.name)
			}
			callArgs = append(callArgs, lispToReflectCustom(arg, paramType))
		}

		// Handle variadic: collapse trailing args into a slice
		if isVariadic && len(args) >= numIn-1 {
			fixedCount := numIn - 1
			if len(args) > fixedCount {
				variadicType := fnType.In(fixedCount)
				slice := reflect.MakeSlice(variadicType, len(args)-fixedCount, len(args)-fixedCount)
				for i := fixedCount; i < len(callArgs); i++ {
					slice.Index(i - fixedCount).Set(callArgs[i])
				}
				callArgs = append(callArgs[:fixedCount], slice)
			} else if len(args) == fixedCount {
				variadicType := fnType.In(fixedCount)
				callArgs = append(callArgs, reflect.Zero(variadicType))
			}
		}

		results := wf.fn.Call(callArgs)

		switch len(results) {
		case 0:
			return vnil(), nil
		case 1:
			if results[0].Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) {
				if results[0].IsNil() {
					return vnil(), nil
				}
				return nil, fmt.Errorf("go:import %s: %v", wf.name, results[0].Interface())
			}
			return reflectToLisp(results[0]), nil
		default:
			// Check if last result is error
			lastIdx := len(results) - 1
			if results[lastIdx].Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) {
				if !results[lastIdx].IsNil() {
					return nil, fmt.Errorf("go:import %s: %v", wf.name, results[lastIdx].Interface())
				}
				// Return remaining results
				lispResults := make([]*Value, lastIdx)
				for i := 0; i < lastIdx; i++ {
					lispResults[i] = reflectToLisp(results[i])
				}
				return listFromSlice(lispResults), nil
			}
			lispResults := make([]*Value, len(results))
			for i, r := range results {
				lispResults[i] = reflectToLisp(r)
			}
			return listFromSlice(lispResults), nil
		}
	}
}

// lispToReflectCustom converts a Lisp Value to a reflect.Value of the given type.
func lispToReflectCustom(v *Value, t reflect.Type) reflect.Value {
	if v == nil || v.typ == VNil {
		return reflect.Zero(t)
	}
	switch t.Kind() {
	case reflect.Float64:
		return reflect.ValueOf(toNum(v))
	case reflect.Float32:
		return reflect.ValueOf(float32(toNum(v)))
	case reflect.Int:
		return reflect.ValueOf(int(toNum(v)))
	case reflect.Int8:
		return reflect.ValueOf(int8(toNum(v)))
	case reflect.Int16:
		return reflect.ValueOf(int16(toNum(v)))
	case reflect.Int32:
		return reflect.ValueOf(int32(toNum(v)))
	case reflect.Int64:
		return reflect.ValueOf(int64(toNum(v)))
	case reflect.Uint:
		return reflect.ValueOf(uint(toNum(v)))
	case reflect.Uint8:
		return reflect.ValueOf(uint8(toNum(v)))
	case reflect.Uint16:
		return reflect.ValueOf(uint16(toNum(v)))
	case reflect.Uint32:
		return reflect.ValueOf(uint32(toNum(v)))
	case reflect.Uint64:
		return reflect.ValueOf(uint64(toNum(v)))
	case reflect.Bool:
		return reflect.ValueOf(v.typ == VBool || v == globalEnv.bindings["#t"])
	case reflect.String:
		if v.typ == VStr {
			return reflect.ValueOf(v.str)
		}
		if v.typ == VChar {
			return reflect.ValueOf(string(v.ch))
		}
		return reflect.ValueOf(ToString(v))
	case reflect.Slice:
		if v.typ == VStr && t.Elem().Kind() == reflect.Uint8 {
			return reflect.ValueOf([]byte(v.str))
		}
		if isList(v) {
			slice := reflect.MakeSlice(t, 0, 8)
			for p := v; !isNil(p); p = p.cdr {
				elem := lispToReflectCustom(p.car, t.Elem())
				slice = reflect.Append(slice, elem)
			}
			return slice
		}
		return reflect.Zero(t)
	case reflect.Interface:
		return reflect.ValueOf(lispToInterface(v))
	default:
		return reflect.ValueOf(lispToInterface(v))
	}
}

// isList checks if a value is a proper list
func isList(v *Value) bool {
	seen := make(map[*Value]bool)
	for p := v; p != nil; p = p.cdr {
		if seen[p] {
			return false
		}
		seen[p] = true
		if p.typ == VNil {
			return true
		}
		if p.typ != VPair {
			return false
		}
	}
	return false
}

// lispToInterface converts a Lisp Value to a natural Go interface{} value.
func lispToInterface(v *Value) interface{} {
	switch v.typ {
	case VNum:
		if v.isFloat {
			return v.num
		}
		if v.num == float64(int64(v.num)) {
			return int64(v.num)
		}
		return v.num
	case VRat:
		return float64(v.irat) / float64(v.iden)
	case VBigInt:
		if v.bigInt != nil {
			return v.bigInt.String()
		}
		return int64(0)
	case VComplex:
		return complex(v.num, v.imag)
	case VStr:
		return v.str
	case VChar:
		return string(v.ch)
	case VBool:
		return v == globalEnv.bindings["#t"]
	case VPair:
		result := make([]interface{}, 0)
		for p := v; !isNil(p); p = p.cdr {
			result = append(result, lispToInterface(p.car))
		}
		return result
	default:
		return ToString(v)
	}
}

// builtinGoList lists all available Go packages and their symbols.
func builtinGoList(args []*Value) (*Value, error) {
	// If a package name is given, list its symbols
	if len(args) >= 1 && args[0].typ == VStr {
		pkgName := args[0].str
		pkg, ok := GoPackageRegistry[pkgName]
		if !ok {
			return nil, fmt.Errorf("go:list: unknown package \"%s\"", pkgName)
		}
		keys := make([]string, 0, len(pkg))
		for k := range pkg {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		result := make([]*Value, len(keys))
		for i, k := range keys {
			result[i] = vstr(pkgName + "." + k)
		}
		return listFromSlice(result), nil
	}

	// List all packages
	pkgs := make([]string, 0, len(GoPackageRegistry))
	for k := range GoPackageRegistry {
		pkgs = append(pkgs, k)
	}
	sort.Strings(pkgs)
	result := make([]*Value, len(pkgs))
	for i, k := range pkgs {
		result[i] = vstr(k)
	}
	return listFromSlice(result), nil
}

// builtinGoRegister allows registering a Go value from Lisp side.
func builtinGoRegister(args []*Value) (*Value, error) {
	if len(args) < 2 || args[0].typ != VStr {
		return nil, fmt.Errorf("go:register: need a string name and a value")
	}
	name := args[0].str

	parts := strings.SplitN(name, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("go:register: invalid format, use \"package.Func\"")
	}
	pkgName, symName := parts[0], parts[1]

	var val interface{}
	switch args[1].typ {
	case VNum:
		val = args[1].num
	case VStr:
		val = args[1].str
	case VBool:
		val = args[1] == globalEnv.bindings["#t"]
	default:
		return nil, fmt.Errorf("go:register: unsupported value type %s", typeStr(args[1]))
	}

	pkg, ok := GoPackageRegistry[pkgName]
	if !ok {
		pkg = make(map[string]reflect.Value)
		GoPackageRegistry[pkgName] = pkg
	}
	pkg[symName] = reflect.ValueOf(val)

	return vstr(name), nil
}