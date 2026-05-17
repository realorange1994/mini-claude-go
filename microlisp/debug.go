package microlisp

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

func builtinUntrace(args []*Value) (*Value, error) {
	if len(args) < 1 {
		names := make([]string, 0, len(traceTable))
		for name := range traceTable {
			names = append(names, name)
		}
		traceTable = make(map[string]bool)
		result := make([]*Value, len(names))
		for i, n := range names {
			result[i] = vsym(n)
		}
		return listFromSlice(result), nil
	}
	var result []*Value
	for _, arg := range args {
		v := primaryValue(arg)
		name := ""
		switch v.typ {
		case VFunc, VPrim:
			name = v.name
		case VSym:
			name = v.str
		}
		if name != "" {
			delete(traceTable, name)
		}
		result = append(result, vsym(name))
	}
	return listFromSlice(result), nil
}

func builtinTrace(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("trace: need function name")
	}
	var result []*Value
	for _, arg := range args {
		v := primaryValue(arg)
		name := ""
		switch v.typ {
		case VFunc, VPrim:
			name = v.name
		case VSym:
			name = v.str
		}
		if name != "" {
			traceTable[name] = true
			result = append(result, vsym(name))
		}
	}
	return listFromSlice(result), nil
}

func builtinDisassemble(args []*Value) (*Value, error) {
	// (disassemble fn) -- returns a string describing the function
	if len(args) < 1 {
		return nil, fmt.Errorf("disassemble: need a function")
	}
	fn := primaryValue(args[0])
	var sb strings.Builder
	switch fn.typ {
	case VNil:
		sb.WriteString("#<NULL>\n")
	case VPrim:
		sb.WriteString("; This is a built-in function.\n")
		sb.WriteString("; Disassembly not available for primitives.\n")
	case VFunc:
		sb.WriteString("; Lambda Expression:\n")
		if fn.name != "" {
			sb.WriteString("; Name: ")
			sb.WriteString(fn.name)
			sb.WriteString("\n")
		}
		sb.WriteString("; Parameters: (")
		for i, p := range fn.params {
			if i > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(p)
		}
		if fn.rest != "" {
			if len(fn.params) > 0 {
				sb.WriteString(" &rest ")
			} else {
				sb.WriteString("&rest ")
			}
			sb.WriteString(fn.rest)
		}
		sb.WriteString(")\n")
		sb.WriteString("; Environment: (closure)\n")
		sb.WriteString("; Compiled: No (interpreted)\n")
	case VMacro:
		sb.WriteString("; This is a macro.\n")
		sb.WriteString("; Name: ")
		sb.WriteString(fn.name)
		sb.WriteString("\n")
	case VInstance:
		if fn.instClass != nil {
			sb.WriteString("; Instance of class: ")
			sb.WriteString(fn.instClass.str)
			sb.WriteString("\n")
		}
	default:
		sb.WriteString("; Not a function.\n")
	}
	fmt.Print(sb.String())
	return vnil(), nil
}

func builtinCompile(args []*Value) (*Value, error) {
	// (compile name &optional definition)
	// In SBCL, compile returns (values compiled-fn warnings-p failure-p)
	// For MicroLisp, we return the function as-is (it's already interpreted)
	var name *Value
	var def *Value
	if len(args) >= 1 {
		name = args[0]
	}
	if len(args) >= 2 {
		def = args[1]
	}
	if name != nil && name.typ == VNil {
		name = nil
	}
	if name != nil && name.typ == VSym && def != nil {
		// Compile a lambda definition
		if def.typ == VPair && def.car != nil && def.car.typ == VSym && def.car.str == "LAMBDA" {
			// It's a lambda, eval it and return
			result, err := Eval(def, globalEnv)
			if err != nil {
				return vnil(), nil
			}
			// Set the function binding
			if name.typ == VSym {
				globalEnv.Set(name.str, result)
			}
			// Return (values result nil nil)
			return multiVal(result, vnil(), vnil()), nil
		}
	}
	if name != nil && name.typ == VSym {
		// Look up existing function
		val, err := globalEnv.Get(name.str)
		if err == nil {
			// Return (values val nil nil)
			return multiVal(val, vnil(), vnil()), nil
		}
	}
	if def != nil && def.typ == VPair && def.car != nil && def.car.typ == VSym && def.car.str == "LAMBDA" {
		result, err := Eval(def, globalEnv)
		if err != nil {
			return vnil(), nil
		}
		return multiVal(result, vnil(), vnil()), nil
	}
	return vnil(), nil
}

func builtinRoom(args []*Value) (*Value, error) {
	var verbose bool
	if len(args) > 0 {
		verbose = !isNil(args[0])
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Fprintf(os.Stderr, "Dynamic Space Usage: %d bytes allocated, %d bytes total\n", m.Alloc, m.TotalAlloc)
	if verbose {
		fmt.Fprintf(os.Stderr, "  HeapSys: %d bytes\n", m.HeapSys)
		fmt.Fprintf(os.Stderr, "  HeapAlloc: %d bytes\n", m.HeapAlloc)
		fmt.Fprintf(os.Stderr, "  HeapIdle: %d bytes\n", m.HeapIdle)
		fmt.Fprintf(os.Stderr, "  HeapReleased: %d bytes\n", m.HeapReleased)
		fmt.Fprintf(os.Stderr, "  NumGC: %d\n", m.NumGC)
	}
	return vnil(), nil
}
