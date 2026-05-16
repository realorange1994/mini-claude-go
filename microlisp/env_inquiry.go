package microlisp

import (
	"fmt"
	"math"
	"strings"
)

// constantp returns true if the form is a constant at compile time.
// Constants are: numbers, characters, strings, symbols with constant values,
// and lists whose car is a special operator like quote.
func builtinConstantp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("constantp: need a form")
	}
	form := args[0]
	if isConstant(form) {
		return vbool(true), nil
	}
	return vbool(false), nil
}

func isConstant(v *Value) bool {
	if v == nil {
		return false
	}
	// Numbers, characters, strings are self-evaluating constants
	if v.typ == VNum || v.typ == VChar || v.typ == VStr {
		return true
	}
	// Quoted forms: (quote x)
	if isPair(v) && v.car != nil && v.car.typ == VSym {
		symName := v.car.str
		if symName == "QUOTE" || symName == "FUNCTION" {
			return true
		}
	}
	return false
}

// -------- ANSI CL Environment Inquiry Functions --------

// variable-information returns information about a variable binding.
// Returns (values binding-type local-p decls), where binding-type is
// :SPECIAL, :LEXICAL, :SYMBOL-MACRO, :CONSTANT, or NIL.
func builtinVariableInformation(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("variable-information: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("variable-information: need a symbol")
	}
	name := sym.str
	// Check if it's a constant (like T, NIL, PI, etc.)
	constants := map[string]bool{
		"T": true, "NIL": true, "PI": true,
		"MOST-POSITIVE-FIXNUM": true, "MOST-NEGATIVE-FIXNUM": true,
		"CHAR-CODE-LIMIT": true,
	}
	if constants[name] {
		return multiVal(vsym(":CONSTANT"), vbool(false), vnil()), nil
	}
	// Check if it's a special variable (*var* pattern)
	if strings.HasPrefix(name, "*") && strings.HasSuffix(name, "*") && len(name) > 1 {
		return multiVal(vsym(":SPECIAL"), vbool(false), vnil()), nil
	}
	// Check global environment
	_, err := globalEnv.Get(name)
	if err != nil {
		// Not bound at all
		return multiVal(vnil(), vbool(false), vnil()), nil
	}
	// Default: treat as special (global) binding
	return multiVal(vsym(":SPECIAL"), vbool(false), vnil()), nil
}

// function-information returns information about a function binding.
// Returns (values binding-type local-p decls), where binding-type is
// :FUNCTION, :MACRO, :SPECIAL-FORM, or NIL.
func builtinFunctionInformation(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("function-information: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("function-information: need a symbol")
	}
	name := sym.str
	// Check if it's a special operator
	specialOps := map[string]bool{
		"IF": true, "QUOTE": true, "SETQ": true, "BLOCK": true, "RETURN-FROM": true,
		"LET": true, "LET*": true, "PROGN": true, "TAGBODY": true, "GO": true,
		"FLET": true, "LABELS": true, "MACROLET": true, "FUNCTION": true,
		"MULTIPLE-VALUE-BIND": true, "MULTIPLE-VALUE-PROG1": true,
		"CATCH": true, "THROW": true, "UNWIND-PROTECT": true,
		"THE": true, "LOCALLY": true, "EVAL-WHEN": true,
		"SYMBOL-MACROLET": true, "LOAD-TIME-VALUE": true,
	}
	if specialOps[name] {
		return multiVal(vsym(":SPECIAL-FORM"), vbool(false), vnil()), nil
	}
	// Check global environment for function/macro
	fn, err := globalEnv.Get(name)
	if err != nil {
		return multiVal(vnil(), vbool(false), vnil()), nil
	}
	if fn.typ == VMacro {
		return multiVal(vsym(":MACRO"), vbool(false), vnil()), nil
	}
	if fn.typ == VPrim || fn.typ == VFunc || fn.typ == VGeneric {
		return multiVal(vsym(":FUNCTION"), vbool(false), vnil()), nil
	}
	return multiVal(vnil(), vbool(false), vnil()), nil
}

// declaration-information returns information about a declaration.
// Returns (values info), where info is declaration-specific.
func builtinDeclarationInformation(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("declaration-information: need a declaration specifier")
	}
	spec := args[0]
	if spec.typ != VSym {
		return multiVal(vnil(), vbool(false)), nil
	}
	name := strings.ToUpper(spec.str)
	switch name {
	case "OPTIMIZE":
		// Return default optimization qualities
		return multiVal(list(
			list(vsym("SPEED"), vnum(1)),
			list(vsym("SAFETY"), vnum(1)),
			list(vsym("DEBUG"), vnum(1)),
			list(vsym("SPACE"), vnum(1)),
			list(vsym("COMPILATION-SPEED"), vnum(1)),
		), vbool(false)), nil
	case "DECLARATION":
		// Return known declaration names
		return multiVal(list(vsym("OPTIMIZE"), vsym("DECLARATION"), vsym("DYNAMIC-EXTENT"), vsym("TYPE"), vsym("FTYPE"), vsym("NOTINLINE"), vsym("INLINE"), vsym("SPECIAL")), vbool(false)), nil
	default:
		return multiVal(vnil(), vbool(false)), nil
	}
}

func builtinIsqrt(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("isqrt: need a number")
	}
	n := toNum(args[0])
	if n < 0 {
		return nil, fmt.Errorf("isqrt: negative argument")
	}
	r := int(math.Sqrt(n))
	return vnum(float64(r)), nil
}
