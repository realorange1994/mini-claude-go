package microlisp

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"syscall"
)

// -------- Go FFI Call Mechanism --------
// These functions provide the Lisp-level interface to GoPackageRegistry.

func init() {
	// Register Go constants that are commonly needed in FFI but not functions
	constants := map[string]map[string]interface{}{
		"os": {
			"ModePerm":     os.ModePerm,
			"ModeDir":      os.ModeDir,
			"ModeAppend":   os.ModeAppend,
			"ModeExclusive": os.ModeExclusive,
			"ModeTemporary": os.ModeTemporary,
			"ModeSymlink":  os.ModeSymlink,
			"ModeDevice":   os.ModeDevice,
			"ModeNamedPipe": os.ModeNamedPipe,
			"ModeSocket":   os.ModeSocket,
			"ModeSetuid":   os.ModeSetuid,
			"ModeSetgid":   os.ModeSetgid,
			"ModeCharDevice": os.ModeCharDevice,
			"ModeSticky":   os.ModeSticky,
			"ModeIrregular": os.ModeIrregular,
			"ModeType":     os.ModeType,
		},
		"syscall": {
			"Stdin":  syscall.Stdin,
			"Stdout": syscall.Stdout,
			"Stderr": syscall.Stderr,
		},
	}
	for pkg, syms := range constants {
		existing, ok := GoPackageRegistry[pkg]
		if !ok {
			existing = make(map[string]reflect.Value)
			GoPackageRegistry[pkg] = existing
		}
		for name, val := range syms {
			if _, exists := existing[name]; !exists {
				existing[name] = reflect.ValueOf(val)
			}
		}
	}
}

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
		// Check if it's a method expression: "Type.Method" (e.g. "time.Time.AddDate")
		typeParts := strings.SplitN(symName, ".", 2)
		if len(typeParts) == 2 {
			typeName, methodName := typeParts[0], typeParts[1]
			if pkgTypes, ok := GoTypeRegistry[pkgName]; ok {
				if t, ok := pkgTypes[typeName]; ok {
					// Create a method expression: func(T, args...) -> result
					// where T is the type (or *T for pointer methods)
					return makeMethodExpr(pkgName, typeName, methodName, t)
				}
			}
		}
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

// makeMethodExpr creates a method expression for a Go type.
// e.g. "time.Time.AddDate" → func(time.Time, int, int, int) time.Time
func makeMethodExpr(pkgName, typeName, methodName string, t reflect.Type) (*Value, error) {
	// Try both value receiver and pointer receiver methods
	var method reflect.Method
	var found bool

	// Try value receiver first
	if m, ok := t.MethodByName(methodName); ok {
		method = m
		found = true
	}
	// Try pointer receiver
	if !found {
		ptrType := reflect.PtrTo(t)
		if m, ok := ptrType.MethodByName(methodName); ok {
			method = m
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("go:import: no method %q on type %s.%s", methodName, pkgName, typeName)
	}

	fullName := pkgName + "." + typeName + "." + methodName
	wrapper := &goFunc{fn: method.Func, name: fullName}
	return &Value{
		typ:  VPrim,
		fn:   makeGoPrim(wrapper),
		name: fullName,
	}, nil
}

// goFunc wraps a reflected Go function
type goFunc struct {
	fn   reflect.Value
	name string
}

// makeGoPrim creates a NativeFunc that calls the wrapped Go function.
func makeGoPrim(wf *goFunc) NativeFunc {
	return func(args []*Value) (ret *Value, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("go:import %s: panic: %v", wf.name, r)
			}
		}()

		fnType := wf.fn.Type()
		numIn := fnType.NumIn()
		isVariadic := fnType.IsVariadic()

		// Validate argument count
		if isVariadic {
			if len(args) < numIn-1 {
				return nil, fmt.Errorf("go:import %s: need at least %d argument(s), got %d", wf.name, numIn-1, len(args))
			}
		} else {
			if len(args) != numIn {
				return nil, fmt.Errorf("go:import %s: need %d argument(s), got %d", wf.name, numIn, len(args))
			}
		}

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
			rv, convErr := lispToReflectSafe(arg, paramType)
			if convErr != nil {
				return nil, fmt.Errorf("go:import %s: arg %d: %w", wf.name, i+1, convErr)
			}
			callArgs = append(callArgs, rv)
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

		// For variadic functions, use CallSlice which properly expands
		// a slice argument into individual variadic arguments.
		// For non-variadic functions, use regular Call.
		var results []reflect.Value
		if isVariadic {
			results = wf.fn.CallSlice(callArgs)
		} else {
			results = wf.fn.Call(callArgs)
		}

		switch len(results) {
		case 0:
			return vnil(), nil
		case 1:
			if isErrorType(results[0]) {
				if results[0].IsNil() {
					return vnil(), nil
				}
				return nil, fmt.Errorf("go:import %s: %v", wf.name, results[0].Interface())
			}
			return reflectToLisp(results[0]), nil
		default:
			lastIdx := len(results) - 1
			if isErrorType(results[lastIdx]) {
				if !results[lastIdx].IsNil() {
					return nil, fmt.Errorf("go:import %s: %v", wf.name, results[lastIdx].Interface())
				}
				// If there's only 1 non-error return value, return it directly (not wrapped in a list)
				if lastIdx == 1 {
					return reflectToLisp(results[0]), nil
				}
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

// isErrorType checks if a reflect.Value implements the error interface.
func isErrorType(v reflect.Value) bool {
	return v.IsValid() && v.Type().Implements(reflect.TypeOf((*error)(nil)).Elem())
}

// lispToReflectSafe converts a Lisp Value to a reflect.Value with error reporting.
func lispToReflectSafe(v *Value, t reflect.Type) (reflect.Value, error) {
	if v == nil || v.typ == VNil {
		return reflect.Zero(t), nil
	}
	// If the Lisp value is a VGoVal, try to use the actual Go value directly.
	if v.typ == VGoVal {
		gv := reflect.ValueOf(v.goVal)
		// For struct values with interface targets, prefer *T over T.
		// Many Go APIs do type switches on pointer types (e.g. x509.MarshalPKIXPublicKey
		// does type switch on *rsa.PublicKey, not rsa.PublicKey), even when the
		// interface is any (crypto.PublicKey = any).
		if gv.Kind() == reflect.Struct && t.Kind() == reflect.Interface {
			ptrType := reflect.PtrTo(gv.Type())
			if ptrType.Implements(t) || t.NumMethod() == 0 {
				newPtr := reflect.New(gv.Type())
				newPtr.Elem().Set(gv)
				return newPtr, nil
			}
		}
		// If the Go value's type is assignable to the target type, use it.
		if gv.Type().AssignableTo(t) {
			return gv, nil
		}

		// Auto-wrap value in pointer when target is *T and value is T
		if t.Kind() == reflect.Ptr && !gv.Type().AssignableTo(t) {
			if gv.Kind() == reflect.Struct {
				if gv.Type().AssignableTo(t.Elem()) {
					newPtr := reflect.New(t.Elem())
					newPtr.Elem().Set(gv)
					return newPtr, nil
				}
			}
			// If stored as pointer (e.g. *rsa.PrivateKey) and target is *X,
			// check if the pointed-to type matches
			if gv.Kind() == reflect.Ptr && gv.Type().Elem().AssignableTo(t.Elem()) {
				return gv, nil
			}
		}
		// Auto-dereference when target is T and value is *T
		if gv.Kind() == reflect.Ptr && !gv.IsNil() && gv.Type().Elem().AssignableTo(t) {
			return gv.Elem(), nil
		}
		// Auto-dereference pointer for interface targets
		if gv.Kind() == reflect.Ptr && !gv.IsNil() {
			ptrElem := gv.Elem()
			if ptrElem.Kind() == reflect.Struct && ptrElem.Type().AssignableTo(t) {
				return ptrElem, nil
			}
		}
		return reflect.Value{}, fmt.Errorf("cannot convert Go value of type %s to %s", gv.Type(), t)
	}
	switch t.Kind() {
	case reflect.Float64:
		if !isNumeric(v) {
			return reflect.Value{}, fmt.Errorf("cannot convert %s to float64", typeStr(v))
		}
		newVal := reflect.New(t).Elem()
		newVal.SetFloat(toNum(v))
		return newVal, nil
	case reflect.Float32:
		if !isNumeric(v) {
			return reflect.Value{}, fmt.Errorf("cannot convert %s to float32", typeStr(v))
		}
		newVal := reflect.New(t).Elem()
		newVal.SetFloat(float64(toNum(v)))
		return newVal, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if !isNumeric(v) {
			return reflect.Value{}, fmt.Errorf("cannot convert %s to %s", typeStr(v), t.Kind())
		}
		n := toNum(v)
		// Create a value of the target type (handles named types like time.Duration)
		newVal := reflect.New(t).Elem()
		newVal.SetInt(int64(n))
		return newVal, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if !isNumeric(v) {
			return reflect.Value{}, fmt.Errorf("cannot convert %s to %s", typeStr(v), t.Kind())
		}
		n := toNum(v)
		if n < 0 {
			return reflect.Value{}, fmt.Errorf("negative value %v cannot convert to %s", n, t.Kind())
		}
		newVal := reflect.New(t).Elem()
		newVal.SetUint(uint64(n))
		return newVal, nil
	case reflect.Bool:
		return reflect.ValueOf(v.typ == VBool && v == globalEnv.bindings["#t"]), nil
	case reflect.String:
		if v.typ == VStr {
			return reflect.ValueOf(v.str), nil
		}
		if v.typ == VChar {
			return reflect.ValueOf(string(v.ch)), nil
		}
		return reflect.Value{}, fmt.Errorf("cannot convert %s to string", typeStr(v))
	case reflect.Slice:
		if v.typ == VStr && t.Elem().Kind() == reflect.Uint8 {
			return reflect.ValueOf([]byte(v.str)), nil
		}
		if isList(v) {
			slice := reflect.MakeSlice(t, 0, 8)
			for p := v; !isNil(p); p = p.cdr {
				elem, err := lispToReflectSafe(p.car, t.Elem())
				if err != nil {
					return reflect.Value{}, fmt.Errorf("list element: %w", err)
				}
				slice = reflect.Append(slice, elem)
			}
			return slice, nil
		}
		return reflect.Value{}, fmt.Errorf("cannot convert %s to %s", typeStr(v), t)
	case reflect.Interface:
		return reflect.ValueOf(lispToInterface(v)), nil
	default:
		return reflect.ValueOf(lispToInterface(v)), nil
	}
	return reflect.Zero(t), nil
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
	case VGoVal:
		return v.goVal
	default:
		return ToString(v)
	}
}

// builtinGoList lists all available Go packages and their symbols.
func builtinGoList(args []*Value) (*Value, error) {
	// If a package name is given, list its symbols (functions + types)
	if len(args) >= 1 && args[0].typ == VStr {
		pkgName := args[0].str
		result := make([]string, 0)

		// List functions from GoPackageRegistry
		if pkg, ok := GoPackageRegistry[pkgName]; ok {
			for k := range pkg {
				result = append(result, pkgName+"."+k)
			}
		}

		// List types from GoTypeRegistry
		if pkgTypes, ok := GoTypeRegistry[pkgName]; ok {
			for k := range pkgTypes {
				result = append(result, pkgName+"."+k)
			}
		}

		if len(result) == 0 {
			return nil, fmt.Errorf("go:list: unknown package \"%s\"", pkgName)
		}

		sort.Strings(result)
		values := make([]*Value, len(result))
		for i, s := range result {
			values[i] = vstr(s)
		}
		return listFromSlice(values), nil
	}

	// List all packages (merge keys from both registries)
	allPkgs := make(map[string]bool)
	for k := range GoPackageRegistry {
		allPkgs[k] = true
	}
	for k := range GoTypeRegistry {
		allPkgs[k] = true
	}
	pkgs := make([]string, 0, len(allPkgs))
	for k := range allPkgs {
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

// vgoval creates a VGoVal Value wrapping a Go interface value.
func vgoval(val interface{}, typ reflect.Type) *Value {
	rv := reflect.ValueOf(val)
	return &Value{
		typ:           VGoVal,
		goVal:         val,
		goValType:     typ,
		goValReflect:  rv,
	}
}

// lazyGoLoaded tracks which Go FFI functions have been loaded (thread-safe).
var lazyGoLoaded sync.Map

// tryLazyGoImport checks GoFFILazyTable for an auto-importable Go function.
// Called when a symbol lookup fails in the environment.
// Returns the imported function (cached in globalEnv) or nil/error.
func tryLazyGoImport(symName string, env *Env) (*Value, error) {
	// Check if this symbol is in the lazy-load table
	goPath, ok := GoFFILazyTable[symName]
	if !ok {
		// Try case-insensitive match (since readtable is UPCASE)
		symLower := strings.ToLower(symName)
		for k, v := range GoFFILazyTable {
			if strings.ToLower(k) == symLower {
				goPath = v
				symName = k // use canonical name
				ok = true
				break
			}
		}
	}
	if !ok {
		return nil, nil
	}

	// Thread-safe: only one goroutine loads this symbol
	if _, loaded := lazyGoLoaded.LoadOrStore(symName, true); loaded {
		// Another goroutine is loading or already loaded.
		// Re-try env.Get in case it was registered.
		val, err := env.Get(symName)
		if err == nil {
			return val, nil
		}
		// If the other goroutine hasn't finished registering yet,
		// fall through and do the import ourselves (idempotent).
	}

	// Parse "pkg.Func" from goPath
	parts := strings.SplitN(goPath, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("lazy-import: invalid entry %q=%q", symName, goPath)
	}

	// Look up in GoPackageRegistry
	pkgName, symGoName := parts[0], parts[1]
	pkg, pkgOk := GoPackageRegistry[pkgName]
	if !pkgOk {
		return nil, fmt.Errorf("lazy-import: unknown package %q", pkgName)
	}
	fnVal, fnOk := pkg[symGoName]
	if !fnOk {
		return nil, fmt.Errorf("lazy-import: unknown symbol %q in package %q", symGoName, pkgName)
	}

	if fnVal.Kind() != reflect.Func {
		// Not a function — register as a variable
		goVal := vgoval(fnVal.Interface(), fnVal.Type())
		globalEnv.Set(symName, goVal)
		return goVal, nil
	}

	// Wrap as callable VPrim
	wrapper := &goFunc{fn: fnVal, name: goPath}
	fn := &Value{
		typ:  VPrim,
		fn:   makeGoPrim(wrapper),
		name: goPath,
	}
	// Cache in globalEnv for future lookups
	globalEnv.Set(symName, fn)
	return fn, nil
}
