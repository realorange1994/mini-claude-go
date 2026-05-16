package microlisp

import (
	"fmt"
	"math"
	"os"
	"reflect"
)

// -------- FFI --------
var ffiRegistry = map[string]interface{}{
	"math/sin":   math.Sin,
	"math/cos":   math.Cos,
	"math/tan":   math.Tan,
	"math/sqrt":  math.Sqrt,
	"math/abs":   math.Abs,
	"math/floor": math.Floor,
	"math/ceil":  math.Ceil,
	"math/round": math.Round,
	"math/exp":   math.Exp,
	"math/log":   math.Log,
	"math/pow":   math.Pow,
	"os/getenv":  os.Getenv,
	"os/getpid":  os.Getpid,
}

func builtinFFI(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VStr {
		return nil, fmt.Errorf("ffi: need string function name")
	}
	name := args[0].str
	fn, ok := ffiRegistry[name]
	if !ok {
		return nil, fmt.Errorf("ffi: unknown function: %s", name)
	}
	fnVal := reflect.ValueOf(fn)
	// Handle non-function registered values
	if fnVal.Kind() != reflect.Func {
		return reflectToLisp(fnVal), nil
	}
	fnType := fnVal.Type()
	numIn := fnType.NumIn()
	// if variadic...
	callArgs := make([]reflect.Value, 0, len(args)-1)
	for i := 1; i < len(args); i++ {
		callArgs = append(callArgs, lispToReflect(args[i], fnType.In(min(i-1, numIn-1))))
	}
	// Handle variadic
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
