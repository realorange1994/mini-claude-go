package microlisp

import (
	"bufio"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
	"unsafe"
)

// -------- List accessors --------
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

func builtinChar(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("char: need string and index")
	}
	if args[0].typ != VStr {
		return nil, fmt.Errorf("char: expected a string")
	}
	if args[1].typ != VNum {
		return nil, fmt.Errorf("char: expected an integer")
	}
	idx := int(args[1].num)
	s := args[0].str
	runes := []rune(s)
	if idx < 0 || idx >= len(runes) {
		return nil, fmt.Errorf("char: index %d out of range", idx)
	}
	return vchar(runes[idx]), nil
}

func builtinCharSetf(args []*Value) (*Value, error) {
	// (char-setf newchar string index)
	if len(args) < 3 {
		return nil, fmt.Errorf("char-setf: need newchar, string, and index")
	}
	newChar := args[0]
	s := args[1]
	idx := args[2]
	if newChar.typ != VChar {
		return nil, fmt.Errorf("char-setf: expected a character for new value")
	}
	if s.typ != VStr {
		return nil, fmt.Errorf("char-setf: expected a string")
	}
	if idx.typ != VNum {
		return nil, fmt.Errorf("char-setf: expected an integer index")
	}
	i := int(idx.num)
	runes := []rune(s.str)
	if i < 0 || i >= len(runes) {
		return nil, fmt.Errorf("char-setf: index %d out of range", i)
	}
	runes[i] = newChar.ch
	s.str = string(runes)
	return newChar, nil
}

func charCompare(op string, args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("char%s: expected at least 2 characters", op)
	}
	runes := make([]rune, len(args))
	for i, a := range args {
		if a.typ != VChar {
			return nil, fmt.Errorf("char%s: expected a character", op)
		}
		runes[i] = a.ch
	}
	for i := 0; i < len(runes)-1; i++ {
		a, b := runes[i], runes[i+1]
		switch op {
		case "=":
			if a != b {
				return vbool(false), nil
			}
		case "<":
			if !(a < b) {
				return vbool(false), nil
			}
		case ">":
			if !(a > b) {
				return vbool(false), nil
			}
		case "<=":
			if !(a <= b) {
				return vbool(false), nil
			}
		case ">=":
			if !(a >= b) {
				return vbool(false), nil
			}
		}
	}
	return vbool(true), nil
}

func builtinCharEq(args []*Value) (*Value, error) { return charCompare("=", args) }
func builtinCharLt(args []*Value) (*Value, error) { return charCompare("<", args) }
func builtinCharGt(args []*Value) (*Value, error) { return charCompare(">", args) }
func builtinCharLe(args []*Value) (*Value, error) { return charCompare("<=", args) }
func builtinCharGe(args []*Value) (*Value, error) { return charCompare(">=", args) }

func builtinCodeChar(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("code-char: need an integer")
	}
	if args[0].typ != VNum {
		return nil, fmt.Errorf("code-char: expected an integer")
	}
	n := int(args[0].num)
	if n < 0 || n > 0x10FFFF {
		return vnil(), nil
	}
	return vchar(rune(n)), nil
}

func builtinCharCode(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("char-code: need a character")
	}
	if args[0].typ != VChar {
		return nil, fmt.Errorf("char-code: expected a character")
	}
	return vnum(float64(args[0].ch)), nil
}

func builtinBitAref(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("bit: need bit-vector and index")
	}
	arr := args[0]
	if arr.typ != VArray {
		return nil, fmt.Errorf("bit: not a bit-vector")
	}
	if !isBitVector(arr.array) {
		return nil, fmt.Errorf("bit: not a bit-vector")
	}
	idx := int(toNum(args[1]))
	if idx < 0 || idx >= arr.array.dims[0] {
		return nil, fmt.Errorf("bit: index %d out of range", idx)
	}
	return arr.array.elements[idx], nil
}

func builtinSbitAref(args []*Value) (*Value, error) {
	// sbit is like bit but for simple bit-vectors (same as bit in microlisp)
	return builtinBitAref(args)
}

func builtinCharInt(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("char-int: need a character")
	}
	if args[0].typ != VChar {
		return nil, fmt.Errorf("char-int: expected a character")
	}
	// In most implementations, char-int is the same as char-code
	return vnum(float64(args[0].ch)), nil
}

// Standard character names
var charNameMap = map[string]rune{
	"space":     ' ',
	"newline":   '\n',
	"tab":       '\t',
	"return":    '\r',
	"backspace": '\x08',
	"bell":      '\x07',
	"page":      '\f',
	"escape":    '\x1b',
	"rubout":    '\x7f',
	"null":      '\x00',
	// Additional standard ASCII control character names
	"soh": '\x01',
	"stx": '\x02',
	"etx": '\x03',
	"eot": '\x04',
	"enq": '\x05',
	"ack": '\x06',
	"nak": '\x15',
	"syn": '\x16',
	"etb": '\x17',
	"can": '\x18',
	"em":  '\x19',
	"sub": '\x1a',
	"fs":  '\x1c',
	"gs":  '\x1d',
	"rs":  '\x1e',
	"us":  '\x1f',
	"del": '\x7f',
	// Common abbreviations
	"xoff": '\x13',
	"xon":  '\x11',
}

func builtinNameChar(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("name-char: need a string")
	}
	name := args[0]
	var nameStr string
	switch name.typ {
	case VStr:
		nameStr = name.str
	case VSym:
		nameStr = name.str
	case VChar:
		// If already a char, return it
		return name, nil
	default:
		return vnil(), nil
	}
	nameStr = strings.ToLower(nameStr)
	if ch, ok := charNameMap[nameStr]; ok {
		return vchar(ch), nil
	}
	return vnil(), nil
}

func builtinCharName(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VChar {
		return nil, fmt.Errorf("char-name: expected a character")
	}
	ch := args[0].ch
	// CL spec: code 127 is named "Rubout" (not "Del" which is an implementation-dependent synonym)
	if ch == 127 {
		return vstr("Rubout"), nil
	}
	// Check named characters (return capitalized name per CL spec)
	for name, r := range charNameMap {
		if r == ch {
			// Capitalize first letter
			return vstr(strings.ToUpper(name[:1]) + name[1:]), nil
		}
	}
	// For control characters (C0: 0-31) that don't have specific names
	if ch < 32 && ch >= 0 {
		// Check if already in charNameMap (specific name like Newline)
		if _, ok := charNameMapRev[ch]; ok {
			// handled above
		} else {
			return vstr(fmt.Sprintf("C%d", ch)), nil
		}
	}
	// C1 control characters (128-159): return "C128", "C129", etc.
	if ch >= 128 && ch < 160 {
		return vstr(fmt.Sprintf("C%d", ch)), nil
	}
	// Other non-printing characters (e.g. NBSP, soft hyphen)
	if !unicode.IsPrint(ch) {
		return vstr("control"), nil
	}
	if ch == 127 {
		return vstr("rubout"), nil
	}
	return vnil(), nil
}

// charNameMapRev is the reverse of charNameMap (rune -> string name)
var charNameMapRev = map[rune]string{
	' ':    "space",
	'\n':   "newline",
	'\t':   "tab",
	'\r':   "return",
	'\x08': "backspace",
	'\x7f': "rubout",
	'\f':   "page",
	'\x00': "null",
	'\x07': "bell",
	'\x1b': "escape",
	'\x01': "soh",
	'\x02': "stx",
	'\x03': "etx",
	'\x04': "eot",
	'\x05': "enq",
	'\x06': "ack",
	'\x15': "nak",
	'\x16': "syn",
	'\x17': "etb",
	'\x18': "can",
	'\x19': "em",
	'\x1a': "sub",
	'\x1c': "fs",
	'\x1d': "gs",
	'\x1e': "rs",
	'\x1f': "us",
	'\x13': "xoff",
	'\x11': "xon",
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

// bindPattern binds a destructuring pattern to a value in the given env.
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

func builtinLength(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("length: need argument")
	}
	v := primaryValue(args[0])
	// In CL, nil is a valid sequence (empty list), length = 0
	if isNil(v) {
		return vnum(0), nil
	}
	if v.typ != VPair && v.typ != VStr && v.typ != VArray {
		return nil, fmt.Errorf("length: %s is not a sequence", ToString(v))
	}
	return vnum(float64(lengthSafe(v))), nil
}

func lengthSafe(v *Value) int64 {
	if v.typ == VStr {
		return int64(utf8.RuneCountInString(v.str))
	}
	if v.typ == VArray && v.array != nil {
		if v.array.fillPtr >= 0 {
			return int64(v.array.fillPtr)
		}
		return int64(len(v.array.elements))
	}
	n := int64(0)
	visited := make(map[*Value]bool)
	for !isNil(v) && v.typ == VPair {
		if visited[v] {
			break // circular list
		}
		visited[v] = true
		n++
		v = v.cdr
	}
	return n
}

func builtinDisplay(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("display: need 1 argument")
	}
	fmt.Print(ToString(args[0]))
	return args[0], nil
}

func builtinNewline(args []*Value) (*Value, error) {
	fmt.Println()
	return vnil(), nil
}

func builtinWriteToString(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("write-to-string: need argument")
	}
	return vstr(writeToString(primaryValue(args[0]))), nil
}

func builtinRead(args []*Value) (*Value, error) {
	if len(args) > 0 {
		// stream argument - return nil for now
		return vnil(), nil
	}
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return vnil(), nil
	}
	return parseExpr(strings.TrimSpace(line))
}

func builtinStr(args []*Value) (*Value, error) {
	var b strings.Builder
	for _, a := range args {
		if a.typ == VStr {
			b.WriteString(a.str)
		} else {
			b.WriteString(ToString(a))
		}
	}
	return vstr(b.String()), nil
}

func builtinStrAppend(args []*Value) (*Value, error) {
	var b strings.Builder
	for _, a := range args {
		if a.typ != VStr {
			return nil, fmt.Errorf("string-append: expected string")
		}
		b.WriteString(a.str)
	}
	return vstr(b.String()), nil
}

func builtinStrLen(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string-length: need string")
	}
	if args[0].typ != VStr {
		return nil, fmt.Errorf("string-length: expected string")
	}
	return vnum(float64(len(args[0].str))), nil
}

func builtinStrFind(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-find: need char/string and string")
	}
	if args[1].typ != VStr {
		return nil, fmt.Errorf("string-find: second arg must be a string")
	}
	var searchStr string
	if args[0].typ == VChar {
		searchStr = string(args[0].ch)
	} else if args[0].typ == VStr {
		searchStr = args[0].str
	} else {
		return nil, fmt.Errorf("string-find: first arg must be string or character")
	}
	idx := strings.Index(args[1].str, searchStr)
	if idx < 0 {
		return vnil(), nil
	}
	return vnum(float64(idx)), nil
}

func builtinSubstring(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("substring: need string and start index")
	}
	if args[0].typ != VStr {
		return nil, fmt.Errorf("substring: expected string")
	}
	s := args[0].str
	runes := []rune(s)
	start, err := safeToNum(args[1], "substring")
	if err != nil {
		return nil, err
	}
	startInt := int(start)
	var endInt int
	if len(args) >= 3 {
		en, err := safeToNum(args[2], "substring")
		if err != nil {
			return nil, err
		}
		endInt = int(en)
	} else {
		endInt = len(runes)
	}
	// Bounds checking
	if startInt < 0 {
		return nil, fmt.Errorf("substring: start index %d out of range [0..%d]", startInt, len(runes))
	}
	if endInt < 0 {
		return nil, fmt.Errorf("substring: end index %d out of range [0..%d]", endInt, len(runes))
	}
	if endInt > len(runes) {
		return nil, fmt.Errorf("substring: end index %d out of range [0..%d]", endInt, len(runes))
	}
	if startInt > endInt {
		return nil, fmt.Errorf("substring: start index %d greater than end index %d", startInt, endInt)
	}
	return vstr(string(runes[startInt:endInt])), nil
}

func builtinNumStr(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("number->string: need a number")
	}
	s := strconv.FormatFloat(toNum(args[0]), 'g', -1, 64)
	return vstr(s), nil
}

// coerceStringDesignator converts a string designator (string, symbol, character, or array)
// to a Go string, per the CL spec.
func coerceStringDesignator(v *Value) (string, error) {
	switch v.typ {
	case VStr:
		return v.str, nil
	case VSym:
		return v.str, nil
	case VChar:
		return string(v.ch), nil
	case VArray:
		return vecToString(v), nil
	default:
		return "", fmt.Errorf("expected a string designator (string, symbol, or character)")
	}
}

func builtinStrUpcase(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string-upcase: expected a string designator")
	}
	s, err := coerceStringDesignator(args[0])
	if err != nil {
		return nil, fmt.Errorf("string-upcase: %v", err)
	}
	runes := []rune(s)
	start, end := 0, len(runes)
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "string-upcase :start")
					if err != nil {
						return nil, err
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "string-upcase :end")
					if err != nil {
						return nil, err
					}
					end = int(n)
				}
			}
		}
	}
	if end > len(runes) {
		end = len(runes)
	}
	for i := start; i < end; i++ {
		runes[i] = unicode.ToUpper(runes[i])
	}
	return vstr(string(runes)), nil
}

func builtinStrDowncase(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string-downcase: expected a string designator")
	}
	s, err := coerceStringDesignator(args[0])
	if err != nil {
		return nil, fmt.Errorf("string-downcase: %v", err)
	}
	runes := []rune(s)
	start, end := 0, len(runes)
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "string-downcase :start")
					if err != nil {
						return nil, err
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "string-downcase :end")
					if err != nil {
						return nil, err
					}
					end = int(n)
				}
			}
		}
	}
	if end > len(runes) {
		end = len(runes)
	}
	for i := start; i < end; i++ {
		runes[i] = unicode.ToLower(runes[i])
	}
	return vstr(string(runes)), nil
}

func builtinStrNum(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string->number: need a string argument")
	}
	if args[0].typ != VStr {
		return nil, fmt.Errorf("string->number: expected a string")
	}
	f, err := strconv.ParseFloat(args[0].str, 64)
	if err != nil {
		return vnil(), nil
	}
	return vnum(f), nil
}

func builtinSymStr(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("symbol->string: need a symbol")
	}
	if args[0].typ != VSym {
		return nil, fmt.Errorf("symbol->string: expected symbol")
	}
	return vstr(args[0].str), nil
}

func builtinStrSym(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string->symbol: need a string")
	}
	if args[0].typ != VStr {
		return nil, fmt.Errorf("string->symbol: expected string")
	}
	return vsym(strings.ToUpper(args[0].str)), nil
}

func builtinErr(args []*Value) (*Value, error) {
	var msg string
	for _, a := range args {
		msg += ToString(a)
	}
	return nil, fmt.Errorf("%s", msg)
}

func builtinExit(args []*Value) (*Value, error) {
	code := 0
	if len(args) > 0 {
		code = int(toNum(args[0]))
	}
	os.Exit(code)
	return vnil(), nil
}

var gensymCounter int64 = 0

func builtinGensym(args []*Value) (*Value, error) {
	gensymCounter++
	prefix := "G"
	if len(args) > 0 {
		prefix = args[0].str
	}
	return vsym(fmt.Sprintf("%s_%d", prefix, gensymCounter)), nil
}

func builtinGentemp(args []*Value) (*Value, error) {
	gensymCounter++
	prefix := "T"
	if len(args) > 0 {
		prefix = args[0].str
	}
	name := fmt.Sprintf("%s%d", prefix, gensymCounter)
	return internSymbol(name, currentPackage), nil
}

func builtinLoopCheck(args []*Value) (*Value, error) {
	loopIterationCount++
	if loopIterationCount > maxLoopIterations {
		loopIterationCount = 0
		return nil, fmt.Errorf("loop iteration limit exceeded (%d)", maxLoopIterations)
	}
	return vnil(), nil
}

// -------- Sequence operations --------

// seqParseKeys extracts keyword arguments from args[startIdx:]
func seqParseKeys(args []*Value, startIdx int) (keyFn, testFn, testNotFn *Value, fromEnd bool, count, start, end int, initialVal *Value, err error) {
	keyFn = vnil()
	testFn = vnil()
	testNotFn = vnil()
	count = -1
	start = 0
	end = -1
	initialVal = vnil()
	fromEnd = false
	for i := startIdx; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":KEY":
				if i+1 < len(args) {
					i++
					keyFn = args[i]
				}
			case ":TEST":
				if i+1 < len(args) {
					i++
					testFn = args[i]
				}
			case ":TEST-NOT":
				if i+1 < len(args) {
					i++
					testNotFn = args[i]
				}
			case ":FROM-END":
				fromEnd = true
			case ":COUNT":
				if i+1 < len(args) {
					i++
					n, e := safeToNum(args[i], "keyword :count")
					if e != nil {
						err = e
						return
					}
					count = int(n)
				}
			case ":START":
				if i+1 < len(args) {
					i++
					n, e := safeToNum(args[i], "keyword :start")
					if e != nil {
						err = e
						return
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, e := safeToNum(args[i], "keyword :end")
					if e != nil {
						err = e
						return
					}
					end = int(n)
				}
			case ":INITIAL-VALUE":
				if i+1 < len(args) {
					i++
					initialVal = args[i]
				}
			}
		}
	}
	return
}

// extractKeyFromRest extracts :key from the end of a sequence args slice.
// Returns the keyFn (or nil) and a new slice without the :key keyword arg.
func extractKeyFromRest(seqs []*Value) (*Value, []*Value) {
	keyFn := (*Value)(nil)
	// Scan from the end looking for :key keyword
	for i := len(seqs) - 1; i >= 0; i-- {
		if seqs[i].typ == VSym && seqs[i].str == ":KEY" && i+1 < len(seqs) {
			keyFn = seqs[i+1]
			// Build new slice without :key and its value
			result := make([]*Value, 0, len(seqs)-2)
			result = append(result, seqs[:i]...)
			result = append(result, seqs[i+2:]...)
			return keyFn, result
		}
	}
	return nil, seqs
}

// seqToList converts a sequence (list or vector-like) to a Go []*Value slice.
// For now, only lists are supported.
func seqToList(v *Value) []*Value {
	var result []*Value
	seen := make(map[*Value]bool)
	for !isNil(v) {
		if v.typ == VPair {
			if seen[v] {
				break // circular list
			}
			seen[v] = true
			result = append(result, v.car)
			v = v.cdr
		} else if v.typ == VStr {
			// Convert string to list of character values (CL: coerce "abc" 'list → #\a #\b #\c)
			for _, r := range v.str {
				result = append(result, vchar(r))
			}
			break
		} else if v.typ == VArray && v.array != nil {
			n := len(v.array.elements)
			if v.array.fillPtr >= 0 && v.array.fillPtr < n {
				n = v.array.fillPtr
			}
			for i := 0; i < n; i++ {
				result = append(result, v.array.elements[i])
			}
			break
		} else {
			break
		}
	}
	return result
}

func callFnOnSeq(fn *Value, args []*Value, env *Env) (*Value, error) {
	// Resource safety check
	if err := stepCheck(); err != nil {
		return nil, err
	}
	// Build argument list
	argList := vnil()
	for i := len(args) - 1; i >= 0; i-- {
		argList = &Value{typ: VPair, car: args[i], cdr: argList}
	}
	// Resolve if it's a symbol naming a function
	callFn := fn
	if fn.typ == VSym {
		resolved, err := env.Get(fn.str)
		if err == nil {
			callFn = resolved
		} else {
			return nil, fmt.Errorf("callFnOnSeq: undefined function %s", fn.str)
		}
	}
	switch callFn.typ {
	case VPrim:
		return callFn.fn(args)
	case VFunc:
		// Direct apply without re-evaluating args
		if callFn.name != "" && traceTable[callFn.name] {
			indent := strings.Repeat("  ", traceDepth)
			argStrs := make([]string, len(args))
			for i, a := range args {
				argStrs[i] = ToString(primaryValue(a))
			}
			fmt.Printf("%s%d: (%s %s)\n", indent, traceDepth, callFn.name, strings.Join(argStrs, " "))

			traceDepth++
		}
		newEnv := NewEnv(callFn.env)
		numRequired := len(callFn.params) - len(callFn.optDefaults) - len(callFn.keySpecs)
		if numRequired < 0 {
			numRequired = 0
		}

		keyVals := make(map[string]*Value)
		positionalArgs := args
		if len(callFn.keySpecs) > 0 {
			var nonKeyword []*Value
			i := 0
			for i < len(args) {
				if args[i] != nil && args[i].typ == VSym && len(args[i].str) > 0 && args[i].str[0] == ':' {
					keyName := args[i].str[1:]
					if i+1 < len(args) {
						keyVals[keyName] = args[i+1]
						i += 2
					} else {
						nonKeyword = append(nonKeyword, args[i])
						i++
					}
				} else {
					nonKeyword = append(nonKeyword, args[i])
					i++
				}
			}
			positionalArgs = nonKeyword
		}

		for i := 0; i < numRequired; i++ {
			if i < len(positionalArgs) {
				newEnv.Set(callFn.params[i], positionalArgs[i])
			} else {
				return nil, fmt.Errorf("call: missing required argument")
			}
		}

		paramIdx := numRequired
		for _, defaultExpr := range callFn.optDefaults {
			if paramIdx < len(positionalArgs) {
				newEnv.Set(callFn.params[paramIdx], positionalArgs[paramIdx])
			} else if !isNil(defaultExpr) {
				defVal, err := Eval(defaultExpr, newEnv)
				if err != nil {
					return nil, err
				}
				newEnv.Set(callFn.params[paramIdx], defVal)
			} else {
				newEnv.Set(callFn.params[paramIdx], vnil())
			}
			paramIdx++
		}

		paramIdx = numRequired + len(callFn.optDefaults)
		for _, spec := range callFn.keySpecs {
			if spec == nil || spec.typ != VPair || spec.car == nil || spec.cdr == nil || spec.cdr.typ != VPair || spec.cdr.cdr == nil || spec.cdr.cdr.typ != VPair {
				paramIdx++
				continue
			}
			keyName := spec.car.str
			if len(keyName) > 0 && keyName[0] == ':' {
				keyName = keyName[1:]
			}
			paramName := spec.cdr.car.str
			defaultExpr := spec.cdr.cdr.car
			if val, ok := keyVals[keyName]; ok {
				newEnv.Set(paramName, val)
			} else if !isNil(defaultExpr) {
				defVal, err := Eval(defaultExpr, newEnv)
				if err != nil {
					return nil, err
				}
				newEnv.Set(paramName, defVal)
			} else {
				newEnv.Set(paramName, vnil())
			}
			paramIdx++
		}

		if callFn.rest != "" {
			var restElems []*Value
			if len(callFn.keySpecs) > 0 {
				restElems = positionalArgs[paramIdx:]
			} else if callFn.optDefaults != nil {
				restElems = positionalArgs[len(callFn.params)-len(callFn.optDefaults):]
			} else {
				restElems = positionalArgs[len(callFn.params):]
			}
			newEnv.Set(callFn.rest, listFromSlice(restElems))
		}
		body := callFn.body
		if isNil(body) {
			ret := vnil()
			if callFn.name != "" && traceTable[callFn.name] {
				traceDepth--
				indent := strings.Repeat("  ", traceDepth)
				fmt.Printf("%s%d: <= %s\n", indent, traceDepth, ToString(ret))

			}
			return ret, nil
		}
		for body.typ == VPair && !isNil(body.cdr) {
			_, e := Eval(body.car, newEnv)
			if e != nil {
				if _, ok := e.(*tailCall); ok {
					return nil, e
				}
				return nil, e
			}
			body = body.cdr
		}
		// Evaluate the final expression, handling tail-call optimization
		for {
			ret, err := Eval(body.car, newEnv)
			if err == nil {
				if callFn.name != "" && traceTable[callFn.name] {
					traceDepth--
					indent := strings.Repeat("  ", traceDepth)
					fmt.Printf("%s%d: <= %s\n", indent, traceDepth, ToString(ret))
				}
				return ret, nil
			}
			if tc, ok := err.(*tailCall); ok {
				// TCO: update form/env and continue looping
				body = tc.form
				newEnv = tc.env
				continue
			}
			return nil, err
		}
	default:
		// Fallback: construct form and eval (for other function types)
		return Eval(&Value{typ: VPair, car: callFn, cdr: argList}, globalEnv)
	}
}

func builtinSeqMap(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-map: need function and sequence")
	}
	fn := args[0]
	seq := args[1]
	elements := seqToList(seq)
	var result []*Value
	for _, el := range elements {
		val, err := callFnOnSeq(fn, []*Value{el}, globalEnv)
		if err != nil {
			return nil, err
		}
		result = append(result, val)
	}
	return listFromSlice(result), nil
}

func builtinSeqReduce(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-reduce: need function and sequence")
	}
	fn := args[0]
	keyFn, _, _, fromEnd, _, start, end, initialVal, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	seq := args[1]
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	if start >= end || len(elements) == 0 {
		return initialVal, nil
	}
	hasInitialValue := boolFromKey(":initial-value", args, 2)

	if !fromEnd {
		// Left-to-right reduce (default)
		acc := initialVal
		startIdx := start
		if acc.typ == VNil && !hasInitialValue {
			acc = elements[startIdx]
			startIdx = start + 1
		}
		for i := startIdx; i < end; i++ {
			v := elements[i]
			if !isNil(keyFn) {
				var err error
				v, err = callFnOnSeq(keyFn, []*Value{v}, globalEnv)
				if err != nil {
					return nil, err
				}
			}
			var err error
			acc, err = callFnOnSeq(fn, []*Value{acc, v}, globalEnv)
			if err != nil {
				return nil, err
			}
		}
		return acc, nil
	} else {
		// Right-to-left reduce (:from-end t)
		acc := initialVal
		endIdx := end - 1
		if acc.typ == VNil && !hasInitialValue {
			acc = elements[endIdx]
			endIdx = end - 2
		}
		for i := endIdx; i >= start; i-- {
			v := elements[i]
			if !isNil(keyFn) {
				var err error
				v, err = callFnOnSeq(keyFn, []*Value{v}, globalEnv)
				if err != nil {
					return nil, err
				}
			}
			var err error
			// For :from-end, function is called as (fn element accumulator)
			acc, err = callFnOnSeq(fn, []*Value{v, acc}, globalEnv)
			if err != nil {
				return nil, err
			}
		}
		return acc, nil
	}
}

func boolFromKey(key string, args []*Value, startIdx int) bool {
	for i := startIdx; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == key {
			return true
		}
	}
	return false
}

func builtinSeqSort(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-sort: need sequence and predicate")
	}
	seq := args[0]
	pred := args[1]
	keyFn, _, _, _, _, _, _, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	// Simple insertion sort (stable)
	for i := 1; i < len(elements); i++ {
		for j := i; j > 0; j-- {
			a, b := elements[j-1], elements[j]
			if !isNil(keyFn) {
				var err error
				a, err = callFnOnSeq(keyFn, []*Value{a}, globalEnv)
				if err != nil {
					return nil, err
				}
				b, err = callFnOnSeq(keyFn, []*Value{b}, globalEnv)
				if err != nil {
					return nil, err
				}
			}
			cmp, err := callFnOnSeq(pred, []*Value{b, a}, globalEnv) // (pred b a) means b < a → swap
			if err != nil {
				return nil, err
			}
			if isTruthy(cmp) {
				elements[j-1], elements[j] = elements[j], elements[j-1]
			} else {
				break
			}
		}
	}
	return listFromSlice(elements), nil
}

func builtinSeqRemoveIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-remove-if: need predicate and sequence")
	}
	pred := args[0]
	seq := args[1]
	keyFn, _, _, _, count, start, end, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	var result []*Value
	removed := 0
	for i, el := range elements {
		if i >= start && i < end && (count < 0 || removed < count) {
			v := el
			if !isNil(keyFn) {
				var err error
				v, err = callFnOnSeq(keyFn, []*Value{v}, globalEnv)
				if err != nil {
					return nil, err
				}
			}
			cmp, err := callFnOnSeq(pred, []*Value{v}, globalEnv)
			if err != nil {
				return nil, err
			}
			if isTruthy(cmp) {
				removed++
				continue
			}
		}
		result = append(result, el)
	}
	// If input was a string and all results are characters, return a string
	if seq.typ == VStr {
		var sb strings.Builder
		for _, v := range result {
			if v.typ == VChar {
				sb.WriteRune(v.ch)
			} else {
				return listFromSlice(result), nil
			}
		}
		return vstr(sb.String()), nil
	}
	// If input was a VArray, return a VArray
	if seq.typ == VArray {
		return varray(result), nil
	}
	return listFromSlice(result), nil
}

func builtinSeqFind(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-find: need item and sequence")
	}
	item := args[0]
	seq := args[1]
	if err := checkSequenceArg(seq, "seq-find"); err != nil {
		return nil, err
	}
	keyFn, testFn, testNotFn, fromEnd, _, start, end, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	if fromEnd {
		for i := end - 1; i >= start; i-- {
			if testItemMatchFull(item, elements[i], testFn, testNotFn, keyFn) {
				return elements[i], nil
			}
		}
	} else {
		for i := start; i < end; i++ {
			if testItemMatchFull(item, elements[i], testFn, testNotFn, keyFn) {
				return elements[i], nil
			}
		}
	}
	return vnil(), nil
}

func testItemMatch(item, el *Value, testFn, keyFn *Value) bool {
	return testItemMatchFull(item, el, testFn, nil, keyFn)
}

func testItemMatchFull(item, el, testFn, testNotFn, keyFn *Value) bool {
	// CL spec: test function is called as (test item (key element)).
	// The key function is applied to the ELEMENT only, not the item.
	a := el
	if !isNil(keyFn) {
		var err error
		a, err = callFnOnSeq(keyFn, []*Value{el}, globalEnv)
		if err != nil {
			return false
		}
	}
	b := item
	if !isNil(testNotFn) {
		cmp, err := callFnOnSeq(testNotFn, []*Value{b, a}, globalEnv)
		if err != nil {
			return false
		}
		return !isTruthy(cmp)
	}
	if !isNil(testFn) {
		cmp, err := callFnOnSeq(testFn, []*Value{b, a}, globalEnv)
		if err != nil {
			return false
		}
		return isTruthy(cmp)
	}
	return eqVal(a, b)
}

func builtinSeqPosition(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-position: need item and sequence")
	}
	item := args[0]
	seq := args[1]
	keyFn, testFn, testNotFn, fromEnd, _, start, end, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	if fromEnd {
		for i := end - 1; i >= start; i-- {
			if testItemMatchFull(item, elements[i], testFn, testNotFn, keyFn) {
				return vnum(float64(i)), nil
			}
		}
	} else {
		for i := start; i < end; i++ {
			if testItemMatchFull(item, elements[i], testFn, testNotFn, keyFn) {
				return vnum(float64(i)), nil
			}
		}
	}
	return vnil(), nil
}

func builtinSeqSubstitute(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("seq-substitute: need new, old, and sequence")
	}
	newVal := args[0]
	old := args[1]
	seq := args[2]
	keyFn, testFn, testNotFn, fromEnd, count, start, end, _, err := seqParseKeys(args, 3)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	result := make([]*Value, len(elements))
	copy(result, elements)
	replaced := 0
	if fromEnd {
		for i := end - 1; i >= start; i-- {
			if count >= 0 && replaced >= count {
				break
			}
			if testItemMatchFull(old, elements[i], testFn, testNotFn, keyFn) {
				result[i] = newVal
				replaced++
			}
		}
	} else {
		for i := start; i < end; i++ {
			if count >= 0 && replaced >= count {
				break
			}
			if testItemMatchFull(old, elements[i], testFn, testNotFn, keyFn) {
				result[i] = newVal
				replaced++
			}
		}
	}
	// If input was a string and all results are characters, return a string
	if seq.typ == VStr {
		var sb strings.Builder
		for _, v := range result {
			if v.typ == VChar {
				sb.WriteRune(v.ch)
			} else {
				return listFromSlice(result), nil
			}
		}
		return vstr(sb.String()), nil
	}
	// If input was a VArray, return a VArray
	if seq.typ == VArray {
		return varray(result), nil
	}
	return listFromSlice(result), nil
}

// -------- subseq, concatenate, mapcan --------

func builtinSeqSubseq(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("subseq: need sequence and start")
	}
	seq := args[0]
	start := int(primaryValue(args[1]).num)
	// Handle strings specially: return a string, not a list
	if seq.typ == VStr {
		runes := []rune(seq.str)
		end := len(runes)
		if len(args) >= 3 && args[2].typ != VNil {
			end = int(primaryValue(args[2]).num)
		}
		if start < 0 {
			start = 0
		}
		if end < 0 {
			end = len(runes)
		}
		if start > len(runes) {
			start = len(runes)
		}
		if end > len(runes) {
			end = len(runes)
		}
		if start >= end {
			return vstr(""), nil
		}
		return vstr(string(runes[start:end])), nil
	}
	// Handle VArray specially: return a vector, not a list
	if seq.typ == VArray {
		return builtinSeqSubseqArray(args)
	}
	end := len(seqToList(seq))
	if len(args) >= 3 && args[2].typ != VNil {
		end = int(primaryValue(args[2]).num)
	}
	elements := seqToList(seq)
	if end < 0 {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	if start > len(elements) {
		start = len(elements)
	}
	if end > len(elements) {
		end = len(elements)
	}
	if start >= end {
		return vnil(), nil
	}
	return listFromSlice(elements[start:end]), nil
}

// subseq for VArray: return a new vector
func builtinSeqSubseqArray(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("subseq: need sequence and start")
	}
	seq := args[0]
	if seq.typ != VArray {
		return nil, fmt.Errorf("subseq: need array for array subseq")
	}
	elements := seqToList(seq)
	start := int(primaryValue(args[1]).num)
	end := len(elements)
	if len(args) >= 3 && args[2].typ != VNil {
		end = int(primaryValue(args[2]).num)
	}
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = len(elements)
	}
	if start > len(elements) {
		start = len(elements)
	}
	if end > len(elements) {
		end = len(elements)
	}
	if start >= end {
		return vnil(), nil
	}
	return varray(elements[start:end]), nil
}

func builtinSubseqSetf(args []*Value) (*Value, error) {
	if len(args) < 4 {
		return nil, fmt.Errorf("subseq-setf: need newval, seq, start, end")
	}
	newval := primaryValue(args[0])
	seq := primaryValue(args[1])
	start := int(primaryValue(args[2]).num)
	end := -1
	if args[3].typ != VNil {
		end = int(primaryValue(args[3]).num)
	}
	if seq.typ == VStr {
		// For strings, construct the modified string and update in place
		s := seq.str
		runes := []rune(s)
		if end < 0 || end > len(runes) {
			end = len(runes)
		}
		if start < 0 {
			start = 0
		}
		newStr := ""
		if newval.typ == VStr {
			newStr = newval.str
		}
		// Build new string: before + newStr + rest
		result := string(runes[:start]) + newStr + string(runes[end:])
		// Modify the string in place
		seq.str = result
		return newval, nil
	}
	// For lists, return as-is (modification of subseq is not meaningful)
	return newval, nil
}

func builtinSeqConcatenate(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("concatenate: need result-type and sequences")
	}
	resultType := primaryValue(args[0])
	typeName := ""
	if resultType.typ == VSym {
		typeName = strings.ToUpper(resultType.str)
	}
	// Collect all elements from input sequences
	var result []*Value
	for i := 1; i < len(args); i++ {
		result = append(result, seqToList(args[i])...)
	}
	switch typeName {
	case "STRING":
		var sb strings.Builder
		for _, v := range result {
			if v.typ == VChar {
				sb.WriteRune(v.ch)
			} else {
				sb.WriteString(ToString(v))
			}
		}
		return vstr(sb.String()), nil
	case "VECTOR", "SIMPLE-VECTOR", "ARRAY", "SIMPLE-ARRAY":
		return varray(result), nil
	case "LIST", "CONS", "NULL":
		return listFromSlice(result), nil
	case "BIT-VECTOR", "SIMPLE-BIT-VECTOR":
		bits := make([]*Value, len(result))
		for i, v := range result {
			if v.typ == VNum {
				if int(v.num) != 0 {
					bits[i] = vnum(1)
				} else {
					bits[i] = vnum(0)
				}
			} else {
				bits[i] = vnum(0)
			}
		}
		return varray(bits), nil
	default:
		// Default: return list
		return listFromSlice(result), nil
	}
}

// checkSequenceArg validates that v is a proper sequence for map functions.
func checkSequenceArg(v *Value, funcName string) error {
	v = primaryValue(v)
	if v.typ != VPair && v.typ != VNil && v.typ != VStr && v.typ != VArray {
		return fmt.Errorf("%s: expected a proper sequence", funcName)
	}
	return nil
}

// safeToNum returns the numeric value or an error for non-numeric types.
func safeToNum(v *Value, funcName string) (float64, error) {
	v = primaryValue(v)
	if v.typ != VNum && v.typ != VBigInt && v.typ != VRat {
		return 0, fmt.Errorf("%s: expected a number", funcName)
	}
	return toNum(v), nil
}

func builtinMapcan(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("mapcan: need function and at least one sequence")
	}
	fn := args[0]
	seqs := args[1:]
	for _, s := range seqs {
		if err := checkSequenceArg(s, "mapcan"); err != nil {
			return nil, err
		}
	}
	lists := make([][]*Value, len(seqs))
	for i, s := range seqs {
		lists[i] = seqToList(s)
	}
	if len(lists) == 0 {
		return vnil(), nil
	}
	maxLen := 0
	for _, l := range lists {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	var result []*Value
	for i := 0; i < maxLen; i++ {
		callArgs := make([]*Value, 0, len(lists))
		for _, l := range lists {
			if i < len(l) {
				callArgs = append(callArgs, l[i])
			}
		}
		callResult, err := callFnOnSeq(fn, callArgs, globalEnv)
		if err != nil {
			return nil, err
		}
		result = append(result, seqToList(callResult)...)
	}
	return listFromSlice(result), nil
}

func builtinMapcar(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("mapcar: need function and at least one list")
	}
	fn := args[0]
	seqs := args[1:]
	for _, s := range seqs {
		if err := checkSequenceArg(s, "mapcar"); err != nil {
			return nil, err
		}
	}
	lists := make([][]*Value, len(seqs))
	for i, s := range seqs {
		lists[i] = seqToList(s)
	}
	if len(lists) == 0 {
		return vnil(), nil
	}
	maxLen := 0
	for _, l := range lists {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	var result []*Value
	for i := 0; i < maxLen; i++ {
		callArgs := make([]*Value, 0, len(lists))
		for _, l := range lists {
			if i < len(l) {
				callArgs = append(callArgs, l[i])
			}
		}
		callResult, err := callFnOnSeq(fn, callArgs, globalEnv)
		if err != nil {
			return nil, err
		}
		result = append(result, callResult)
	}
	return listFromSlice(result), nil
}

func builtinMapInto(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("map-into: need result-sequence and function and sequence")
	}
	resultSeq := args[0]
	fn := args[1]
	seqs := args[2:]
	for _, s := range seqs {
		if err := checkSequenceArg(s, "map-into"); err != nil {
			return nil, err
		}
	}
	lists := make([][]*Value, len(seqs))
	for i, s := range seqs {
		lists[i] = seqToList(s)
	}
	maxLen := 0
	for _, l := range lists {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	var result []*Value
	if resultSeq.typ == VStr {
		var sb strings.Builder
		for i := 0; i < maxLen; i++ {
			callArgs := make([]*Value, 0, len(lists))
			for _, l := range lists {
				if i < len(l) {
					callArgs = append(callArgs, l[i])
				}
			}
			val, err := callFnOnSeq(fn, callArgs, globalEnv)
			if err != nil {
				return nil, err
			}
			sb.WriteString(ToString(primaryValue(val)))
		}
		return vstr(sb.String()), nil
	}
	// Handle VArray: modify elements in place and return the original array
	if resultSeq.typ == VArray && resultSeq.array != nil {
		elements := resultSeq.array.elements
		n := len(elements)
		if resultSeq.array.fillPtr >= 0 && resultSeq.array.fillPtr < n {
			n = resultSeq.array.fillPtr
		}
		for i := 0; i < maxLen && i < n; i++ {
			callArgs := make([]*Value, 0, len(lists))
			for _, l := range lists {
				if i < len(l) {
					callArgs = append(callArgs, l[i])
				}
			}
			val, err := callFnOnSeq(fn, callArgs, globalEnv)
			if err != nil {
				return nil, err
			}
			resultSeq.array.elements[i] = primaryValue(val)
		}
		return resultSeq, nil
	}
	result = seqToList(resultSeq)
	if len(result) == 0 && maxLen > 0 {
		result = make([]*Value, maxLen)
	}
	for i := 0; i < maxLen && i < len(result); i++ {
		callArgs := make([]*Value, 0, len(lists))
		for _, l := range lists {
			if i < len(l) {
				callArgs = append(callArgs, l[i])
			}
		}
		val, err := callFnOnSeq(fn, callArgs, globalEnv)
		if err != nil {
			return nil, err
		}
		result[i] = val
	}
	return listFromSlice(result), nil
}

func builtinMap(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("map: need result-type, function and sequences")
	}
	resultType := primaryValue(args[0])
	fn := args[1]
	seqs := args[2:]
	if len(seqs) == 0 {
		return nil, fmt.Errorf("map: need at least one sequence")
	}
	for _, s := range seqs {
		if err := checkSequenceArg(s, "map"); err != nil {
			return nil, err
		}
	}
	lists := make([][]*Value, len(seqs))
	for i, s := range seqs {
		lists[i] = seqToList(s)
	}
	maxLen := 0
	for _, l := range lists {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	var result []*Value
	for i := 0; i < maxLen; i++ {
		callArgs := make([]*Value, 0, len(lists))
		for _, l := range lists {
			if i < len(l) {
				callArgs = append(callArgs, l[i])
			}
		}
		val, err := callFnOnSeq(fn, callArgs, globalEnv)
		if err != nil {
			return nil, err
		}
		result = append(result, val)
	}
	// Convert based on result-type
	if isNil(resultType) || (resultType.typ == VSym && resultType.str == "NIL") {
		return vnil(), nil
	}
	if resultType.typ != VSym {
		return nil, fmt.Errorf("map: result-type must be a symbol, got %v", resultType)
	}
	switch resultType.str {
	case "LIST", "CONS":
		return listFromSlice(result), nil
	case "VECTOR":
		av := gcv()
		av.typ = VArray
		av.array = &LispArray{dims: []int{len(result)}, elements: result, fillPtr: -1}
		return av, nil
	case "STRING":
		var sb strings.Builder
		for _, v := range result {
			if v.typ == VChar {
				sb.WriteRune(v.ch)
			} else {
				sb.WriteString(ToString(primaryValue(v)))
			}
		}
		return vstr(sb.String()), nil
	default:
		return nil, fmt.Errorf("map: unsupported result-type: %s", resultType.str)
	}
}

func builtinRevappend(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("revappend: need two lists")
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	// Reverse list1 and append list2
	for i, j := 0, len(list1)-1; i < j; i, j = i+1, j-1 {
		list1[i], list1[j] = list1[j], list1[i]
	}
	result := make([]*Value, len(list1)+len(list2))
	copy(result, list1)
	copy(result[len(list1):], list2)
	return listFromSlice(result), nil
}

func builtinLdiff(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("ldiff: need two lists")
	}
	list := args[0]
	obj := args[1]
	var result []*Value
	seen := make(map[*Value]bool)
	for !isNil(list) && list.typ == VPair {
		if seen[list] {
			break
		}
		seen[list] = true
		if list == obj {
			break
		}
		result = append(result, list.car)
		list = list.cdr
	}
	return listFromSlice(result), nil
}

func builtinTailp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	sublist := args[0]
	list := args[1]
	seen := make(map[*Value]bool)
	for !isNil(list) && list.typ == VPair {
		if seen[list] {
			break
		}
		seen[list] = true
		if list == sublist {
			return vbool(true), nil
		}
		list = list.cdr
	}
	return vbool(list == sublist), nil
}

func builtinNthValue(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("nth-value: need index and form")
	}
	n := int(toNum(args[0]))
	form := args[1]
	result, err := Eval(form, globalEnv)
	if err != nil {
		return nil, err
	}
	// Get the nth value from a multiple-values result
	if result != nil && result.typ == VMultiVal {
		vals := cons(result.car, result.cdr) // full list of values
		i := 0
		for !isNil(vals) && vals.typ == VPair {
			if i == n {
				return vals.car, nil
			}
			vals = vals.cdr
			i++
		}
		return vnil(), nil
	}
	if n == 0 {
		return primaryValue(result), nil
	}
	return vnil(), nil
}

func builtinSome(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("some: need predicate and sequence")
	}
	fn := args[0]
	seqs := args[1:]
	keyFn, seqs := extractKeyFromRest(seqs)
	lists := make([][]*Value, len(seqs))
	for i, s := range seqs {
		lists[i] = seqToList(s)
	}
	maxLen := 0
	for _, l := range lists {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	for i := 0; i < maxLen; i++ {
		callArgs := make([]*Value, 0, len(lists))
		for _, l := range lists {
			if i < len(l) {
				el := l[i]
				if keyFn != nil {
					el, _ = callFnOnSeq(keyFn, []*Value{el}, globalEnv)
				}
				callArgs = append(callArgs, el)
			}
		}
		result, err := callFnOnSeq(fn, callArgs, globalEnv)
		if err != nil {
			return nil, err
		}
		if isTruthy(result) {
			return result, nil
		}
	}
	return vnil(), nil
}

func builtinEvery(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("every: need predicate and sequence")
	}
	fn := args[0]
	seqs := args[1:]
	keyFn, seqs := extractKeyFromRest(seqs)
	lists := make([][]*Value, len(seqs))
	for i, s := range seqs {
		lists[i] = seqToList(s)
	}
	maxLen := 0
	for _, l := range lists {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	for i := 0; i < maxLen; i++ {
		callArgs := make([]*Value, 0, len(lists))
		for _, l := range lists {
			if i < len(l) {
				el := l[i]
				if keyFn != nil {
					el, _ = callFnOnSeq(keyFn, []*Value{el}, globalEnv)
				}
				callArgs = append(callArgs, el)
			}
		}
		result, err := callFnOnSeq(fn, callArgs, globalEnv)
		if err != nil {
			return nil, err
		}
		if !isTruthy(result) {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinNotany(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("notany: need predicate and sequence")
	}
	fn := args[0]
	seqs := args[1:]
	keyFn, seqs := extractKeyFromRest(seqs)
	lists := make([][]*Value, len(seqs))
	for i, s := range seqs {
		lists[i] = seqToList(s)
	}
	maxLen := 0
	for _, l := range lists {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	for i := 0; i < maxLen; i++ {
		callArgs := make([]*Value, 0, len(lists))
		for _, l := range lists {
			if i < len(l) {
				el := l[i]
				if keyFn != nil {
					el, _ = callFnOnSeq(keyFn, []*Value{el}, globalEnv)
				}
				callArgs = append(callArgs, el)
			}
		}
		result, err := callFnOnSeq(fn, callArgs, globalEnv)
		if err != nil {
			return nil, err
		}
		if isTruthy(result) {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinNotevery(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("notevery: need predicate and sequence")
	}
	fn := args[0]
	seqs := args[1:]
	keyFn, seqs := extractKeyFromRest(seqs)
	lists := make([][]*Value, len(seqs))
	for i, s := range seqs {
		lists[i] = seqToList(s)
	}
	maxLen := 0
	for _, l := range lists {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	for i := 0; i < maxLen; i++ {
		callArgs := make([]*Value, 0, len(lists))
		for _, l := range lists {
			if i < len(l) {
				el := l[i]
				if keyFn != nil {
					el, _ = callFnOnSeq(keyFn, []*Value{el}, globalEnv)
				}
				callArgs = append(callArgs, el)
			}
		}
		result, err := callFnOnSeq(fn, callArgs, globalEnv)
		if err != nil {
			return nil, err
		}
		if !isTruthy(result) {
			return vbool(true), nil
		}
	}
	return vbool(false), nil
}

func builtinNconc(args []*Value) (*Value, error) {
	// nconc: destructive list concatenation
	var result, tail *Value
	for _, arg := range args {
		if isNil(arg) {
			continue
		}
		list := seqToList(arg)
		if len(list) == 0 {
			continue
		}
		if isNil(result) {
			result = listFromSlice(list)
			tail = result
		} else {
			tail.cdr = listFromSlice(list)
		}
		// Find new tail with cycle detection
		seen := make(map[*Value]bool)
		for !isNil(tail) && tail.typ == VPair && !isNil(tail.cdr) {
			if seen[tail] {
				break
			}
			seen[tail] = true
			tail = tail.cdr
		}
	}
	if isNil(result) {
		return vnil(), nil
	}
	return result, nil
}

func builtinAdjoin(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("adjoin: need item and list")
	}
	item := args[0]
	lst := args[1]
	// Check if item is already in list
	elements := seqToList(lst)
	for _, el := range elements {
		if eqVal(item, el) {
			return lst, nil
		}
	}
	return cons(item, lst), nil
}

func substHelper(new, old, tree *Value) *Value {
	if eqVal(tree, old) {
		return new
	}
	if tree.typ == VPair {
		// Cycle detection: use map-based tracking to prevent infinite loops on circular structures
		visited := make(map[*Value]bool)
		return substHelperCycle(new, old, tree, visited)
	}
	return tree
}

func substHelperCycle(new, old, tree *Value, visited map[*Value]bool) *Value {
	if eqVal(tree, old) {
		return new
	}
	if tree.typ != VPair {
		return tree
	}
	if visited[tree] {
		return tree // cycle detected, stop recursion
	}
	visited[tree] = true
	return cons(substHelperCycle(new, old, tree.car, visited), substHelperCycle(new, old, tree.cdr, visited))
}

func builtinSubst(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("subst: need new, old, and tree")
	}
	return substHelper(args[0], args[1], args[2]), nil
}

func sublisHelper(alist, tree *Value) *Value {
	visited := make(map[*Value]bool)
	return sublisHelperCycle(alist, tree, visited)
}

func sublisHelperCycle(alist, tree *Value, visited map[*Value]bool) *Value {
	if tree.typ == VPair {
		if visited[tree] {
			return tree // cycle detected
		}
		visited[tree] = true
		// Check if tree itself is a key in alist
		for cur := alist; !isNil(cur) && cur.typ == VPair; cur = cur.cdr {
			pair := cur.car
			if pair.typ == VPair && eqVal(pair.car, tree) {
				return pair.cdr
			}
		}
		return cons(sublisHelperCycle(alist, tree.car, visited), sublisHelperCycle(alist, tree.cdr, visited))
	}
	// For atoms, check alist
	for cur := alist; !isNil(cur) && cur.typ == VPair; cur = cur.cdr {
		pair := cur.car
		if pair.typ == VPair && eqVal(pair.car, tree) {
			return pair.cdr
		}
	}
	return tree
}

func builtinSublis(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("sublis: need alist and tree")
	}
	return sublisHelper(args[0], args[1]), nil
}

func builtinTreeEqual(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	// Cycle detection to prevent infinite recursion on circular structures
	visitedA := make(map[*Value]bool)
	visitedB := make(map[*Value]bool)
	var teq func(a, b *Value) bool
	teq = func(a, b *Value) bool {
		if a == b {
			return true
		}
		if visitedA[a] || visitedB[b] {
			return true // already compared, assume equal to avoid infinite loop
		}
		if a.typ == VPair {
			visitedA[a] = true
		}
		if b.typ == VPair {
			visitedB[b] = true
		}
		if a.typ == VPair && b.typ == VPair {
			return teq(a.car, b.car) && teq(a.cdr, b.cdr)
		}
		if a.typ == VNil && b.typ == VNil {
			return true
		}
		return eqVal(a, b)
	}
	return vbool(teq(args[0], args[1])), nil
}

// -------- subst-if --------
func builtinSubstIf(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("subst-if: need new, predicate, and tree")
	}
	newVal := args[0]
	pred := args[1]
	tree := args[2]
	visited := make(map[*Value]bool)
	var helper func(t *Value) *Value
	helper = func(t *Value) *Value {
		if visited[t] {
			return t
		}
		if t.typ == VPair {
			visited[t] = true
		}
		result, err := callFnOnSeq(pred, []*Value{t}, globalEnv)
		if err == nil && isTruthy(result) {
			return newVal
		}
		if t.typ == VPair {
			return cons(helper(t.car), helper(t.cdr))
		}
		return t
	}
	return helper(tree), nil
}

// -------- subst-if-not --------
func builtinSubstIfNot(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("subst-if-not: need new, predicate, and tree")
	}
	newVal := args[0]
	pred := args[1]
	tree := args[2]
	visited := make(map[*Value]bool)
	var helper func(t *Value) *Value
	helper = func(t *Value) *Value {
		if visited[t] {
			return t
		}
		if t.typ == VPair {
			visited[t] = true
		}
		result, err := callFnOnSeq(pred, []*Value{t}, globalEnv)
		if err == nil && !isTruthy(result) {
			return newVal
		}
		if t.typ == VPair {
			return cons(helper(t.car), helper(t.cdr))
		}
		return t
	}
	return helper(tree), nil
}

// -------- nsubst / nsubst-if / nsublis --------
func builtinNsubst(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("nsubst: need new, old, and tree")
	}
	newVal := args[0]
	old := args[1]
	visited := make(map[*Value]bool)
	var helper func(t *Value) *Value
	helper = func(t *Value) *Value {
		if eqVal(t, old) {
			return newVal
		}
		if t.typ == VPair {
			if visited[t] {
				return t
			}
			visited[t] = true
			t.car = helper(t.car)
			t.cdr = helper(t.cdr)
		}
		return t
	}
	return helper(args[2]), nil
}

func builtinNsubstIf(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("nsubst-if: need new, predicate, and tree")
	}
	newVal := args[0]
	pred := args[1]
	visited := make(map[*Value]bool)
	var helper func(t *Value) *Value
	helper = func(t *Value) *Value {
		result, err := callFnOnSeq(pred, []*Value{t}, globalEnv)
		if err == nil && isTruthy(result) {
			return newVal
		}
		if t.typ == VPair {
			if visited[t] {
				return t
			}
			visited[t] = true
			t.car = helper(t.car)
			t.cdr = helper(t.cdr)
		}
		return t
	}
	return helper(args[2]), nil
}

func builtinNsubstIfNot(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("nsubst-if-not: need new, predicate, and tree")
	}
	newVal := args[0]
	pred := args[1]
	visited := make(map[*Value]bool)
	var helper func(t *Value) *Value
	helper = func(t *Value) *Value {
		result, err := callFnOnSeq(pred, []*Value{t}, globalEnv)
		if err == nil && !isTruthy(result) {
			return newVal
		}
		if t.typ == VPair {
			if visited[t] {
				return t
			}
			visited[t] = true
			t.car = helper(t.car)
			t.cdr = helper(t.cdr)
		}
		return t
	}
	return helper(args[2]), nil
}

func builtinNsublis(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("nsublis: need alist and tree")
	}
	alist := args[0]
	visited := make(map[*Value]bool)
	var helper func(t *Value) *Value
	helper = func(t *Value) *Value {
		for cur := alist; !isNil(cur) && cur.typ == VPair; cur = cur.cdr {
			pair := cur.car
			if pair.typ == VPair && eqVal(pair.car, t) {
				return pair.cdr
			}
		}
		if t.typ == VPair {
			if visited[t] {
				return t
			}
			visited[t] = true
			t.car = helper(t.car)
			t.cdr = helper(t.cdr)
		}
		return t
	}
	return helper(args[1]), nil
}

// -------- stable-sort (builtin) --------
func builtinStableSort(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("stable-sort: need sequence and predicate")
	}
	seq := args[0]
	pred := args[1]
	keyFn := vnil()
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":KEY" && i+1 < len(args) {
			i++
			keyFn = args[i]
		}
	}
	elems := seqToList(seq)
	// Insertion sort (stable)
	for i := 1; i < len(elems); i++ {
		key := elems[i]
		var keyVal *Value
		if !isNil(keyFn) {
			var err error
			keyVal, err = callFnOnSeq(keyFn, []*Value{key}, globalEnv)
			if err != nil {
				return nil, err
			}
		} else {
			keyVal = key
		}
		j := i - 1
		for j >= 0 {
			var jVal *Value
			if !isNil(keyFn) {
				var err error
				jVal, err = callFnOnSeq(keyFn, []*Value{elems[j]}, globalEnv)
				if err != nil {
					return nil, err
				}
			} else {
				jVal = elems[j]
			}
			cmp, err := callFnOnSeq(pred, []*Value{keyVal, jVal}, globalEnv)
			if err != nil {
				return nil, err
			}
			if !isTruthy(cmp) {
				break
			}
			elems[j+1] = elems[j]
			j--
		}
		elems[j+1] = key
	}
	if seq.typ == VStr {
		var sb strings.Builder
		for _, v := range elems {
			sb.WriteString(ToString(v))
		}
		return vstr(sb.String()), nil
	}
	return listFromSlice(elems), nil
}

func builtinUnion(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("union: need two lists")
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	seen := make(map[string]bool)
	var result []*Value
	for _, v := range list1 {
		key := ToString(v)
		if !seen[key] {
			seen[key] = true
			result = append(result, v)
		}
	}
	for _, v := range list2 {
		key := ToString(v)
		if !seen[key] {
			seen[key] = true
			result = append(result, v)
		}
	}
	return listFromSlice(result), nil
}

func builtinIntersection(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("intersection: need two lists")
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	set2 := make(map[string]bool)
	for _, v := range list2 {
		set2[ToString(v)] = true
	}
	seen := make(map[string]bool)
	var result []*Value
	for _, v := range list1 {
		key := ToString(v)
		if set2[key] && !seen[key] {
			seen[key] = true
			result = append(result, v)
		}
	}
	return listFromSlice(result), nil
}

func builtinSetDifference(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-difference: need two lists")
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	set2 := make(map[string]bool)
	for _, v := range list2 {
		set2[ToString(v)] = true
	}
	var result []*Value
	for _, v := range list1 {
		if !set2[ToString(v)] {
			result = append(result, v)
		}
	}
	return listFromSlice(result), nil
}

func builtinSetExclusiveOr(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("set-exclusive-or: need two lists")
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	set1 := make(map[string]bool)
	set2 := make(map[string]bool)
	for _, v := range list1 {
		set1[ToString(v)] = true
	}
	for _, v := range list2 {
		set2[ToString(v)] = true
	}
	var result []*Value
	for _, v := range list1 {
		if !set2[ToString(v)] {
			result = append(result, v)
		}
	}
	for _, v := range list2 {
		if !set1[ToString(v)] {
			result = append(result, v)
		}
	}
	return listFromSlice(result), nil
}

func builtinNsetExclusiveOr(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("nset-exclusive-or: need two lists")
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	set1 := make(map[string]bool)
	set2 := make(map[string]bool)
	for _, v := range list1 {
		set1[ToString(v)] = true
	}
	for _, v := range list2 {
		set2[ToString(v)] = true
	}
	var result []*Value
	for _, v := range list1 {
		if !set2[ToString(v)] {
			result = append(result, v)
		}
	}
	for _, v := range list2 {
		if !set1[ToString(v)] {
			result = append(result, v)
		}
	}
	return listFromSlice(result), nil
}

func builtinSubsetp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return vbool(false), nil
	}
	list1 := seqToList(args[0])
	list2 := seqToList(args[1])
	set2 := make(map[string]bool)
	for _, v := range list2 {
		set2[ToString(v)] = true
	}
	for _, v := range list1 {
		if !set2[ToString(v)] {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
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

// -------- assoc --------
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

func builtinSeqCount(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-count: need item and sequence")
	}
	item := args[0]
	seq := args[1]
	keyFn, testFn, testNotFn, _, _, start, end, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	cnt := 0
	for i := start; i < end; i++ {
		if testItemMatchFull(item, elements[i], testFn, testNotFn, keyFn) {
			cnt++
		}
	}
	return vnum(float64(cnt)), nil
}

func builtinSeqCountIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-count-if: need predicate and sequence")
	}
	pred := args[0]
	seq := args[1]
	keyFn, _, _, _, _, start, end, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	cnt := 0
	for i := start; i < end; i++ {
		v := elements[i]
		if !isNil(keyFn) {
			var err error
			v, err = callFnOnSeq(keyFn, []*Value{v}, globalEnv)
			if err != nil {
				return nil, err
			}
		}
		cmp, err := callFnOnSeq(pred, []*Value{v}, globalEnv)
		if err != nil {
			return nil, err
		}
		if isTruthy(cmp) {
			cnt++
		}
	}
	return vnum(float64(cnt)), nil
}

func builtinSeqRemove(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-remove: need item and sequence")
	}
	item := args[0]
	seq := args[1]
	keyFn, testFn, testNotFn, fromEnd, count, start, end, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	var result []*Value
	removed := 0
	if fromEnd {
		// Build from end: collect indices to remove
		removeSet := map[int]bool{}
		for i := end - 1; i >= start; i-- {
			if count >= 0 && removed >= count {
				break
			}
			if testItemMatchFull(item, elements[i], testFn, testNotFn, keyFn) {
				removeSet[i] = true
				removed++
			}
		}
		for i, el := range elements {
			if !removeSet[i] {
				result = append(result, el)
			}
		}
	} else {
		for i, el := range elements {
			if i >= start && i < end && (count < 0 || removed < count) {
				if testItemMatchFull(item, el, testFn, testNotFn, keyFn) {
					removed++
					continue
				}
			}
			result = append(result, el)
		}
	}
	// If input was a string and all results are characters, return a string
	if seq.typ == VStr {
		var sb strings.Builder
		for _, v := range result {
			if v.typ == VChar {
				sb.WriteRune(v.ch)
			} else {
				return listFromSlice(result), nil
			}
		}
		return vstr(sb.String()), nil
	}
	// If input was a VArray, return a VArray
	if seq.typ == VArray {
		return varray(result), nil
	}
	return listFromSlice(result), nil
}

// builtinDelete is the destructive version of remove - it modifies the list in-place
func builtinDelete(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("delete: need item and sequence")
	}
	item := args[0]
	seq := args[1]
	if seq.typ == VNil {
		return seq, nil
	}
	// For strings, delegate to remove (strings are immutable in CL)
	if seq.typ == VStr {
		return builtinSeqRemove(args)
	}
	if seq.typ != VPair {
		return nil, fmt.Errorf("delete: expected a list, got %s", typeStr(seq))
	}
	keyFn, testFn, _, fromEnd, count, start, end, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}

	// Normalize start/end
	if end < 0 {
		// Compute length to get real end
		seen := make(map[*Value]bool)
		length := 0
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			if seen[cur] {
				break
			}
			seen[cur] = true
			length++
			cur = cur.cdr
		}
		end = length
	}
	if start < 0 {
		start = 0
	}

	// Destructive in-place deletion by rewiring .cdr pointers
	var head *Value = seq
	if fromEnd {
		// First pass: find indices to remove (count from end)
		// First pass: find indices to remove (count from end)
		elements := seqToList(seq)
		removeSet := map[int]bool{}
		removed := 0
		for i := len(elements) - 1; i >= 0; i-- {
			if count >= 0 && removed >= count {
				break
			}
			if testItemMatch(item, elements[i], testFn, keyFn) {
				removeSet[i] = true
				removed++
			}
		}
		// Second pass: rewiring (skip matching elements)
		// head stays as is; we rewire the tail
		prev := (*Value)(nil)
		cur := head
		i := 0
		for !isNil(cur) && cur.typ == VPair {
			if !removeSet[i] {
				prev = cur
			} else {
				if prev == nil {
					head = cur.cdr
				} else {
					prev.cdr = cur.cdr
				}
			}
			cur = cur.cdr
			i++
		}
	} else {
		// Forward pass: build result list by rewiring .cdr
		// head may change if first element is removed
		prev := (*Value)(nil)
		cur := head
		i := 0
		removed := 0
		for !isNil(cur) && cur.typ == VPair {
			withinRange := i >= start && (count < 0 || removed < count)
			if withinRange && testItemMatch(item, cur.car, testFn, keyFn) {
				// Remove this cell by rewiring prev.cdr
				if prev == nil {
					head = cur.cdr
				} else {
					prev.cdr = cur.cdr
				}
				cur = cur.cdr
				removed++
			} else {
				prev = cur
				cur = cur.cdr
			}
			i++
		}
	}
	return head, nil
}

// builtinDeleteIf - destructive version of remove-if
func builtinDeleteIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("delete-if: need predicate and sequence")
	}
	pred := args[0]
	seq := args[1]
	if seq.typ == VNil {
		return seq, nil
	}
	// For strings and vectors, delegate to remove-if (non-destructive for vectors)
	if seq.typ == VStr || seq.typ == VArray {
		return builtinSeqRemoveIf(args)
	}
	if seq.typ != VPair {
		return nil, fmt.Errorf("delete-if: expected a list, got %s", typeStr(seq))
	}
	keyFn, _, _, _, count, start, end, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	if end < 0 {
		seen := make(map[*Value]bool)
		length := 0
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			if seen[cur] {
				break
			}
			seen[cur] = true
			length++
			cur = cur.cdr
		}
		end = length
	}
	if start < 0 {
		start = 0
	}
	head := seq
	prev := (*Value)(nil)
	cur := seq
	i := 0
	removed := 0
	for !isNil(cur) && cur.typ == VPair {
		match := false
		if i >= start && i < end && (count < 0 || removed < count) {
			v := cur.car
			if !isNil(keyFn) {
				var err2 error
				v, err2 = callFnOnSeq(keyFn, []*Value{cur.car}, globalEnv)
				if err2 != nil {
					return nil, err2
				}
			}
			res, err := callFnOnSeq(pred, []*Value{v}, globalEnv)
			if err == nil && isTruthy(res) {
				match = true
				removed++
			}
		}
		if match {
			if prev == nil {
				head = cur.cdr
			} else {
				prev.cdr = cur.cdr
			}
			cur = cur.cdr
		} else {
			prev = cur
			cur = cur.cdr
		}
		i++
	}
	return head, nil
}

// builtinDeleteIfNot - destructive version of remove-if-not
func builtinDeleteIfNot(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("delete-if-not: need predicate and sequence")
	}
	pred := args[0]
	seq := args[1]
	if seq.typ == VNil {
		return seq, nil
	}
	// For strings and vectors, delegate to remove-if-not (non-destructive)
	if seq.typ == VStr || seq.typ == VArray {
		return builtinSeqRemoveIfNot(args)
	}
	if seq.typ != VPair {
		return nil, fmt.Errorf("delete-if-not: expected a list, got %s", typeStr(seq))
	}
	negPred := &Value{typ: VFunc, params: []string{"x"}, body: list(list(vsym("not"), list(pred, vsym("x")))), env: globalEnv}
	newArgs := []*Value{negPred, seq}
	newArgs = append(newArgs, args[2:]...)
	return builtinDeleteIf(newArgs)
}

// builtinDeleteDuplicates - destructive version of remove-duplicates
func builtinDeleteDuplicates(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("delete-duplicates: need sequence")
	}
	seq := args[0]
	if seq.typ == VNil {
		return seq, nil
	}
	// For vectors and strings, delegate to remove-duplicates (non-destructive)
	if seq.typ == VArray || seq.typ == VStr {
		return builtinSeqRemoveDuplicates(args)
	}
	if seq.typ != VPair {
		return nil, fmt.Errorf("delete-duplicates: expected a list, got %s", typeStr(seq))
	}
	keyFn, _, _, _, _, start, end, _, err := seqParseKeys(args, 1)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	head := seq
	prev := (*Value)(nil)
	cur := seq
	i := 0
	for !isNil(cur) && cur.typ == VPair {
		withinRange := i >= start && (end < 0 || i < end)
		if withinRange {
			key := cur.car
			if !isNil(keyFn) {
				key, _ = callFnOnSeq(keyFn, []*Value{cur.car}, globalEnv)
			}
			keyStr := ToString(key)
			if seen[keyStr] {
				// duplicate: remove by rewiring
				if prev == nil {
					head = cur.cdr
				} else {
					prev.cdr = cur.cdr
				}
				cur = cur.cdr
			} else {
				seen[keyStr] = true
				prev = cur
				cur = cur.cdr
			}
		} else {
			prev = cur
			cur = cur.cdr
		}
		i++
	}
	return head, nil
}

// builtinNsubstituteIf - destructive version of substitute-if
func builtinNsubstituteIf(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("nsubstitute-if: need new, predicate, and sequence")
	}
	newVal := args[0]
	pred := args[1]
	seq := args[2]
	keyFn, _, _, fromEnd, count, start, end, _, err := seqParseKeys(args, 3)
	if err != nil {
		return nil, err
	}

	// Handle VStr: strings are immutable in Go, so create a new one
	if seq.typ == VStr {
		runes := []rune(seq.str)
		if end < 0 || end > len(runes) {
			end = len(runes)
		}
		if start < 0 {
			start = 0
		}
		replaced := 0
		if fromEnd {
			for i := end - 1; i >= start; i-- {
				if count >= 0 && replaced >= count {
					break
				}
				v := vchar(runes[i])
				if !isNil(keyFn) {
					v2, err2 := callFnOnSeq(keyFn, []*Value{v}, globalEnv)
					if err2 != nil {
						return nil, err2
					}
					v = v2
				}
				predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
				if perr == nil && isTruthy(predVal) {
					if newVal.typ == VChar {
						runes[i] = newVal.ch
					} else if newVal.typ == VStr && len(newVal.str) == 1 {
						runes[i] = rune(newVal.str[0])
					}
					replaced++
				}
			}
		} else {
			for i := start; i < end; i++ {
				if count >= 0 && replaced >= count {
					break
				}
				v := vchar(runes[i])
				if !isNil(keyFn) {
					v2, err2 := callFnOnSeq(keyFn, []*Value{vchar(runes[i])}, globalEnv)
					if err2 != nil {
						return nil, err2
					}
					v = v2
				}
				predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
				if perr == nil && isTruthy(predVal) {
					if newVal.typ == VChar {
						runes[i] = newVal.ch
					} else if newVal.typ == VStr && len(newVal.str) == 1 {
						runes[i] = rune(newVal.str[0])
					}
					replaced++
				}
			}
		}
		seq.str = string(runes)
		return seq, nil
	}

	// Handle VArray: modify elements in-place
	if seq.typ == VArray {
		elems := seq.array.elements
		seqLen := len(elems)
		if end < 0 || end > seqLen {
			end = seqLen
		}
		if start < 0 {
			start = 0
		}
		replaced := 0
		if fromEnd {
			for i := end - 1; i >= start; i-- {
				if count >= 0 && replaced >= count {
					break
				}
				v := elems[i]
				if !isNil(keyFn) {
					v, err2 := callFnOnSeq(keyFn, []*Value{elems[i]}, globalEnv)
					if err2 != nil {
						return nil, err2
					}
					_ = v
				}
				predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
				if perr == nil && isTruthy(predVal) {
					elems[i] = newVal
					replaced++
				}
			}
		} else {
			for i := start; i < end; i++ {
				if count >= 0 && replaced >= count {
					break
				}
				v := elems[i]
				if !isNil(keyFn) {
					v2, err2 := callFnOnSeq(keyFn, []*Value{elems[i]}, globalEnv)
					if err2 != nil {
						return nil, err2
					}
					v = v2
				}
				predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
				if perr == nil && isTruthy(predVal) {
					elems[i] = newVal
					replaced++
				}
			}
		}
		return seq, nil
	}

	if end < 0 {
		seen := make(map[*Value]bool)
		length := 0
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			if seen[cur] {
				break
			}
			seen[cur] = true
			length++
			cur = cur.cdr
		}
		end = length
	}
	if start < 0 {
		start = 0
	}
	replaced := 0
	if fromEnd {
		var elements []*Value
		var cellPtrs []*Value
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			elements = append(elements, cur.car)
			cellPtrs = append(cellPtrs, cur)
			cur = cur.cdr
		}
		for i := end - 1; i >= start; i-- {
			if count >= 0 && replaced >= count {
				break
			}
			v := elements[i]
			if !isNil(keyFn) {
				var err2 error
				v, err2 = callFnOnSeq(keyFn, []*Value{elements[i]}, globalEnv)
				if err2 != nil {
					return nil, err2
				}
			}
			predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
			if perr == nil && isTruthy(predVal) {
				cellPtrs[i].car = newVal
				replaced++
			}
		}
	} else {
		i := 0
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			if i >= start && i < end && (count < 0 || replaced < count) {
				v := cur.car
				if !isNil(keyFn) {
					var err2 error
					v, err2 = callFnOnSeq(keyFn, []*Value{cur.car}, globalEnv)
					if err2 != nil {
						return nil, err2
					}
				}
				predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
				if perr == nil && isTruthy(predVal) {
					cur.car = newVal
					replaced++
				}
			}
			i++
			cur = cur.cdr
		}
	}
	return seq, nil
}

// builtinNsubstituteIfNot - destructive version of substitute-if-not
func builtinNsubstituteIfNot(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("nsubstitute-if-not: need new, predicate, and sequence")
	}
	newVal := args[0]
	pred := args[1]
	seq := args[2]
	keyFn, _, _, fromEnd, count, start, end, _, err := seqParseKeys(args, 3)
	if err != nil {
		return nil, err
	}

	// Handle VStr: strings are immutable in Go, so create a new one
	if seq.typ == VStr {
		runes := []rune(seq.str)
		if end < 0 || end > len(runes) {
			end = len(runes)
		}
		if start < 0 {
			start = 0
		}
		replaced := 0
		if fromEnd {
			for i := end - 1; i >= start; i-- {
				if count >= 0 && replaced >= count {
					break
				}
				v := vchar(runes[i])
				if !isNil(keyFn) {
					v2, err2 := callFnOnSeq(keyFn, []*Value{v}, globalEnv)
					if err2 != nil {
						return nil, err2
					}
					v = v2
				}
				predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
				if perr == nil && !isTruthy(predVal) {
					if newVal.typ == VChar {
						runes[i] = newVal.ch
					} else if newVal.typ == VStr && len(newVal.str) == 1 {
						runes[i] = rune(newVal.str[0])
					}
					replaced++
				}
			}
		} else {
			for i := start; i < end; i++ {
				if count >= 0 && replaced >= count {
					break
				}
				v := vchar(runes[i])
				if !isNil(keyFn) {
					v2, err2 := callFnOnSeq(keyFn, []*Value{vchar(runes[i])}, globalEnv)
					if err2 != nil {
						return nil, err2
					}
					v = v2
				}
				predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
				if perr == nil && !isTruthy(predVal) {
					if newVal.typ == VChar {
						runes[i] = newVal.ch
					} else if newVal.typ == VStr && len(newVal.str) == 1 {
						runes[i] = rune(newVal.str[0])
					}
					replaced++
				}
			}
		}
		seq.str = string(runes)
		return seq, nil
	}

	// Handle VArray: modify elements in-place
	if seq.typ == VArray {
		elems := seq.array.elements
		seqLen := len(elems)
		if end < 0 || end > seqLen {
			end = seqLen
		}
		if start < 0 {
			start = 0
		}
		replaced := 0
		if fromEnd {
			for i := end - 1; i >= start; i-- {
				if count >= 0 && replaced >= count {
					break
				}
				v := elems[i]
				if !isNil(keyFn) {
					v2, err2 := callFnOnSeq(keyFn, []*Value{elems[i]}, globalEnv)
					if err2 != nil {
						return nil, err2
					}
					v = v2
				}
				predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
				if perr == nil && !isTruthy(predVal) {
					elems[i] = newVal
					replaced++
				}
			}
		} else {
			for i := start; i < end; i++ {
				if count >= 0 && replaced >= count {
					break
				}
				v := elems[i]
				if !isNil(keyFn) {
					v2, err2 := callFnOnSeq(keyFn, []*Value{elems[i]}, globalEnv)
					if err2 != nil {
						return nil, err2
					}
					v = v2
				}
				predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
				if perr == nil && !isTruthy(predVal) {
					elems[i] = newVal
					replaced++
				}
			}
		}
		return seq, nil
	}

	// Handle lists: modify cons cells in-place
	if end < 0 {
		seen := make(map[*Value]bool)
		length := 0
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			if seen[cur] {
				break
			}
			seen[cur] = true
			length++
			cur = cur.cdr
		}
		end = length
	}
	if start < 0 {
		start = 0
	}
	replaced := 0
	if fromEnd {
		var elements []*Value
		var cellPtrs []*Value
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			elements = append(elements, cur.car)
			cellPtrs = append(cellPtrs, cur)
			cur = cur.cdr
		}
		for i := end - 1; i >= start; i-- {
			if count >= 0 && replaced >= count {
				break
			}
			v := elements[i]
			if !isNil(keyFn) {
				var err2 error
				v, err2 = callFnOnSeq(keyFn, []*Value{elements[i]}, globalEnv)
				if err2 != nil {
					return nil, err2
				}
			}
			predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
			if perr == nil && !isTruthy(predVal) {
				cellPtrs[i].car = newVal
				replaced++
			}
		}
	} else {
		i := 0
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			if i >= start && i < end && (count < 0 || replaced < count) {
				v := cur.car
				if !isNil(keyFn) {
					var err2 error
					v, err2 = callFnOnSeq(keyFn, []*Value{cur.car}, globalEnv)
					if err2 != nil {
						return nil, err2
					}
				}
				predVal, perr := callFnOnSeq(pred, []*Value{v}, globalEnv)
				if perr == nil && !isTruthy(predVal) {
					cur.car = newVal
					replaced++
				}
			}
			i++
			cur = cur.cdr
		}
	}
	return seq, nil
}

func builtinSeqRemoveDuplicates(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("seq-remove-duplicates: need sequence")
	}
	seq := args[0]
	keyFn, testFn, _, fromEnd, _, start, end, _, err := seqParseKeys(args, 1)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	// Track seen indices
	seen := map[int]bool{}
	dupSet := map[int]bool{}
	if fromEnd {
		for i := end - 1; i >= start; i-- {
			if seen[i] {
				continue
			}
			for j := i - 1; j >= start; j-- {
				if dupSet[j] {
					continue
				}
				if testItemMatch(elements[i], elements[j], testFn, keyFn) {
					dupSet[j] = true
				}
			}
			seen[i] = true
		}
	} else {
		for i := start; i < end; i++ {
			if seen[i] {
				continue
			}
			for j := i + 1; j < end; j++ {
				if dupSet[j] {
					continue
				}
				if testItemMatch(elements[i], elements[j], testFn, keyFn) {
					dupSet[j] = true
				}
			}
			seen[i] = true
		}
	}
	var result []*Value
	for i, el := range elements {
		if !dupSet[i] {
			result = append(result, el)
		}
	}
	// Return same type as input
	if seq.typ == VArray && seq.array != nil {
		arr := &LispArray{
			dims:     []int{len(result)},
			elements: result,
			fillPtr:  -1, // no fill-pointer for result
		}
		return &Value{typ: VArray, array: arr}, nil
	}
	if seq.typ == VStr {
		var b strings.Builder
		for _, el := range result {
			if el != nil && el.typ == VChar {
				b.WriteRune(el.ch)
			}
		}
		return vstr(b.String()), nil
	}
	return listFromSlice(result), nil
}

func builtinSeqFindIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-find-if: need predicate and sequence")
	}
	pred := args[0]
	seq := args[1]
	keyFn, _, _, fromEnd, _, start, end, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	if fromEnd {
		for i := end - 1; i >= start; i-- {
			v := elements[i]
			if !isNil(keyFn) {
				var err error
				v, err = callFnOnSeq(keyFn, []*Value{v}, globalEnv)
				if err != nil {
					return nil, err
				}
			}
			cmp, err := callFnOnSeq(pred, []*Value{v}, globalEnv)
			if err != nil {
				return nil, err
			}
			if isTruthy(cmp) {
				return elements[i], nil
			}
		}
	} else {
		for i := start; i < end; i++ {
			v := elements[i]
			if !isNil(keyFn) {
				var err error
				v, err = callFnOnSeq(keyFn, []*Value{v}, globalEnv)
				if err != nil {
					return nil, err
				}
			}
			cmp, err := callFnOnSeq(pred, []*Value{v}, globalEnv)
			if err != nil {
				return nil, err
			}
			if isTruthy(cmp) {
				return elements[i], nil
			}
		}
	}
	return vnil(), nil
}

func builtinSeqPositionIf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-position-if: need predicate and sequence")
	}
	pred := args[0]
	seq := args[1]
	keyFn, _, _, fromEnd, _, start, end, _, err := seqParseKeys(args, 2)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	if fromEnd {
		for i := end - 1; i >= start; i-- {
			v := elements[i]
			if !isNil(keyFn) {
				var err error
				v, err = callFnOnSeq(keyFn, []*Value{v}, globalEnv)
				if err != nil {
					return nil, err
				}
			}
			cmp, err := callFnOnSeq(pred, []*Value{v}, globalEnv)
			if err != nil {
				return nil, err
			}
			if isTruthy(cmp) {
				return vnum(float64(i)), nil
			}
		}
	} else {
		for i := start; i < end; i++ {
			v := elements[i]
			if !isNil(keyFn) {
				var err error
				v, err = callFnOnSeq(keyFn, []*Value{v}, globalEnv)
				if err != nil {
					return nil, err
				}
			}
			cmp, err := callFnOnSeq(pred, []*Value{v}, globalEnv)
			if err != nil {
				return nil, err
			}
			if isTruthy(cmp) {
				return vnum(float64(i)), nil
			}
		}
	}
	return vnil(), nil
}

func builtinSeqRemoveIfNot(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-remove-if-not: need predicate and sequence")
	}
	negPred := &Value{typ: VFunc, params: []string{"x"}, body: list(list(vsym("not"), list(args[0], vsym("x")))), env: globalEnv}
	newArgs := append([]*Value{negPred}, args[1:]...)
	return builtinSeqRemoveIf(newArgs)
}

func builtinSeqCountIfNot(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-count-if-not: need predicate and sequence")
	}
	negPred := &Value{typ: VFunc, params: []string{"x"}, body: list(list(vsym("not"), list(args[0], vsym("x")))), env: globalEnv}
	newArgs := append([]*Value{negPred}, args[1:]...)
	return builtinSeqCountIf(newArgs)
}

func builtinSeqFindIfNot(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-find-if-not: need predicate and sequence")
	}
	negPred := &Value{typ: VFunc, params: []string{"x"}, body: list(list(vsym("not"), list(args[0], vsym("x")))), env: globalEnv}
	newArgs := append([]*Value{negPred}, args[1:]...)
	return builtinSeqFindIf(newArgs)
}

func builtinSeqPositionIfNot(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("seq-position-if-not: need predicate and sequence")
	}
	negPred := &Value{typ: VFunc, params: []string{"x"}, body: list(list(vsym("not"), list(args[0], vsym("x")))), env: globalEnv}
	newArgs := append([]*Value{negPred}, args[1:]...)
	return builtinSeqPositionIf(newArgs)
}

func builtinSeqSubstituteIf(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("seq-substitute-if: need new, predicate, and sequence")
	}
	newVal := args[0]
	pred := args[1]
	seq := args[2]
	keyFn, _, _, fromEnd, count, start, end, _, err := seqParseKeys(args, 3)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	result := make([]*Value, len(elements))
	copy(result, elements)
	replaced := 0
	if fromEnd {
		for i := end - 1; i >= start; i-- {
			if count >= 0 && replaced >= count {
				break
			}
			v := elements[i]
			if !isNil(keyFn) {
				var err error
				v, err = callFnOnSeq(keyFn, []*Value{v}, globalEnv)
				if err != nil {
					return nil, err
				}
			}
			cmp, err := callFnOnSeq(pred, []*Value{v}, globalEnv)
			if err != nil {
				return nil, err
			}
			if isTruthy(cmp) {
				result[i] = newVal
				replaced++
			}
		}
	} else {
		for i := start; i < end; i++ {
			if count >= 0 && replaced >= count {
				break
			}
			v := elements[i]
			if !isNil(keyFn) {
				var err error
				v, err = callFnOnSeq(keyFn, []*Value{v}, globalEnv)
				if err != nil {
					return nil, err
				}
			}
			cmp, err := callFnOnSeq(pred, []*Value{v}, globalEnv)
			if err != nil {
				return nil, err
			}
			if isTruthy(cmp) {
				result[i] = newVal
				replaced++
			}
		}
	}
	// If input was a string and all results are characters, return a string
	if seq.typ == VStr {
		var sb strings.Builder
		for _, v := range result {
			if v.typ == VChar {
				sb.WriteRune(v.ch)
			} else {
				return listFromSlice(result), nil
			}
		}
		return vstr(sb.String()), nil
	}
	// If input was a VArray, return a VArray
	if seq.typ == VArray {
		return varray(result), nil
	}
	return listFromSlice(result), nil
}

// builtinNsubstitute is the destructive version - modifies the list in-place
func builtinNsubstitute(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("nsubstitute: need new, old, and sequence")
	}
	newVal := args[0]
	oldVal := args[1]
	seq := args[2]
	keyFn, testFn, _, fromEnd, count, start, end, _, err := seqParseKeys(args, 3)
	if err != nil {
		return nil, err
	}

	// Handle VArray: modify elements in-place
	if seq.typ == VArray {
		elems := seq.array.elements
		seqLen := len(elems)
		if end < 0 || end > seqLen {
			end = seqLen
		}
		if start < 0 {
			start = 0
		}
		replaced := 0
		if fromEnd {
			for i := end - 1; i >= start; i-- {
				if count >= 0 && replaced >= count {
					break
				}
				if testItemMatch(oldVal, elems[i], testFn, keyFn) {
					elems[i] = newVal
					replaced++
				}
			}
		} else {
			for i := start; i < end; i++ {
				if count >= 0 && replaced >= count {
					break
				}
				if testItemMatch(oldVal, elems[i], testFn, keyFn) {
					elems[i] = newVal
					replaced++
				}
			}
		}
		return seq, nil
	}

	// Normalize end
	if end < 0 {
		seen := make(map[*Value]bool)
		length := 0
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			if seen[cur] {
				break
			}
			seen[cur] = true
			length++
			cur = cur.cdr
		}
		end = length
	}
	if start < 0 {
		start = 0
	}

	// Destructive: modify .car of matching cells in-place
	replaced := 0
	if fromEnd {
		// First, build index of elements
		seen := make(map[*Value]bool)
		var elements []*Value
		var cellPtrs []*Value
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			if seen[cur] {
				break
			}
			seen[cur] = true
			elements = append(elements, cur.car)
			cellPtrs = append(cellPtrs, cur)
			cur = cur.cdr
		}
		for i := end - 1; i >= start; i-- {
			if count >= 0 && replaced >= count {
				break
			}
			if testItemMatch(oldVal, elements[i], testFn, keyFn) {
				cellPtrs[i].car = newVal
				replaced++
			}
		}
	} else {
		i := 0
		for cur := seq; !isNil(cur) && cur.typ == VPair; {
			if i >= start && i < end && (count < 0 || replaced < count) {
				if testItemMatch(oldVal, cur.car, testFn, keyFn) {
					cur.car = newVal
					replaced++
				}
			}
			i++
			cur = cur.cdr
		}
	}
	return seq, nil
}

func builtinSeqSubstituteIfNot(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("seq-substitute-if-not: need new, predicate, and sequence")
	}
	negPred := &Value{typ: VFunc, params: []string{"x"}, body: list(list(vsym("not"), list(args[1], vsym("x")))), env: globalEnv}
	newArgs := []*Value{args[0], negPred, args[2]}
	newArgs = append(newArgs, args[3:]...)
	return builtinSeqSubstituteIf(newArgs)
}

func builtinSeqMerge(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("seq-merge: need two sequences and a predicate")
	}
	seq1 := args[0]
	seq2 := args[1]
	pred := args[2]
	a := seqToList(seq1)
	b := seqToList(seq2)
	var result []*Value
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		cmp, err := callFnOnSeq(pred, []*Value{a[i], b[j]}, globalEnv)
		if err != nil {
			return nil, err
		}
		if isTruthy(cmp) {
			result = append(result, a[i])
			i++
		} else {
			result = append(result, b[j])
			j++
		}
	}
	for ; i < len(a); i++ {
		result = append(result, a[i])
	}
	for ; j < len(b); j++ {
		result = append(result, b[j])
	}
	return listFromSlice(result), nil
}

func builtinSeqFill(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("fill: need sequence and item")
	}
	seq := args[0]
	item := args[1]
	start := 0
	end := -1
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					start = int(toNum(args[i]))
				}
			case ":END":
				if i+1 < len(args) {
					i++
					end = int(toNum(args[i]))
				}
			}
		}
	}
	if seq.typ == VStr {
		s := seq.str
		if end < 0 || end > len(s) {
			end = len(s)
		}
		if start < 0 {
			start = 0
		}
		c := " "
		if item.typ == VStr && len(item.str) > 0 {
			c = item.str[:1]
		} else if item.typ == VChar {
			c = string(item.ch)
		}
		result := s[:start] + strings.Repeat(c, end-start) + s[end:]
		return vstr(result), nil
	}
	// For vectors, modify in place
	if seq.typ == VArray && seq.array != nil {
		elements := seq.array.elements
		n := len(elements)
		if seq.array.fillPtr >= 0 && seq.array.fillPtr < n {
			n = seq.array.fillPtr
		}
		if end < 0 || end > n {
			end = n
		}
		if start < 0 {
			start = 0
		}
		for i := start; i < end; i++ {
			elements[i] = item
		}
		return seq, nil
	}
	elements := seqToList(seq)
	if end < 0 || end > len(elements) {
		end = len(elements)
	}
	if start < 0 {
		start = 0
	}
	result := make([]*Value, len(elements))
	copy(result, elements)
	for i := start; i < end; i++ {
		result[i] = item
	}
	return listFromSlice(result), nil
}

func builtinSeqReplace(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("replace: need destination and source sequences")
	}
	dest := args[0]
	src := args[1]
	start1 := 0
	end1 := -1
	start2 := 0
	end2 := -1
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START1":
				if i+1 < len(args) {
					i++
					start1 = int(toNum(args[i]))
				}
			case ":END1":
				if i+1 < len(args) {
					i++
					end1 = int(toNum(args[i]))
				}
			case ":START2":
				if i+1 < len(args) {
					i++
					start2 = int(toNum(args[i]))
				}
			case ":END2":
				if i+1 < len(args) {
					i++
					end2 = int(toNum(args[i]))
				}
			}
		}
	}
	// String destination: must create new string (strings are immutable)
	if dest.typ == VStr {
		destRunes := []rune(dest.str)
		srcRunes := []rune(src.str)
		if end1 < 0 || end1 > len(destRunes) {
			end1 = len(destRunes)
		}
		if end2 < 0 || end2 > len(srcRunes) {
			end2 = len(srcRunes)
		}
		result := make([]rune, len(destRunes))
		copy(result, destRunes)
		j := start2
		for i := start1; i < end1 && j < end2; i++ {
			result[i] = srcRunes[j]
			j++
		}
		return vstr(string(result)), nil
	}
	// Array/vector destination: modify in place
	if dest.typ == VArray && dest.array != nil {
		srcList := seqToList(src)
		if end1 < 0 || end1 > len(dest.array.elements) {
			end1 = len(dest.array.elements)
		}
		if end2 < 0 || end2 > len(srcList) {
			end2 = len(srcList)
		}
		j := start2
		for i := start1; i < end1 && j < end2; i++ {
			dest.array.elements[i] = srcList[j]
			j++
		}
		return dest, nil
	}
	// List destination: modify in place
	destList := seqToList(dest)
	srcList := seqToList(src)
	if end1 < 0 || end1 > len(destList) {
		end1 = len(destList)
	}
	if end2 < 0 || end2 > len(srcList) {
		end2 = len(srcList)
	}
	// Modify the original list cons cells in place
	cur := dest
	j := start2
	for i := 0; i < end1 && cur != nil && cur.typ == VPair; i++ {
		if i >= start1 && j < end2 {
			cur.car = srcList[j]
			j++
		}
		cur = cur.cdr
	}
	return dest, nil
}

// -------- Additional CL sequence/string functions --------
func builtinSeqSearch(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("search: need two sequences")
	}
	seq1 := args[0]
	seq2 := args[1]
	var testFn, testNotFn, keyFn *Value
	testFn = nil
	testNotFn = nil
	keyFn = nil
	start1, end1, start2, end2 := 0, -1, 0, -1
	fromEnd := false
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":TEST":
				if i+1 < len(args) {
					i++
					testFn = args[i]
				}
			case ":TEST-NOT":
				if i+1 < len(args) {
					i++
					testNotFn = args[i]
				}
			case ":KEY":
				if i+1 < len(args) {
					i++
					keyFn = args[i]
				}
			case ":START1":
				if i+1 < len(args) {
					i++
					start1 = int(toNum(args[i]))
				}
			case ":END1":
				if i+1 < len(args) {
					i++
					end1 = int(toNum(args[i]))
				}
			case ":START2":
				if i+1 < len(args) {
					i++
					start2 = int(toNum(args[i]))
				}
			case ":END2":
				if i+1 < len(args) {
					i++
					end2 = int(toNum(args[i]))
				}
			case ":FROM-END":
				if i+1 < len(args) {
					i++
					fromEnd = isTruthy(args[i])
				}
			}
		}
	}
	s1 := seqToList(seq1)
	s2 := seqToList(seq2)
	if end1 < 0 || end1 > len(s1) {
		end1 = len(s1)
	}
	if end2 < 0 || end2 > len(s2) {
		end2 = len(s2)
	}
	// Helper to apply :key function
	applyKey := func(v *Value) *Value {
		if keyFn != nil {
			r, err := callFnOnSeq(keyFn, []*Value{v}, globalEnv)
			if err != nil {
				return v
			}
			return r
		}
		return v
	}
	// Helper to compare two elements
	elemsEqual := func(a, b *Value) bool {
		ka := applyKey(a)
		kb := applyKey(b)
		if testNotFn != nil {
			cmp, err := callFnOnSeq(testNotFn, []*Value{ka, kb}, globalEnv)
			if err != nil {
				return false
			}
			return !isTruthy(cmp)
		}
		if testFn != nil {
			cmp, err := callFnOnSeq(testFn, []*Value{ka, kb}, globalEnv)
			if err != nil {
				return false
			}
			return isTruthy(cmp)
		}
		return eqVal(ka, kb)
	}
	slen := end1 - start1
	if slen <= 0 {
		return vnil(), nil
	}
	if fromEnd {
		for i := end2 - slen; i >= start2; i-- {
			match := true
			for j := 0; j < slen; j++ {
				if !elemsEqual(s1[start1+j], s2[i+j]) {
					match = false
					break
				}
			}
			if match {
				return vnum(float64(i)), nil
			}
		}
		return vnil(), nil
	}
	for i := start2; i <= end2-slen; i++ {
		match := true
		for j := 0; j < slen; j++ {
			if !elemsEqual(s1[start1+j], s2[i+j]) {
				match = false
				break
			}
		}
		if match {
			return vnum(float64(i)), nil
		}
	}
	return vnil(), nil
}

func builtinSeqCopySeq(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("copy-seq: need sequence")
	}
	seq := args[0]
	if seq.typ == VStr {
		return vstr(seq.str), nil
	}
	if seq.typ == VArray && seq.array != nil {
		n := len(seq.array.elements)
		elems := make([]*Value, n)
		copy(elems, seq.array.elements)
		arr := &LispArray{
			dims:     make([]int, len(seq.array.dims)),
			elements: elems,
			fillPtr:  seq.array.fillPtr,
		}
		copy(arr.dims, seq.array.dims)
		result := gcv()
		result.typ = VArray
		result.array = arr
		return result, nil
	}
	elems := seqToList(seq)
	result := make([]*Value, len(elems))
	copy(result, elems)
	return listFromSlice(result), nil
}

func builtinSeqNReverse(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("nreverse: need sequence")
	}
	seq := args[0]
	if seq.typ == VStr {
		runes := []rune(seq.str)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return vstr(string(runes)), nil
	}
	if seq.typ == VArray {
		elems := seq.array.elements
		for i, j := 0, len(elems)-1; i < j; i, j = i+1, j-1 {
			elems[i], elems[j] = elems[j], elems[i]
		}
		return seq, nil
	}
	// Walk the list collecting elements and the final tail
	elems := []*Value{}
	cur := seq
	for cur != nil && cur.typ == VPair {
		elems = append(elems, cur.car)
		cur = cur.cdr
	}
	// cur is the final tail (nil for proper list, dotted value otherwise)
	tail := cur
	// Rebuild reversed list with the same tail
	for i := 0; i < len(elems); i++ {
		tail = cons(elems[i], tail)
	}
	return tail, nil
}

func builtinStringTrim(args []*Value) (*Value, error) {
	// CL: (string-trim char-bag string)
	if len(args) < 2 {
		return nil, fmt.Errorf("string-trim: need char-bag and string")
	}
	if args[1].typ != VStr {
		return nil, fmt.Errorf("string-trim: second argument must be a string")
	}
	s := args[1].str
	cSeq := seqToList(args[0])
	var cb strings.Builder
	for _, v := range cSeq {
		if v.typ == VStr {
			cb.WriteString(v.str)
		} else if v.typ == VChar {
			cb.WriteRune(v.ch)
		}
	}
	chars := cb.String()
	if len(chars) == 0 {
		chars = " \t\n\r"
	}
	charSet := make(map[rune]bool)
	for _, r := range chars {
		charSet[r] = true
	}
	runes := []rune(s)
	start := 0
	for _, r := range runes {
		if charSet[r] {
			start++
		} else {
			break
		}
	}
	end := len(runes)
	for i := len(runes) - 1; i >= 0; i-- {
		if charSet[runes[i]] {
			end = i
		} else {
			break
		}
	}
	if start >= end {
		return vstr(""), nil
	}
	return vstr(string(runes[start:end])), nil
}

func builtinStringLeftTrim(args []*Value) (*Value, error) {
	// CL: (string-left-trim char-bag string)
	if len(args) < 2 {
		return nil, fmt.Errorf("string-left-trim: need char-bag and string")
	}
	if args[1].typ != VStr {
		return nil, fmt.Errorf("string-left-trim: second argument must be a string")
	}
	s := args[1].str
	cSeq := seqToList(args[0])
	var cb strings.Builder
	for _, v := range cSeq {
		if v.typ == VStr {
			cb.WriteString(v.str)
		} else if v.typ == VChar {
			cb.WriteRune(v.ch)
		}
	}
	chars := cb.String()
	if len(chars) == 0 {
		chars = " \t\n\r"
	}
	charSet := make(map[rune]bool)
	for _, r := range chars {
		charSet[r] = true
	}
	runes := []rune(s)
	start := 0
	for _, r := range runes {
		if charSet[r] {
			start++
		} else {
			break
		}
	}
	return vstr(string(runes[start:])), nil
}

func builtinStringRightTrim(args []*Value) (*Value, error) {
	// CL: (string-right-trim char-bag string)
	if len(args) < 2 {
		return nil, fmt.Errorf("string-right-trim: need char-bag and string")
	}
	if args[1].typ != VStr {
		return nil, fmt.Errorf("string-right-trim: second argument must be a string")
	}
	s := args[1].str
	cSeq := seqToList(args[0])
	var cb strings.Builder
	for _, v := range cSeq {
		if v.typ == VStr {
			cb.WriteString(v.str)
		} else if v.typ == VChar {
			cb.WriteRune(v.ch)
		}
	}
	chars := cb.String()
	if len(chars) == 0 {
		chars = " \t\n\r"
	}
	charSet := make(map[rune]bool)
	for _, r := range chars {
		charSet[r] = true
	}
	runes := []rune(s)
	end := len(runes)
	for i := len(runes) - 1; i >= 0; i-- {
		if charSet[runes[i]] {
			end = i
		} else {
			break
		}
	}
	return vstr(string(runes[:end])), nil
}

func builtinStrCapitalize(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string-capitalize: expected a string designator")
	}
	s, err := coerceStringDesignator(args[0])
	if err != nil {
		return nil, fmt.Errorf("string-capitalize: %v", err)
	}
	runes := []rune(s)
	start, end := 0, len(runes)
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "string-capitalize :start")
					if err != nil {
						return nil, err
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "string-capitalize :end")
					if err != nil {
						return nil, err
					}
					end = int(n)
				}
			}
		}
	}
	if end > len(runes) {
		end = len(runes)
	}
	var result strings.Builder
	prevAlpha := false
	for i, r := range runes {
		if i >= start && i < end {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				if !prevAlpha {
					result.WriteRune(unicode.ToUpper(r))
				} else {
					result.WriteRune(unicode.ToLower(r))
				}
				prevAlpha = true
			} else {
				result.WriteRune(r)
				prevAlpha = false
			}
		} else {
			result.WriteRune(r)
		}
	}
	return vstr(result.String()), nil
}

// checkStringArg returns the string value or an error for string comparison builtins.
// Accepts string designators (string, symbol, or character).
func checkStringArg(v *Value, funcName string) (string, error) {
	v = primaryValue(v)
	s, err := coerceStringDesignator(v)
	if err != nil {
		return "", fmt.Errorf("%s: %v", funcName, err)
	}
	return s, nil
}

// strCmpKw holds parsed :start1/:end1/:start2/:end2 keyword arguments
type strCmpKw struct {
	start1 int
	end1   int
	start2 int
	end2   int
}

// runesSlice returns a slice of runes from start to end.
func runesSlice(rs []rune, start, end int) []rune {
	if start < 0 {
		start = 0
	}
	if end > len(rs) {
		end = len(rs)
	}
	if start >= end {
		return []rune{}
	}
	return rs[start:end]
}

// parseStrCmpKwArgs parses :start1/:end1/:start2/:end2 from keyword args.
// s1len and s2len are the rune lengths of the two strings.
func parseStrCmpKwArgs(args []*Value, s1len, s2len int) strCmpKw {
	p := strCmpKw{start1: 0, end1: s1len, start2: 0, end2: s2len}
	for i := 2; i < len(args); i++ {
		if args[i].typ != VSym {
			continue
		}
		switch args[i].str {
		case ":START1":
			if i+1 < len(args) {
				i++
				p.start1 = int(toNum(args[i]))
			}
		case ":END1":
			if i+1 < len(args) {
				i++
				p.end1 = int(toNum(args[i]))
			}
		case ":START2":
			if i+1 < len(args) {
				i++
				p.start2 = int(toNum(args[i]))
			}
		case ":END2":
			if i+1 < len(args) {
				i++
				p.end2 = int(toNum(args[i]))
			}
		}
	}
	if p.start1 < 0 {
		p.start1 = 0
	}
	if p.start1 > s1len {
		p.start1 = s1len
	}
	if p.end1 < 0 || p.end1 > s1len {
		p.end1 = s1len
	}
	if p.start2 < 0 {
		p.start2 = 0
	}
	if p.start2 > s2len {
		p.start2 = s2len
	}
	if p.end2 < 0 || p.end2 > s2len {
		p.end2 = s2len
	}
	return p
}

func builtinStrEqual(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string=: need two strings")
	}
	s1, err := checkStringArg(args[0], "string=")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string=")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	sub1 := runesSlice(r1, p.start1, p.end1)
	sub2 := runesSlice(r2, p.start2, p.end2)
	return vbool(string(sub1) == string(sub2)), nil
}

func builtinStrLess(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string<: need two strings")
	}
	s1, err := checkStringArg(args[0], "string<")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string<")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		if runes1[i] < runes2[i] {
			return vnum(float64(i)), nil
		}
		if runes1[i] > runes2[i] {
			return vnil(), nil
		}
	}
	if len(runes1) < len(runes2) {
		return vnum(float64(len(runes1))), nil
	}
	return vnil(), nil
}

func builtinIdentity(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("identity: need argument")
	}
	return args[0], nil
}

func builtinComplement(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("complement: need function")
	}
	fn := args[0]
	if fn.typ != VPrim && fn.typ != VFunc {
		return nil, fmt.Errorf("complement: first argument must be a function")
	}
	// Return a VPrim that calls the original function and returns its logical NOT
	return &Value{typ: VPrim, fn: func(innerArgs []*Value) (*Value, error) {
		var result *Value
		var err error
		switch fn.typ {
		case VPrim:
			result, err = fn.fn(innerArgs)
		case VFunc:
			newEnv := NewEnv(fn.env)
			for i, p := range fn.params {
				if i < len(innerArgs) {
					newEnv.Set(p, innerArgs[i])
				} else {
					newEnv.Set(p, vnil())
				}
			}
			if fn.rest != "" {
				newEnv.Set(fn.rest, listFromSlice(innerArgs[len(fn.params):]))
			}
			body := fn.body
			if isNil(body) {
				return vbool(true), nil // not(nil) = true
			}
			for body.typ == VPair && !isNil(body.cdr) {
				_, e := Eval(body.car, newEnv)
				if e != nil {
					return nil, e
				}
				body = body.cdr
			}
			result, err = Eval(body.car, newEnv)
		}
		if err != nil {
			return nil, err
		}
		return vbool(!isTruthy(result)), nil
	}}, nil
}

func builtinConstantly(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("constantly: need at least one argument")
	}
	constVal := args[0]
	// Return a function that ignores its args and returns constVal
	return &Value{typ: VFunc, params: []string{},
		body: list(list(vsym("quote"), constVal)), env: globalEnv}, nil
}

// -------- parse-integer --------

// -------- getf --------
func builtinGetf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("getf: need plist and indicator")
	}
	plist := args[0]
	indicator := args[1]
	defaultVal := vnil()
	if len(args) >= 3 {
		defaultVal = args[2]
	}
	cur := plist
	for cur != nil && cur.typ == VPair {
		key := cur.car
		if cur.cdr != nil && cur.cdr.typ == VPair {
			val := cur.cdr.car
			if eqVal(key, indicator) {
				return val, nil
			}
			cur = cur.cdr.cdr
		} else {
			break
		}
	}
	return defaultVal, nil
}

// -------- get-properties --------
func builtinGetProperties(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("get-properties: need plist and indicator-list")
	}
	plist := args[0]
	indicators := seqToList(args[1])
	cur := plist
	for cur != nil && cur.typ == VPair {
		key := cur.car
		if cur.cdr != nil && cur.cdr.typ == VPair {
			val := cur.cdr.car
			for _, ind := range indicators {
				if eqVal(key, ind) {
					// ANSI CL: returns (values indicator value tail)
					return multiVal(key, val, cur), nil
				}
			}
			cur = cur.cdr.cdr
		} else {
			break
		}
	}
	return multiVal(vnil(), vnil(), vnil()), nil
}

// -------- remf --------
func builtinRemf(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("remf: need plist and indicator")
	}
	plist := args[0]
	indicator := args[1]
	// Find and remove the indicator/value pair
	elems := seqToList(plist)
	result := make([]*Value, 0)
	i := 0
	for i < len(elems)-1 {
		if eqVal(elems[i], indicator) {
			i += 2 // skip key and value
			continue
		}
		result = append(result, elems[i], elems[i+1])
		i += 2
	}
	return listFromSlice(result), nil
}

// -------- make-string --------
func builtinMakeString(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-string: need size")
	}
	size := int(toNum(args[0]))
	initChar := ' '
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":INITIAL-ELEMENT" {
			if i+1 < len(args) {
				i++
				c := args[i]
				if c.typ == VChar {
					initChar = c.ch
				} else if c.typ == VStr && len(c.str) > 0 {
					initChar = []rune(c.str)[0]
				}
			}
		}
	}
	if size <= 0 {
		return vstr(""), nil
	}
	runes := make([]rune, size)
	for i := range runes {
		runes[i] = initChar
	}
	return vstr(string(runes)), nil
}

// -------- make-list --------
func builtinMakeList(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-list: need size")
	}
	size := int(toNum(args[0]))
	initVal := vnil()
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":INITIAL-ELEMENT" {
			if i+1 < len(args) {
				i++
				initVal = args[i]
			}
		}
	}
	result := make([]*Value, size)
	for i := range result {
		result[i] = initVal
	}
	return listFromSlice(result), nil
}

// -------- make-sequence --------
func builtinMakeSequence(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("make-sequence: need type and size")
	}
	typeVal := args[0]
	size := int(toNum(args[1]))
	initVal := vnil()
	for i := 2; i < len(args); i++ {
		if args[i].typ == VSym && strings.EqualFold(args[i].str, ":INITIAL-ELEMENT") {
			if i+1 < len(args) {
				i++
				initVal = args[i]
			}
		}
	}

	typeName := ""
	switch typeVal.typ {
	case VSym:
		typeName = typeVal.str
	case VStr:
		typeName = typeVal.str
	default:
		typeName = ToString(typeVal)
	}
	typeName = strings.ToUpper(typeName)

	switch typeName {
	case "LIST", "CONS", "NULL":
		result := make([]*Value, size)
		for i := range result {
			result[i] = initVal
		}
		return listFromSlice(result), nil
	case "VECTOR", "SIMPLE-VECTOR", "ARRAY", "SIMPLE-ARRAY":
		return makeArrayWithInit(size, initVal), nil
	case "STRING", "SIMPLE-STRING", "BASE-STRING":
		var ch rune = ' '
		if initVal.typ == VChar {
			ch = initVal.ch
		} else if initVal.typ == VStr && len(initVal.str) > 0 {
			ch = []rune(initVal.str)[0]
		} else if initVal.typ == VSym && len(initVal.str) > 0 {
			ch = []rune(initVal.str)[0]
		} else if !isNil(initVal) {
			return nil, fmt.Errorf("make-sequence: :initial-element is not a character designator: %s", ToString(initVal))
		}
		return vstr(strings.Repeat(string(ch), size)), nil
	case "BIT-VECTOR", "SIMPLE-BIT-VECTOR":
		var bitVal int
		if initVal.typ == VNum {
			bitVal = int(initVal.num)
			if bitVal != 0 && bitVal != 1 {
				return nil, fmt.Errorf("make-sequence: bit must be 0 or 1")
			}
		}
		elems := make([]*Value, size)
		for i := range elems {
			elems[i] = vnum(float64(bitVal))
		}
		arr := &LispArray{dims: []int{size}, elements: elems, adjustable: false}
		v := gcv()
		v.typ = VArray
		v.array = arr
		return v, nil
	default:
		return nil, fmt.Errorf("make-sequence: unsupported type: %s", typeName)
	}
}

func makeArrayWithInit(size int, initVal *Value) *Value {
	elems := make([]*Value, size)
	for i := range elems {
		elems[i] = initVal
	}
	arr := &LispArray{dims: []int{size}, elements: elems, adjustable: false}
	v := gcv()
	v.typ = VArray
	v.array = arr
	return v
}

// -------- random --------
func builtinRandom(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("random: need limit")
	}
	v := primaryValue(args[0])
	if v.typ != VNum && v.typ != VBigInt && v.typ != VRat {
		return nil, fmt.Errorf("random: argument must be a positive number")
	}
	// Determine which rng to use
	var randState *rand.Rand
	for i := 1; i+1 < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":RANDOM-STATE" && args[i+1].typ == VRandomState {
			if args[i+1].randState != nil {
				randState = args[i+1].randState
			}
			break
		}
	}
	// If no :random-state provided, use *random-state* special variable
	if randState == nil {
		if rv, err := globalEnv.Get("*random-state*"); err == nil && rv.typ == VRandomState && rv.randState != nil {
			randState = rv.randState
		}
	}
	// Big integer: use big.Int for proper arbitrary-precision random
	if v.typ == VBigInt {
		if v.bigInt.Sign() <= 0 {
			return nil, fmt.Errorf("random: limit must be > 0")
		}
		var rnd *rand.Rand
		if randState != nil {
			rnd = randState
		} else {
			rnd = rand.New(rand.NewSource(time.Now().UnixNano()))
		}
		result := new(big.Int).Rand(rnd, v.bigInt)
		return vbigInt(result), nil
	}
	// Rational: truncate to integer
	if v.typ == VRat {
		if v.irat <= 0 {
			return nil, fmt.Errorf("random: limit must be > 0")
		}
		intLimit := int(v.irat / v.iden)
		if intLimit < 1 {
			return nil, fmt.Errorf("random: limit must be >= 1")
		}
		if randState != nil {
			return vnum(float64(randState.Intn(intLimit))), nil
		}
		return vnum(float64(rand.Intn(intLimit))), nil
	}
	// VNum: float returns random float in [0, limit); integer returns random int in [0, limit)
	limit := v.num
	if limit <= 0 {
		return nil, fmt.Errorf("random: limit must be > 0")
	}
	if math.Abs(limit-math.Trunc(limit)) > 1e-12 {
		// Float limit: return random float
		if randState != nil {
			return vnum(randState.Float64() * limit), nil
		}
		return vnum(rand.Float64() * limit), nil
	}
	// Integer limit
	intLimit := int(limit)
	if intLimit < 1 {
		return nil, fmt.Errorf("random: limit must be >= 1")
	}
	if randState != nil {
		return vnum(float64(randState.Intn(intLimit))), nil
	}
	return vnum(float64(rand.Intn(intLimit))), nil
}

// vecToString converts a VArray of characters to a Go string.
// Uses fill-pointer if present.
func vecToString(v *Value) string {
	arr := v.array
	end := len(arr.elements)
	if arr.fillPtr >= 0 && arr.fillPtr <= len(arr.elements) {
		end = arr.fillPtr
	}
	var sb strings.Builder
	for i := 0; i < end; i++ {
		elem := arr.elements[i]
		if elem != nil {
			if elem.typ == VChar {
				sb.WriteRune(elem.ch)
			} else if elem.typ == VStr && len(elem.str) == 1 {
				sb.WriteRune(rune(elem.str[0]))
			}
		}
	}
	return sb.String()
}

func builtinNStringUpcase(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("nstring-upcase: need a string")
	}
	v := primaryValue(args[0])
	start, end := 0, -1
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "nstring-upcase :start")
					if err != nil {
						return nil, err
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "nstring-upcase :end")
					if err != nil {
						return nil, err
					}
					end = int(n)
				}
			}
		}
	}
	switch v.typ {
	case VStr:
		runes := []rune(v.str)
		e := len(runes)
		if end >= 0 && end < e {
			e = end
		}
		for i := start; i < e; i++ {
			runes[i] = unicode.ToUpper(runes[i])
		}
		v.str = string(runes)
		return v, nil
	case VArray:
		arr := v.array
		e := len(arr.elements)
		if arr.fillPtr >= 0 && arr.fillPtr < e {
			e = arr.fillPtr
		}
		if end >= 0 && end < e {
			e = end
		}
		for i := start; i < e; i++ {
			elem := arr.elements[i]
			if elem != nil && elem.typ == VChar {
				arr.elements[i] = vchar(unicode.ToUpper(elem.ch))
			}
		}
		return v, nil
	default:
		return nil, fmt.Errorf("nstring-upcase: need a string")
	}
}

func builtinNStringDowncase(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("nstring-downcase: need a string")
	}
	v := primaryValue(args[0])
	start, end := 0, -1
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "nstring-downcase :start")
					if err != nil {
						return nil, err
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "nstring-downcase :end")
					if err != nil {
						return nil, err
					}
					end = int(n)
				}
			}
		}
	}
	switch v.typ {
	case VStr:
		runes := []rune(v.str)
		e := len(runes)
		if end >= 0 && end < e {
			e = end
		}
		for i := start; i < e; i++ {
			runes[i] = unicode.ToLower(runes[i])
		}
		v.str = string(runes)
		return v, nil
	case VArray:
		arr := v.array
		e := len(arr.elements)
		if arr.fillPtr >= 0 && arr.fillPtr < e {
			e = arr.fillPtr
		}
		if end >= 0 && end < e {
			e = end
		}
		for i := start; i < e; i++ {
			elem := arr.elements[i]
			if elem != nil && elem.typ == VChar {
				arr.elements[i] = vchar(unicode.ToLower(elem.ch))
			}
		}
		return v, nil
	default:
		return nil, fmt.Errorf("nstring-downcase: need a string")
	}
}

func builtinNStringCapitalize(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("nstring-capitalize: need a string")
	}
	v := primaryValue(args[0])
	start, end := 0, -1
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "nstring-capitalize :start")
					if err != nil {
						return nil, err
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "nstring-capitalize :end")
					if err != nil {
						return nil, err
					}
					end = int(n)
				}
			}
		}
	}
	switch v.typ {
	case VStr:
		runes := []rune(v.str)
		e := len(runes)
		if end >= 0 && end < e {
			e = end
		}
		prevAlpha := false
		for i := start; i < e; i++ {
			if (runes[i] >= 'a' && runes[i] <= 'z') || (runes[i] >= 'A' && runes[i] <= 'Z') {
				if !prevAlpha {
					runes[i] = unicode.ToUpper(runes[i])
				} else {
					runes[i] = unicode.ToLower(runes[i])
				}
				prevAlpha = true
			} else {
				prevAlpha = false
			}
		}
		v.str = string(runes)
		return v, nil
	case VArray:
		arr := v.array
		e := len(arr.elements)
		if arr.fillPtr >= 0 && arr.fillPtr < e {
			e = arr.fillPtr
		}
		if end >= 0 && end < e {
			e = end
		}
		prevAlpha := false
		for i := start; i < e; i++ {
			elem := arr.elements[i]
			if elem != nil && elem.typ == VChar {
				r := elem.ch
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
					if !prevAlpha {
						arr.elements[i] = vchar(unicode.ToUpper(r))
					} else {
						arr.elements[i] = vchar(unicode.ToLower(r))
					}
					prevAlpha = true
				} else {
					prevAlpha = false
				}
			} else {
				prevAlpha = false
			}
		}
		return v, nil
	default:
		return nil, fmt.Errorf("nstring-capitalize: need a string")
	}
}

func titleCaseString(s string) string {
	var result []rune
	capitalize := true
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			capitalize = true
		}
		if capitalize && unicode.IsLetter(r) {
			result = append(result, unicode.ToTitle(r))
			capitalize = false
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}

// -------- string-equal (case-insensitive) --------
func builtinStrEqualCI(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-equal: need two strings")
	}
	s1, err := checkStringArg(args[0], "string-equal")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string-equal")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	sub1 := runesSlice(r1, p.start1, p.end1)
	sub2 := runesSlice(r2, p.start2, p.end2)
	if len(sub1) != len(sub2) {
		return vbool(false), nil
	}
	for i := range sub1 {
		if unicode.ToLower(sub1[i]) != unicode.ToLower(sub2[i]) {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

// -------- string-not-equal, string-greaterp, string-lessp, etc. --------
func builtinStringNotEqual(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-not-equal: need two strings")
	}
	s1, err := checkStringArg(args[0], "string-not-equal")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string-not-equal")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	if len(runes1) != len(runes2) {
		return vnum(float64(0)), nil
	}
	for i := 0; i < len(runes1); i++ {
		if unicode.ToLower(runes1[i]) != unicode.ToLower(runes2[i]) {
			return vnum(float64(i)), nil
		}
	}
	return vnil(), nil
}

func builtinStringGreaterp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-greaterp: need two strings")
	}
	s1, err := checkStringArg(args[0], "string-greaterp")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string-greaterp")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		c1 := unicode.ToLower(runes1[i])
		c2 := unicode.ToLower(runes2[i])
		if c1 > c2 {
			return vnum(float64(i)), nil
		}
		if c1 < c2 {
			return vnil(), nil
		}
	}
	if len(runes1) > len(runes2) {
		return vnum(float64(len(runes2))), nil
	}
	return vnil(), nil
}

func builtinStringLessp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-lessp: need two strings")
	}
	s1, err := checkStringArg(args[0], "string-lessp")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string-lessp")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		c1 := unicode.ToLower(runes1[i])
		c2 := unicode.ToLower(runes2[i])
		if c1 < c2 {
			return vnum(float64(i)), nil
		}
		if c1 > c2 {
			return vnil(), nil
		}
	}
	if len(runes1) < len(runes2) {
		return vnum(float64(len(runes1))), nil
	}
	return vnil(), nil
}

func builtinStringNotGreaterp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-not-greaterp: need two strings")
	}
	s1, err := checkStringArg(args[0], "string-not-greaterp")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string-not-greaterp")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		c1 := unicode.ToLower(runes1[i])
		c2 := unicode.ToLower(runes2[i])
		if c1 > c2 {
			return vnil(), nil
		}
		if c1 < c2 {
			return vnum(float64(i)), nil
		}
	}
	if len(runes1) > len(runes2) {
		return vnil(), nil
	}
	return vnum(float64(len(runes1))), nil
}

func builtinStringNotLessp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-not-lessp: need two strings")
	}
	s1, err := checkStringArg(args[0], "string-not-lessp")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string-not-lessp")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		c1 := unicode.ToLower(runes1[i])
		c2 := unicode.ToLower(runes2[i])
		if c1 < c2 {
			return vnil(), nil
		}
		if c1 > c2 {
			return vnum(float64(i)), nil
		}
	}
	if len(runes1) < len(runes2) {
		return vnil(), nil
	}
	return vnum(float64(len(runes2))), nil
}

// -------- write-to-string (already defined above, skip duplicate) --------

// -------- prin1-to-string --------
func builtinPrin1ToString(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("prin1-to-string: need an object")
	}
	return vstr(ToString(primaryValue(args[0]))), nil
}

// -------- princ-to-string --------
func builtinPrincToString(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("princ-to-string: need an object")
	}
	return vstr(princToString(primaryValue(args[0]))), nil
}

// -------- string-elt --------
func builtinStringElt(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-elt: need string and index")
	}
	s, err := checkStringArg(args[0], "string-elt")
	if err != nil {
		return nil, err
	}
	idx := int(toNum(args[1]))
	runes := []rune(s)
	if idx < 0 || idx >= len(runes) {
		return nil, fmt.Errorf("string-elt: index %d out of bounds", idx)
	}
	return vchar(runes[idx]), nil
}

// -------- nreverse (already exists as builtinSeqNReverse, add alias) --------

// -------- reverse --------
func builtinReverse(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("reverse: need sequence")
	}
	seq := args[0]
	if err := checkSequenceArg(seq, "reverse"); err != nil {
		return nil, err
	}
	if seq.typ == VStr {
		runes := []rune(seq.str)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return vstr(string(runes)), nil
	}
	if seq.typ == VArray {
		elems := seq.array.elements
		result := make([]*Value, len(elems))
		copy(result, elems)
		for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
			result[i], result[j] = result[j], result[i]
		}
		arr := &LispArray{dims: seq.array.dims, elements: result}
		r := gcv()
		r.typ = VArray
		r.array = arr
		return r, nil
	}
	// Walk the list collecting elements and the final tail
	elems := []*Value{}
	cur := seq
	for cur != nil && cur.typ == VPair {
		elems = append(elems, cur.car)
		cur = cur.cdr
	}
	// cur is the final tail (nil for proper list, dotted value otherwise)
	tail := cur
	// Rebuild reversed list with the same tail
	for i := 0; i < len(elems); i++ {
		tail = cons(elems[i], tail)
	}
	return tail, nil
}

// -------- acons --------
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

// -------- pairlis (already exists) --------

// -------- rassoc --------
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

// -------- rassoc-if --------
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

// -------- nth --------
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

// -------- nthcdr --------
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

// -------- set-difference (already exists) --------

// -------- string= (already exists as builtinStrEqual) --------

// -------- string/= --------
func builtinStringNotEq(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string/=: need two strings")
	}
	s1, err := checkStringArg(args[0], "string/=")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string/=")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	if string(runes1) == string(runes2) {
		return vnil(), nil
	}
	minLen := len(runes1)
	if len(runes2) < minLen {
		minLen = len(runes2)
	}
	for i := 0; i < minLen; i++ {
		if runes1[i] != runes2[i] {
			return vnum(float64(i)), nil
		}
	}
	return vnum(float64(minLen)), nil
}

func builtinStringGreater(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string>: need two strings")
	}
	s1, err := checkStringArg(args[0], "string>")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string>")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		if runes1[i] > runes2[i] {
			return vnum(float64(i)), nil
		}
		if runes1[i] < runes2[i] {
			return vnil(), nil
		}
	}
	if len(runes1) > len(runes2) {
		return vnum(float64(len(runes2))), nil
	}
	return vnil(), nil
}

func builtinStringLe(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string<=: need two strings")
	}
	s1, err := checkStringArg(args[0], "string<=")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string<=")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		if runes1[i] < runes2[i] {
			return vnum(float64(i)), nil
		}
		if runes1[i] > runes2[i] {
			return vnil(), nil
		}
	}
	if len(runes1) <= len(runes2) {
		return vnum(float64(len(runes1))), nil
	}
	return vnil(), nil
}

func builtinStringGe(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string>=: need two strings")
	}
	s1, err := checkStringArg(args[0], "string>=")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string>=")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		if runes1[i] > runes2[i] {
			return vnum(float64(i)), nil
		}
		if runes1[i] < runes2[i] {
			return vnil(), nil
		}
	}
	if len(runes1) >= len(runes2) {
		return vnum(float64(len(runes2))), nil
	}
	return vnil(), nil
}
