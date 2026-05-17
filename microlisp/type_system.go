package microlisp

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func builtinTypeOf(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("type-of: need 1 argument")
	}
	return vsym(typeStr(args[0])), nil
}

func builtinDescribe(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("describe: need 1 argument")
	}
	obj := args[0]
	var sb strings.Builder
	sb.WriteString(ToString(obj))
	sb.WriteString(" is of type ")
	sb.WriteString(typeStr(obj))
	sb.WriteString("\n")
	switch obj.typ {
	case VNil:
		sb.WriteString("It is the canonical false value (nil).\n")
	case VBool:
		if obj == globalEnv.bindings["#t"] {
			sb.WriteString("It is the canonical true value (t).\n")
		} else {
			sb.WriteString("It is the canonical false value (nil).\n")
		}
	case VNum, VRat, VComplex:
		sb.WriteString("Value: ")
		sb.WriteString(ToString(obj))
		sb.WriteString("\nType: ")
		sb.WriteString(typeStr(obj))
		sb.WriteString("\n")
	case VStr:
		sb.WriteString("Value: ")
		sb.WriteString(ToString(obj))
		sb.WriteString("\nLength: ")
		sb.WriteString(strconv.Itoa(len(obj.str)))
		sb.WriteString("\n")
	case VChar:
		sb.WriteString("Character: ")
		sb.WriteString(ToString(obj))
		sb.WriteString("\nCode: ")
		sb.WriteString(strconv.Itoa(int(obj.ch)))
		sb.WriteString("\n")
	case VSym:
		sb.WriteString("Name: ")
		sb.WriteString(obj.str)
		sb.WriteString("\n")
		if obj.fn != nil {
			sb.WriteString("It has a function binding.\n")
		}
		val, err := globalEnv.Get(obj.str)
		if err == nil {
			sb.WriteString("Value: ")
			sb.WriteString(ToString(val))
			sb.WriteString("\n")
		}
		if val != nil && val.typ == VMacro {
			sb.WriteString("It is a macro.\n")
		}
	case VPair:
		length := 0
		cur := obj
		for cur != nil && cur.typ == VPair {
			length++
			cur = cur.cdr
		}
		sb.WriteString("It is a cons of length ")
		sb.WriteString(strconv.Itoa(length))
		sb.WriteString(".\nCar: ")
		sb.WriteString(ToString(obj.car))
		sb.WriteString("\nCdr: ")
		sb.WriteString(ToString(obj.cdr))
		sb.WriteString("\n")
	case VArray:
		if len(obj.array.dims) == 1 && obj.array.fillPtr < 0 {
			sb.WriteString("It is a vector.\n")
		} else {
			sb.WriteString("It is an array.\n")
		}
		sb.WriteString("Dimensions: ")
		for i, d := range obj.array.dims {
			if i > 0 {
				sb.WriteString(" x ")
			}
			sb.WriteString(strconv.Itoa(d))
		}
		sb.WriteString("\n")
	case VInstance:
		if obj.instClass != nil {
			sb.WriteString("It is an instance of class ")
			sb.WriteString(obj.instClass.str)
			sb.WriteString(".\nSlots:\n")
			for name, val := range obj.instSlots {
				sb.WriteString("  ")
				sb.WriteString(name)
				sb.WriteString(" = ")
				sb.WriteString(ToString(val))
				sb.WriteString("\n")
			}
		}
	case VClass:
		if obj.str != "" {
			sb.WriteString("It is a class named ")
			sb.WriteString(obj.str)
			sb.WriteString(".\n")
		}
		if obj.classParents != nil {
			parents := make([]string, len(obj.classParents))
			for i, p := range obj.classParents {
				if p != nil && p.str != "" {
					parents[i] = p.str
				} else {
					parents[i] = "(unknown)"
				}
			}
			sb.WriteString("Superclasses: ")
			for i, p := range parents {
				if i > 0 {
					sb.WriteString(" ")
				}
				sb.WriteString(p)
			}
			sb.WriteString("\n")
		}
	case VFunc:
		sb.WriteString("It is a function.\n")
		if obj.name != "" {
			sb.WriteString("Name: ")
			sb.WriteString(obj.name)
			sb.WriteString("\n")
		}
		sb.WriteString("Arity: ")
		sb.WriteString(strconv.Itoa(len(obj.params)))
		sb.WriteString("\n")
	case VPrim:
		sb.WriteString("It is a built-in function.\n")
	case VMacro:
		sb.WriteString("It is a macro.\n")
	case VStream:
		sb.WriteString("It is a stream.\n")
		if obj.stream.isInput {
			sb.WriteString("Direction: input\n")
		}
		if obj.stream.isOutput {
			sb.WriteString("Direction: output\n")
		}
	case VVHash:
		sb.WriteString("It is a hash-table.\n")
		sb.WriteString("Size: ")
		sb.WriteString(strconv.Itoa(obj.hashTab.count))
		sb.WriteString("\n")
	}
	return vstr(sb.String()), nil
}

func builtinMakeLoadForm(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-load-form: need an object")
	}
	// Default: return (make-load-form-saving-slots object) if the object is a structure
	if args[0].typ == VInstance {
		return vsym("MAKE-LOAD-FORM-SAVING-SLOTS"), nil
	}
	return vnil(), fmt.Errorf("make-load-form: cannot make load form for %s", typeStr(args[0]))
}

func builtinMakeLoadFormSavingSlots(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-load-form-saving-slots: need an object")
	}
	if args[0].typ != VInstance {
		return nil, fmt.Errorf("make-load-form-saving-slots: expected a structure")
	}
	inst := args[0]
	slotNames := listFromSlice([]*Value{})
	if inst.instClass != nil {
		// Try to get slot names from the class
		class := globalEnv.bindings[inst.instClass.str]
		if class != nil && class.typ == VClass {
			for _, sn := range class.classSlots {
				slotNames = cons(vstr(sn), slotNames)
			}
		}
	}
	// Return: (make-load-form-saving-slots-helper object slot-names)
	// For simplicity, return a list form
	return listFromSlice([]*Value{vsym("MAKE-LOAD-FORM-SAVING-SLOTS-HELPERS"), inst, slotNames}), nil
}

var docstrings = make(map[string]string)

func builtinDocumentation(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("documentation: need symbol and doc-type")
	}
	sym := args[0]
	if sym.typ != VSym {
		return vnil(), nil
	}
	docType := ""
	if args[1].typ == VSym {
		docType = args[1].str
	}
	key := sym.str + "_" + docType
	if doc, ok := docstrings[key]; ok {
		return vstr(doc), nil
	}
	return vnil(), nil
}

func builtinApropos(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("apropos: need a string")
	}
	searchStr := ""
	if args[0].typ == VStr {
		searchStr = args[0].str
	} else if args[0].typ == VSym {
		searchStr = args[0].str
	} else {
		searchStr = ToString(args[0])
	}
	var results []*Value
	for name := range globalEnv.bindings {
		if strings.Contains(strings.ToLower(name), strings.ToLower(searchStr)) {
			results = append(results, vsym(name))
		}
	}
	// Sort results
	sort.Slice(results, func(i, j int) bool {
		return results[i].str < results[j].str
	})
	return listFromSlice(results), nil
}

func builtinAproposList(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("apropos-list: need a string")
	}
	searchStr := ""
	if args[0].typ == VStr {
		searchStr = args[0].str
	} else if args[0].typ == VSym {
		searchStr = args[0].str
	} else {
		searchStr = ToString(args[0])
	}
	var results []*Value
	for name := range globalEnv.bindings {
		if strings.Contains(strings.ToLower(name), strings.ToLower(searchStr)) {
			results = append(results, vsym(name))
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].str < results[j].str
	})
	return listFromSlice(results), nil
}

func subtypepChecks(v1, v2 *Value) (bool, bool) {
	typeName := func(v *Value) string {
		if v.typ == VSym {
			return v.str
		}
		if v.typ == VPair && v.car != nil && v.car.typ == VSym {
			return v.car.str
		}
		return ""
	}

	simpleSubtype := func(t1, t2 string) bool {
		if t1 == t2 {
			return true
		}
		types := map[string][]string{"INTEGER": {"RATIONAL", "REAL", "NUMBER", "ATOM", "T"}, "FLOAT": {"REAL", "NUMBER", "ATOM", "T"}, "RATIONAL": {"REAL", "NUMBER", "ATOM", "T"}, "COMPLEX": {"NUMBER", "ATOM", "T"}, "REAL": {"NUMBER", "ATOM", "T"}, "RATIO": {"RATIONAL", "REAL", "NUMBER", "ATOM", "T"}, "FIXNUM": {"INTEGER", "RATIONAL", "REAL", "NUMBER", "ATOM", "T"}, "BIGNUM": {"INTEGER", "RATIONAL", "REAL", "NUMBER", "ATOM", "T"}, "BIT": {"INTEGER", "RATIONAL", "REAL", "NUMBER", "ATOM", "T"}, "SHORT-FLOAT": {"FLOAT", "REAL", "NUMBER", "ATOM", "T"}, "SINGLE-FLOAT": {"FLOAT", "REAL", "NUMBER", "ATOM", "T"}, "DOUBLE-FLOAT": {"FLOAT", "REAL", "NUMBER", "ATOM", "T"}, "LONG-FLOAT": {"FLOAT", "REAL", "NUMBER", "ATOM", "T"}, "STRING": {"ARRAY", "VECTOR", "SEQUENCE", "ATOM", "T"}, "SIMPLE-STRING": {"STRING", "ARRAY", "VECTOR", "SEQUENCE", "ATOM", "T"}, "CHARACTER": {"ATOM", "T"}, "BASE-CHAR": {"CHARACTER", "ATOM", "T"}, "STANDARD-CHAR": {"BASE-CHAR", "CHARACTER", "ATOM", "T"}, "EXTENDED-CHAR": {"CHARACTER", "ATOM", "T"}, "SYMBOL": {"ATOM", "T"}, "KEYWORD": {"SYMBOL", "ATOM", "T"}, "NULL": {"SYMBOL", "LIST", "SEQUENCE", "ATOM", "T"}, "CONS": {"LIST", "SEQUENCE", "T"}, "PAIR": {"CONS", "LIST", "SEQUENCE", "T"}, "LIST": {"SEQUENCE", "T"}, "SEQUENCE": {"T"}, "VECTOR": {"ARRAY", "SEQUENCE", "T"}, "SIMPLE-VECTOR": {"VECTOR", "ARRAY", "SEQUENCE", "T"}, "ARRAY": {"T"}, "FUNCTION": {"T"}, "COMPILED-FUNCTION": {"FUNCTION", "T"}, "HASH-TABLE": {"T"}, "STREAM": {"T"}, "PACKAGE": {"T"}, "PATHNAME": {"T"}, "RANDOM-STATE": {"T"}, "READTABLE": {"T"}, "INSTANCE": {"T"}, "STRUCTURE": {"INSTANCE", "T"}, "METHOD": {"T"}, "BOOLEAN": {"ATOM", "T"}}
		if subtypes, ok := types[t1]; ok {
			for _, s := range subtypes {
				if s == t2 {
					return true
				}
			}
		}
		return false
	}

	n1 := strings.ToUpper(typeName(v1))
	n2 := strings.ToUpper(typeName(v2))

	// If both are simple type names
	if v1.typ == VSym && v2.typ == VSym {
		if n1 == n2 {
			return true, true
		}
		// * is universal type
		if n1 == "*" {
			if n2 == "*" || n2 == "T" {
				return true, true
			}
			return true, false
		}
		if n2 == "*" || n2 == "T" {
			return true, true
		}
		n1u := strings.ToUpper(n1)
		n2u := strings.ToUpper(n2)
		if simpleSubtype(n1u, n2u) {
			return true, true
		}
		cls1 := findClass(n1)
		cls2 := findClass(n2)
		if cls1 != nil && cls2 != nil {
			if classHasAncestor(cls1, n2) {
				return true, true
			}
		}
		return true, false
	}

	// Handle compound (cons ...) types
	if n1 == "CONS" || n2 == "CONS" {
		extractConsParts := func(v *Value) (carType *Value, cdrType *Value) {
			cdrType = vsym("*")
			if v.typ == VSym {
				carType = vsym("*")
				return
			}
			if v.typ == VPair && v.car != nil && v.car.typ == VSym && v.car.str == "CONS" {
				rest := v.cdr
				if rest != nil && !isNil(rest) && rest.typ == VPair {
					carType = rest.car
					rest = rest.cdr
					if rest != nil && !isNil(rest) && rest.typ == VPair {
						cdrType = rest.car
					}
				} else {
					carType = vsym("*")
				}
			}
			return
		}

		ct1, cd1 := extractConsParts(v1)
		ct2, cd2 := extractConsParts(v2)

		if ct1 != nil && ct2 != nil {
			isCarKnown, isCarSub := subtypepChecks(ct1, ct2)
			isCdrKnown, isCdrSub := subtypepChecks(cd1, cd2)
			if !isCarKnown || !isCdrKnown {
				return false, false
			}
			if isCarSub && isCdrSub {
				return true, true
			}
			return true, false
		}
		return false, false
	}

	// If t2 is t or *, anything is a subtype
	if n2 == "T" || n2 == "*" {
		return true, true
	}

	return false, false
}

func builtinSubtypep(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return multiVal(vbool(false), vbool(false)), nil
	}
	t1v := primaryValue(args[0])
	t2v := primaryValue(args[1])

	isSub, result := subtypepChecks(t1v, t2v)
	if isSub && result {
		return multiVal(vbool(true), vbool(true)), nil
	}
	if isSub && !result {
		return multiVal(vbool(false), vbool(true)), nil
	}
	return multiVal(vbool(false), vbool(false)), nil
}
func builtinNumberP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("numberp: need 1 argument")
	}
	return vbool(args[0].typ == VNum), nil
}

func builtinCharP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("character?: need 1 argument")
	}
	return vbool(args[0].typ == VChar), nil
}

func builtinProcP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("procedure?: need 1 argument")
	}
	return vbool(args[0].typ == VPrim || args[0].typ == VFunc), nil
}

func builtinBoolP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("boolean?: need 1 argument")
	}
	return vbool(args[0].typ == VBool), nil
}

func builtinStringP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("stringp: need 1 argument")
	}
	return vbool(args[0].typ == VStr), nil
}

func builtinSymP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("symbol?: need 1 argument")
	}
	return vbool(args[0].typ == VSym), nil
}

func builtinStrP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string?: need 1 argument")
	}
	return vbool(args[0].typ == VStr), nil
}

func builtinNumP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("number?: need 1 argument")
	}
	return vbool(args[0].typ == VNum || args[0].typ == VRat || args[0].typ == VComplex), nil
}

func builtinListP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("listp: need 1 argument")
	}
	v := args[0]
	return vbool(isNil(v) || v.typ == VPair), nil
}

func builtinPairP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("pair?: need 1 argument")
	}
	return vbool(isPair(args[0])), nil
}

func builtinNullP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("null?: need 1 argument")
	}
	return vbool(isNil(args[0])), nil
}
