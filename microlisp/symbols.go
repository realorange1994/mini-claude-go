package microlisp

import (
	"fmt"
	"strings"
)

func builtinSymbolValue(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("symbol-value: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("symbol-value: not a symbol")
	}
	val, err := globalEnv.Get(sym.str)
	if err != nil {
		return vnil(), nil
	}
	return val, nil
}

func builtinMacroFunction(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("macro-function: need a symbol or setter form")
	}
	// Setter form: (macro-function (macro-function sym) new-fn)
	if args[0].typ == VPair && args[0].car != nil && args[0].car.typ == VSym && args[0].car.str == "MACRO-FUNCTION" {
		// args[0] = (MACRO-FUNCTION sym), args[1] = new-fn
		accessorArgs := args[0].cdr
		if accessorArgs == nil || accessorArgs.typ != VPair {
			return nil, fmt.Errorf("macro-function setf: malformed")
		}
		sym := accessorArgs.car
		// Evaluate (quote sym) form if present
		if sym.typ == VPair && sym.car != nil && sym.car.typ == VSym && sym.car.str == "QUOTE" {
			sym = sym.cdr.car
		}
		if sym.typ != VSym {
			return nil, fmt.Errorf("macro-function setf: symbol required")
		}
		if len(args) < 2 {
			return nil, fmt.Errorf("macro-function setf: need new value")
		}
		return builtinMacroFunctionSetf([]*Value{args[1], sym})
	}
	// Getter: (macro-function sym)
	sym := args[0]
	// Evaluate (quote sym) form if present
	if sym.typ == VPair && sym.car != nil && sym.car.typ == VSym && sym.car.str == "QUOTE" {
		sym = sym.cdr.car
	}
	if sym.typ != VSym {
		return nil, fmt.Errorf("macro-function: not a symbol")
	}
	val, err := globalEnv.Get(sym.str)
	if err != nil || val == nil || val.typ != VMacro {
		return vnil(), nil
	}
	return val, nil
}

func builtinMacroFunctionSetf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("setf (macro-function): need value and symbol")
	}
	newFn := args[0]
	sym := args[1]
	if sym.typ != VSym {
		return nil, fmt.Errorf("setf (macro-function): symbol required")
	}
	// Use expandMacro's &whole mechanism to pass the whole form (macro-name . args).
	// Set m.whole = "#FORM" so expandMacro binds the whole form to #FORM.
	// Then the body is (fn-ref #FORM #ENV) where #FORM = whole form and #ENV = nil.
	var closureForm *Value
	if newFn.typ == VSym {
		closureForm = list(newFn, vsym("#FORM"), vsym("#ENV"))
	} else {
		closureForm = list(newFn, vsym("#FORM"), vsym("#ENV"))
	}
	m := gcv()
	m.typ = VMacro
	m.str = sym.str
	m.params = nil // no macro lambda-list params
	m.rest = ""
	m.whole = "#FORM" // expandMacro binds whole form to #FORM
	m.body = closureForm
	if newFn.typ == VFunc || newFn.typ == VMacro {
		m.env = newFn.env
	} else {
		m.env = globalEnv
	}
	globalEnv.Set(sym.str, m)
	return newFn, nil
}

func builtinCompilerMacroFunction(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("compiler-macro-function: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("compiler-macro-function: not a symbol")
	}
	cm, ok := compilerMacros[sym.str]
	if !ok || cm == nil {
		return vnil(), nil
	}
	return cm, nil
}

func builtinCompilerMacroFunctionSetf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("setf (compiler-macro-function): need value and symbol")
	}
	newFn := args[0]
	sym := args[1]
	if sym.typ != VSym {
		return nil, fmt.Errorf("setf (compiler-macro-function): symbol required")
	}
	if isNil(newFn) {
		delete(compilerMacros, sym.str)
	} else {
		m := gcv()
		m.typ = VMacro
		m.str = sym.str
		m.params = nil
		m.rest = ""
		m.whole = ""
		m.body = list(newFn, vsym("#FORM"), vsym("#ENV"))
		m.env = globalEnv
		compilerMacros[sym.str] = m
	}
	return newFn, nil
}

func builtinSetClassPrintFn(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-class-print-fn: need class-name and function")
	}
	name := args[0]
	printFn := args[1]
	if name.typ != VSym {
		return nil, fmt.Errorf("set-class-print-fn: class-name must be a symbol")
	}
	if printFn.typ != VFunc && printFn.typ != VPrim {
		return nil, fmt.Errorf("set-class-print-fn: function must be callable")
	}
	structPrintFns[strings.ToUpper(name.str)] = printFn
	return vnil(), nil
}

func builtinRemoveClassPrintFn(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("remove-class-print-fn: need class-name")
	}
	name := args[0]
	if name.typ != VSym {
		return nil, fmt.Errorf("remove-class-print-fn: class-name must be a symbol")
	}
	delete(structPrintFns, strings.ToUpper(name.str))
	return vnil(), nil
}

func builtinSymbolFunction(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("symbol-function: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("symbol-function: not a symbol")
	}
	val, err := globalEnv.Get(sym.str)
	if err != nil {
		return nil, fmt.Errorf("symbol-function: %s has no function", sym.str)
	}
	if val.typ == VPrim || val.typ == VFunc || val.typ == VGeneric || val.typ == VMacro {
		return val, nil
	}
	return nil, fmt.Errorf("symbol-function: %s is not a function", sym.str)
}

func builtinSymbolPlist(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("symbol-plist: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("symbol-plist: not a symbol")
	}
	if sym.plist == nil {
		return vnil(), nil
	}
	return sym.plist, nil
}

func builtinSymbolPlistSetf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("symbol-plist-setf: need value and symbol")
	}
	newVal := args[0]
	sym := args[1]
	if sym.typ != VSym {
		return nil, fmt.Errorf("symbol-plist-setf: need a symbol")
	}
	sym.plist = newVal
	return newVal, nil
}

func builtinBoundp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("boundp: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("boundp: not a symbol")
	}
	_, err := globalEnv.Get(sym.str)
	if err == nil {
		return vsym("T"), nil
	}
	return vnil(), nil
}

func builtinFboundp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("fboundp: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("fboundp: not a symbol")
	}
	val, err := globalEnv.Get(sym.str)
	if err != nil {
		return vnil(), nil
	}
	if val.typ == VPrim || val.typ == VFunc || val.typ == VGeneric || val.typ == VMacro {
		return vsym("T"), nil
	}
	return vnil(), nil
}

var specialOperators = map[string]bool{
	"QUOTE":               true,
	"IF":                  true,
	"LAMBDA":              true,
	"PROGN":               true,
	"PROGV":               true,
	"THE":                 true,
	"FLET":                true,
	"LABELS":              true,
	"LET":                 true,
	"LET*":                true,
	"BLOCK":               true,
	"RETURN-FROM":         true,
	"TAGBODY":             true,
	"GO":                  true,
	"CATCH":               true,
	"THROW":               true,
	"MACROLET":            true,
	"MACRO-FUNCTION":      true,
	"SETF":                true,
	"SETQ":                true,
	"FUNCTION":            true,
	"MULTIPLE-VALUE-CALL": true,
}

func builtinSpecialOperatorP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("special-operator-p: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("special-operator-p: not a symbol")
	}
	return vbool(specialOperators[sym.str]), nil
}

func builtinFunctionP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("functionp: need an argument")
	}
	return vbool(args[0].typ == VPrim || args[0].typ == VFunc || args[0].typ == VGeneric), nil
}

func builtinMakeGenericFunction(args []*Value) (*Value, error) {
	gf := gcv()
	gf.typ = VGeneric
	if len(args) > 0 && args[0].typ == VSym {
		gf.str = args[0].str
	} else if len(args) > 0 && args[0].typ == VStr {
		gf.str = args[0].str
	}
	return gf, nil
}

func builtinEnsureGenericFunction(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("ensure-generic-function: need a function name")
	}
	name := args[0]
	var nameStr string
	if name.typ == VSym {
		nameStr = name.str
	} else if name.typ == VStr {
		nameStr = name.str
	} else {
		return nil, fmt.Errorf("ensure-generic-function: function name must be a symbol or string")
	}
	// Check if it already exists as a generic function
	existing, err := globalEnv.Get(nameStr)
	if err == nil && existing != nil && existing.typ == VGeneric {
		return existing, nil
	}
	// Create a new generic function
	gf := gcv()
	gf.typ = VGeneric
	gf.str = nameStr
	// Parse optional keyword arguments
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":LAMBDA-LIST" && i+1 < len(args) {
			// Store lambda-list info (simplified: just skip)
			i++
		} else if args[i].typ == VSym && args[i].str == ":DOCUMENTATION" && i+1 < len(args) {
			i++
		} else if args[i].typ == VSym && args[i].str == ":METHOD-COMBINATION" && i+1 < len(args) {
			if args[i+1].typ == VSym {
				gf.methodCombo = args[i+1].str
			}
			i++
		}
	}
	globalEnv.Set(nameStr, gf)
	return gf, nil
}

func builtinMakunbound(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("makunbound: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("makunbound: not a symbol")
	}
	delete(globalEnv.bindings, sym.str)
	return sym, nil
}

func builtinFmakunbound(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("fmakunbound: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("fmakunbound: not a symbol")
	}
	delete(globalEnv.bindings, sym.str)
	return sym, nil
}
func globalEnvGetFunction(name string) *Value {
	fn, _ := globalEnv.Get(name)
	if fn != nil && (fn.typ == VFunc || fn.typ == VPrim || fn.typ == VMacro) {
		return fn
	}
	return nil
}

func builtinFdefinition(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("fdefinition: need a function name")
	}
	name := args[0]
	if name.typ == VSym {
		fn := globalEnvGetFunction(name.str)
		if fn != nil {
			return fn, nil
		}
		return nil, fmt.Errorf("fdefinition: %s is not a function", name.str)
	}
	return nil, fmt.Errorf("fdefinition: expected a symbol")
}

func builtinDefinedP(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VSym {
		return vbool(false), nil
	}
	_, err := globalEnv.Get(args[0].str)
	return vbool(err == nil), nil
}

func builtinSymbolValueSetf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("setf (symbol-value): need value and symbol")
	}
	val := args[0]
	sym := args[1]
	if sym.typ != VSym {
		return nil, fmt.Errorf("setf (symbol-value): second argument must be a symbol")
	}
	if _, err := globalEnv.Get(sym.str); err != nil {
		return nil, fmt.Errorf("setf (symbol-value): symbol %s is unbound", sym.str)
	}
	globalEnv.Set(sym.str, val)
	return val, nil
}
