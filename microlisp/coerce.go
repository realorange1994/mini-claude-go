package microlisp

import (
	"fmt"
	"math"
	"strings"
)

// -------- coerce improvements --------
func builtinCoerce(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("coerce: need object and result-type")
	}
	obj := args[0]
	resultType := args[1]
	typeStr := ""
	typeSub := ""
	if resultType.typ == VSym {
		typeStr = strings.ToLower(resultType.str)
	} else if isPair(resultType) && resultType.car != nil && resultType.car.typ == VSym {
		// Compound type specifier like (complex float)
		typeStr = strings.ToLower(resultType.car.str)
		if isPair(resultType.cdr) && resultType.cdr.car != nil && resultType.cdr.car.typ == VSym {
			typeSub = strings.ToLower(resultType.cdr.car.str)
		}
	}

	switch typeStr {
	case "string":
		if obj.typ == VStr {
			return obj, nil
		}
		if obj.typ == VChar {
			return vstr(string(obj.ch)), nil
		}
		if obj.typ == VSym {
			return vstr(obj.str), nil
		}
		if obj.typ == VPair || isNil(obj) {
			// list of characters/numbers/symbols/strings -> string
			var sb strings.Builder
			cur := obj
			for !isNil(cur) {
				if cur.typ == VPair {
					elt := cur.car
					if elt.typ == VChar {
						sb.WriteRune(elt.ch)
					} else if elt.typ == VNum {
						sb.WriteRune(rune(toNum(elt)))
					} else if elt.typ == VSym {
						sb.WriteString(elt.str)
					} else if elt.typ == VStr {
						sb.WriteString(elt.str)
					}
					cur = cur.cdr
				} else {
					break
				}
			}
			return vstr(sb.String()), nil
		}
		elems := seqToList(obj)
		var sb2 strings.Builder
		for _, v := range elems {
			if v.typ == VChar {
				sb2.WriteRune(v.ch)
			} else {
				sb2.WriteString(ToString(v))
			}
		}
		return vstr(sb2.String()), nil
	case "list", ":list":
		if obj.typ == VPair || isNil(obj) {
			return obj, nil
		}
		if obj.typ == VComplex {
			return listFromSlice([]*Value{vnum(obj.num), vnum(obj.imag)}), nil
		}
		if obj.typ == VStr {
			return listFromSlice(stringToCharList(obj.str)), nil
		}
		return listFromSlice(seqToList(obj)), nil
	case "float", ":float", "single-float", "double-float":
		if obj.typ != VNum && obj.typ != VRat && obj.typ != VBigInt && obj.typ != VComplex {
			return nil, fmt.Errorf("coerce: %v cannot be coerced to type %s", ToString(obj), typeStr)
		}
		return vfloat(toNum(obj)), nil
	case "rational", "ratio", ":rational", ":ratio":
		if obj.typ == VRat {
			return obj, nil
		}
		if obj.typ == VNum {
			n := toNum(obj)
			if n == float64(int(n)) {
				return vrat(int64(n), 1), nil
			}
			return toRational(n), nil
		}
		return nil, fmt.Errorf("coerce: cannot coerce to rational")
	case "complex", ":complex", "COMPLEX":
		// Check for compound type specifier: (complex float), (complex single-float), (complex double-float)
		switch typeSub {
		case "float", "single-float", "FLOAT", "SINGLE-FLOAT":
			if obj.typ == VComplex {
				r := float64(float32(obj.num))
				i := float64(float32(obj.imag))
				return vcomplexAlways(r, i), nil
			}
			r := float64(float32(toNum(obj)))
			return vcomplexAlways(r, 0), nil
		case "double-float", "DOUBLE-FLOAT":
			if obj.typ == VComplex {
				r := toNum(obj)
				i := obj.imag
				return vcomplexAlways(r, i), nil
			}
			return vcomplexAlways(toNum(obj), 0), nil
		case "rational", "integer", "RATIONAL", "INTEGER":
			// (complex rational) or (complex integer) - must always produce a VComplex
			if obj.typ == VComplex {
				return obj, nil
			}
			return vcomplexAlways(toNum(obj), 0), nil
		default:
			// Plain (complex) or unknown subtype - use vcomplex which simplifies #c(x 0) to x
			if obj.typ == VComplex {
				return obj, nil
			}
			return vcomplex(toNum(obj), 0), nil
		}
	case "character", ":character":
		if obj.typ == VChar {
			return obj, nil
		}
		if obj.typ == VStr && len(obj.str) == 1 {
			return vchar([]rune(obj.str)[0]), nil
		}
		if obj.typ == VStr && len(obj.str) > 1 {
			return nil, fmt.Errorf("coerce: string has more than one character")
		}
		if obj.typ == VNum {
			return vchar(rune(int(toNum(obj)))), nil
		}
		// Symbol designator: (coerce 'a 'character) => #\a
		if obj.typ == VSym && len(obj.str) == 1 {
			return vchar(rune(obj.str[0])), nil
		}
		return nil, fmt.Errorf("coerce: cannot coerce to character")
	case "standard-char", "base-char", ":standard-char", ":base-char":
		// Coerce to character, then verify it's a standard-char/base-char
		var ch rune
		switch obj.typ {
		case VChar:
			ch = obj.ch
		case VStr:
			if len(obj.str) == 0 {
				return nil, fmt.Errorf("coerce: string is empty")
			}
			ch = []rune(obj.str)[0]
		case VNum:
			ch = rune(int(toNum(obj)))
		default:
			return nil, fmt.Errorf("coerce: cannot coerce to %s", typeStr)
		}
		// Check if it's a standard-char
		if !isStandardChar(ch) {
			return nil, fmt.Errorf("coerce: %c is not of type %s", ch, strings.ToUpper(typeStr))
		}
		return vchar(ch), nil
	case "function", ":function":
		if obj.typ == VFunc || obj.typ == VPrim {
			return obj, nil
		}
		// (coerce 'name 'function) - look up function by name
		if obj.typ == VSym {
			fn, err := globalEnv.Get(obj.str)
			if err == nil && (fn.typ == VFunc || fn.typ == VPrim) {
				return fn, nil
			}
		}
		return nil, fmt.Errorf("coerce: cannot coerce to function")
	case "integer", ":integer":
		if obj.typ != VNum && obj.typ != VRat && obj.typ != VBigInt {
			return nil, fmt.Errorf("coerce: %v cannot be coerced to type INTEGER", ToString(obj))
		}
		n := toNum(obj)
		return vnum(math.Floor(n)), nil
	case "sequence", ":sequence":
		return obj, nil
	case "vector", ":vector", "simple-vector", ":simple-vector":
		if obj.typ == VArray && len(obj.array.dims) == 1 {
			return obj, nil
		}
		var elems []*Value
		if obj.typ == VStr {
			elems = stringToCharList(obj.str)
		} else {
			elems = seqToList(obj)
		}
		arr := &LispArray{dims: []int{len(elems)}, elements: elems, fillPtr: -1, adjustable: false}
		v := gcv()
		v.typ = VArray
		v.array = arr
		return v, nil
	case "bit-vector", "simple-bit-vector", ":bit-vector", ":simple-bit-vector":
		// Bit vector: 1D array containing only 0s and 1s
		var elems []*Value
		if obj.typ == VArray && len(obj.array.dims) == 1 {
			elems = obj.array.elements
		} else if obj.typ == VStr {
			// String of 0/1 characters
			for _, ch := range obj.str {
				if ch == '0' {
					elems = append(elems, vnum(0))
				} else if ch == '1' {
					elems = append(elems, vnum(1))
				} else {
					return nil, fmt.Errorf("coerce: string contains non-bit character %c", ch)
				}
			}
		} else {
			elems = seqToList(obj)
		}
		// Verify all elements are 0 or 1
		for i, e := range elems {
			if e.typ != VNum {
				return nil, fmt.Errorf("coerce: element %d is not a number", i)
			}
			n := toNum(e)
			if n != 0 && n != 1 {
				return nil, fmt.Errorf("coerce: element %d is not a bit (%d)", i, int(n))
			}
		}
		arr := &LispArray{dims: []int{len(elems)}, elements: elems, fillPtr: -1, adjustable: false}
		v := gcv()
		v.typ = VArray
		v.array = arr
		return v, nil
	case "array", ":array":
		if obj.typ == VArray {
			return obj, nil
		}
		var elems []*Value
		if obj.typ == VStr {
			elems = stringToCharList(obj.str)
		} else {
			elems = seqToList(obj)
		}
		arr := &LispArray{dims: []int{len(elems)}, elements: elems, fillPtr: -1, adjustable: false}
		v := gcv()
		v.typ = VArray
		v.array = arr
		return v, nil
	case "simple-array":
		// (coerce obj '(simple-array type (dim))) or (coerce obj 'simple-array)
		if obj.typ == VArray {
			return obj, nil
		}
		var elems []*Value
		if obj.typ == VStr {
			elems = stringToCharList(obj.str)
		} else {
			elems = seqToList(obj)
		}
		arr := &LispArray{dims: []int{len(elems)}, elements: elems, fillPtr: -1, adjustable: false}
		v := gcv()
		v.typ = VArray
		v.array = arr
		return v, nil
	case "real":
		if obj.typ == VComplex {
			return nil, fmt.Errorf("coerce: cannot coerce complex to type REAL")
		}
		return obj, nil
	case "number":
		return obj, nil
	case "symbol":
		if obj.typ == VSym {
			return obj, nil
		}
		if obj.typ == VStr {
			return vsym(obj.str), nil
		}
		return nil, fmt.Errorf("coerce: cannot coerce to symbol")
	default:
		return nil, fmt.Errorf("coerce: unsupported result-type %s", typeStr)
	}
}

func stringToCharList(s string) []*Value {
	runes := []rune(s)
	result := make([]*Value, len(runes))
	for i, r := range runes {
		result[i] = vchar(r)
	}
	return result
}

// character takes a character designator and returns the character.
// Designators: character, string of length 1, symbol of length 1, integer (code point).
func builtinCharacter(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("character: need a character designator")
	}
	designator := args[0]
	if designator.typ == VChar {
		return designator, nil
	}
	if designator.typ == VStr && len(designator.str) == 1 {
		return vchar([]rune(designator.str)[0]), nil
	}
	if designator.typ == VNum {
		return vchar(rune(int(toNum(designator)))), nil
	}
	if designator.typ == VSym && len(designator.str) == 1 {
		return vchar(rune(designator.str[0])), nil
	}
	return nil, fmt.Errorf("character: %v is not a character designator", designator)
}
