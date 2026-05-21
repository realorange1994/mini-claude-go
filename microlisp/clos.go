package microlisp

import (
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

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

func subtypepCheck(typeA, typeB string, env *Env) bool {
	return typeIsSubtype(strings.ToUpper(typeA), strings.ToUpper(typeB))
}

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
	// Special case: time.Time — convert to an inspectable VGoVal that preserves
	// the full value including timezone. Without this, time.Time round-trips
	// through interface{} can lose the *time.Location pointer, causing UTC↔local
	// mismatches (e.g. NotBefore=UTC but NotAfter=CST).
	if v.IsValid() && v.Type() == reflect.TypeOf(time.Time{}) {
		t := v.Interface().(time.Time)
		return &Value{
			typ:        VGoVal,
			goVal:      t,
			goValType:  reflect.TypeOf(time.Time{}),
			goValReflect: v,
		}
	}
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
	case reflect.Complex64:
		c := v.Complex()
		return vcomplex(float64(real(c)), float64(imag(c)))
	case reflect.Complex128:
		c := v.Complex()
		return vcomplex(real(c), imag(c))
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return vstr(string(v.Bytes()))
		}
		// Convert []string to Lisp list of strings
		if v.Type().Elem().Kind() == reflect.String {
			n := v.Len()
			result := vnil()
			for i := n - 1; i >= 0; i-- {
				result = cons(vstr(v.Index(i).String()), result)
			}
			return result
		}
		// For other slice types, wrap as VGoVal to preserve the value
		return vgoval(v.Interface(), v.Type())
	default:
		// For interfaces, pointers, structs, maps, arrays, channels:
		// create a VGoVal that preserves the actual Go value and type.
		return vgoval(v.Interface(), v.Type())
	}
}
func builtinCopyStructure(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("copy-structure: need an instance")
	}
	inst := args[0]
	if inst.typ != VInstance {
		return nil, fmt.Errorf("copy-structure: expected a structure instance, got %s", typeStr(inst))
	}
	newSlots := make(map[string]*Value, len(inst.instSlots))
	for k, v := range inst.instSlots {
		newSlots[k] = v // shallow copy of slots
	}
	result := gcv()
	result.typ = VInstance
	result.instClass = inst.instClass
	result.instSlots = newSlots
	return result, nil
}
