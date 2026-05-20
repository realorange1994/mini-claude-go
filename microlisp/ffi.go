package microlisp

import (
	"fmt"
	"reflect"
	"strings"
)

// -------- FFI --------
// The old ffiRegistry is kept for backward compatibility with ffi-register,
// but (ffi "pkg.Func") now delegates to GoPackageRegistry (same as go:import).

var ffiRegistry = map[string]interface{}{}

func builtinFFI(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VStr {
		return nil, fmt.Errorf("ffi: need string function name")
	}
	name := args[0].str

	// First try the old ffiRegistry for backward compat
	if fn, ok := ffiRegistry[name]; ok {
		fnVal := reflect.ValueOf(fn)
		if fnVal.Kind() != reflect.Func {
			return reflectToLisp(fnVal), nil
		}
		fnType := fnVal.Type()
		numIn := fnType.NumIn()
		callArgs := make([]reflect.Value, 0, len(args)-1)
		for i := 1; i < len(args); i++ {
			callArgs = append(callArgs, lispToReflect(args[i], fnType.In(min(i-1, numIn-1))))
		}
		if fnType.IsVariadic() {
			fixedArgs := callArgs
			if len(fixedArgs) > numIn-1 {
				fixedArgs = callArgs[:numIn-1]
				varArgs := callArgs[numIn-1:]
				fixedArgs = append(fixedArgs, varArgs...)
				callArgs = fixedArgs
			}
		}
		results := fnVal.Call(callArgs)
		if len(results) == 0 {
			return vnil(), nil
		}
		return reflectToLisp(results[0]), nil
	}

	// Convert "pkg/func" format to "pkg.Func" for GoPackageRegistry
	goName := name
	if strings.Contains(name, "/") && !strings.Contains(name, ".") {
		// "crypto/rand/Reader" -> find the package and symbol
		// Try splitting at last /
		lastSlash := strings.LastIndex(name, "/")
		pkgPart := name[:lastSlash]
		symPart := name[lastSlash+1:]
		goName = pkgPart + "." + symPart
	}

	// Delegate to go:import logic
	parts := strings.SplitN(goName, ".", 2)
	if len(parts) == 2 {
		pkgName, symName := parts[0], parts[1]
		if pkg, ok := GoPackageRegistry[pkgName]; ok {
			if fnVal, ok := pkg[symName]; ok {
				if fnVal.Kind() == reflect.Func {
					// If extra arguments are provided, call the function directly
					if len(args) > 1 {
						wrapper := &goFunc{fn: fnVal, name: goName}
						callArgs := make([]*Value, len(args)-1)
						copy(callArgs, args[1:])
						return makeGoPrim(wrapper)(callArgs)
					}
					wrapper := &goFunc{fn: fnVal, name: goName}
					return &Value{
						typ:  VPrim,
						fn:   makeGoPrim(wrapper),
						name: goName,
					}, nil
				}
				return reflectToLisp(fnVal), nil
			}
		}
	}

	return nil, fmt.Errorf("ffi: unknown function: %s", name)
}

func builtinFFIRegister(args []*Value) (*Value, error) {
	if len(args) < 2 || args[0].typ != VStr {
		return nil, fmt.Errorf("ffi-register: need string name and value")
	}
	name := args[0].str
	// We can only register basic types here from Lisp
	// For Go functions, use the Go side
	if args[1].typ == VNum {
		ffiRegistry[name] = float64(toNum(args[1]))
	} else if args[1].typ == VStr {
		ffiRegistry[name] = args[1].str
	} else {
		return nil, fmt.Errorf("ffi-register: unsupported value type")
	}
	return vsym(name), nil
}
