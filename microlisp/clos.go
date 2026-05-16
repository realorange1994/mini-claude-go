package microlisp

import (
	"fmt"
	"math/big"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
)

// -------- CLOS Builtins --------

func builtinMakeInstance(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VSym {
		return nil, fmt.Errorf("make-instance: need class name")
	}
	className := args[0].str
	cv := findClass(className)
	if cv == nil || cv.typ != VClass {
		return nil, fmt.Errorf("make-instance: %s is not a class", className)
	}
	inst := gcv()
	inst.typ = VInstance
	inst.instClass = cv
	inst.instSlots = make(map[string]*Value)

	// Collect initarg mappings and initforms from slot definitions
	// Walk the entire CPL so inherited slots are included
	slotInitforms := make(map[string]*Value)
	slotInitargs := make(map[string]string) // initarg keyword -> slotName
	// Process each class in CPL (most-specific first)
	for _, c := range cv.cpl {
		if c.typ != VClass {
			continue
		}
		for slotDef := c.body; !isNil(slotDef); slotDef = slotDef.cdr {
			slotName := ""
			if slotDef.car != nil && slotDef.car.typ == VSym {
				slotName = slotDef.car.str
				if _, ok := inst.instSlots[slotName]; !ok {
					inst.instSlots[slotName] = nil // unbound sentinel
				}
			} else if slotDef.car != nil && slotDef.car.typ == VPair && slotDef.car.car != nil && slotDef.car.car.typ == VSym {
				slotName = slotDef.car.car.str
				if _, ok := inst.instSlots[slotName]; !ok {
					inst.instSlots[slotName] = nil // unbound sentinel
				}
				// Parse slot options: :initform, :initarg
				opts := slotDef.car.cdr
				for !isNil(opts) && !isNil(opts.cdr) {
					if opts.car != nil && opts.car.typ == VSym {
						switch opts.car.str {
						case ":INITFORM":
							if _, exists := slotInitforms[slotName]; !exists {
								if opts.cdr == nil || opts.cdr.typ != VPair {
									opts = opts.cdr.cdr
									continue
								}
								if opts.cdr.car != nil {
									initVal, e := Eval(opts.cdr.car, globalEnv)
									if e == nil {
										slotInitforms[slotName] = initVal
									}
								}
								opts = opts.cdr.cdr
								continue
							}
						case ":INITARG":
							if opts.cdr != nil && opts.cdr.typ == VPair && opts.cdr.car != nil && opts.cdr.car.typ == VSym {
								if _, exists := slotInitargs[opts.cdr.car.str]; !exists {
									slotInitargs[opts.cdr.car.str] = slotName
								}
							}
							opts = opts.cdr.cdr
							continue
						}
					}
					opts = opts.cdr
				}
			}
		}
		// Also add bare slot names from classSlots that weren't in body
		for _, slot := range c.classSlots {
			if _, ok := inst.instSlots[slot]; !ok {
				inst.instSlots[slot] = nil // unbound sentinel
			}
		}
	}
	// Process keyword arguments (:initarg -> slot, or direct slot name for condition classes)
	for i := 1; i < len(args)-1; i += 2 {
		if args[i].typ == VSym {
			key := args[i].str
			if i+1 >= len(args) {
				continue
			}
			if slotName, ok := slotInitargs[key]; ok {
				inst.instSlots[slotName] = args[i+1]
			} else if len(key) > 1 && key[0] == ':' {
				// Direct slot name mapping (e.g., :datum -> datum)
				slotName := key[1:]
				if _, exists := inst.instSlots[slotName]; exists || args[0].typ == VSym {
					inst.instSlots[slotName] = args[i+1]
				}
			}
		}
	}
	// Apply initforms for slots not yet set (nil = unbound sentinel)
	for slotName, initVal := range slotInitforms {
		if inst.instSlots[slotName] == nil {
			inst.instSlots[slotName] = initVal
		}
	}
	return inst, nil
}

func builtinSlotValue(args []*Value) (*Value, error) {
	if len(args) < 2 || args[0].typ != VInstance || args[1].typ != VSym {
		return nil, fmt.Errorf("slot-value: need instance and slot name")
	}
	inst := args[0]
	slotName := args[1].str
	v, ok := inst.instSlots[slotName]
	if !ok {
		// Check parent class slots
		for _, c := range inst.instClass.cpl {
			if c.typ == VClass {
				for _, s := range c.classSlots {
					if s == slotName {
						v, ok = inst.instSlots[s]
						break
					}
				}
			}
		}
	}
	if !ok {
		return nil, fmt.Errorf("slot-value: slot %s not found in %s", slotName, inst.instClass.str)
	}
	if v == nil {
		return vnil(), nil // unbound slot returns nil for backward compat
	}
	return v, nil
}

func builtinSlotValueSetf(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("slot-value-setf: need value, instance, and slot name")
	}
	val := args[0]
	inst := args[1]
	slot := args[2]
	if inst.typ != VInstance {
		return nil, fmt.Errorf("slot-value-setf: second argument must be an instance")
	}
	if slot.typ != VSym {
		return nil, fmt.Errorf("slot-value-setf: third argument must be a symbol")
	}
	inst.instSlots[slot.str] = val
	return val, nil
}

func builtinSlotSet(args []*Value) (*Value, error) {
	if len(args) < 3 || args[0].typ != VInstance || args[1].typ != VSym {
		return nil, fmt.Errorf("slot-set!: need instance, slot name, and value")
	}
	inst := args[0]
	slotName := args[1].str
	inst.instSlots[slotName] = args[2]
	return args[2], nil
}

func builtinSlotBoundp(args []*Value) (*Value, error) {
	if len(args) < 2 || args[0].typ != VInstance || args[1].typ != VSym {
		return nil, fmt.Errorf("slot-boundp: need instance and slot name")
	}
	inst := args[0]
	slotName := args[1].str
	v, ok := inst.instSlots[slotName]
	if ok && v != nil {
		return vbool(true), nil // slot exists in map and is not unbound sentinel
	}
	// Check CPL for inherited slot declarations
	for _, c := range inst.instClass.cpl {
		if c.typ == VClass {
			for _, s := range c.classSlots {
				if s == slotName {
					v, ok = inst.instSlots[s]
					return vbool(ok && v != nil), nil
				}
			}
		}
	}
	return vbool(false), nil
}

func builtinSlotExistsP(args []*Value) (*Value, error) {
	if len(args) < 2 || args[0].typ != VInstance || args[1].typ != VSym {
		return nil, fmt.Errorf("slot-exists-p: need instance and slot name")
	}
	inst := args[0]
	slotName := args[1].str
	for _, c := range inst.instClass.cpl {
		if c.typ == VClass {
			for _, s := range c.classSlots {
				if s == slotName {
					return vbool(true), nil
				}
			}
		}
	}
	return vbool(false), nil
}

func builtinSlotMakunbound(args []*Value) (*Value, error) {
	if len(args) < 2 || args[0].typ != VInstance || args[1].typ != VSym {
		return nil, fmt.Errorf("slot-makunbound: need instance and slot name")
	}
	inst := args[0]
	slotName := args[1].str
	inst.instSlots[slotName] = nil // unbound sentinel
	return args[0], nil
}

func builtinClassName(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("class-name: need a class")
	}
	c := args[0]
	if c.typ == VClass {
		return vsym(c.str), nil
	}
	if c.typ == VSym {
		if cls, ok := classRegistry[c.str]; ok {
			return vsym(cls.str), nil
		}
		return nil, fmt.Errorf("class-name: %s is not a class", c.str)
	}
	return nil, fmt.Errorf("class-name: need a class or symbol")
}

// builtinClassNameSetf - (setf (class-name class) new-name)
func builtinClassNameSetf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("(setf class-name): need class and new-name")
	}
	// setf convention: args[0] = new-value, args[1] = place-argument
	newName := args[0]
	cls := args[1]
	if cls.typ != VClass {
		return nil, fmt.Errorf("(setf class-name): second argument must be a class")
	}
	if newName.typ != VSym {
		return nil, fmt.Errorf("(setf class-name): new name must be a symbol")
	}
	// Remove from old name, add under new name
	delete(classRegistry, cls.str)
	cls.str = newName.str
	classRegistry[cls.str] = cls
	return cls, nil
}

func builtinFindClass(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("find-class: need a symbol")
	}
	sym := args[0]
	if sym.typ != VSym {
		return nil, fmt.Errorf("find-class: need a symbol")
	}
	if cls, ok := classRegistry[sym.str]; ok {
		return cls, nil
	}
	return vnil(), nil
}

// builtinFindClassSetf - (setf (find-class symbol) class)
func builtinFindClassSetf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("(setf find-class): need class and symbol")
	}
	newClass := args[0]
	sym := args[1]
	if sym.typ != VSym {
		return nil, fmt.Errorf("(setf find-class): second argument must be a symbol")
	}
	if newClass.typ != VClass {
		return nil, fmt.Errorf("(setf find-class): first argument must be a class")
	}
	classRegistry[sym.str] = newClass
	return newClass, nil
}

func builtinClassOf(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("class-of: need argument")
	}
	v := args[0]
	// For instances, return the class object directly
	if v.typ == VInstance && v.instClass != nil {
		return v.instClass, nil
	}
	// For built-in types, look up the class in classRegistry by type name
	typeName := strings.ToUpper(typeStr(v))
	if cls, ok := classRegistry[typeName]; ok {
		return cls, nil
	}
	// If no class registered for this type, return the built-in-class symbol as fallback
	return vsym(typeName), nil
}

func builtinIsA(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("is-a?: need obj and class-name")
	}
	obj := args[0]
	if obj.typ != VInstance {
		return vnil(), nil
	}
	var cls *Value
	if args[1].typ == VSym {
		var err error
		cls, err = globalEnv.Get(args[1].str)
		if err != nil || cls.typ != VClass {
			return vnil(), nil
		}
	} else if args[1].typ == VClass {
		cls = args[1]
	} else {
		return vnil(), nil
	}
	for _, c := range obj.instClass.cpl {
		if c == cls {
			return vbool(true), nil
		}
	}
	return vnil(), nil
}

func builtinClassSlots(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("class-slots: need class name")
	}
	var cls *Value = args[0]
	// Support both VClass and VSym (symbol naming a class)
	if cls.typ == VSym {
		cls, _ = globalEnv.Get(cls.str)
	}
	if cls == nil || cls.typ != VClass {
		return nil, fmt.Errorf("class-slots: not a class")
	}
	// Build a list of slot name symbols
	var result *Value = vnil()
	for i := len(cls.classSlots) - 1; i >= 0; i-- {
		result = &Value{typ: VPair, car: vsym(cls.classSlots[i]), cdr: result}
	}
	return result, nil
}

func builtinClassSlotDefs(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("class-slot-defs: need class name")
	}
	var cls *Value = args[0]
	if cls.typ == VSym {
		cls, _ = globalEnv.Get(cls.str)
	}
	if cls == nil || cls.typ != VClass {
		return nil, fmt.Errorf("class-slot-defs: not a class")
	}
	if cls.body != nil {
		return cls.body, nil
	}
	return vnil(), nil
}

// valueToEQLKey produces a stable string key for an EQL specializer value
func valueToEQLKey(v *Value) string {
	v = primaryValue(v)
	switch v.typ {
	case VSym:
		return "S:" + v.str
	case VNum:
		if v.imag != 0 {
			return fmt.Sprintf("C:%v+%vi", v.num, v.imag)
		}
		return fmt.Sprintf("N:%v", v.num)
	case VStr:
		return "STR:" + v.str
	case VChar:
		return fmt.Sprintf("CH:%d", v.ch)
	case VNil:
		return "NIL"
	case VBool:
		if v.num != 0 {
			return "T"
		}
		return "NIL"
	default:
		return fmt.Sprintf("V:%s", ToString(v))
	}
}

// valueMatchesEQLKey checks if a runtime value matches an EQL specializer key
func valueMatchesEQLKey(arg *Value, key string) bool {
	arg = primaryValue(arg)
	switch {
	case key == "NIL":
		return isNil(arg)
	case key == "T":
		return arg.typ == VBool && arg.num != 0
	case strings.HasPrefix(key, "S:"):
		name := key[2:]
		if arg.typ == VSym && arg.str == name {
			return true
		}
		// Special case: (eql nil) / (eql t) — the parsed form is a symbol,
		// but the evaluated argument may be VNil/VBool
		if name == "nil" && isNil(arg) {
			return true
		}
		if name == "t" && arg.typ == VBool && arg.num != 0 {
			return true
		}
		return false
	case strings.HasPrefix(key, "N:"):
		if arg.typ != VNum {
			return false
		}
		n, _ := strconv.ParseFloat(key[2:], 64)
		return arg.num == n
	case strings.HasPrefix(key, "STR:"):
		return arg.typ == VStr && arg.str == key[4:]
	case strings.HasPrefix(key, "CH:"):
		if arg.typ != VChar {
			return false
		}
		n, _ := strconv.ParseInt(key[3:], 10, 32)
		return arg.ch == rune(n)
	case strings.HasPrefix(key, "C:"):
		if arg.typ != VNum {
			return false
		}
		parts := strings.SplitN(key[2:], "+", 2)
		if len(parts) == 2 {
			re, _ := strconv.ParseFloat(parts[0], 64)
			imStr := strings.TrimSuffix(parts[1], "i")
			im, _ := strconv.ParseFloat(imStr, 64)
			return arg.num == re && arg.imag == im
		}
		return false
	}
	return false
}

// isTypeSpecializerMatch checks if arg matches a built-in type specializer
func isTypeSpecializerMatch(arg *Value, typeName string) bool {
	arg = primaryValue(arg)
	switch typeName {
	case "t", "T":
		return true
	case "null", "NULL":
		return isNil(arg)
	case "list", "LIST":
		return isNil(arg) || arg.typ == VPair
	case "cons", "CONS":
		return arg.typ == VPair
	case "symbol", "SYMBOL":
		return arg.typ == VSym || isNil(arg)
	case "string", "STRING":
		return arg.typ == VStr
	case "number", "NUMBER":
		return arg.typ == VNum
	case "integer", "INTEGER":
		return arg.typ == VNum && !arg.isFloat && arg.num == float64(int64(arg.num))
	case "float", "single-float", "double-float", "FLOAT", "SINGLE-FLOAT", "DOUBLE-FLOAT":
		return arg.typ == VNum && arg.isFloat
	case "character", "CHARACTER":
		return arg.typ == VChar
	case "vector", "VECTOR":
		return arg.typ == VArray && arg.array != nil && len(arg.array.dims) == 1
	case "array", "ARRAY":
		return arg.typ == VArray
	case "function", "FUNCTION":
		return arg.typ == VFunc || arg.typ == VPrim || arg.typ == VGeneric
	case "hash-table", "HASH-TABLE":
		return arg.typ == VVHash
	case "stream", "STREAM":
		return arg.typ == VStream
	}
	return false
}

// methodApplicable checks if a method is applicable to the given evaluated arguments
func methodApplicable(m genMethod, evArgs []*Value) bool {
	if len(m.specializers) == 0 {
		return true // unspecialized method applies to all
	}
	for i, sp := range m.specializers {
		if sp == "" {
			continue // no specializer for this param
		}
		if i >= len(evArgs) {
			return false
		}
		arg := evArgs[i]
		// Handle EQL specializer: "#EQL:<key>"
		if strings.HasPrefix(sp, "#EQL:") {
			if valueMatchesEQLKey(arg, sp[5:]) {
				continue
			}
			return false
		}
		// Handle built-in type specializers (t, null, list, cons, etc.)
		if isTypeSpecializerMatch(arg, sp) {
			continue
		}
		// Handle class specializers for instances
		if arg.typ == VInstance {
			found := false
			for _, c := range arg.instClass.cpl {
				if c.typ == VClass && c.str == sp {
					found = true
					break
				}
			}
			if found {
				continue
			}
		}
		return false
	}
	return true
}

// methodSpecificity returns a score for method specificity (lower = more specific)
// Only meaningful for methods with specializers on the first argument
func methodSpecificity(m genMethod, evArgs []*Value) int {
	baseScore := 999
	if len(m.specializers) > 0 && m.specializers[0] != "" && len(evArgs) > 0 {
		sp := m.specializers[0]
		// EQL specializers are most specific (score 0)
		if strings.HasPrefix(sp, "#EQL:") {
			baseScore = 0
		} else if evArgs[0].typ == VInstance {
			for i, c := range evArgs[0].instClass.cpl {
				if c.typ == VClass && c.str == sp {
					baseScore = i
					break
				}
			}
		} else if isTypeSpecializerMatch(evArgs[0], sp) {
			spUp := strings.ToUpper(sp)
			// Score hierarchy: lower = more specific
			switch spUp {
			case "NULL":
				baseScore = 1
			case "CONS":
				baseScore = 2
			case "LIST":
				baseScore = 3
			case "SYMBOL":
				baseScore = 10
			case "STRING":
				baseScore = 11
			case "SIMPLE-STRING":
				baseScore = 12
			case "CHARACTER":
				baseScore = 13
			case "BASE-CHAR":
				baseScore = 14
			case "STANDARD-CHAR":
				baseScore = 15
			case "KEYWORD":
				baseScore = 16
			case "NUMBER":
				baseScore = 200
			case "REAL":
				baseScore = 150
			case "RATIONAL":
				baseScore = 140
			case "RATIO":
				baseScore = 135
			case "INTEGER":
				baseScore = 130
			case "FIXNUM":
				baseScore = 120
			case "BIGNUM":
				baseScore = 115
			case "BIT":
				baseScore = 110
			case "FLOAT":
				baseScore = 145
			case "SINGLE-FLOAT":
				baseScore = 125
			case "DOUBLE-FLOAT":
				baseScore = 118
			case "SHORT-FLOAT":
				baseScore = 122
			case "LONG-FLOAT":
				baseScore = 112
			case "COMPLEX":
				baseScore = 160
			case "SEQUENCE":
				baseScore = 180
			case "VECTOR":
				baseScore = 165
			case "ARRAY":
				baseScore = 175
			case "SIMPLE-VECTOR":
				baseScore = 155
			case "HASH-TABLE":
				baseScore = 300
			case "PACKAGE":
				baseScore = 301
			case "PATHNAME":
				baseScore = 302
			case "RANDOM-STATE":
				baseScore = 303
			case "READTABLE":
				baseScore = 304
			case "STREAM":
				baseScore = 305
			case "CLASS":
				baseScore = 306
			case "FUNCTION":
				baseScore = 400
			case "COMPILED-FUNCTION":
				baseScore = 390
			case "METHOD":
				baseScore = 395
			case "T":
				baseScore = 999
			default:
				// For user-defined types, check subtype relationship with known types.
				// Start with default score, then refine based on subtype relationships.
				score := 500
				// If sp is a subtype of a known type, give it a more specific score
				for other, otherScore := range map[string]int{
					"NUMBER": 200, "REAL": 150, "RATIONAL": 140, "INTEGER": 130,
					"FIXNUM": 120, "BIGNUM": 115, "BIT": 110, "RATIO": 135,
					"FLOAT": 145, "SINGLE-FLOAT": 125, "DOUBLE-FLOAT": 118,
					"SHORT-FLOAT": 122, "LONG-FLOAT": 112, "COMPLEX": 160,
					"SEQUENCE": 180, "LIST": 3, "VECTOR": 165, "ARRAY": 175,
					"SIMPLE-VECTOR": 155, "STRING": 11, "CHARACTER": 13,
					"SYMBOL": 10, "HASH-TABLE": 300, "STREAM": 305,
					"PACKAGE": 301, "PATHNAME": 302, "RANDOM-STATE": 303,
					"READTABLE": 304, "FUNCTION": 400, "CONS": 2, "NULL": 1,
				} {
					if other == spUp {
						continue
					}
					if typeIsSubtype(spUp, other) {
						if otherScore > 0 && otherScore <= score {
							score = otherScore - 1
						}
					}
				}
				baseScore = score
			}
		}
	}
	// Parse numeric auxiliary qualifier (e.g., ":around 2" -> 2)
	// Higher number = more specific = lower score
	if m.qualifier != "" {
		parts := strings.Split(m.qualifier, " ")
		if len(parts) == 2 {
			if n, err := strconv.Atoi(parts[1]); err == nil {
				baseScore -= n * 1000
			}
		}
	}
	return baseScore
}

// subtypepCheck checks if typeA is a subtype of typeB using the built-in subtypep logic
func subtypepCheck(typeA, typeB string, env *Env) bool {
	return typeIsSubtype(strings.ToUpper(typeA), strings.ToUpper(typeB))
}

// typeIsSubtype checks if t1 is a subtype of t2 using the same hierarchy as subtypepChecks
func typeIsSubtype(t1, t2 string) bool {
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

// callNextMethodChain applies the next method(s) in a chain with the original arguments
func callNextMethodChain(methods []genMethod, evArgs []*Value) (*Value, error) {
	if len(methods) == 0 {
		return nil, fmt.Errorf("call-next-method: no next method")
	}
	pm := methods[0]
	pEnv := NewEnv(pm.env)
	for j, p := range pm.params {
		if j < len(evArgs) {
			pEnv.Set(p, evArgs[j])
		}
	}
	// Bind call-next-method for further chaining
	if len(methods) > 1 {
		remaining := methods[1:]
		pEnv.Set("call-next-method", &Value{
			typ: VPrim,
			fn: func(ignored []*Value) (*Value, error) {
				return callNextMethodChain(remaining, evArgs)
			},
		})
		pEnv.Set("next-method-p", &Value{
			typ: VPrim,
			fn: func(ignored []*Value) (*Value, error) {
				return vbool(true), nil
			},
		})
	} else {
		pEnv.Set("call-next-method", &Value{
			typ: VPrim,
			fn: func(args2 []*Value) (*Value, error) {
				return nil, fmt.Errorf("call-next-method: no next method")
			},
		})
		pEnv.Set("next-method-p", &Value{
			typ: VPrim,
			fn: func(ignored []*Value) (*Value, error) {
				return vbool(false), nil
			},
		})
	}
	// Evaluate body (list of expressions)
	var result *Value = vnil()
	if pm.body != nil {
		bodyList := pm.body
		for !isNil(bodyList) {
			r, e := Eval(bodyList.car, pEnv)
			if e != nil {
				return nil, e
			}
			result = r
			bodyList = bodyList.cdr
		}
	}
	return result, nil
}

func builtinInstanceP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VInstance), nil
}

// builtinFindMethod implements CL: FIND-METHOD
// (find-method generic-function qualifier specializer-list &optional errorp)
func builtinFindMethod(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("find-method: need generic-function, qualifier, and specializer-list")
	}
	gf := primaryValue(args[0])
	if gf.typ != VGeneric {
		return nil, fmt.Errorf("find-method: first argument must be a generic function")
	}
	qualifier := ""
	qv := primaryValue(args[1])
	if qv.typ == VSym && isKeyword(qv.str) {
		qualifier = qv.str
	} else if qv.typ == VNil || isNil(qv) {
		qualifier = ""
	} else if qv.typ == VStr || qv.typ == VSym {
		qualifier = strings.ToUpper(qv.str)
	} else {
		return nil, fmt.Errorf("find-method: qualifier must be a keyword, string, symbol, or nil")
	}
	specList := primaryValue(args[2])
	errorp := true
	if len(args) >= 4 {
		ep := primaryValue(args[3])
		if isNil(ep) || !isTruthy(ep) {
			errorp = false
		}
	}
	// Convert specializer-list to string slice
	var specStrings []string
	for !isNil(specList) && specList.typ == VPair {
		sp := specList.car
		if sp == nil || isNil(sp) {
			specStrings = append(specStrings, "")
		} else if sp.typ == VSym {
			if sp.str == "T" {
				specStrings = append(specStrings, "")
			} else {
				specStrings = append(specStrings, strings.ToUpper(sp.str))
			}
		} else if sp.typ == VPair && sp.car != nil && sp.car.typ == VSym && sp.car.str == "EQL" {
			// (eql value) specializer
			eqlVal := sp.cdr
			if eqlVal != nil && eqlVal.typ == VPair && eqlVal.car != nil {
				key := eqlVal.car
				if key.typ == VSym && len(key.str) > 0 && key.str[0] == ':' {
					specStrings = append(specStrings, "#EQL:"+key.str)
				} else {
					specStrings = append(specStrings, "#EQL:"+ToString(key))
				}
			} else {
				specStrings = append(specStrings, "#EQL:")
			}
		} else if sp.typ == VClass {
			classStr := strings.ToUpper(sp.str)
			if classStr == "T" {
				specStrings = append(specStrings, "") // t = unspecialized
			} else {
				specStrings = append(specStrings, classStr)
			}
		} else {
			specStrings = append(specStrings, "")
		}
		specList = specList.cdr
	}
	// Search for matching method
	for i, m := range gf.genMethods {
		if m.qualifier != qualifier {
			continue
		}
		if len(m.specializers) != len(specStrings) {
			continue
		}
		match := true
		for j, ms := range m.specializers {
			req := specStrings[j]
			if ms == req {
				continue
			}
			// Check if both are class names that match up to case
			if strings.EqualFold(ms, req) {
				continue
			}
			// t/T/"" all mean "unspecified" type
			if (ms == "" || strings.EqualFold(ms, "T")) && (req == "" || strings.EqualFold(req, "T")) {
				continue
			}
			match = false
			break
		}
		if match {
			mv := gcv()
			mv.typ = VMethod
			mv.str = gf.str
			mv.methodGF = gf
			mv.methodIdx = i
			return mv, nil
		}
	}
	if errorp {
		return nil, fmt.Errorf("find-method: no method matches qualifier %s and specializer-list", qualifier)
	}
	return vnil(), nil
}

// builtinRemoveMethod implements CL: REMOVE-METHOD
// (remove-method generic-function method)
func builtinRemoveMethod(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("remove-method: need generic-function and method")
	}
	gf := primaryValue(args[0])
	if gf.typ != VGeneric {
		return nil, fmt.Errorf("remove-method: first argument must be a generic function")
	}
	method := primaryValue(args[1])
	if method.typ == VMethod && method.methodGF == gf {
		idx := method.methodIdx
		if idx >= 0 && idx < len(gf.genMethods) {
			gf.genMethods = append(gf.genMethods[:idx], gf.genMethods[idx+1:]...)
		}
		return gf, nil
	}
	// Also allow passing a method found by find-method on a different GF reference
	// (same generic function but different Value pointer)
	if method.typ == VMethod {
		// Search by matching qualifier+specializers
		mQual := ""
		mSpecs := []string{}
		if method.methodGF != nil && method.methodIdx >= 0 && method.methodIdx < len(method.methodGF.genMethods) {
			srcM := method.methodGF.genMethods[method.methodIdx]
			mQual = srcM.qualifier
			mSpecs = srcM.specializers
		}
		for i, m := range gf.genMethods {
			if m.qualifier != mQual || len(m.specializers) != len(mSpecs) {
				continue
			}
			match := true
			for j, ms := range m.specializers {
				if ms != mSpecs[j] && !strings.EqualFold(ms, mSpecs[j]) {
					match = false
					break
				}
			}
			if match {
				gf.genMethods = append(gf.genMethods[:i], gf.genMethods[i+1:]...)
				return gf, nil
			}
		}
	}
	return nil, fmt.Errorf("remove-method: method not found in generic function")
}

// builtinComputeApplicableMethods implements CL: COMPUTE-APPLICABLE-METHODS
// (compute-applicable-methods generic-function arguments-list)
// Returns a list of applicable methods in order of decreasing precedence.
func builtinComputeApplicableMethods(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("compute-applicable-methods: need generic-function and arguments-list")
	}
	gf := primaryValue(args[0])
	if gf.typ != VGeneric {
		return nil, fmt.Errorf("compute-applicable-methods: first argument must be a generic function")
	}
	argList := args[1]
	evArgs := toSlice(argList)
	// Filter applicable methods
	var applicable []genMethod
	for _, m := range gf.genMethods {
		if methodApplicable(m, evArgs) {
			applicable = append(applicable, m)
		}
	}
	// Sort by specificity (lower score = more specific = comes first)
	sort.SliceStable(applicable, func(i, j int) bool {
		return methodSpecificity(applicable[i], evArgs) < methodSpecificity(applicable[j], evArgs)
	})
	// Convert to VMethod list
	result := vnil()
	for i := len(applicable) - 1; i >= 0; i-- {
		m := &applicable[i]
		mv := gcv()
		mv.typ = VMethod
		mv.str = m.qualifier
		mv.methodGF = gf
		mv.methodIdx = -1
		result = cons(mv, result)
	}
	return result, nil
}

// builtinMethodQualifiers implements CL: METHOD-QUALIFIERS
// (method-qualifiers method)
// Returns a list of qualifiers for the method.
func builtinMethodQualifiers(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("method-qualifiers: need a method")
	}
	m := primaryValue(args[0])
	if m.typ != VMethod {
		return nil, fmt.Errorf("method-qualifiers: argument must be a method")
	}
	if m.methodGF != nil && m.methodIdx >= 0 && m.methodIdx < len(m.methodGF.genMethods) {
		qual := m.methodGF.genMethods[m.methodIdx].qualifier
		if qual == "" {
			return vnil(), nil
		}
		return cons(vsym(qual), vnil()), nil
	}
	// Fallback: use the stored qualifier string
	if m.str == "" {
		return vnil(), nil
	}
	return cons(vsym(m.str), vnil()), nil
}

// generic-function-p is a type predicate
func builtinGenericFunctionP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("generic-function-p: need an object")
	}
	v := primaryValue(args[0])
	return vbool(v.typ == VGeneric), nil
}

func builtinSetMethodCombination(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-method-combination: need generic-function and combination-type")
	}
	gf := primaryValue(args[0])
	if gf.typ != VGeneric {
		return nil, fmt.Errorf("set-method-combination: first argument must be a generic function")
	}
	combo := primaryValue(args[1])
	comboStr := ""
	if combo.typ == VSym {
		comboStr = strings.ToUpper(combo.str)
	} else if combo.typ == VStr {
		comboStr = strings.ToUpper(combo.str)
	} else if combo.typ == VPair && combo.car != nil && combo.car.typ == VSym {
		// (progn) or (progn most-specific-first) etc.
		comboStr = strings.ToUpper(combo.car.str)
	}
	validCombos := map[string]bool{
		"STANDARD": true, "PROGN": true, "AND": true, "OR": true,
		"LIST": true, "APPEND": true, "NCONC": true,
		"MIN": true, "MAX": true, "+": true,
	}
	if !validCombos[comboStr] {
		return nil, fmt.Errorf("set-method-combination: unknown combination type %s", comboStr)
	}
	gf.methodCombo = strings.ToLower(comboStr)
	return gf, nil
}

// -------- Symbol introspection --------
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

// builtinMacroFunction handles both:
// (macro-function sym) - returns the macro function or nil
// (macro-function (macro-function sym) new-fn) - setter form for setf
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

// builtinMacroFunctionSetf implements (setf (macro-function sym) fn).
// fn must be a function of two arguments: (form environment) -> expansion.
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

// builtinCompilerMacroFunction returns the compiler macro function for a symbol, or nil.
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

// builtinCompilerMacroFunctionSetf implements (setf (compiler-macro-function sym) fn).
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

// builtinSetClassPrintFn stores a print function for a defstruct class (used by :print-function)
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

// builtinRemoveClassPrintFn removes a print function for a defstruct class
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
	return vbool(err == nil), nil
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
		return vbool(false), nil
	}
	return vbool(val.typ == VPrim || val.typ == VFunc || val.typ == VGeneric || val.typ == VMacro), nil
}

// specialOperators is the set of CL special operators (not functions).
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

// builtinEnsureGenericFunction - ANSI CL ensure-generic-function
// If a generic function with the given name already exists, return it;
// otherwise create a new one and register it.
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

// -------- room --------
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

// -------- make-load-form --------
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

// -------- make-load-form-saving-slots --------
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

// docstrings stores documentation strings: map[symbolName_docType] -> docstring
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

func builtinReplace(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("replace: need at least two sequences")
	}
	target := args[0]
	source := args[1]
	start1 := 0
	end1 := -1
	start2 := 0
	end2 := -1
	for i := 2; i+1 < len(args); i += 2 {
		key := primaryValue(args[i])
		if key.typ == VSym {
			switch key.str {
			case ":START1":
				start1 = int(primaryValue(args[i+1]).num)
			case ":END1":
				end1 = int(primaryValue(args[i+1]).num)
			case ":START2":
				start2 = int(primaryValue(args[i+1]).num)
			case ":END2":
				end2 = int(primaryValue(args[i+1]).num)
			}
		}
	}
	switch target.typ {
	case VStr:
		ts := []rune(target.str)
		tLen := len(ts)
		if end1 < 0 || end1 > tLen {
			end1 = tLen
		}
		if start1 < 0 {
			start1 = 0
		}
		ss := []rune(source.str)
		sLen := len(ss)
		if end2 < 0 || end2 > sLen {
			end2 = sLen
		}
		if start2 < 0 {
			start2 = 0
		}
		j := start2
		for i := start1; i < end1 && j < end2; i++ {
			ts[i] = ss[j]
			j++
		}
		target.str = string(ts)
		return target, nil
	case VArray:
		te := target.array.elements
		tLen := len(te)
		if end1 < 0 || end1 > tLen {
			end1 = tLen
		}
		if start1 < 0 {
			start1 = 0
		}
		se := seqToList(source)
		sLen := len(se)
		if end2 < 0 || end2 > sLen {
			end2 = sLen
		}
		if start2 < 0 {
			start2 = 0
		}
		j := start2
		for i := start1; i < end1 && j < end2; i++ {
			te[i] = se[j]
			j++
		}
		return target, nil
	case VPair:
		// For lists, rebuild with replaced portion
		if end1 < 0 {
			end1 = 999999
		}
		result := vnil()
		var tail **Value = &result
		idx := 0
		srcList := seqToList(source)
		sLen := len(srcList)
		if end2 < 0 || end2 > sLen {
			end2 = sLen
		}
		if start2 < 0 {
			start2 = 0
		}
		for i := 0; i < start2; i++ {
			// Copy source elements before replacement
		}
		cur := target
		idx = 0
		for !isNil(cur) && idx < start1 {
			*tail = cons(cur.car, vnil())
			tail = &((*tail).cdr)
			cur = cur.cdr
			idx++
		}
		// Skip target elements in range
		for !isNil(cur) && idx < end1 {
			cur = cur.cdr
			idx++
		}
		// Insert source elements
		for j := start2; j < end2 && j < sLen; j++ {
			*tail = cons(srcList[j], vnil())
			tail = &((*tail).cdr)
		}
		// Append remaining target
		for !isNil(cur) {
			*tail = cons(cur.car, vnil())
			tail = &((*tail).cdr)
			cur = cur.cdr
		}
		*tail = vnil()
		return result, nil
	default:
		return nil, fmt.Errorf("replace: not a sequence")
	}
}

func builtinParseInteger(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("parse-integer: need a string")
	}
	strVal := primaryValue(args[0])
	var str string
	if strVal.typ == VStr {
		str = strVal.str
	} else {
		str = ToString(strVal)
	}
	start := 0
	end := -1
	radix := 10
	junkAllowed := false
	for i := 1; i+1 < len(args); i += 2 {
		key := primaryValue(args[i])
		if key.typ == VSym {
			switch key.str {
			case ":START":
				start = int(primaryValue(args[i+1]).num)
			case ":END":
				end = int(primaryValue(args[i+1]).num)
			case ":RADIX":
				radix = int(primaryValue(args[i+1]).num)
			case ":JUNK-ALLOWED":
				junkAllowed = !isNil(args[i+1])
			}
		}
	}
	if end < 0 {
		end = len(str)
	}
	s := strings.TrimSpace(str[start:end])
	s = strings.ReplaceAll(s, "_", "")
	if s == "" || s == "-" || s == "+" {
		if junkAllowed {
			return vnil(), nil
		}
		return nil, fmt.Errorf("parse-integer: no integer at position %d", start)
	}
	// Position: count from start, skip whitespace then count sign+digits
	pos := start
	for pos < len(str) && (str[pos] == ' ' || str[pos] == '\t' || str[pos] == '\n' || str[pos] == '\r') {
		pos++
	}
	// s contains sign+digits (or just digits); len(s) counts all
	pos += len(s)

	n, err := strconv.ParseInt(s, radix, 64)
	if err != nil {
		// Try to parse as much as possible for junk-allowed
		if junkAllowed {
			// Find longest valid prefix of s
			for l := len(s); l > 0; l-- {
				partial, e2 := strconv.ParseInt(s[:l], radix, 64)
				if e2 == nil {
					p := start
					for p < len(str) && (str[p] == ' ' || str[p] == '\t' || str[p] == '\n' || str[p] == '\r') {
						p++
					}
					p += l
					return multiVal(vnum(float64(partial)), vnum(float64(p))), nil
				}
			}
			return vnil(), nil
		}
		return nil, fmt.Errorf("parse-integer: not an integer")
	}
	return multiVal(vnum(float64(n)), vnum(float64(pos))), nil
}

func builtinDigitCharP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	radix := 10
	if len(args) > 1 {
		radix = int(toNum(args[1]))
	}
	c := primaryValue(args[0])
	if c.typ != VChar {
		return vnil(), nil
	}
	d := unicode.ToLower(c.ch)
	baseDigit := '0'
	radixChar := rune('a' + radix - 10)
	if d >= baseDigit && d < baseDigit+rune(radix) {
		return vnum(float64(int(d - baseDigit))), nil
	}
	if d >= 'a' && d < radixChar {
		return vnum(float64(int(d - 'a' + 10))), nil
	}
	return vnil(), nil
}

func builtinAlphanumericP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	c := primaryValue(args[0])
	if c.typ == VChar {
		return vbool(unicode.IsLetter(c.ch) || unicode.IsDigit(c.ch)), nil
	}
	return vbool(false), nil
}

func builtinCharUpcase(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("char-upcase: need a character")
	}
	c := primaryValue(args[0])
	if c.typ != VChar {
		return nil, fmt.Errorf("char-upcase: not a character")
	}
	return vchar(unicode.ToUpper(c.ch)), nil
}

func builtinCharDowncase(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("char-downcase: need a character")
	}
	c := primaryValue(args[0])
	if c.typ != VChar {
		return nil, fmt.Errorf("char-downcase: not a character")
	}
	return vchar(unicode.ToLower(c.ch)), nil
}

func builtinReadSequence(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("read-sequence: need sequence and stream")
	}
	seq := args[0]
	stream := args[1]
	if stream.typ != VStream {
		return nil, fmt.Errorf("read-sequence: second argument must be a stream")
	}
	start := 0
	end := -1
	for i := 2; i+1 < len(args); i += 2 {
		key := primaryValue(args[i])
		if key.typ == VSym {
			switch key.str {
			case ":START":
				start = int(primaryValue(args[i+1]).num)
			case ":END":
				end = int(primaryValue(args[i+1]).num)
			}
		}
	}
	switch seq.typ {
	case VStr:
		s := seq.str
		runes := []rune(s)
		total := len(runes)
		if end < 0 || end > total {
			end = total
		}
		if start < 0 {
			start = 0
		}
		count := 0
		for i := start; i < end; i++ {
			r, err := stream.stream.readChar()
			if err != nil {
				break
			}
			runes[i] = r
			count++
		}
		seq.str = string(runes)
		return vnum(float64(start + count)), nil
	case VArray:
		arr := seq.array
		total := len(arr.elements)
		if end < 0 || end > total {
			end = total
		}
		if start < 0 {
			start = 0
		}
		count := 0
		for i := start; i < end; i++ {
			r, err := stream.stream.readChar()
			if err != nil {
				break
			}
			arr.elements[i] = vstr(string(r))
			count++
		}
		return vnum(float64(start + count)), nil
	case VPair:
		lst := seq
		seen := make(map[*Value]bool)
		idx := 0
		count := 0
		for !isNil(lst) {
			if seen[lst] {
				break
			}
			seen[lst] = true
			if idx >= start && (end < 0 || idx < end) {
				r, err := stream.stream.readChar()
				if err != nil {
					break
				}
				lst.car = vstr(string(r))
				count++
			}
			lst = lst.cdr
			idx++
		}
		return vnum(float64(start + count)), nil
	default:
		return nil, fmt.Errorf("read-sequence: not a sequence")
	}
}

func builtinWriteSequence(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("write-sequence: need a sequence")
	}
	seq := args[0]
	stream := stdoutStream
	if len(args) > 1 {
		stream = args[1]
		if stream.typ != VStream {
			return nil, fmt.Errorf("write-sequence: not a stream")
		}
	}
	start := 0
	end := -1
	for i := 2; i+1 < len(args); i += 2 {
		key := primaryValue(args[i])
		if key.typ == VSym {
			switch key.str {
			case ":START":
				start = int(primaryValue(args[i+1]).num)
			case ":END":
				end = int(primaryValue(args[i+1]).num)
			}
		}
	}
	switch seq.typ {
	case VStr:
		s := seq.str
		if end < 0 || end > len(s) {
			end = len(s)
		}
		if start < 0 {
			start = 0
		}
		if err := stream.stream.writeString(s[start:end]); err != nil {
			return nil, err
		}
		stream.stream.flush()
		return seq, nil
	case VArray:
		arr := seq.array
		total := len(arr.elements)
		if end < 0 || end > total {
			end = total
		}
		if start < 0 {
			start = 0
		}
		for i := start; i < end; i++ {
			if err := stream.stream.writeString(ToString(primaryValue(arr.elements[i]))); err != nil {
				return nil, err
			}
		}
		stream.stream.flush()
		return seq, nil
	case VPair:
		lst := seq
		seen := make(map[*Value]bool)
		idx := 0
		for !isNil(lst) {
			if seen[lst] {
				break
			}
			seen[lst] = true
			if idx >= start && (end < 0 || idx < end) {
				if err := stream.stream.writeString(ToString(primaryValue(lst.car))); err != nil {
					return nil, err
				}
			}
			lst = lst.cdr
			idx++
		}
		stream.stream.flush()
		return seq, nil
	default:
		return nil, fmt.Errorf("write-sequence: not a sequence")
	}
}

// subtypepChecks recursively checks subtype relationship between two type specifier Values.
// It returns (isKnown, isSubtype).
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

var traceTable = make(map[string]bool)
var traceDepth = 0

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

func builtinMakeCondVar(args []*Value) (*Value, error) {
	cid := atomic.AddInt64(&nextCondID, 1)
	condMu.Lock()
	// Create a condition variable associated with no specific lock initially
	// When wait is called with a lock, we associate the cond with that lock
	condVars[cid] = sync.NewCond(&sync.Mutex{})
	condMu.Unlock()
	return &Value{typ: VCondition, num: float64(cid)}, nil
}

func builtinConditionWait(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("condition-wait: need condition and lock")
	}
	if args[0].typ != VCondition || args[1].typ != VLock {
		return nil, fmt.Errorf("condition-wait: need condition and lock objects")
	}
	cid := int64(args[0].num)
	lid := int64(args[1].num)
	// Release the user lock, wait on condition, then reacquire
	condMu.Lock()
	cv, ok := condVars[cid]
	condMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("condition-wait: invalid condition")
	}
	// The condition variable uses its own internal mutex for signaling
	cv.L.Lock()
	// Signal that we're about to wait (release user lock)
	lockMapMu.Lock()
	userMu, ok2 := lockMutexMap[lid]
	lockMapMu.Unlock()
	if !ok2 {
		return nil, fmt.Errorf("condition-wait: invalid lock")
	}
	userMu.Unlock() // Release the user lock
	cv.Wait()       // Wait on condition
	userMu.Lock()   // Reacquire the user lock
	return vnil(), nil
}

func builtinConditionNotify(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VCondition {
		return nil, fmt.Errorf("condition-notify: need a condition object")
	}
	cid := int64(args[0].num)
	condMu.Lock()
	cv, ok := condVars[cid]
	condMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("condition-notify: invalid condition")
	}
	cv.Signal()
	return vnil(), nil
}

func builtinConditionBroadcast(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VCondition {
		return nil, fmt.Errorf("condition-broadcast: need a condition object")
	}
	cid := int64(args[0].num)
	condMu.Lock()
	cv, ok := condVars[cid]
	condMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("condition-broadcast: invalid condition")
	}
	cv.Broadcast()
	return vnil(), nil
}

// Thread/Lock/Condition predicates
func builtinThreadP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VThread), nil
}

func builtinLockP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VLock), nil
}

func builtinCondVarP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	return vbool(args[0].typ == VCondition), nil
}

// Atomic operations
func builtinAtomicIncf(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("atomic-incf: need a reference")
	}
	delta := int64(1)
	if len(args) >= 2 {
		delta = int64(primaryValue(args[1]).num)
	}
	newVal := atomic.AddInt64(&atomicCounter, delta)
	return vnum(float64(newVal)), nil
}

func builtinAtomicDecf(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("atomic-decf: need a reference")
	}
	delta := int64(1)
	if len(args) >= 2 {
		delta = int64(primaryValue(args[1]).num)
	}
	newVal := atomic.AddInt64(&atomicCounter, -delta)
	return vnum(float64(newVal)), nil
}

func builtinAtomicGet(args []*Value) (*Value, error) {
	return vnum(float64(atomic.LoadInt64(&atomicCounter))), nil
}

func builtinAtomicSet(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("atomic-set: need a value")
	}
	val := int64(primaryValue(args[0]).num)
	atomic.StoreInt64(&atomicCounter, val)
	return vnum(float64(val)), nil
}

func builtinEval(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("eval: need expression")
	}
	return Eval(args[0], globalEnv)
}

func builtinEvalString(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("eval-string: need a string")
	}
	s := primaryValue(args[0])
	if s.typ != VStr {
		return nil, fmt.Errorf("eval-string: need a string, got %s", ToString(s))
	}
	return EvalString(s.str, globalEnv)
}

func builtinHandlerEval(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("handler-eval: need expression")
	}
	result, err := Eval(args[0], globalEnv)
	if err != nil {
		// Signal as a condition so handler-case can catch it
		return builtinError([]*Value{vstr(err.Error())})
	}
	return result, nil
}

func builtinFuncall(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("funcall: need function")
	}
	fn := args[0]
	callArgs := args[1:]
	return callFnOnSeq(fn, callArgs, globalEnv)
}

// eql: like eq, but numbers and characters with same value are equal
func builtinEql(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	return vbool(eqVal(args[0], args[1])), nil
}

// eq: identity equality (pointer/symbol)
func builtinEqIdentity(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	a, b := args[0], args[1]
	if a == b {
		return vbool(true), nil
	}
	// In CL, nil (symbol) and () (empty list/VNil) are eq
	if isNil(a) || isNil(b) {
		if (a.typ == VSym && strings.EqualFold(a.str, "nil") && b.typ == VNil) ||
			(b.typ == VSym && strings.EqualFold(b.str, "nil") && a.typ == VNil) {
			return vbool(true), nil
		}
	}
	return vbool(false), nil
}

// equal: structural equality (recursive)
func builtinEqual(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	return vbool(eqVal(args[0], args[1])), nil
}

// equalp: case-insensitive string/char comparison, numeric equality, recursive
func builtinEqualp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	return vbool(equalpVal(args[0], args[1])), nil
}

func equalpVal(a, b *Value) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// Numbers: same mathematical value
	if isNumeric(a) && isNumeric(b) {
		return equalpNumeric(a, b)
	}
	// Characters: case-insensitive
	if a.typ == VChar && b.typ == VChar {
		ac, bc := unicode.ToLower(a.ch), unicode.ToLower(b.ch)
		return ac == bc
	}
	// Strings: case-insensitive
	if a.typ == VStr && b.typ == VStr {
		return strings.EqualFold(a.str, b.str)
	}
	// Lists: recursively compare
	if a.typ == VPair && b.typ == VPair {
		return equalpVal(a.car, b.car) && equalpVal(a.cdr, b.cdr)
	}
	// Symbols: case-insensitive
	if a.typ == VSym && b.typ == VSym {
		return strings.EqualFold(a.str, b.str)
	}
	// Arrays: element-wise comparison
	if a.typ == VArray && b.typ == VArray {
		return equalpArray(a, b)
	}
	// Otherwise use eql
	return eqVal(a, b)
}

func equalpNumeric(a, b *Value) bool {
	switch a.typ {
	case VNum:
		switch b.typ {
		case VNum:
			return a.num == b.num
		case VRat:
			return float64(a.num) == float64(b.irat)/float64(b.iden)
		}
	case VRat:
		switch b.typ {
		case VNum:
			return float64(a.irat)/float64(a.iden) == b.num
		case VRat:
			return a.irat == b.irat && a.iden == b.iden
		}
	case VComplex:
		if b.typ == VComplex {
			return a.num == b.num && a.imag == b.imag
		}
	}

	return false
}

// -------- CLOS --------

// c3Linearize computes the C3 class precedence list
// parents is a list of VClass values in declaration order
func c3Linearize(c *Value, parents []*Value) []*Value {
	if len(parents) == 0 {
		return []*Value{c}
	}
	// Build the merge lists: each parent's CPL + the direct parents list
	lists := make([][]*Value, 0, len(parents)+1)
	for _, p := range parents {
		if p.typ == VClass {
			lists = append(lists, p.cpl)
		}
	}
	// Add direct parents as a list
	direct := make([]*Value, len(parents))
	copy(direct, parents)
	lists = append(lists, direct)

	result := []*Value{c}
	// C3 merge
	for {
		// Find a candidate: first element of any list that's not in the tail of any list
		candidate := -1
		for i, lst := range lists {
			if len(lst) == 0 {
				continue
			}
			cand := lst[0]
			inTail := false
			for j, lst2 := range lists {
				if j == i || len(lst2) <= 1 {
					continue
				}
				for k := 1; k < len(lst2); k++ {
					if lst2[k] == cand {
						inTail = true
						break
					}
				}
				if inTail {
					break
				}
			}
			if !inTail {
				candidate = i
				break
			}
		}
		if candidate < 0 {
			// All lists consumed or inconsistency
			break
		}
		cand := lists[candidate][0]
		result = append(result, cand)
		// Remove cand from all lists
		for i := range lists {
			if len(lists[i]) > 0 && lists[i][0] == cand {
				lists[i] = lists[i][1:]
			}
		}
	}
	return result
}

func lispToReflect(v *Value, t reflect.Type) reflect.Value {
	switch v.typ {
	case VNum:
		switch t.Kind() {
		case reflect.Float64:
			return reflect.ValueOf(v.num)
		case reflect.Float32:
			return reflect.ValueOf(float32(v.num))
		case reflect.Int:
			return reflect.ValueOf(int(v.num))
		case reflect.Int64:
			return reflect.ValueOf(int64(v.num))
		case reflect.Uint:
			return reflect.ValueOf(uint(v.num))
		default:
			return reflect.ValueOf(v.num)
		}
	case VRat:
		f := float64(v.irat) / float64(v.iden)
		switch t.Kind() {
		case reflect.Float64:
			return reflect.ValueOf(f)
		case reflect.Float32:
			return reflect.ValueOf(float32(f))
		case reflect.Int:
			return reflect.ValueOf(int(f))
		case reflect.Int64:
			return reflect.ValueOf(int64(f))
		default:
			return reflect.ValueOf(f)
		}
	case VBigInt:
		if v.bigInt != nil {
			f, _ := new(big.Float).SetInt(v.bigInt).Float64()
			return reflect.ValueOf(f)
		}
		return reflect.ValueOf(0.0)
	case VComplex:
		return reflect.ValueOf(v.num)
	case VChar:
		return reflect.ValueOf(string(v.ch))
	case VStr:
		return reflect.ValueOf(v.str)
	case VBool:
		return reflect.ValueOf(v == globalEnv.bindings["#t"])
	default:
		return reflect.ValueOf(ToString(v))
	}
}

func reflectToLisp(v reflect.Value) *Value {
	switch v.Kind() {
	case reflect.Float64, reflect.Float32:
		return vnum(v.Float())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return vnum(float64(v.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return vnum(float64(v.Uint()))
	case reflect.String:
		return vstr(v.String())
	case reflect.Bool:
		return vbool(v.Bool())
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return vstr(string(v.Bytes()))
		}
		return vnil()
	default:
		return vstr(fmt.Sprint(v.Interface()))
	}
}
