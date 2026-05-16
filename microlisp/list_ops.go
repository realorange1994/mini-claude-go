package microlisp

import (
	"fmt"
	"strings"
	"unsafe"
)

func builtinRest(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	if args[0] == nil || args[0].typ != VPair {
		return vnil(), nil
	}
	return args[0].cdr, nil
}

func builtinFirst(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	if v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinSecond(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 1; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinThird(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 2; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinFourth(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 3; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinFifth(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 4; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinSixth(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 5; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinSeventh(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 6; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinEighth(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 7; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinNinth(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 8; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinTenth(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	for i := 0; i < 9; i++ {
		if v == nil || v.typ != VPair {
			return vnil(), nil
		}
		v = v.cdr
	}
	if v != nil && v.typ == VPair {
		return v.car, nil
	}
	return vnil(), nil
}

func builtinNullP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("null?: need 1 argument")
	}
	return vbool(isNil(args[0])), nil
}

func builtinPairP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("pair?: need 1 argument")
	}
	return vbool(isPair(args[0])), nil
}

func builtinListP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("listp: need 1 argument")
	}
	v := args[0]
	return vbool(isNil(v) || v.typ == VPair), nil
}

func builtinNumP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("number?: need 1 argument")
	}
	return vbool(args[0].typ == VNum || args[0].typ == VRat || args[0].typ == VComplex), nil
}

func builtinStrP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string?: need 1 argument")
	}
	return vbool(args[0].typ == VStr), nil
}

func builtinSymP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("symbol?: need 1 argument")
	}
	return vbool(args[0].typ == VSym), nil
}

func builtinStringP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("stringp: need 1 argument")
	}
	return vbool(args[0].typ == VStr), nil
}

func builtinBoolP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("boolean?: need 1 argument")
	}
	return vbool(args[0].typ == VBool), nil
}

func builtinProcP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("procedure?: need 1 argument")
	}
	return vbool(args[0].typ == VPrim || args[0].typ == VFunc), nil
}

func builtinCharP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("character?: need 1 argument")
	}
	return vbool(args[0].typ == VChar), nil
}

func builtinNumberP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("numberp: need 1 argument")
	}
	return vbool(args[0].typ == VNum), nil
}

func eqVal(a, b *Value) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// In CL, nil (symbol) and () (empty list/VNil) are equal
	if a.typ == VNil && b.typ == VNil {
		return true
	}
	if a.typ == VNil && b.typ == VSym && strings.EqualFold(b.str, "nil") ||
		b.typ == VNil && a.typ == VSym && strings.EqualFold(a.str, "nil") {
		return true
	}
	if a.typ != b.typ {
		return false
	}
	return eqValSeen(a, b, make(map[[2]uintptr]bool))
}

func eqValSeen(a, b *Value, seen map[[2]uintptr]bool) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.typ != b.typ {
		return false
	}
	switch a.typ {
	case VNum:
		return a.num == b.num
	case VRat:
		return a.irat == b.irat && a.iden == b.iden
	case VComplex:
		return a.num == b.num && a.imag == b.imag
	case VStr:
		return a.str == b.str
	case VSym:
		return a.str == b.str
	case VChar:
		return a.ch == b.ch
	case VPackage:
		return a.pkg == b.pkg
	case VReadtable:
		return a.readtable == b.readtable
	case VBool, VNil, VVHash:
		return a == b
	case VPair:
		ka := [2]uintptr{uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(b))}
		if seen[ka] {
			return true
		}
		seen[ka] = true
		return eqValSeen(a.car, b.car, seen) && eqValSeen(a.cdr, b.cdr, seen)
	case VArray:
		if a.array == nil || b.array == nil {
			return a.array == b.array
		}
		if len(a.array.elements) != len(b.array.elements) {
			return false
		}
		for i := range a.array.elements {
			if !eqValSeen(a.array.elements[i], b.array.elements[i], seen) {
				return false
			}
		}
		return true
	}
	return false
}

func bindPattern(pattern *Value, val *Value, env *Env) error {
	return bindPatternRec(pattern, val, env, make(map[*Value]bool))
}

func bindPatternRec(pattern *Value, val *Value, env *Env, seen map[*Value]bool) error {
	if pattern.typ == VSym {
		// Simple variable: bind the whole value
		env.Set(pattern.str, val)
		return nil
	}
	if pattern.typ != VPair {
		return fmt.Errorf("destructuring-bind: invalid pattern")
	}
	// List pattern: bind each element
	// Handle lambda-list keywords: &rest, &optional, &key
	vp := pattern
	vv := val
	localSeen := make(map[*Value]bool)
	for !isNil(vp) {
		if localSeen[vp] {
			return fmt.Errorf("destructuring-bind: circular pattern")
		}
		localSeen[vp] = true
		// Check for dotted pair (rest parameter) - must come before &rest handling
		if !isNil(vp) && vp.typ == VSym {
			// Dotted pair: (a b . rest)
			env.Set(vp.str, vv)
			return nil
		}
		if vp.typ != VPair {
			break
		}
		head := vp.car
		// Skip &rest, &optional, &key keywords and handle them specially
		if head != nil && head.typ == VSym {
			symName := strings.ToUpper(head.str)
			if symName == "&REST" || symName == "&BODY" {
				// &rest var: bind var to the remaining value list
				restVar := vp.cdr
				if restVar == nil || restVar.typ != VPair || restVar.car == nil || restVar.car.typ != VSym {
					return fmt.Errorf("destructuring-bind: malformed &rest pattern")
				}
				env.Set(restVar.car.str, vv)
				return nil
			}
			if symName == "&OPTIONAL" || symName == "&KEY" {
				// Process &optional vars: bind from remaining values
				if symName == "&OPTIONAL" {
					vp = vp.cdr
					for !isNil(vp) {
						if vp.typ != VPair {
							break
						}
						elem := vp.car
						// Check if we've hit another lambda-list keyword
						if elem != nil && elem.typ == VSym {
							elemUpper := strings.ToUpper(elem.str)
							if elemUpper == "&REST" || elemUpper == "&BODY" || elemUpper == "&KEY" || elemUpper == "&AUX" || elemUpper == "&ALLOW-OTHER-KEYS" || elemUpper == "&ENVIRONMENT" || elemUpper == "&WHOLE" {
								break // let outer loop handle it
							}
						}
						if elem == nil || elem.typ == VNil {
							// nil element, skip
						} else if elem.typ == VSym {
							// Simple optional var: bind to value or nil
							if !isNil(vv) {
								env.Set(elem.str, vv.car)
								if !isNil(vv) {
									vv = vv.cdr
								}
							} else {
								env.Set(elem.str, vnil())
							}
						} else if elem.typ == VPair {
							// (var default-value supplied-p) or (var default-value) or (var)
							varName := elem.car
							if varName != nil && varName.typ == VSym {
								var optSuppliedPSym *Value
								if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.cdr != nil && elem.cdr.cdr.typ == VPair && elem.cdr.cdr.car != nil && elem.cdr.cdr.car.typ == VSym {
									optSuppliedPSym = elem.cdr.cdr.car
								}
								if !isNil(vv) {
									env.Set(varName.str, vv.car)
									if optSuppliedPSym != nil {
										env.Set(optSuppliedPSym.str, vbool(true))
									}
									if !isNil(vv) {
										vv = vv.cdr
									}
								} else {
									var optDefault *Value
									if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.car != nil {
										optDefault = elem.cdr.car
									}
									if optDefault != nil {
										evalDef, _ := Eval(optDefault, env)
										if evalDef == nil {
											evalDef = vnil()
										}
										env.Set(varName.str, evalDef)
									} else {
										env.Set(varName.str, vnil())
									}
									if optSuppliedPSym != nil {
										env.Set(optSuppliedPSym.str, vbool(false))
									}
								}
							}
						} else {
							// Non-symbol/non-list element (e.g., number literal in default),
							// not a valid pattern variable - skip
						}
						vp = vp.cdr
					}
					// Don't return - continue outer loop to handle &rest/&key/&aux
					continue
				}
				// Process &key vars: keyword-based binding
				// Build a keyword-to-index map from the value list
				// Keywords are symbols starting with ':'; bind var matching after ':'
				vp = vp.cdr
				// Collect keyword-value pairs from the value list
				keyValMap := make(map[string]*Value)
				for !isNil(vv) && vv.typ == VPair {
					key := vv.car
					val := vnil()
					if !isNil(vv.cdr) && vv.cdr.typ == VPair {
						val = vv.cdr.car
					}
					if key != nil && key.typ == VSym && len(key.str) > 0 && key.str[0] == ':' {
						keywordName := key.str[1:] // strip leading ':'
						keyValMap[keywordName] = val
					}
					vv = vv.cdr
					if !isNil(vv) && vv.typ == VPair {
						vv = vv.cdr
					} else {
						break
					}
				}
				// Bind each key pattern variable
				for !isNil(vp) {
					if vp.typ != VPair {
						break
					}
					elem := vp.car
					if elem == nil || elem.typ == VNil {
						// nil element, skip
					} else if elem.typ == VSym {
						// Simple key var: bind matching keyword or nil
						if val, ok := keyValMap[elem.str]; ok {
							env.Set(elem.str, val)
						} else {
							env.Set(elem.str, vnil())
						}
					} else if elem.typ == VPair {
						// (var default-value) or ((:keyword var) default-value) or (:keyword var) or (:keyword var default-value)
						varName := elem.car
						if varName != nil && varName.typ == VSym {
							var keySuppliedPSym *Value
							if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.cdr != nil && elem.cdr.cdr.typ == VPair && elem.cdr.cdr.car != nil && elem.cdr.cdr.car.typ == VSym {
								keySuppliedPSym = elem.cdr.cdr.car
							}
							if val, ok := keyValMap[varName.str]; ok {
								env.Set(varName.str, val)
								if keySuppliedPSym != nil {
									env.Set(keySuppliedPSym.str, vbool(true))
								}
							} else if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.car != nil {
								evalDef, _ := Eval(elem.cdr.car, env)
								if evalDef == nil {
									evalDef = vnil()
								}
								env.Set(varName.str, evalDef)
								if keySuppliedPSym != nil {
									env.Set(keySuppliedPSym.str, vbool(false))
								}
							} else {
								env.Set(varName.str, vnil())
								if keySuppliedPSym != nil {
									env.Set(keySuppliedPSym.str, vbool(false))
								}
							}
						} else if varName != nil && varName.typ == VPair && varName.car != nil && varName.car.typ == VSym {
							// (:keyword var) form
							keywordName := varName.car.str
							if keywordName[0] == ':' {
								keywordName = keywordName[1:]
							}
							subVar := varName.cdr
							if subVar != nil && subVar.typ == VSym {
								// (:keyword var) simple form - bind var from keyword map or nil
								if val, ok := keyValMap[keywordName]; ok {
									env.Set(subVar.str, val)
								} else {
									env.Set(subVar.str, vnil())
								}
								// Handle (:keyword var default) and (:keyword var default supplied-p)
								if elem.cdr != nil && elem.cdr.typ == VPair {
									var kwDefault *Value
									var kwSuppliedP *Value
									if elem.cdr.car != nil {
										kwDefault = elem.cdr.car
									}
									if elem.cdr.cdr != nil && elem.cdr.cdr.typ == VPair && elem.cdr.cdr.car != nil && elem.cdr.cdr.car.typ == VSym {
										kwSuppliedP = elem.cdr.cdr.car
									}
									if _, ok := keyValMap[keywordName]; !ok {
										if kwDefault != nil {
											evalDef, _ := Eval(kwDefault, env)
											if evalDef == nil {
												evalDef = vnil()
											}
											env.Set(subVar.str, evalDef)
										}
										if kwSuppliedP != nil {
											env.Set(kwSuppliedP.str, vbool(false))
										}
									} else if kwSuppliedP != nil {
										env.Set(kwSuppliedP.str, vbool(true))
									}
								}
							} else if subVar != nil && subVar.typ == VPair && subVar.car != nil && subVar.car.typ == VSym {
								// subVar is a list like (VAR) - extract car
								subVar = subVar.car
								var kwSuppliedPSym *Value
								if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.cdr != nil && elem.cdr.cdr.typ == VPair && elem.cdr.cdr.car != nil && elem.cdr.cdr.car.typ == VSym {
									kwSuppliedPSym = elem.cdr.cdr.car
								}
								if val, ok := keyValMap[keywordName]; ok {
									env.Set(subVar.str, val)
									if kwSuppliedPSym != nil {
										env.Set(kwSuppliedPSym.str, vbool(true))
									}
								} else if elem.cdr != nil && elem.cdr.typ == VPair && elem.cdr.car != nil {
									evalDef2, _ := Eval(elem.cdr.car, env)
									if evalDef2 == nil {
										evalDef2 = vnil()
									}
									env.Set(subVar.str, evalDef2)
									if kwSuppliedPSym != nil {
										env.Set(kwSuppliedPSym.str, vbool(false))
									}
								} else {
									env.Set(subVar.str, vnil())
									if kwSuppliedPSym != nil {
										env.Set(kwSuppliedPSym.str, vbool(false))
									}
								}
							}
						}
					}
					vp = vp.cdr
				}
				return nil
			}
		}
		if isNil(vv) {
			// Not enough values — bind variable to nil
			if head == nil || head.typ != VSym {
				return fmt.Errorf("destructuring-bind: malformed var pattern")
			}
			env.Set(head.str, vnil())
		} else if vv.typ != VPair {
			return fmt.Errorf("destructuring-bind: expected a list, got %s", typeStr(vv))
		} else {
			if err := bindPatternRec(head, vv.car, env, seen); err != nil {
				return err
			}
		}
		vp = vp.cdr
		if !isNil(vp) && vp.typ == VSym {
			// Dotted pair: (a b . rest)
			if isNil(vv) {
				env.Set(vp.str, vnil())
			} else {
				env.Set(vp.str, vv.cdr)
			}
			return nil
		}
		if !isNil(vv) {
			vv = vv.cdr
		}
	}
	return nil
}

func builtinEqvP(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("eqv?: need 2 arguments")
	}
	a, b := args[0], args[1]
	if a.typ != b.typ {
		return vbool(false), nil
	}
	switch a.typ {
	case VNum:
		return vbool(a.num == b.num), nil
	case VRat:
		return vbool(a.str == b.str), nil
	case VComplex:
		return vbool(a.str == b.str), nil
	case VStr:
		return vbool(a.str == b.str), nil
	case VChar:
		return vbool(a.ch == b.ch), nil
	case VPackage:
		return vbool(a.pkg == b.pkg), nil
	case VReadtable:
		return vbool(a.readtable == b.readtable), nil
	default:
		return vbool(a == b), nil
	}
}

func builtinEqualP(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("equal?: need 2 arguments")
	}
	return vbool(eqVal(args[0], args[1])), nil
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

func builtinCopyList(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	elems := seqToList(args[0])
	result := make([]*Value, len(elems))
	copy(result, elems)
	return listFromSlice(result), nil
}

func copyTreeHelper(v *Value) *Value {
	return copyTreeCycle(v, make(map[*Value]bool))
}

func copyTreeCycle(v *Value, seen map[*Value]bool) *Value {
	if v == nil || v.typ != VPair {
		return v
	}
	if seen[v] {
		// Cycle detected — create a self-referencing pair by returning nil
		return vnil()
	}
	seen[v] = true
	return cons(copyTreeCycle(v.car, seen), copyTreeCycle(v.cdr, seen))
}

func builtinCopyTree(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	return copyTreeHelper(args[0]), nil
}

func builtinListLength(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnum(0), nil
	}
	count := 0
	v := args[0]
	seen := make(map[*Value]bool)
	for v != nil && v.typ == VPair {
		if seen[v] {
			return nil, fmt.Errorf("list-length: circular list")
		}
		seen[v] = true
		count++
		v = v.cdr
	}
	return vnum(float64(count)), nil
}

func builtinLast(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	n := 1
	if len(args) > 1 {
		n = int(toNum(args[1]))
	}
	v := args[0]
	var elems []*Value
	seen := make(map[*Value]bool)
	for v != nil && v.typ == VPair {
		if seen[v] {
			return nil, fmt.Errorf("last: circular list")
		}
		seen[v] = true
		elems = append(elems, v)
		v = v.cdr
	}
	if n <= 0 || len(elems) == 0 {
		return vnil(), nil
	}
	if n >= len(elems) {
		return args[0], nil
	}
	return elems[len(elems)-n], nil
}

func builtinLastPair(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	v := args[0]
	if v.typ != VPair {
		return vnil(), nil
	}
	seen := make(map[*Value]bool)
	for v.typ == VPair && !isNil(v.cdr) && v.cdr.typ == VPair {
		if seen[v] {
			return nil, fmt.Errorf("last-pair: circular list")
		}
		seen[v] = true
		v = v.cdr
	}
	return v, nil
}

func builtinButlast(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	n := 1
	if len(args) > 1 {
		n = int(toNum(args[1]))
	}
	list := args[0]
	if n <= 0 {
		// butlast with n=0 returns a copy of the list (including dotted tail)
		var result *Value = vnil()
		var tail *Value
		c := list
		for !isNil(c) && c.typ == VPair {
			newCell := cons(c.car, vnil())
			if result == nil || result.typ != VPair {
				result = newCell
				tail = newCell
			} else {
				tail.cdr = newCell
				tail = newCell
			}
			c = c.cdr
		}
		// Preserve dotted tail
		if !isNil(c) && result.typ == VPair {
			// find last pair
			last := result
			for last.cdr.typ == VPair {
				last = last.cdr
			}
			last.cdr = c
		}
		return result, nil
	}
	// n > 0: walk list counting elements
	// When n > 0, the dotted tail is part of the last cons cells being removed,
	// so we should NOT preserve it in the result.
	cur := list
	var elems []*Value
	for cur.typ == VPair {
		elems = append(elems, cur.car)
		cur = cur.cdr
	}
	if n >= len(elems) {
		return vnil(), nil
	}
	keep := len(elems) - n
	// Rebuild with first 'keep' elements as a proper list
	result := vnil()
	for i := keep - 1; i >= 0; i-- {
		result = cons(elems[i], result)
	}
	return result, nil
}

func builtinNbutlast(args []*Value) (*Value, error) {
	return builtinButlast(args)
}

func builtinPairlis(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("pairlis: need two lists")
	}
	keys := seqToList(args[0])
	vals := seqToList(args[1])
	var result []*Value
	for i := 0; i < len(keys) && i < len(vals); i++ {
		result = append(result, cons(keys[i], vals[i]))
	}
	alist := vnil()
	if len(args) >= 3 {
		alist = args[2]
	}
	// Append result to alist: (nconc result alist)
	if len(result) == 0 {
		return alist, nil
	}
	res := listFromSlice(result)
	// Find end of result list and set cdr to alist
	t := res
	for t.typ == VPair && !isNil(t.cdr) {
		t = t.cdr
	}
	t.cdr = alist
	return res, nil
}

func builtinAssoc(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("assoc: need item and alist")
	}
	item := args[0]
	alist := args[1]
	keyFn, testFn, testNotFn, _, _, _, _, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	cur := alist
	for !isNil(cur) && cur.typ == VPair {
		entry := cur.car
		// Skip nil elements (per CL spec)
		if isNil(entry) {
			cur = cur.cdr
			continue
		}
		if entry.typ == VPair {
			if testItemMatchFull(item, entry.car, testFn, testNotFn, keyFn) {
				return entry, nil
			}
		}
		cur = cur.cdr
	}
	return vnil(), nil
}

func builtinAssocIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("assoc-if: need predicate and alist")
	}
	fn := args[0]
	alist := args[1]
	seen := make(map[*Value]bool)
	for !isNil(alist) && alist.typ == VPair {
		if seen[alist] {
			break
		}
		seen[alist] = true
		pair := alist.car
		if pair.typ == VPair {
			result, err := callFnOnSeq(fn, []*Value{pair.car}, globalEnv)
			if err != nil {
				return nil, err
			}
			if isTruthy(result) {
				return pair, nil
			}
		}
		alist = alist.cdr
	}
	return vnil(), nil
}

func builtinMemberIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("member-if: need predicate and list")
	}
	fn := args[0]
	lst := args[1]
	seen := make(map[*Value]bool)
	for !isNil(lst) && lst.typ == VPair {
		if seen[lst] {
			break
		}
		seen[lst] = true
		result, err := callFnOnSeq(fn, []*Value{lst.car}, globalEnv)
		if err != nil {
			return nil, err
		}
		if isTruthy(result) {
			return lst, nil
		}
		lst = lst.cdr
	}
	return vnil(), nil
}

func builtinMemberIfNot(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("member-if-not: need predicate and list")
	}
	fn := args[0]
	lst := args[1]
	seen := make(map[*Value]bool)
	for !isNil(lst) && lst.typ == VPair {
		if seen[lst] {
			break
		}
		seen[lst] = true
		result, err := callFnOnSeq(fn, []*Value{lst.car}, globalEnv)
		if err != nil {
			return nil, err
		}
		if !isTruthy(result) {
			return lst, nil
		}
		lst = lst.cdr
	}
	return vnil(), nil
}

func builtinAssocIfNot(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("assoc-if-not: need predicate and alist")
	}
	fn := args[0]
	alist := args[1]
	seen := make(map[*Value]bool)
	for !isNil(alist) && alist.typ == VPair {
		if seen[alist] {
			break
		}
		seen[alist] = true
		pair := alist.car
		if pair.typ == VPair {
			result, err := callFnOnSeq(fn, []*Value{pair.car}, globalEnv)
			if err != nil {
				return nil, err
			}
			if !isTruthy(result) {
				return pair, nil
			}
		}
		alist = alist.cdr
	}
	return vnil(), nil
}

func builtinRassocIfNot(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("rassoc-if-not: need predicate and alist")
	}
	fn := args[0]
	alist := args[1]
	cur := alist
	for cur != nil && cur.typ == VPair {
		entry := cur.car
		if entry.typ == VPair {
			result, err := callFnOnSeq(fn, []*Value{entry.cdr}, globalEnv)
			if err != nil {
				return nil, err
			}
			if !isTruthy(result) {
				return entry, nil
			}
		}
		cur = cur.cdr
	}
	return vnil(), nil
}

func builtinMember(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("member: need item and list")
	}
	item := args[0]
	lst := args[1]
	if lst.typ != VPair && lst.typ != VNil {
		return nil, fmt.Errorf("member: expected a proper list")
	}
	keyFn, testFn, testNotFn, _, _, _, _, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	seen := make(map[*Value]bool)
	for !isNil(lst) && lst.typ == VPair {
		if seen[lst] {
			break
		}
		seen[lst] = true
		el := lst.car
		if testItemMatchFull(item, el, testFn, testNotFn, keyFn) {
			return lst, nil
		}
		lst = lst.cdr
	}
	return vnil(), nil
}

func builtinPosition(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("position: need item and sequence")
	}
	item := args[0]
	seq := args[1]
	if seq.typ != VStr {
		if err := checkSequenceArg(seq, "position"); err != nil {
			return nil, err
		}
	}
	start, end := 0, -1
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, e := safeToNum(args[i], "position")
					if e != nil {
						return nil, e
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, e := safeToNum(args[i], "position")
					if e != nil {
						return nil, e
					}
					end = int(n)
				}
			}
		}
	}
	if seq.typ == VStr {
		s := seq.str
		runes := []rune(s)
		var targetCh rune
		if item.typ == VChar {
			targetCh = item.ch
		} else {
			return vnil(), nil
		}
		if end < 0 || end > len(runes) {
			end = len(runes)
		}
		for i := start; i < end; i++ {
			if runes[i] == targetCh {
				return vnum(float64(i)), nil
			}
		}
		return vnil(), nil
	}
	elems := seqToList(seq)
	if end < 0 || end > len(elems) {
		end = len(elems)
	}
	for i := start; i < end; i++ {
		if eqVal(elems[i], item) {
			return vnum(float64(i)), nil
		}
	}
	return vnil(), nil
}

func builtinPositionIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("position-if: need predicate and sequence")
	}
	fn := args[0]
	seq := args[1]
	elems := seqToList(seq)
	for i, el := range elems {
		result, err := callFnOnSeq(fn, []*Value{el}, globalEnv)
		if err != nil {
			return nil, err
		}
		if isTruthy(result) {
			return vnum(float64(i)), nil
		}
	}
	return vnil(), nil
}

func builtinAcons(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("acons: need key, value, and alist")
	}
	key := args[0]
	val := args[1]
	alist := vnil()
	if len(args) >= 3 {
		alist = args[2]
	}
	return cons(cons(key, val), alist), nil
}

func builtinRassoc(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("rassoc: need item and alist")
	}
	item := args[0]
	alist := args[1]
	pred := vnil()
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":TEST" && i+1 < len(args) {
			i++
			pred = args[i]
		}
	}
	cur := alist
	for cur != nil && cur.typ == VPair {
		entry := cur.car
		if entry.typ == VPair {
			if pred.typ != VNil {
				cmp, err := callFnOnSeq(pred, []*Value{entry.cdr, item}, globalEnv)
				if err == nil && isTruthy(cmp) {
					return entry, nil
				}
			} else {
				if eqVal(entry.cdr, item) {
					return entry, nil
				}
			}
		}
		cur = cur.cdr
	}
	return vnil(), nil
}

func builtinRassocIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("rassoc-if: need predicate and alist")
	}
	fn := args[0]
	alist := args[1]
	cur := alist
	for cur != nil && cur.typ == VPair {
		entry := cur.car
		if entry.typ == VPair {
			result, err := callFnOnSeq(fn, []*Value{entry.cdr}, globalEnv)
			if err == nil && isTruthy(result) {
				return entry, nil
			}
		}
		cur = cur.cdr
	}
	return vnil(), nil
}

func builtinNth(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("nth: need index and list")
	}
	n, err := safeToNum(args[0], "nth")
	if err != nil {
		return nil, err
	}
	lst := args[1]
	cur := lst
	for i := 0; i < int(n); i++ {
		if cur == nil || cur.typ != VPair {
			return vnil(), nil
		}
		cur = cur.cdr
	}
	if cur == nil || cur.typ != VPair {
		return vnil(), nil
	}
	return cur.car, nil
}

func builtinNthCdr(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("nthcdr: need index and list")
	}
	n, err := safeToNum(args[0], "nthcdr")
	if err != nil {
		return nil, err
	}
	lst := args[1]
	cur := lst
	for i := 0; i < int(n); i++ {
		if cur == nil || cur.typ != VPair {
			return vnil(), nil
		}
		cur = cur.cdr
	}
	return cur, nil
}
func builtinNreconc(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("nreconc: need two lists")
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	// Reverse list1 and prepend to list2
	for i, j := 0, len(list1)-1; i < j; i, j = i+1, j-1 {
		list1[i], list1[j] = list1[j], list1[i]
	}
	result := make([]*Value, len(list1)+len(list2))
	copy(result, list1)
	copy(result[len(list1):], list2)
	return listFromSlice(result), nil
}

// -------- concatenate (already exists) --------

// -------- mapl / maplist / mapc / mapcon --------
func builtinMaplist(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("maplist: need function and list")
	}
	fn := args[0]
	lst := args[1]
	var results []*Value
	seen := make(map[*Value]bool)
	cur := lst
	for cur != nil && cur.typ == VPair {
		if seen[cur] {
			break
		}
		seen[cur] = true
		r, err := callFnOnSeq(fn, []*Value{cur}, globalEnv)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
		cur = cur.cdr
	}
	return listFromSlice(results), nil
}

func builtinMapc(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("mapc: need function and list")
	}
	fn := args[0]
	lst := args[1]
	seen := make(map[*Value]bool)
	cur := lst
	for cur != nil && cur.typ == VPair {
		if seen[cur] {
			break
		}
		seen[cur] = true
		_, err := callFnOnSeq(fn, []*Value{cur.car}, globalEnv)
		if err != nil {
			return nil, err
		}
		cur = cur.cdr
	}
	return lst, nil
}

func builtinMapl(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("mapl: need function and list")
	}
	fn := args[0]
	lst := args[1]
	seen := make(map[*Value]bool)
	cur := lst
	for cur != nil && cur.typ == VPair {
		if seen[cur] {
			break
		}
		seen[cur] = true
		_, err := callFnOnSeq(fn, []*Value{cur}, globalEnv)
		if err != nil {
			return nil, err
		}
		cur = cur.cdr
	}
	return lst, nil
}

func builtinMapcon(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("mapcon: need function and list")
	}
	fn := args[0]
	lst := args[1]
	seen := make(map[*Value]bool)
	cur := lst
	// nconc-2: append list2 to end of list1, returning list1 (destructive)
	nconc2 := func(list1, list2 *Value) *Value {
		if isNil(list2) {
			return list1
		}
		if isNil(list1) {
			return list2
		}
		t := list1
		for t.typ == VPair && !isNil(t.cdr) {
			t = t.cdr
		}
		t.cdr = list2
		return list1
	}
	var result *Value
	for !isNil(cur) && cur.typ == VPair {
		if seen[cur] {
			break
		}
		seen[cur] = true
		r, err := callFnOnSeq(fn, []*Value{cur}, globalEnv)
		if err != nil {
			return nil, err
		}
		result = nconc2(result, r)
		cur = cur.cdr
	}
	if result == nil {
		return vnil(), nil
	}
	return result, nil
}

// -------- list* --------
func builtinListStar(args []*Value) (*Value, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("list*: need at least one argument")
	}
	if len(args) == 1 {
		return args[0], nil
	}
	result := make([]*Value, len(args)-1)
	for i := 0; i < len(args)-1; i++ {
		result[i] = args[i]
	}
	return appendList(listFromSlice(result), args[len(args)-1]), nil
}

// appendList appends a tail to a proper list
func appendList(lst, tail *Value) *Value {
	if isNil(lst) {
		return tail
	}
	seen := make(map[*Value]bool)
	return appendListRec(lst, tail, seen)
}
func appendListRec(lst, tail *Value, seen map[*Value]bool) *Value {
	if isNil(lst) {
		return tail
	}
	if seen[lst] {
		return tail // break cycle
	}
	seen[lst] = true
	if lst.typ == VPair {
		return cons(lst.car, appendListRec(lst.cdr, tail, seen))
	}
	return tail
}
func builtinCopyAlist(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("copy-alist: need an alist")
	}
	alist := args[0]
	if isNil(alist) {
		return vnil(), nil
	}
	var result *Value = vnil()
	elems := seqToList(alist)
	for i := len(elems) - 1; i >= 0; i-- {
		entry := elems[i]
		if entry.typ == VPair {
			result = cons(cons(entry.car, entry.cdr), result)
		} else {
			result = cons(entry, result)
		}
	}
	return result, nil
}
